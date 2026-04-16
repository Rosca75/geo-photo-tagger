package main

// app.go — Application struct and all Wails-bound methods.
// Every public method on *App is callable from JavaScript via window.go.main.App.*
// All return values are automatically serialised to JSON by Wails.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App holds all runtime application state and is bound to the Wails frontend.
type App struct {
	// ctx is the Wails application context — needed for native OS dialogs.
	ctx context.Context

	// targetFolder is the path last passed to ScanTargetFolder.
	// Used by ClearAllBackups to know which folder to search for .bak files.
	targetFolder string

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
	// Debug-level logging is opt-in via the GPT_DEBUG_LOG env var so typical
	// wails dev sessions are not flooded with one log line per photo.
	// Set GPT_DEBUG_LOG=1 before launching wails dev to restore per-file timing.
	setupLogger(os.Getenv("GPT_DEBUG_LOG") == "1")
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

	// Remember the target folder so ClearAllBackups knows where to look later.
	a.targetFolder = path

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

// GetScanStatus returns the current scan or match operation progress.
// The frontend polls this during long-running operations to update the UI.
func (a *App) GetScanStatus() ScanStatus {
	return a.scanStatus
}

// ReverseGeocode returns a human-readable location string for the given
// coordinates by calling the free OpenStreetMap Nominatim API. A 5-second
// timeout is enforced so the UI never hangs waiting on the network.
//
// Returns an empty string on any failure rather than an error — reverse
// geocoding is a nice-to-have, and the UI gracefully degrades to raw
// coordinates when no location name is available.
func (a *App) ReverseGeocode(lat, lon float64) string {
	url := fmt.Sprintf("https://nominatim.openstreetmap.org/reverse?lat=%f&lon=%f&format=json&zoom=10", lat, lon)

	// Short timeout — the user is waiting; better to show raw coords than to stall.
	client := &http.Client{Timeout: 5 * time.Second}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return ""
	}
	// Nominatim's usage policy requires a descriptive User-Agent.
	req.Header.Set("User-Agent", "GeoPhotoTagger/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	// Minimal struct — only the address fields we want to display.
	var result struct {
		DisplayName string `json:"display_name"`
		Address     struct {
			City    string `json:"city"`
			Town    string `json:"town"`
			Village string `json:"village"`
			County  string `json:"county"`
			State   string `json:"state"`
			Country string `json:"country"`
		} `json:"address"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ""
	}

	// Pick the most specific "place name" available — Nominatim returns
	// different fields depending on feature type.
	city := result.Address.City
	if city == "" {
		city = result.Address.Town
	}
	if city == "" {
		city = result.Address.Village
	}
	if city == "" {
		city = result.Address.County
	}

	// Build a concise "City, State, Country" string, skipping empty parts.
	parts := make([]string, 0, 3)
	if city != "" {
		parts = append(parts, city)
	}
	if result.Address.State != "" {
		parts = append(parts, result.Address.State)
	}
	if result.Address.Country != "" {
		parts = append(parts, result.Address.Country)
	}

	if len(parts) == 0 {
		return result.DisplayName
	}
	return strings.Join(parts, ", ")
}
