package main

// types.go — Shared type definitions for GeoPhotoTagger
// This file contains all struct definitions used across the application.
// No business logic lives here — only data shapes.

import "time"

// TargetPhoto represents a photo that lacks GPS data and needs geotagging.
// These are the files the user wants to add coordinates to.
type TargetPhoto struct {
	// Path is the absolute filesystem path to the photo file.
	Path string `json:"path"`

	// Filename is just the base name (e.g. "DSC_1234.JPG") for display.
	Filename string `json:"filename"`

	// Extension is the lowercase file extension (e.g. ".jpg", ".dng").
	Extension string `json:"extension"`

	// DateTimeOriginal is the timestamp from EXIF when the photo was taken.
	// This is the key field used for matching against reference photos.
	DateTimeOriginal time.Time `json:"dateTimeOriginal"`

	// CameraModel is the EXIF camera model string (e.g. "NIKON D850").
	CameraModel string `json:"cameraModel"`

	// FileSizeBytes is the file size in bytes, for display purposes.
	FileSizeBytes int64 `json:"fileSizeBytes"`

	// Status tracks the current state of this photo in the workflow.
	// Possible values: "unmatched", "matched", "applied", "error"
	Status string `json:"status"`

	// BestMatch holds the best matching candidate after RunMatching, if any.
	// Nil if no match was found within the configured time threshold.
	BestMatch *MatchCandidate `json:"bestMatch"`
}

// ReferencePhoto represents a geolocated photo from a phone, tablet, or other device.
// These provide the GPS coordinates to copy to target photos.
type ReferencePhoto struct {
	// Path is the absolute filesystem path to the reference photo.
	Path string `json:"path"`

	// Filename is the base name for display.
	Filename string `json:"filename"`

	// Extension is the lowercase file extension.
	Extension string `json:"extension"`

	// DateTimeOriginal is the EXIF timestamp when this reference photo was taken.
	DateTimeOriginal time.Time `json:"dateTimeOriginal"`

	// GPS holds the extracted GPS coordinates from this photo's EXIF data.
	GPS GPSCoord `json:"gps"`

	// CameraModel is the device that took this photo (e.g. "iPhone 15 Pro").
	CameraModel string `json:"cameraModel"`

	// SourceFolder is the reference folder this photo was found in.
	// Useful when the user has added multiple reference folders.
	SourceFolder string `json:"sourceFolder"`

	// IsHEIC indicates this is a HEIC file — no thumbnail can be generated.
	// The frontend should show a placeholder icon instead.
	IsHEIC bool `json:"isHeic"`
}

// GPSCoord holds a latitude/longitude pair.
type GPSCoord struct {
	// Latitude in decimal degrees. Positive = North, Negative = South.
	Latitude float64 `json:"latitude"`

	// Longitude in decimal degrees. Positive = East, Negative = West.
	Longitude float64 `json:"longitude"`
}

// GPSTrackPoint represents a single point from a GPX, KML, or CSV track file.
// Track files provide a continuous GPS trail for interpolation.
type GPSTrackPoint struct {
	// Time is the UTC timestamp of this track point.
	Time time.Time `json:"time"`

	// GPS holds the coordinates at this point in the track.
	GPS GPSCoord `json:"gps"`

	// SourceFile is the track file this point came from.
	SourceFile string `json:"sourceFile"`
}

// MatchOptions configures the GPS matching engine.
// These are set by the user in the UI before running a match.
type MatchOptions struct {
	// MaxTimeDeltaMinutes is the maximum time difference (in minutes) between
	// a target photo and a reference photo/track point for a match to be considered.
	// Default: 30. Common values: 10, 30, 60.
	MaxTimeDeltaMinutes int `json:"maxTimeDeltaMinutes"`
}

// MatchResult holds the matching output for a single target photo.
// It contains the best match and all candidates within the time threshold.
type MatchResult struct {
	// TargetPath is the absolute path to the target photo.
	TargetPath string `json:"targetPath"`

	// BestCandidate is the highest-scored candidate, or nil if no match found.
	BestCandidate *MatchCandidate `json:"bestCandidate"`

	// Candidates is the full list of matches within the time threshold,
	// sorted by score descending. Includes the best candidate.
	Candidates []MatchCandidate `json:"candidates"`
}

