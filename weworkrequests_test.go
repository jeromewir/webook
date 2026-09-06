package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestParseTimezoneOffset(t *testing.T) {
	tests := []struct {
		name           string
		tzOffset       string
		expectedOffset float64
		expectError    bool
	}{
		{
			name:           "Summer time GMT+2",
			tzOffset:       "GMT +02:00",
			expectedOffset: 2.0,
			expectError:    false,
		},
		{
			name:           "Winter time GMT+1",
			tzOffset:       "GMT +01:00",
			expectedOffset: 1.0,
			expectError:    false,
		},
		{
			name:           "Negative offset GMT-5",
			tzOffset:       "GMT -05:00",
			expectedOffset: -5.0,
			expectError:    false,
		},
		{
			name:           "No space after GMT",
			tzOffset:       "GMT+03:00",
			expectedOffset: 3.0,
			expectError:    false,
		},
		{
			name:           "Half hour offset",
			tzOffset:       "GMT +05:30",
			expectedOffset: 5.5,
			expectError:    false,
		},
		{
			name:        "Invalid format",
			tzOffset:    "Invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			offset, err := parseTimezoneOffset(tt.tzOffset)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if offset != tt.expectedOffset {
					t.Errorf("Expected offset %f, got %f", tt.expectedOffset, offset)
				}
			}
		})
	}
}

func TestCalculateUTCTime(t *testing.T) {
	testDate := time.Date(2025, 11, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		localTime   string
		tzOffset    string
		expectedUTC string
		expectError bool
	}{
		{
			name:        "Summer time 06:00 local (GMT+2)",
			localTime:   "06:00",
			tzOffset:    "GMT +02:00",
			expectedUTC: "2025-11-15T04:00:00Z",
			expectError: false,
		},
		{
			name:        "Summer time 23:59 local (GMT+2)",
			localTime:   "23:59",
			tzOffset:    "GMT +02:00",
			expectedUTC: "2025-11-15T21:59:00Z",
			expectError: false,
		},
		{
			name:        "Winter time 06:00 local (GMT+1)",
			localTime:   "06:00",
			tzOffset:    "GMT +01:00",
			expectedUTC: "2025-11-15T05:00:00Z",
			expectError: false,
		},
		{
			name:        "Winter time 23:59 local (GMT+1)",
			localTime:   "23:59",
			tzOffset:    "GMT +01:00",
			expectedUTC: "2025-11-15T22:59:00Z",
			expectError: false,
		},
		{
			name:        "Negative offset 12:00 local (GMT-5)",
			localTime:   "12:00",
			tzOffset:    "GMT -05:00",
			expectedUTC: "2025-11-15T17:00:00Z",
			expectError: false,
		},
		{
			name:        "Invalid time format",
			localTime:   "invalid",
			tzOffset:    "GMT +02:00",
			expectError: true,
		},
		{
			name:        "Invalid timezone format",
			localTime:   "06:00",
			tzOffset:    "invalid",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			utcTime, err := calculateUTCTime(testDate, tt.localTime, tt.tzOffset)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if utcTime != tt.expectedUTC {
					t.Errorf("Expected UTC time %s, got %s", tt.expectedUTC, utcTime)
				}
			}
		})
	}
}

func TestFindWeWorkPropertyByName(t *testing.T) {
	properties := []WeWorkProperty{
		newTestWeWorkProperty("115 Broadway", "115 Broadway, New York, NY"),
		newTestWeWorkProperty("1 Poultry", "1 Poultry, London"),
		newTestWeWorkProperty("Coeur Marais", "64-66 Rue Des Archives, Paris"),
	}

	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{name: "exact match", input: "115 Broadway", expected: "115 Broadway"},
		{name: "case insensitive", input: "115 broadway", expected: "115 Broadway"},
		{name: "extra whitespace", input: "  115   Broadway  ", expected: "115 Broadway"},
		{name: "unique partial match", input: "Poultry", expected: "1 Poultry"},
		{name: "ligature insensitive", input: "Cœur Marais", expected: "Coeur Marais"},
		{name: "address match", input: "Rue Des Archives", expected: "Coeur Marais"},
		{name: "missing match", input: "Waterloo", expectError: true},
		{name: "blank name", input: " ", expectError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			property, err := findWeWorkPropertyByName(properties, tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if property.Title != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, property.Title)
			}
		})
	}
}

func TestFindWeWorkPropertyByNameRejectsAmbiguousPartialMatches(t *testing.T) {
	properties := []WeWorkProperty{
		newTestWeWorkProperty("Waterloo Station", "Waterloo Station"),
		newTestWeWorkProperty("Waterloo Road", "Waterloo Road"),
	}

	if _, err := findWeWorkPropertyByName(properties, "Waterloo"); err == nil {
		t.Errorf("Expected ambiguous match error")
	}
}

func TestWeWorkPropertyAllowsStringCoworkingPropertyID(t *testing.T) {
	var property WeWorkProperty
	if err := json.Unmarshal([]byte(`{"id":"location-id","title":"33 Rue la Fayette","coworkingPropertyId":"456","position":{"lat":"48.87","lng":"2.33"}}`), &property); err != nil {
		t.Fatalf("Unexpected error decoding property: %v", err)
	}

	if property.Title != "33 Rue la Fayette" {
		t.Fatalf("Expected property title to be decoded, got %q", property.Title)
	}
}

func newTestWeWorkProperty(title string, address string) WeWorkProperty {
	return WeWorkProperty{ID: title, Title: title, Address: address}
}

func TestMakeBookingRequestUsesLocationTimezone(t *testing.T) {
	var location WeWorkLocation
	response := `{"uuid":"space-id","capacity":"schema-change","reservable":{"KubeId":"kube-id","capacity":"1"},"location":{"uuid":"location-id","name":"Test Location","timezoneOffset":"GMT +02:00","latitude":"48.87","address":{"line1":"Test Street","city":"Paris","country":"France"}},"openTime":"08:00","closeTime":"18:00"}`
	if err := json.Unmarshal([]byte(response), &location); err != nil {
		t.Fatalf("Unexpected error decoding location: %v", err)
	}

	if location.Location.TimezoneOffset != "GMT +02:00" {
		t.Fatalf("Unexpected timezone offset: %q", location.Location.TimezoneOffset)
	}
	if location.Reservable.KubeID != "kube-id" {
		t.Fatalf("Unexpected Kube ID: %q", location.Reservable.KubeID)
	}
}

func TestBookingResponseAllowsStructuredErrors(t *testing.T) {
	var response BookingResponse
	if err := json.Unmarshal([]byte(`{"BookingStatus":"BookingFailed","Errors":[{"code":"NO_CREDITS","message":"No credits remaining"}]}`), &response); err != nil {
		t.Fatalf("Unexpected error decoding booking response: %v", err)
	}
}

func TestTruncateForLog(t *testing.T) {
	if got := truncateForLog("  123456  ", 4); got != "1234..." {
		t.Fatalf("Unexpected truncated value: %q", got)
	}
}
