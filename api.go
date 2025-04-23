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
		log.Println("Received booking request", r.Method)
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

		log.Println("Received booking request for", date)

		// Validate the date format (e.g., "Feb 18, 2025")
		dateString, err := reformatDate(date)

		if err != nil {
			log.Println(err)
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

			log.Println("Navigating to bookings page")

			chromedp.Run(taskCtx,
				// Wait for page to load, so cookies are set
				chromedp.WaitReady(`//h2[text()="Building Information"]`, chromedp.BySearch),
				chromedp.Navigate(`https://members.wework.com/workplaceone/content2/bookings/desks`),
				chromedp.WaitReady(`wework-member-web-city-selector`, chromedp.ByQuery),
			)
		}

		log.Println("Making booking")

		if err := makeBooking(taskCtx, coworkingName, dateString); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Println("Booking successful for date:", dateString)

		// If the date is valid, respond with success
		w.WriteHeader(http.StatusOK)

		fmt.Fprintf(w, "Booking successful for date: %s", dateString)
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
