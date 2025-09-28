package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"github.com/chromedp/chromedp"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserDataDir("./chrome-data"),
		chromedp.Flag("headless", false),
	)

	email := os.Getenv("WEWORK_EMAIL")
	password := os.Getenv("WEWORK_PASSWORD")
	coworkingLocationID := os.Getenv("WEWORK_COWORKING_LOCATION_ID")

	if email == "" || password == "" || coworkingLocationID == "" {
		log.Fatal("WEWORK_EMAIL, WEWORK_PASSWORD and WEWORK_COWORKING_LOCATION_ID must be set")
	}

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	// also set up a custom logger
	http.HandleFunc("/api/book", registerBookHandler(allocCtx, email, password, coworkingLocationID))
	log.Println("Starting server on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))

	log.Println("done")
}
