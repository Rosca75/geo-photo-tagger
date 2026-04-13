package main

// app.go — Application struct and all Wails-bound methods.
// Every public method on *App is callable from JavaScript via window.go.main.App.*
// All return values are automatically serialised to JSON by Wails.

import (
	"context"
	"fmt"
	"log/slog"

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

	// gpsTrackPoints holds all track points accumulated from every imported file.
	gpsTrackPoints []GPSTrackPoint

	// gpsTrackFiles holds one descriptor per imported track file (path, point count).
	// Used by GetGPSTracks() so the frontend can list what has been imported.
	gpsTrackFiles []GPSTrackFile

	// referenceFolderList tracks folders added via AddReferenceFolder.
	// Each entry holds the path and number of geolocated photos found.
	referenceFolderList []ReferenceFolderInfo

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
	// Enable debug logging during development so per-file EXIF timing is visible.
	// Change to setupLogger(false) for production builds.
	setupLogger(true)
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

	// Use the parallel scanner — 0 means "default workers" (min(NumCPU, 8)).
	photos, err := a.ScanForTargetPhotosParallel(path, 0)
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

// GetThumbnail returns a base64-encoded JPEG thumbnail for the image at path.
// Returns "" for HEIC files or when thumbnail generation fails.
// The frontend uses: img.src = "data:image/jpeg;base64," + result
func (a *App) GetThumbnail(path string) string {
	thumb, err := GenerateThumbnail(path, 200)
	if err != nil {
		slog.Warn("thumbnail_failed", "path", path, "error", err.Error())
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
