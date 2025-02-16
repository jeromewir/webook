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
	coworkingName := os.Getenv("WEWORK_COWORKING_NAME")

	if email == "" || password == "" || coworkingName == "" {
		log.Fatal("WEWORK_EMAIL, WEWORK_PASSWORD and WEWORK_CORWORKING_NAME must be set")
	}

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	// also set up a custom logger
	http.HandleFunc("/api/book", registerBookHandler(allocCtx, email, password, coworkingName))
	log.Println("Starting server on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))

	log.Println("done")
}
