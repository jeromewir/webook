package main

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/joho/godotenv"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/store/go_cache/v4"
	gocache "github.com/patrickmn/go-cache"
)

func main() {
	godotenv.Load()

	email := os.Getenv("WEWORK_EMAIL")
	password := os.Getenv("WEWORK_PASSWORD")

	if email == "" || password == "" {
		log.Fatal("WEWORK_EMAIL and WEWORK_PASSWORD must be set")
	}

	gocacheClient := gocache.New(7*time.Hour*24, 30*time.Minute)
	gocacheStore := go_cache.NewGoCache(gocacheClient)

	cacheManager := cache.New[[]byte](gocacheStore)
	auth := NewWeWorkAuthenticator(email, password)

	// also set up a custom logger
	http.HandleFunc("/api/book", registerBookHandler(auth, cacheManager))
	http.HandleFunc("/api/book/batch", registerBatchBookHandler(auth, cacheManager))
	http.HandleFunc("/api/bookings/next", registerNextBookingsHandler(auth))
	log.Println("Starting server on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))

	log.Println("done")
}
