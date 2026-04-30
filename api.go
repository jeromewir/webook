package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/eko/gocache/lib/v4/cache"
)

func registerBookHandler(auth *WeWorkAuthenticator, cacheManager *cache.Cache[[]byte]) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		date := r.URL.Query().Get("date")
		if date == "" {
			http.Error(w, "Missing 'date' query parameter", http.StatusBadRequest)
			return
		}

		locationName := r.URL.Query().Get("wework")
		if locationName == "" {
			http.Error(w, "Missing 'wework' query parameter", http.StatusBadRequest)
			return
		}

		log.Println("Received booking request for", date, "at", locationName)

		// Validate the date format (e.g., "Feb 18, 2025")
		dateString, err := reformatDate(date)

		if err != nil {
			log.Println(err)
			http.Error(w, "Invalid date format. Expected format: 'Feb 18, 2025'", http.StatusBadRequest)
			return
		}

		taskCtx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()

		log.Println("Making booking")

		if err := makeBooking(taskCtx, auth, locationName, dateString, cacheManager); err != nil {
			if errors.Is(err, ErrDateInOlderThanOneMonthFuture) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			if errors.Is(err, ErrWeWorkLocationNotFound) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Println("Booking successful for date:", dateString, "at", locationName)

		// If the date is valid, respond with success
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, "Booking successful for date: %s at %s", dateString, locationName)
	}
}

type batchBookRequest struct {
	Wework string   `json:"wework"`
	Dates  []string `json:"dates"`
}

type batchBookResult struct {
	Date   string `json:"date"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type batchBookResponse struct {
	Wework       string            `json:"wework"`
	SuccessCount int               `json:"successCount"`
	FailureCount int               `json:"failureCount"`
	Summary      string            `json:"summary"`
	Results      []batchBookResult `json:"results"`
}

func registerBatchBookHandler(auth *WeWorkAuthenticator, cacheManager *cache.Cache[[]byte]) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload batchBookRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		if payload.Wework == "" {
			http.Error(w, "Missing 'wework' field", http.StatusBadRequest)
			return
		}

		dates, parsedDates, err := normalizeBatchDates(payload.Dates)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		taskCtx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
		defer cancel()

		log.Println("Received batch booking request for", len(dates), "dates at", payload.Wework)

		bearerToken, weworkLocation, err := prepareBooking(taskCtx, auth, payload.Wework, cacheManager)
		if err != nil {
			if errors.Is(err, ErrWeWorkLocationNotFound) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		results := runBatchBookings(taskCtx, bearerToken, weworkLocation, dates, parsedDates, 3)
		response := newBatchBookResponse(payload.Wework, results)

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if hasBatchBookingError(results) {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.WriteHeader(http.StatusOK)
		}

		json.NewEncoder(w).Encode(response)
	}
}

func newBatchBookResponse(wework string, results []batchBookResult) batchBookResponse {
	response := batchBookResponse{Wework: wework, Results: results}
	for _, result := range results {
		if result.Status == "success" {
			response.SuccessCount++
		} else {
			response.FailureCount++
		}
	}

	total := response.SuccessCount + response.FailureCount
	if response.FailureCount == 0 {
		response.Summary = fmt.Sprintf("Successfully booked all %d dates at %s.", total, wework)
	} else if response.SuccessCount == 0 {
		response.Summary = fmt.Sprintf("Could not book any of the %d dates at %s.", total, wework)
	} else {
		response.Summary = fmt.Sprintf("Booked %d of %d dates at %s; %d failed.", response.SuccessCount, total, wework, response.FailureCount)
	}
	return response
}

func normalizeBatchDates(input []string) ([]string, []time.Time, error) {
	if len(input) == 0 {
		return nil, nil, errors.New("Missing 'dates' field")
	}

	dates := make([]string, 0, len(input))
	parsedDates := make([]time.Time, 0, len(input))
	seen := make(map[string]struct{}, len(input))
	for _, date := range input {
		dateString, err := reformatDate(date)
		if err != nil {
			return nil, nil, fmt.Errorf("invalid date %q. Expected format: 'Feb 18, 2025'", date)
		}
		if _, ok := seen[dateString]; ok {
			return nil, nil, fmt.Errorf("duplicate date %q", dateString)
		}
		seen[dateString] = struct{}{}

		parsedDate, err := parseBookingDate(dateString)
		if err != nil {
			return nil, nil, err
		}

		dates = append(dates, dateString)
		parsedDates = append(parsedDates, parsedDate)
	}

	return dates, parsedDates, nil
}

func runBatchBookings(ctx context.Context, token string, location WeWorkLocation, dates []string, parsedDates []time.Time, limit int) []batchBookResult {
	if limit <= 0 {
		limit = 1
	}

	results := make([]batchBookResult, len(dates))
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup

	for i := range dates {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			results[i] = batchBookResult{Date: dates[i], Status: "success"}
			if err := makeBookingRequestFunc(ctx, token, parsedDates[i], location); err != nil {
				results[i].Status = "error"
				results[i].Error = err.Error()
			}
		}(i)
	}

	wg.Wait()
	return results
}

func hasBatchBookingError(results []batchBookResult) bool {
	for _, result := range results {
		if result.Status == "error" {
			return true
		}
	}

	return false
}

// reformatDate validates the date string against the format "Feb 18, 2025"
func reformatDate(date string) (string, error) {
	const layout = "Jan 2, 2006"
	d, err := time.Parse(layout, date)

	if err != nil {
		return "", err
	}

	// Reformating the date so we don't have Mar 03, 2025 which does not work
	return d.Format("Jan 2, 2006"), nil
}
