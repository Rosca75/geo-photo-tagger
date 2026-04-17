package main

import (
	"testing"
	"time"
)

func TestParseEXIFDateTimeToUTC(t *testing.T) {
	tests := []struct {
		name    string
		dt      string
		offset  string
		wantUTC string // RFC3339
	}{
		{
			name:    "iPhone with negative offset",
			dt:      "2025:02:14 10:21:19",
			offset:  "-05:00",
			wantUTC: "2025-02-14T15:21:19Z",
		},
		{
			name:    "CET photo with positive offset",
			dt:      "2025:02:14 16:05:59",
			offset:  "+01:00",
			wantUTC: "2025-02-14T15:05:59Z",
		},
		// The no-offset case depends on GetDefaultTimezone() which reads
		// settings from disk; cover that via an integration test rather
		// than a unit test.
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEXIFDateTimeToUTC(tc.dt, tc.offset)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			want, _ := time.Parse(time.RFC3339, tc.wantUTC)
			if !got.Equal(want) {
				t.Errorf("got %v, want %v", got.UTC(), want)
			}
		})
	}
}
