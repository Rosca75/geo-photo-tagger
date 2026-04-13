package main

// csv_parser.go — CSV GPS track file parsing.
// Expects three columns: timestamp, latitude, longitude.
// Column order is fixed; extra columns are ignored.
// The first row is skipped gracefully when it looks like a header (timestamp
// column cannot be parsed as a time value).

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ParseCSVFile reads a CSV track file and returns one GPSTrackPoint per valid row.
// Rows with fewer than three columns, unparseable timestamps, or non-numeric
// coordinates are silently skipped — partial data does not cause an error.
func ParseCSVFile(path string) ([]GPSTrackPoint, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening CSV %q: %w", path, err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.TrimLeadingSpace = true // handles "  48.8566" style padding
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("reading CSV rows from %q: %w", path, err)
	}

	var pts []GPSTrackPoint
	for i, row := range rows {
		if len(row) < 3 {
			continue // malformed row — skip
		}

		// parseTimestamp is defined in gpx_parser.go; same package, accessible here.
		t, err := parseTimestamp(row[0])
		if err != nil {
			if i == 0 {
				continue // row 0 with unparseable time → likely a header row
			}
			continue // subsequent bad timestamps are silently skipped
		}

		lat, err := strconv.ParseFloat(strings.TrimSpace(row[1]), 64)
		if err != nil {
			continue
		}
		lon, err := strconv.ParseFloat(strings.TrimSpace(row[2]), 64)
		if err != nil {
			continue
		}

		pts = append(pts, GPSTrackPoint{
			Time:       t,
			GPS:        GPSCoord{Latitude: lat, Longitude: lon},
			SourceFile: path,
		})
	}
	return pts, nil
}
