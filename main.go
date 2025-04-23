package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"

	"github.com/chromedp/chromedp"
	"github.com/joho/godotenv"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

func getCredentials() (string, string, string, error) {
	email := os.Getenv("WEWORK_EMAIL")
	password := os.Getenv("WEWORK_PASSWORD")
	coworkingName := os.Getenv("WEWORK_COWORKING_NAME")

	if email == "" || password == "" || coworkingName == "" {
		return "", "", "", errors.New("WEWORK_EMAIL, WEWORK_PASSWORD and WEWORK_CORWORKING_NAME must be set")
	}

	return email, password, coworkingName, nil
}

func getChromeAllocator() (context.Context, context.CancelFunc) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserDataDir("./chrome-data"),
		chromedp.Flag("headless", false),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)

	return allocCtx, cancel
}

func newHTTPHandler(allocCtx context.Context, email, password, coworkingName string) http.Handler {
	mux := http.NewServeMux()

	// Register handlers.
	mux.Handle("/api/book", otelhttp.NewHandler(http.HandlerFunc(registerBookHandler(allocCtx, email, password, coworkingName)), "/api/book"))

	// Add HTTP instrumentation for the whole server.
	handler := otelhttp.NewHandler(mux, "/")

	return handler
}

func main() {
	godotenv.Load()

	cleanup := initTracer(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"), "webook")
	defer cleanup(context.Background())

	email, password, coworkingName, err := getCredentials()

	if err != nil {
		log.Fatal(err)
	}

	allocCtx, cancel := getChromeAllocator()
	defer cancel()

	handler := newHTTPHandler(allocCtx, email, password, coworkingName)

	if err := http.ListenAndServe(":8081", handler); err != nil {
		log.Fatal(err)
	}

	log.Println("done")
}
