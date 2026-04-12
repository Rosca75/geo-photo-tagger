package main

// app.go — Application struct and all Wails-bound methods.
// Every public method on *App is callable from JavaScript via window.go.main.App.*
// All return values are automatically serialised to JSON by Wails.

import (
	"context"
	"fmt"
	"log"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App holds all runtime application state and is bound to the Wails frontend.
type App struct {
	// ctx is the Wails application context — needed for native OS dialogs.
	ctx context.Context

	// targetPhotos holds photos without GPS, set by ScanTargetFolder.
	targetPhotos []TargetPhoto

	// referencePhotos accumulates geolocated photos from all reference folders.
	referencePhotos []ReferencePhoto

	// gpsTrackPoints holds all points from imported GPX/KML/CSV track files.
	gpsTrackPoints []GPSTrackPoint

	// matchResults stores the output of the GPS matching engine.
	matchResults []MatchResult

	// scanStatus tracks the current scan or match operation progress.
	scanStatus ScanStatus
}

// NewApp creates a new App instance with zero/empty state.
func NewApp() *App {
	return &App{}
}

// startup is called by Wails when the application window is ready.
// It stores the context which is required for runtime.OpenDirectoryDialog etc.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// OpenFolderDialog opens the native OS folder picker dialog.
// Returns the selected path, or an empty string if the user cancels.
func (a *App) OpenFolderDialog() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select Folder",
	})
}

// ScanTargetFolder walks folderPath for photos without GPS EXIF data.
// Updates scanStatus during the operation so GetScanStatus can report progress.
// Returns the full list of target photos directly to the frontend.
func (a *App) ScanTargetFolder(path string) ([]TargetPhoto, error) {
	// Signal to any polling frontend that a scan is running
	a.scanStatus = ScanStatus{
		InProgress: true,
		Phase:      "scanning_targets",
		Message:    "Scanning for photos without GPS...",
	}

	photos, err := ScanForTargetPhotos(path)
	if err != nil {
		a.scanStatus = ScanStatus{Phase: "idle", Message: err.Error()}
		return nil, err
	}

	// Persist results in app state for other methods to reference
	a.targetPhotos = photos
	a.scanStatus = ScanStatus{
		Phase:    "idle",
		Progress: len(photos),
		Total:    len(photos),
		Message:  fmt.Sprintf("Found %d photos without GPS", len(photos)),
	}
	return photos, nil
}

// AddReferenceFolder scans folderPath for geolocated reference photos.
// Appends to the existing list — multiple reference folders are supported.
// TODO: Implement in Phase 2
func (a *App) AddReferenceFolder(path string) map[string]interface{} {
	return map[string]interface{}{"status": "not_implemented", "path": path}
}

// ImportGPSTrack parses a GPX, KML, or CSV file and stores the track points.
// TODO: Implement in Phase 3
func (a *App) ImportGPSTrack(path string) map[string]interface{} {
	return map[string]interface{}{"status": "not_implemented", "path": path}
}

// RunMatching executes the GPS matching engine against targets and references.
// TODO: Implement in Phase 4
func (a *App) RunMatching(opts MatchOptions) map[string]interface{} {
	return map[string]interface{}{"status": "not_implemented"}
}

// GetMatchResults returns current matching results for frontend polling.
// TODO: Implement in Phase 4
func (a *App) GetMatchResults() map[string]interface{} {
	return map[string]interface{}{"status": "not_implemented", "results": []MatchResult{}}
}

// GetThumbnail returns a base64-encoded JPEG thumbnail for the image at path.
// Returns "" for HEIC files or when thumbnail generation fails.
// The frontend uses: img.src = "data:image/jpeg;base64," + result
func (a *App) GetThumbnail(path string) string {
	thumb, err := GenerateThumbnail(path, 200)
	if err != nil {
		log.Printf("Thumbnail error for %s: %v", path, err)
		return ""
	}
	return thumb
}

// ApplyGPS writes GPS coordinates to a single target photo's EXIF data.
// Creates a .bak backup before modifying the file.
// TODO: Implement in Phase 6
func (a *App) ApplyGPS(targetPath string, lat float64, lon float64) map[string]interface{} {
	return map[string]interface{}{"status": "not_implemented"}
}

// ApplyAllMatches batch-applies GPS to all accepted matches with backups.
// TODO: Implement in Phase 6
func (a *App) ApplyAllMatches() map[string]interface{} {
	return map[string]interface{}{"status": "not_implemented"}
}

// GetScanStatus returns the current scan or match operation progress.
// The frontend polls this during long-running operations to update the UI.
func (a *App) GetScanStatus() ScanStatus {
	return a.scanStatus
}
