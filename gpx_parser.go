package main

// gpx_parser.go — GPS track file parsing (GPX, KML, CSV)
// Parses external GPS data files into a flat list of GPSTrackPoint entries.
//
// Supported formats:
//   - GPX (GPS Exchange Format) — XML, standard from GPS devices and apps
//   - KML (Keyhole Markup Language) — XML, Google Earth/Maps export
//   - CSV — expects columns: timestamp, latitude, longitude
//
// All timestamps are parsed and normalized to UTC.
// GPX uses ISO 8601 format. KML may use various date formats.
// CSV timestamp format is auto-detected (ISO 8601 preferred).
//
// Key functions:
//   - ParseGPXFile(path) → ([]GPSTrackPoint, error)
//   - ParseKMLFile(path) → ([]GPSTrackPoint, error)
//   - ParseCSVFile(path) → ([]GPSTrackPoint, error)
//   - DetectAndParseTrackFile(path) → ([]GPSTrackPoint, error)  — dispatches by extension
//
// TODO: Implement in Phase 3
