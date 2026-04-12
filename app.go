package main

// app.go — Application struct and Wails-bound methods
// Every public method on *App is automatically callable from JavaScript
// via window.go.main.App.MethodName(args).
// All methods must return JSON-serialisable types.

import (
	"context"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App is the main application struct. It holds all runtime state and
// is bound to the Wails frontend. Public methods become JS-callable.
type App struct {
	// ctx is the Wails application context, set during startup.
	// Used for native dialogs, events, and lifecycle management.
	ctx context.Context

	// targetPhotos holds the scanned photos that lack GPS data.
	targetPhotos []TargetPhoto

	// referencePhotos holds all geolocated reference photos from added folders.
	referencePhotos []ReferencePhoto

	// gpsTrackPoints holds all GPS track points from imported GPX/KML/CSV files.
	gpsTrackPoints []GPSTrackPoint

	// matchResults stores the output of the matching engine.
	matchResults []MatchResult

	// scanInProgress indicates whether a scan or match operation is running.
	scanInProgress bool
}

// NewApp creates a new App instance with empty state.
func NewApp() *App {
	return &App{}
}

// startup is called when the Wails app is ready. It stores the context
// which is needed for native OS dialogs and other Wails runtime features.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// OpenFolderDialog opens the native OS folder picker and returns the selected path.
// Returns an empty string if the user cancels the dialog.
func (a *App) OpenFolderDialog() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Folder",
	})
}

// ScanTargetFolder scans the given folder for photos without GPS data.
// Results are stored in a.targetPhotos and returned to the frontend.
// TODO: Implement in Phase 1
func (a *App) ScanTargetFolder(path string) map[string]interface{} {
	return map[string]interface{}{
		"status": "not_implemented",
		"path":   path,
	}
}

// AddReferenceFolder scans the given folder for geolocated reference photos.
// Results are appended to a.referencePhotos (supports multiple folders).
// TODO: Implement in Phase 2
func (a *App) AddReferenceFolder(path string) map[string]interface{} {
	return map[string]interface{}{
		"status": "not_implemented",
		"path":   path,
	}
}

// ImportGPSTrack parses a GPX, KML, or CSV file and adds track points
// to a.gpsTrackPoints. File format is detected by extension.
// TODO: Implement in Phase 3
func (a *App) ImportGPSTrack(path string) map[string]interface{} {
	return map[string]interface{}{
		"status": "not_implemented",
		"path":   path,
	}
}

// RunMatching executes the GPS matching engine. For each target photo,
// it finds the best reference photo or track point by timestamp proximity.
// TODO: Implement in Phase 4
func (a *App) RunMatching(opts MatchOptions) map[string]interface{} {
	return map[string]interface{}{
		"status": "not_implemented",
	}
}

// GetMatchResults returns the current matching results.
// Called by the frontend to poll progress during matching.
// TODO: Implement in Phase 4
func (a *App) GetMatchResults() map[string]interface{} {
	return map[string]interface{}{
		"status":  "not_implemented",
		"results": []MatchResult{},
	}
}

// GetThumbnail generates and returns a base64-encoded JPEG thumbnail for the given image.
// Returns an empty string for HEIC files (no pixel decoding possible without CGo).
// Supported: JPG, PNG, DNG, ARW.
// TODO: Implement in Phase 1
func (a *App) GetThumbnail(path string) string {
	return ""
}

// ApplyGPS writes GPS coordinates to a single target photo's EXIF data.
// Creates a .bak backup before modifying the file.
// TODO: Implement in Phase 6
func (a *App) ApplyGPS(targetPath string, lat float64, lon float64) map[string]interface{} {
	return map[string]interface{}{
		"status": "not_implemented",
	}
}

// ApplyAllMatches batch-applies GPS coordinates to all accepted matches.
// Creates backups for each file. Operation is cancellable.
// TODO: Implement in Phase 6
func (a *App) ApplyAllMatches() map[string]interface{} {
	return map[string]interface{}{
		"status": "not_implemented",
	}
}

// GetScanStatus returns the current progress of any running scan or match operation.
// TODO: Implement in Phase 1
func (a *App) GetScanStatus() ScanStatus {
	return ScanStatus{
		InProgress: false,
		Phase:      "idle",
		Progress:   0,
		Total:      0,
	}
}
