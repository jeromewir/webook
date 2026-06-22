package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestReformatDate(t *testing.T) {
	tests := []struct {
		input    string
		expected string
		hasError bool
	}{
		{"Feb 18, 2025", "Feb 18, 2025", false},
		{"Mar 03, 2025", "Mar 3, 2025", false},
		{"Invalid Date", "", true},
		{"2025-02-18", "", true},
		{"Feb 30, 2025", "", true},
	}

	for _, test := range tests {
		result, err := reformatDate(test.input)
		if test.hasError {
			if err == nil {
				t.Errorf("Expected error for input %s, but got none", test.input)
			}
		} else {
			if err != nil {
				t.Errorf("Did not expect error for input %s, but got %v", test.input, err)
			}
			if result != test.expected {
				t.Errorf("For input %s, expected %s, but got %s", test.input, test.expected, result)
			}
		}
	}
}

func TestNormalizeBatchDates(t *testing.T) {
	dates, parsedDates, err := normalizeBatchDates([]string{"Mar 03, 2026", "Mar 4, 2026"})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if len(dates) != 2 || len(parsedDates) != 2 {
		t.Fatalf("Expected 2 dates, got %d and %d", len(dates), len(parsedDates))
	}

	if dates[0] != "Mar 3, 2026" {
		t.Errorf("Expected reformatted first date, got %s", dates[0])
	}
}

func TestNormalizeBatchDatesRejectsEmptyDates(t *testing.T) {
	if _, _, err := normalizeBatchDates(nil); err == nil {
		t.Errorf("Expected error for missing dates")
	}
}

func TestNormalizeBatchDatesRejectsDuplicateDates(t *testing.T) {
	if _, _, err := normalizeBatchDates([]string{"Mar 03, 2026", "Mar 3, 2026"}); err == nil {
		t.Errorf("Expected error for duplicate dates")
	}
}

func TestNewBatchBookResponseIncludesCountsAndSummary(t *testing.T) {
	response := newBatchBookResponse("Coeur Marais", []batchBookResult{
		{Date: "Mar 1, 2026", Status: "success"},
		{Date: "Mar 2, 2026", Status: "error", Error: "failed"},
		{Date: "Mar 3, 2026", Status: "success"},
	})

	if response.SuccessCount != 2 {
		t.Errorf("Expected 2 successes, got %d", response.SuccessCount)
	}
	if response.FailureCount != 1 {
		t.Errorf("Expected 1 failure, got %d", response.FailureCount)
	}
	if response.Summary != "Booked 2 of 3 dates at Coeur Marais; 1 failed." {
		t.Errorf("Unexpected summary: %s", response.Summary)
	}
}

func TestNormalizeNextBookingsDateRangeAllowsEmptyDates(t *testing.T) {
	startDate, endDate, err := normalizeNextBookingsDateRange("", "")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if startDate != "" || endDate != "" {
		t.Fatalf("Expected empty dates, got %q and %q", startDate, endDate)
	}
}

func TestNormalizeNextBookingsDateRangeAcceptsDateOnly(t *testing.T) {
	startDate, endDate, err := normalizeNextBookingsDateRange("2026-03-01", "2026-03-31")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if startDate != "2026-03-01" || endDate != "2026-03-31" {
		t.Fatalf("Unexpected dates: %q and %q", startDate, endDate)
	}
}

func TestNormalizeNextBookingsDateRangeRejectsInvalidDate(t *testing.T) {
	if _, _, err := normalizeNextBookingsDateRange("Mar 1, 2026", ""); err == nil {
		t.Fatalf("Expected error for invalid date")
	}
}

func TestNormalizeNextBookingsDateRangeRejectsEndBeforeStart(t *testing.T) {
	if _, _, err := normalizeNextBookingsDateRange("2026-03-31", "2026-03-01"); err == nil {
		t.Fatalf("Expected error for endDate before startDate")
	}
}

func TestNewWeWorkCancelBookingRequestBuildsSingleCancelPayload(t *testing.T) {
	booking := newTestCancelBookingItem("booking-123", "location-123", "space-123")
	booking.CreditCost = 2
	booking.StartDate = "2026-03-01T06:00:00Z"
	booking.EndDate = "2026-03-01T23:59:00Z"
	booking.SpaceType = 1
	booking.BookingDate = "2026-03-01"
	booking.Location.SourceType = 2
	booking.Location.Address.Line1 = "115 Broadway"
	booking.Location.Address.Line2 = "Floor 4"
	booking.Location.Address.Country = "US"

	cancelRequest, err := newWeWorkCancelBookingRequest(booking)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if cancelRequest.BookingID != "booking-123" {
		t.Fatalf("Unexpected booking id: %s", cancelRequest.BookingID)
	}
	if cancelRequest.BookingType != 4 {
		t.Fatalf("Expected default shared workspace booking type, got %d", cancelRequest.BookingType)
	}
	if cancelRequest.SpaceID != "space-123" || cancelRequest.ReservableID != "space-123" {
		t.Fatalf("Unexpected space fields: %+v", cancelRequest)
	}
	if cancelRequest.CreditsUsed != 2 {
		t.Fatalf("Unexpected credits used: %f", cancelRequest.CreditsUsed)
	}
	if cancelRequest.ReservationID != "booking-123" {
		t.Fatalf("Unexpected reservation id: %s", cancelRequest.ReservationID)
	}
	if cancelRequest.MailParams.LocationAddress != "115 Broadway Floor 4" {
		t.Fatalf("Unexpected mail location: %s", cancelRequest.MailParams.LocationAddress)
	}
}

