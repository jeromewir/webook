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

func getWeWorkLocationFromCache(ctx context.Context, cacheManager *cache.Cache[[]byte], locationName string) (WeWorkLocation, error) {
	var weworkLocation WeWorkLocation

	cacheKey := weWorkLocationCacheKey(locationName)

	cachedData, err := cacheManager.Get(ctx, cacheKey)

	if err == nil && cachedData != nil {
		log.Println("Using cached location data")
		if err := json.Unmarshal(cachedData, &weworkLocation); err == nil {
			return weworkLocation, nil
		}
	}

	return WeWorkLocation{}, errors.New("no cached location found")
}

func makeBooking(ctx context.Context, auth *WeWorkAuthenticator, locationName string, date string, cacheManager *cache.Cache[[]byte]) error {
	layout := "Jan 2, 2006"
	// We do not need to check the error as this was already checked
	d, _ := time.Parse(layout, date)

	now := time.Now()

	if d.Sub(now) > 31*24*time.Hour {
		return ErrDateInOlderThanOneMonthFuture
	}

	type tokenResult struct {
		token string
		err   error
	}
	type locationResult struct {
		location WeWorkLocation
		err      error
	}

	tokenCh := make(chan tokenResult, 1)
	locationCh := make(chan locationResult, 1)

	go func() {
		token, err := auth.BearerToken(ctx)
		tokenCh <- tokenResult{token: token, err: err}
	}()

	go func() {
		location, err := getWeWorkLocationFromCache(ctx, cacheManager, locationName)
		locationCh <- locationResult{location: location, err: err}
	}()

	locResult := <-locationCh
	tokResult := <-tokenCh
	if tokResult.err != nil {
		return tokResult.err
	}
	bearerToken := tokResult.token

	weworkLocation := locResult.location
	if locResult.err != nil {
		// If not in cache, fetch from API
		log.Println("Fetching location from API")
		var err error
		weworkLocation, err = FetchWeWorkLocationByName(ctx, bearerToken, locationName)

		if err != nil {
			return err
		}

		// Store in cache for 7 days
		cacheKey := weWorkLocationCacheKey(locationName)
		data, err := json.Marshal(weworkLocation)

		if err == nil {
			cacheManager.Set(ctx, cacheKey, data, store.WithExpiration(24*time.Hour*7))
		}
	}

	return makeBookingRequest(ctx, bearerToken, d, weworkLocation)
}

func weWorkLocationCacheKey(locationName string) string {
	return "wework_location_name_" + normalizeLocationName(locationName)
}
