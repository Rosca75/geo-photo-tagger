package main

// app_track.go — GPS track file import/management methods for the App struct.
// These methods are bound to the Wails frontend alongside those in app.go.
//
// Exported methods (callable from JavaScript via window.go.main.App.*):
//   OpenFileDialog()           → (string, error)       — native file picker for tracks
//   ImportGPSTrack(path)       → (GPSTrackFile, error)  — parse + store a track file
//   GetGPSTracks()             → []GPSTrackFile         — list currently imported tracks
//   RemoveGPSTrack(path)       → error                  — drop a track and its points

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// OpenFileDialog opens the native OS file picker restricted to GPS track formats.
// Returns the selected file path, or "" if the user cancels without choosing a file.
func (a *App) OpenFileDialog() (string, error) {
	return runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import GPS Track File",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "GPS Track Files (*.gpx, *.kml, *.csv)",
				Pattern:     "*.gpx;*.kml;*.csv",
			},
		},
	})
}

// ImportGPSTrack parses a GPS track file and appends its points to the shared pool.
// The format is detected automatically by file extension (.gpx, .kml, .csv).
//
// If the same file is imported again, its previously loaded points are replaced
// with fresh data from the file (useful when the file has been edited on disk).
//
// Returns a GPSTrackFile descriptor containing the filename and point count.
func (a *App) ImportGPSTrack(path string) (GPSTrackFile, error) {
	// Parse the file — DetectAndParseTrackFile dispatches by extension
	pts, err := DetectAndParseTrackFile(path)
	if err != nil {
		return GPSTrackFile{}, fmt.Errorf("importing GPS track: %w", err)
	}

	// Build the descriptor we'll return to the frontend
	tf := GPSTrackFile{
		Path:       path,
		Filename:   filepath.Base(path),
		PointCount: len(pts),
		// Strip the leading dot from the extension: ".gpx" → "gpx"
		Format: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."),
	}

	// Remove any points that were previously loaded from this same file,
	// then append the freshly parsed points.
	a.gpsTrackPoints = removePointsFromSource(a.gpsTrackPoints, path)
	a.gpsTrackPoints = append(a.gpsTrackPoints, pts...)

	// Update the file list: replace existing entry or append a new one
	updated := false
	for i, existing := range a.gpsTrackFiles {
		if existing.Path == path {
			a.gpsTrackFiles[i] = tf
			updated = true
			break
		}
	}
	if !updated {
		a.gpsTrackFiles = append(a.gpsTrackFiles, tf)
	}

	return tf, nil
}

// GetGPSTracks returns the list of GPS track files currently imported in this session.
// Each entry includes the path, filename, format, and number of track points.
// Returns an empty slice (never nil) when no tracks have been imported.
func (a *App) GetGPSTracks() []GPSTrackFile {
	if a.gpsTrackFiles == nil {
		// Return an empty slice so JSON serialisation produces [] not null
		return []GPSTrackFile{}
	}
	return a.gpsTrackFiles
}

// RemoveGPSTrack removes all track points sourced from the given file and
// drops the file entry from the imported files list.
// This is the reverse of ImportGPSTrack.
func (a *App) RemoveGPSTrack(path string) error {
	// Drop all GPS track points that came from this file
	a.gpsTrackPoints = removePointsFromSource(a.gpsTrackPoints, path)

	// Remove the file descriptor from the list
	kept := a.gpsTrackFiles[:0] // reuse underlying array to avoid allocation
	for _, tf := range a.gpsTrackFiles {
		if tf.Path != path {
			kept = append(kept, tf)
		}
	}
	a.gpsTrackFiles = kept

	return nil
}

// removePointsFromSource returns a filtered copy of pts that excludes every point
// whose SourceFile matches sourcePath. Reuses the input slice's backing array.
func removePointsFromSource(pts []GPSTrackPoint, sourcePath string) []GPSTrackPoint {
	out := pts[:0]
	for _, p := range pts {
		if p.SourceFile != sourcePath {
			out = append(out, p)
		}
	}
	return out
}
