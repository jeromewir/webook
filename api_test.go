package main

import "testing"

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
