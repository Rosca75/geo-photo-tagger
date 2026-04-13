package main

// kml_parser.go — KML GPS track file parsing.
// Supports two KML variants:
//   Standard KML — <Placemark> with <TimeStamp><when> + <Point><coordinates>
//   Google gx:Track — parallel <when> and <gx:coord> sibling lists
//
// Go's encoding/xml requires full namespace URIs for prefixed elements like <gx:coord>.
// We strip all "gx:" prefixes from the raw bytes before decoding — safe for GPS track
// KML files because "gx:" only appears in element names, never in text content.

import (
	"encoding/xml"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ─── KML XML struct types ────────────────────────────────────────────────────

// kmlRoot maps the root <kml> element.
// Captures placemarks at document level and one folder deep (common export structures).
type kmlRoot struct {
	XMLName    xml.Name       `xml:"kml"`
	Placemarks []kmlPlacemark `xml:"Document>Placemark"`
	FolderPMs  []kmlPlacemark `xml:"Document>Folder>Placemark"`
}

// kmlPlacemark represents one KML Placemark.
// After "gx:" stripping, <gx:Track> → <Track> and <gx:coord> → <coord>.
type kmlPlacemark struct {
	// Standard KML fields
	When        string `xml:"TimeStamp>when"`
	Coordinates string `xml:"Point>coordinates"`
	// Google gx:Track fields (after namespace prefix stripping)
	GxWhens  []string `xml:"Track>when"`
	GxCoords []string `xml:"Track>coord"`
}

// ─── KML parsing ─────────────────────────────────────────────────────────────

// ParseKMLFile reads a KML file and returns all timestamped GPS points.
// Placemarks without both a timestamp and coordinates are silently skipped.
func ParseKMLFile(path string) ([]GPSTrackPoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading KML %q: %w", path, err)
	}
	// Strip "gx:" so the XML decoder can match Track/coord without namespace URIs
	stripped := strings.NewReplacer("gx:", "").Replace(string(data))

	var root kmlRoot
	if err := xml.Unmarshal([]byte(stripped), &root); err != nil {
		return nil, fmt.Errorf("parsing KML XML in %q: %w", path, err)
	}

	all := append(root.Placemarks, root.FolderPMs...)
	var pts []GPSTrackPoint
	for _, pm := range all {
		pts = append(pts, kmlPlacemarkToPoints(pm, path)...)
	}
	return pts, nil
}

// kmlPlacemarkToPoints extracts track points from one KML Placemark.
// Tries gx:Track (multiple points) first, then standard Placemark (one point).
func kmlPlacemarkToPoints(pm kmlPlacemark, src string) []GPSTrackPoint {
	var pts []GPSTrackPoint

	// gx:Track branch — parallel <when>/<coord> lists
	if len(pm.GxWhens) > 0 {
		count := len(pm.GxWhens)
		if len(pm.GxCoords) < count {
			count = len(pm.GxCoords)
		}
		for i := 0; i < count; i++ {
			t, err := parseTimestamp(pm.GxWhens[i])
			if err != nil {
				continue
			}
			lat, lon, ok := parseGxCoord(pm.GxCoords[i])
			if !ok {
				continue
			}
			pts = append(pts, GPSTrackPoint{
				Time: t, GPS: GPSCoord{Latitude: lat, Longitude: lon}, SourceFile: src,
			})
		}
		return pts
	}

	// Standard KML Placemark — one timestamp + one Point
	if pm.When == "" || pm.Coordinates == "" {
		return pts
	}
	t, err := parseTimestamp(pm.When)
	if err != nil {
		return pts
	}
	lat, lon, ok := parseKMLCoordinates(pm.Coordinates)
	if !ok {
		return pts
	}
	pts = append(pts, GPSTrackPoint{
		Time: t, GPS: GPSCoord{Latitude: lat, Longitude: lon}, SourceFile: src,
	})
	return pts
}

// parseGxCoord parses a "lon lat alt" gx:coord string (space-separated, longitude first).
func parseGxCoord(s string) (lat, lon float64, ok bool) {
	f := strings.Fields(strings.TrimSpace(s))
	if len(f) < 2 {
		return 0, 0, false
	}
	lon, err := strconv.ParseFloat(f[0], 64)
	if err != nil {
		return 0, 0, false
	}
	lat, err = strconv.ParseFloat(f[1], 64)
	if err != nil {
		return 0, 0, false
	}
	return lat, lon, true
}

// parseKMLCoordinates parses a standard KML "lon,lat,alt" string (comma-separated).
// KML stores longitude first — opposite of the common lat/lon convention.
func parseKMLCoordinates(s string) (lat, lon float64, ok bool) {
	parts := strings.Split(strings.TrimSpace(s), ",")
	if len(parts) < 2 {
		return 0, 0, false
	}
	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, false
	}
	lat, err = strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, false
	}
	return lat, lon, true
}
