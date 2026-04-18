package main

// gpx_parser.go — GPX track file parsing and shared dispatcher.
// Handles .gpx files (GPS Exchange Format) — the standard XML format from
// GPS devices and sports / hiking apps.
//
// Also contains parseTimestamp() (used by kml_parser.go and csv_parser.go)
// and DetectAndParseTrackFile() which dispatches by file extension.

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ─── GPX XML struct types ────────────────────────────────────────────────────

// gpxFile maps the root <gpx> element.
type gpxFile struct {
	XMLName   xml.Name   `xml:"gpx"`
	Tracks    []gpxTrack `xml:"trk"`
	Waypoints []gpxPoint `xml:"wpt"` // standalone waypoints
}

// gpxTrack maps a <trk> element (one recorded journey).
type gpxTrack struct {
	Segments []gpxSegment `xml:"trkseg"`
}

// gpxSegment maps a <trkseg> (continuous section; new segment when signal lost).
type gpxSegment struct {
	Points []gpxPoint `xml:"trkpt"`
}

// gpxPoint maps a <trkpt> or <wpt> element.
// Lat/Lon are XML attributes; Time is an optional child.
type gpxPoint struct {
	Lat  float64 `xml:"lat,attr"`
	Lon  float64 `xml:"lon,attr"`
	Time string  `xml:"time"`
}

// ─── GPX parsing ─────────────────────────────────────────────────────────────

// ParseGPXFile reads a GPX file and returns all track points and waypoints as a
// flat slice. GPX timestamps are ISO 8601 / RFC 3339, always in UTC.
func ParseGPXFile(path string) ([]GPSTrackPoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading GPX %q: %w", path, err)
	}
	var g gpxFile
	if err := xml.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("parsing GPX XML in %q: %w", path, err)
	}

	var pts []GPSTrackPoint
	// Track segment points (the main source in GPX files from GPS devices)
	for _, trk := range g.Tracks {
		for _, seg := range trk.Segments {
			for _, p := range seg.Points {
				if tp, err := gpxPointToTrackPoint(p, path); err == nil {
					pts = append(pts, tp)
				}
				// Points with unparseable timestamps are silently skipped
			}
		}
	}
	// Standalone waypoints — some exporters use <wpt> instead of <trkpt>
	for _, wpt := range g.Waypoints {
		if tp, err := gpxPointToTrackPoint(wpt, path); err == nil {
			pts = append(pts, tp)
		}
	}
	return pts, nil
}

// gpxPointToTrackPoint converts a raw gpxPoint to a GPSTrackPoint.
// Returns an error when the time string cannot be parsed.
func gpxPointToTrackPoint(p gpxPoint, src string) (GPSTrackPoint, error) {
	t, err := parseTimestamp(p.Time)
	if err != nil {
		return GPSTrackPoint{}, err
	}
	return GPSTrackPoint{
		Time:       t,
		GPS:        GPSCoord{Latitude: p.Lat, Longitude: p.Lon},
		SourceFile: src,
	}, nil
}

// ─── Shared timestamp utility ─────────────────────────────────────────────────

// parseTimestamp tries several common timestamp layouts and returns a UTC time.
// Used by GPX, KML, and CSV parsers — all live in the same package so any can call this.
func parseTimestamp(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	layouts := []string{
		time.RFC3339,           // "2006-01-02T15:04:05Z07:00" — GPX standard
		"2006-01-02T15:04:05Z", // UTC with literal Z (subset of RFC 3339)
		"2006-01-02T15:04:05",  // ISO 8601 no timezone (treated as UTC)
		"2006-01-02 15:04:05",  // space-separated (common CSV export)
		"2006/01/02 15:04:05",  // slash-date (some apps)
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised timestamp format: %q", s)
}

// ─── Format dispatcher ────────────────────────────────────────────────────────

// DetectAndParseTrackFile chooses the correct parser by file extension.
// Supported: .gpx, .kml, .csv — returns an error for anything else.
func DetectAndParseTrackFile(path string) ([]GPSTrackPoint, error) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".gpx":
		return ParseGPXFile(path)
	case ".kml":
		return ParseKMLFile(path)
	case ".csv":
		return ParseCSVFile(path)
	default:
		return nil, fmt.Errorf("unsupported track format for %q (expected .gpx, .kml, or .csv)",
			filepath.Base(path))
	}
}
