// api.js — Wails API isolation layer
// This module wraps ALL window.go.main.App.* calls.
// NO other module should call window.go.* directly. See CLAUDE.md rule #9.
//
// Each function returns a Promise that resolves to the Go method's return value.

// Open the native OS folder picker dialog.
// Returns the selected folder path, or empty string if cancelled.
export async function openFolderDialog() {
    return window.go.main.App.OpenFolderDialog();
}

// Open the native OS file picker dialog, filtered to GPS track formats (.gpx, .kml, .csv).
// Returns the selected file path, or empty string if cancelled.
export async function openFileDialog() {
    return window.go.main.App.OpenFileDialog();
}

// Scan the target folder for photos without GPS data.
export async function scanTargetFolder(path) {
    return window.go.main.App.ScanTargetFolder(path);
}

// Add a reference folder containing geolocated photos.
export async function addReferenceFolder(path) {
    return window.go.main.App.AddReferenceFolder(path);
}

// Import a GPS track file (GPX, KML, or CSV).
// Returns a GPSTrackFile object: { path, filename, pointCount, format }.
export async function importGPSTrack(path) {
    return window.go.main.App.ImportGPSTrack(path);
}

// Return the list of GPS track files currently imported in this session.
export async function getGPSTracks() {
    return window.go.main.App.GetGPSTracks();
}

// Remove a previously imported GPS track file and its points from the pool.
export async function removeGPSTrack(path) {
    return window.go.main.App.RemoveGPSTrack(path);
}

// Run the GPS matching engine with the given options.
export async function runMatching(opts) {
    return window.go.main.App.RunMatching(opts);
}

// Get the current matching results (poll during matching).
export async function getMatchResults() {
    return window.go.main.App.GetMatchResults();
}

// Get a base64-encoded JPEG thumbnail for the given image path.
// Returns empty string for HEIC files.
export async function getThumbnail(path) {
    return window.go.main.App.GetThumbnail(path);
}

// Apply GPS coordinates to a single target photo.
export async function applyGPS(targetPath, lat, lon) {
    return window.go.main.App.ApplyGPS(targetPath, lat, lon);
}

// Batch-apply GPS coordinates to all accepted matches.
export async function applyAllMatches() {
    return window.go.main.App.ApplyAllMatches();
}

// Get the current scan/match progress status.
export async function getScanStatus() {
    return window.go.main.App.GetScanStatus();
}