func TestFindCancelBookingItemFindsBookingByID(t *testing.T) {
	bookings := json.RawMessage(`[{"bookingId":"booking-123","spaceId":"space-123","location":{"id":"location-123"}}]`)

	booking, err := findCancelBookingItem(bookings, "booking-123")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	if booking.BookingID != "booking-123" {
		t.Fatalf("Unexpected booking id: %s", booking.BookingID)
	}
}

func TestFindCancelBookingItemRejectsMissingBooking(t *testing.T) {
	bookings := json.RawMessage(`[{"bookingId":"booking-123"}]`)

	if _, err := findCancelBookingItem(bookings, "missing"); err == nil {
		t.Fatalf("Expected error for missing booking")
	}
}

func TestNewWeWorkCancelBookingRequestRejectsMissingBookingID(t *testing.T) {
	booking := newTestCancelBookingItem("", "location-123", "space-123")

	if _, err := newWeWorkCancelBookingRequest(booking); err == nil {
		t.Fatalf("Expected error for missing booking id")
	}
}

func TestNewWeWorkCancelBookingRequestRejectsMissingLocationID(t *testing.T) {
	booking := newTestCancelBookingItem("booking-123", "", "space-123")

	if _, err := newWeWorkCancelBookingRequest(booking); err == nil {
		t.Fatalf("Expected error for missing location id")
	}
}

func TestNewWeWorkCancelBookingRequestRejectsMissingSpaceID(t *testing.T) {
	booking := newTestCancelBookingItem("booking-123", "location-123", "")

	if _, err := newWeWorkCancelBookingRequest(booking); err == nil {
		t.Fatalf("Expected error for missing space id")
	}
}

func newTestCancelBookingItem(bookingID string, locationID string, spaceID string) cancelBookingItem {
	booking := cancelBookingItem{BookingID: bookingID, SpaceID: spaceID}
	booking.Location.ID = locationID
	return booking
}

func TestNewBatchBookResponseSummarizesFullSuccess(t *testing.T) {
	response := newBatchBookResponse("Coeur Marais", []batchBookResult{
		{Date: "Mar 1, 2026", Status: "success"},
		{Date: "Mar 2, 2026", Status: "success"},
	})

	if response.Summary != "Successfully booked all 2 dates at Coeur Marais." {
		t.Errorf("Unexpected summary: %s", response.Summary)
	}
}

func TestNewBatchBookResponseSummarizesFullFailure(t *testing.T) {
	response := newBatchBookResponse("Coeur Marais", []batchBookResult{
		{Date: "Mar 1, 2026", Status: "error", Error: "failed"},
		{Date: "Mar 2, 2026", Status: "error", Error: "failed"},
	})

	if response.Summary != "Could not book any of the 2 dates at Coeur Marais." {
		t.Errorf("Unexpected summary: %s", response.Summary)
	}
}

func TestRunBatchBookingsLimitsConcurrency(t *testing.T) {
	original := makeBookingRequestFunc
	defer func() { makeBookingRequestFunc = original }()

	started := make(chan struct{}, 10)
	block := make(chan struct{})
	makeBookingRequestFunc = func(context.Context, string, time.Time, WeWorkLocation) error {
		started <- struct{}{}
		<-block
		return nil
	}

	dates := []string{"Mar 1, 2026", "Mar 2, 2026", "Mar 3, 2026", "Mar 4, 2026", "Mar 5, 2026"}
	parsedDates := make([]time.Time, len(dates))
	done := make(chan []batchBookResult, 1)
	go func() {
		done <- runBatchBookings(context.Background(), "token", WeWorkLocation{}, dates, parsedDates, 3)
	}()

	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("Timed out waiting for booking %d to start", i+1)
		}
	}

	select {
	case <-started:
		t.Fatalf("Started more than 3 concurrent bookings")
	case <-time.After(50 * time.Millisecond):
	}

	close(block)

	results := <-done
	if len(results) != len(dates) {
		t.Fatalf("Expected %d results, got %d", len(dates), len(results))
	}

	for _, result := range results {
		if result.Status != "success" {
			t.Errorf("Expected success result, got %+v", result)
		}
	}
}