// MatchCandidate represents one potential GPS source for a target photo.
// It could be a reference photo or an interpolated track point.
type MatchCandidate struct {
	// Source indicates where this GPS data came from.
	// Either "photo" (from a reference photo) or "track" (from a GPS track file).
	Source string `json:"source"`

	// SourcePath is the reference photo path or track file path.
	SourcePath string `json:"sourcePath"`

	// SourceFilename is the base name of the source, for display.
	SourceFilename string `json:"sourceFilename"`

	// GPS holds the proposed coordinates to apply to the target photo.
	GPS GPSCoord `json:"gps"`

	// TimeDelta is the absolute time difference between target and reference.
	TimeDelta time.Duration `json:"timeDelta"`

	// TimeDeltaFormatted is a human-readable time delta (e.g. "2m30s", "15m").
	TimeDeltaFormatted string `json:"timeDeltaFormatted"`

	// Score is the match quality score from 0 to 100.
	// See CLAUDE.md §6 for the scoring formula.
	Score int `json:"score"`

	// IsHEIC indicates the source is a HEIC file (no thumbnail available).
	IsHEIC bool `json:"isHeic"`

	// IsInterpolated indicates the GPS was interpolated between two track points
	// rather than taken directly from a single reference.
	IsInterpolated bool `json:"isInterpolated"`
}

// ScanStatus reports the progress of the current scan or match operation.
// The frontend polls this to update the progress bar.
type ScanStatus struct {
	// InProgress is true while a scan or match operation is running.
	InProgress bool `json:"inProgress"`

	// Phase describes what is currently happening.
	// Values: "idle", "scanning_targets", "scanning_references", "matching", "applying"
	Phase string `json:"phase"`

	// Progress is the number of items processed so far.
	Progress int `json:"progress"`

	// Total is the total number of items to process (0 if unknown).
	Total int `json:"total"`

	// Message is an optional human-readable status message.
	Message string `json:"message"`
}

// EXIFData holds the parsed EXIF fields we care about from a photo.
// This is the output of ReadEXIF() and ReadHEICExif().
type EXIFData struct {
	// HasGPS is true if the photo has valid GPS latitude and longitude.
	HasGPS bool `json:"hasGps"`

	// Latitude in decimal degrees (0 if no GPS).
	Latitude float64 `json:"latitude"`

	// Longitude in decimal degrees (0 if no GPS).
	Longitude float64 `json:"longitude"`

	// DateTimeOriginal is when the photo was taken.
	// Zero value if the EXIF field is missing or unparseable.
	DateTimeOriginal time.Time `json:"dateTimeOriginal"`

	// HasDateTime is true if DateTimeOriginal was successfully parsed.
	HasDateTime bool `json:"hasDateTime"`

	// CameraModel is the camera/device model string from EXIF.
	CameraModel string `json:"cameraModel"`
}

// GPSApplyRequest is the payload sent by the frontend for each photo in a batch GPS write.
// ApplyBatchGPS accepts a slice of these so the JS acceptedMatches Map can be sent over.
type GPSApplyRequest struct {
	// TargetPath is the absolute path to the photo file that should be geotagged.
	TargetPath string `json:"targetPath"`

	// Lat is the decimal-degree latitude to write (positive = North).
	Lat float64 `json:"lat"`

	// Lon is the decimal-degree longitude to write (positive = East).
	Lon float64 `json:"lon"`
}

// GPSTrackFile describes a single imported GPS track file.
// Returned by ImportGPSTrack and GetGPSTracks so the frontend can list what was loaded.
type GPSTrackFile struct {
	// Path is the absolute filesystem path to the track file.
	Path string `json:"path"`

	// Filename is just the base name (e.g. "hike.gpx") for display in the UI.
	Filename string `json:"filename"`

	// PointCount is the number of valid GPS track points parsed from the file.
	PointCount int `json:"pointCount"`

	// Format is the lowercase extension without the dot: "gpx", "kml", or "csv".
	Format string `json:"format"`
}
