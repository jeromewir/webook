package main

import (
	"context"
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
