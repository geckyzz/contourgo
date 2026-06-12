package scraper

import (
	"testing"
	"time"
)

func TestParseATTime(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{
			name:     "Today format",
			input:    "Today 18:38",
			expected: time.Date(now.Year(), now.Month(), now.Day(), 18, 38, 0, 0, time.UTC).Unix(),
		},
		{
			name:     "Yesterday format",
			input:    "Yesterday 16:35",
			expected: time.Date(now.Year(), now.Month(), now.Day(), 16, 35, 0, 0, time.UTC).AddDate(0, 0, -1).Unix(),
		},
		{
			name:     "Absolute date format",
			input:    "24/05/2026 12:48",
			expected: time.Date(2026, time.May, 24, 12, 48, 0, 0, time.UTC).Unix(),
		},
		{
			name:     "Absolute date format with short year",
			input:    "24/05/26 12:48",
			expected: time.Date(2026, time.May, 24, 12, 48, 0, 0, time.UTC).Unix(),
		},
		{
			name:     "Absolute date format with UTC suffix",
			input:    "12/06/2026 18:46 UTC",
			expected: time.Date(2026, time.June, 12, 18, 46, 0, 0, time.UTC).Unix(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseATTime(tt.input)
			if got != tt.expected {
				t.Errorf("parseATTime(%q) = %v, want %v", tt.input, time.Unix(got, 0).UTC(), time.Unix(tt.expected, 0).UTC())
			}
		})
	}
}
