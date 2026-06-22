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

type cancelBookingRequest struct {
	BookingID string `json:"bookingId"`
}

type cancelBookingItem struct {
	BookingID                    string  `json:"bookingId"`
	KubeBookingExternalReference string  `json:"kubeBookingExternalReference"`
	IsFranchiseBooking           bool    `json:"isFranchiseBooking"`
	UseKubeAPI                   bool    `json:"useKubeApi"`
	CreditCost                   float64 `json:"creditCost"`
	StartDate                    string  `json:"startDate"`
	EndDate                      string  `json:"endDate"`
	SpaceID                      string  `json:"spaceId"`
	SpaceExternalReference       string  `json:"spaceExternalReference"`
	SpaceType                    int     `json:"spaceType"`
	BookingType                  int     `json:"bookingType"`
	IsHybridSpace                bool    `json:"isHybridSpace"`
	BookingDate                  string  `json:"bookingDate"`
	Location                     struct {
		ID         string `json:"id"`
		SourceType int    `json:"sourceType"`
		Address    struct {
			ID      string `json:"id"`
			Line1   string `json:"line1"`
			Line2   string `json:"line2"`
			Country string `json:"country"`
		} `json:"address"`
	} `json:"location"`
}

type weWorkCancelBookingRequest struct {
	BookingID           string                  `json:"bookingId"`
	BookingLocationType int                     `json:"bookingLocationType"`
	CreditsUsed         float64                 `json:"creditsUsed"`
	StartTime           string                  `json:"startTime"`
	EndTime             string                  `json:"endTime"`
	LocationID          string                  `json:"locationId"`
	ReservableID        string                  `json:"reservableId"`
	IsBookingApprovalOn bool                    `json:"isBookingApprovalOn"`
	BookingType         int                     `json:"bookingType"`
	SpaceID             string                  `json:"spaceId"`
	CancellationNote    string                  `json:"cancellationNote"`
	MailParams          cancelBookingMailParams `json:"mailParams"`
	ReservationID       string                  `json:"reservationId"`
}

type cancelBookingMailParams struct {
	WorkspaceType      int    `json:"workspaceType"`
	DayFormatted       string `json:"dayFormatted"`
	StartTimeFormatted string `json:"startTimeFormatted"`
	EndTimeFormatted   string `json:"endTimeFormatted"`
	FloorAddress       string `json:"floorAddress"`
	LocationAddress    string `json:"locationAddress"`
	LocationCountry    string `json:"locationCountry"`
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

func registerNextBookingsHandler(auth *WeWorkAuthenticator) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		startDate, endDate, err := normalizeNextBookingsDateRange(r.URL.Query().Get("startDate"), r.URL.Query().Get("endDate"))
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		taskCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		bearerToken, err := auth.BearerToken(taskCtx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		bookings, err := FetchNextBookings(taskCtx, bearerToken, startDate, endDate)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(bookings)
	}
}

func registerCancelBookingHandler(auth *WeWorkAuthenticator) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var payload cancelBookingRequest
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&payload); err != nil {
			http.Error(w, "Invalid JSON body", http.StatusBadRequest)
			return
		}

		if payload.BookingID == "" {
			http.Error(w, "missing bookingId", http.StatusBadRequest)
			return
		}

		taskCtx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		bearerToken, err := auth.BearerToken(taskCtx)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		bookings, err := FetchNextBookings(taskCtx, bearerToken, "", "")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		booking, err := findCancelBookingItem(bookings, payload.BookingID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		cancelRequest, err := newWeWorkCancelBookingRequest(booking)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		response, err := CancelWeWorkBooking(taskCtx, bearerToken, cancelRequest)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		w.Write(response)
	}
}

func findCancelBookingItem(bookings json.RawMessage, bookingID string) (cancelBookingItem, error) {
	var items []cancelBookingItem
	if err := json.Unmarshal(bookings, &items); err != nil {
		return cancelBookingItem{}, errors.New("unexpected bookings response format")
	}

	for _, item := range items {
		if item.BookingID == bookingID || item.KubeBookingExternalReference == bookingID {
			return item, nil
		}
	}

	return cancelBookingItem{}, fmt.Errorf("bookingId %q not found in upcoming bookings", bookingID)
}

func newWeWorkCancelBookingRequest(booking cancelBookingItem) (weWorkCancelBookingRequest, error) {
	bookingID := booking.BookingID
	if booking.IsFranchiseBooking && booking.KubeBookingExternalReference != "" {
		bookingID = booking.KubeBookingExternalReference
	}
	if bookingID == "" {
		return weWorkCancelBookingRequest{}, errors.New("booking is missing bookingId")
	}

	reservationID := bookingID
	if booking.UseKubeAPI && booking.KubeBookingExternalReference != "" {
		reservationID = booking.KubeBookingExternalReference
	}

	locationID := booking.Location.Address.ID
	if locationID == "" {
		locationID = booking.Location.ID
	}
	if locationID == "" {
		return weWorkCancelBookingRequest{}, errors.New("booking is missing location id")
	}

	spaceID := booking.SpaceID
	if booking.IsFranchiseBooking && booking.SpaceExternalReference != "" {
		spaceID = booking.SpaceExternalReference
	}
	if spaceID == "" {
		return weWorkCancelBookingRequest{}, errors.New("booking is missing spaceId")
	}

	bookingType := booking.BookingType
	if bookingType == 0 {
		bookingType = 4
	}

	return weWorkCancelBookingRequest{
		BookingID:           bookingID,
		BookingLocationType: booking.Location.SourceType,
		CreditsUsed:         booking.CreditCost,
		StartTime:           booking.StartDate,
		EndTime:             booking.EndDate,
		LocationID:          locationID,
		ReservableID:        spaceID,
		IsBookingApprovalOn: booking.IsHybridSpace,
		BookingType:         bookingType,
		SpaceID:             spaceID,
		CancellationNote:    "",
		MailParams: cancelBookingMailParams{
			WorkspaceType:      booking.SpaceType,
			DayFormatted:       booking.BookingDate,
			StartTimeFormatted: booking.StartDate,
			EndTimeFormatted:   booking.EndDate,
			FloorAddress:       "",
			LocationAddress:    booking.Location.Address.Line1 + " " + booking.Location.Address.Line2,
			LocationCountry:    booking.Location.Address.Country,
		},
		ReservationID: reservationID,
	}, nil
}

func normalizeNextBookingsDateRange(startDate string, endDate string) (string, string, error) {
	start, err := normalizeNextBookingsDate(startDate, "startDate")
	if err != nil {
		return "", "", err
	}

	end, err := normalizeNextBookingsDate(endDate, "endDate")
	if err != nil {
		return "", "", err
	}

	if start != "" && end != "" {
		parsedStart, _ := time.Parse(time.DateOnly, start)
		parsedEnd, _ := time.Parse(time.DateOnly, end)
		if parsedEnd.Before(parsedStart) {
			return "", "", errors.New("endDate must be on or after startDate")
		}
	}

	return start, end, nil
}

func normalizeNextBookingsDate(date string, field string) (string, error) {
	if date == "" {
		return "", nil
	}

	parsed, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return "", fmt.Errorf("invalid %s format. Expected format: 'YYYY-MM-DD'", field)
	}

	return parsed.Format(time.DateOnly), nil
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
