package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/chromedp/chromedp"
)

func registerBookHandler(allocCtx context.Context, email string, password string, coworkingName string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract the "date" query parameter
		date := r.URL.Query().Get("date")

		if date == "" {
			http.Error(w, "Missing 'date' query parameter", http.StatusBadRequest)
			return
		}

		log.Println("Received booking request for ", date)

		// Validate the date format (e.g., "Feb 18, 2025")
		if !isValidDate(date) {
			http.Error(w, "Invalid date format. Expected format: 'Feb 18, 2025'", http.StatusBadRequest)
			return
		}

		taskCtx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
		defer cancel()

		// Save cookies
		defer chromedp.Cancel(taskCtx)

		currentPage, err := getPage(taskCtx)

		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		if currentPage == PageLogin {
			log.Println("Logging in")

			if err := login(taskCtx, email, password); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
		}

		log.Println("Making booking")

		if err := makeBooking(taskCtx, coworkingName, date); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Println("Booking successful for date: ", date)

		// If the date is valid, respond with success
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, "Booking successful for date: %s", date)
	}
}

// isValidDate validates the date string against the format "Feb 18, 2025"
func isValidDate(date string) bool {
	const layout = "Jan 2, 2006"
	_, err := time.Parse(layout, date)

	return err == nil
}
