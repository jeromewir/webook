package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
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
