package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/eko/gocache/lib/v4/cache"
	"github.com/eko/gocache/lib/v4/store"
)

var ErrDateInOlderThanOneMonthFuture = errors.New("date is more than 31 days in the future")

func getWeWorkLocationFromCache(ctx context.Context, cacheManager *cache.Cache[[]byte], coworkingLocationID string) (WeWorkLocation, error) {
	var weworkLocation WeWorkLocation

	cacheKey := "wework_location_" + coworkingLocationID

	cachedData, err := cacheManager.Get(ctx, cacheKey)

	if err == nil && cachedData != nil {
		log.Println("Using cached location data")
		if err := json.Unmarshal(cachedData, &weworkLocation); err == nil {
			return weworkLocation, nil
		}
	}

	return WeWorkLocation{}, errors.New("no cached location found")
}

func makeBooking(ctx context.Context, auth *WeWorkAuthenticator, coworkingLocationID string, date string, cacheManager *cache.Cache[[]byte]) error {
	layout := "Jan 2, 2006"
	// We do not need to check the error as this was already checked
	d, _ := time.Parse(layout, date)

	now := time.Now()

	if d.Sub(now) > 31*24*time.Hour {
		return ErrDateInOlderThanOneMonthFuture
	}

	bearerToken, err := auth.BearerToken(ctx)

	if err != nil {
		return err
	}

	// First try to get location from cache
	weworkLocation, err := getWeWorkLocationFromCache(ctx, cacheManager, coworkingLocationID)

	if err != nil {
		// If not in cache, fetch from API
		log.Println("Fetching location from API")
		weworkLocation, err = FetchWeWorkLocation(ctx, bearerToken, coworkingLocationID)

		if err != nil {
			return err
		}

		// Store in cache for 7 days
		cacheKey := "wework_location_" + coworkingLocationID
		data, err := json.Marshal(weworkLocation)

		if err == nil {
			cacheManager.Set(ctx, cacheKey, data, store.WithExpiration(24*time.Hour*7))
		}
	}

	if err != nil {
		return err
	}

	return makeBookingRequest(ctx, bearerToken, d, weworkLocation)
}
