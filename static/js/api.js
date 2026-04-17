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
// recursive: true to descend into subfolders (pre-phase-4 default behavior).
export async function scanTargetFolder(path, recursive) {
    return window.go.main.App.ScanTargetFolder(path, recursive);
}

// Add a reference folder containing geolocated photos.
// Returns { path, photoCount } on success.
export async function addReferenceFolder(path, recursive) {
    return window.go.main.App.AddReferenceFolder(path, recursive);
}

// Return the list of reference folders currently loaded.
export async function getReferenceFolders() {
    return window.go.main.App.GetReferenceFolders();
}

// Remove a reference folder and its photos from the pool.
export async function removeReferenceFolder(path) {
    return window.go.main.App.RemoveReferenceFolder(path);
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

// Run GPS matching for a single target photo identified by its absolute path.
// Returns a MatchResult for that photo only.
export async function runMatchingSingle(targetPath, opts) {
    return window.go.main.App.RunMatchingSingle(targetPath, opts);
}

// Run Module-3 "same-source" matching: use the scanned source folder as
// both target pool and reference pool. See app_match_same.go.
export async function runSameSourceMatching(opts) {
    return window.go.main.App.RunSameSourceMatching(opts);
}

// Reverse-geocode GPS coordinates to a human-readable location string.
// Returns a string like "Paris, Île-de-France, France" or "" on error.
export async function reverseGeocode(lat, lon) {
    return window.go.main.App.ReverseGeocode(lat, lon);
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

// Get a small JPEG data URL for the hover-preview on candidate cards.
// Returns "" for HEIC or on decode failure; the caller treats "" as "no preview".
export async function getCandidateThumbnail(path) {
    return window.go.main.App.GetCandidateThumbnail(path);
}

// Apply GPS coordinates to a single target photo.
export async function applyGPS(targetPath, lat, lon) {
    return window.go.main.App.ApplyGPS(targetPath, lat, lon);
}

// Batch-apply GPS coordinates to an explicit list of {targetPath, lat, lon} objects.
// The frontend builds this list from state.acceptedMatches before calling.
export async function applyBatchGPS(matches) {
    return window.go.main.App.ApplyBatchGPS(matches);
}

// Restore a single target photo from its .bak backup file.
export async function undoGPS(targetPath) {
    return window.go.main.App.UndoGPS(targetPath);
}

// Delete all .bak files in the target folder scanned this session.
export async function clearAllBackups() {
    return window.go.main.App.ClearAllBackups();
}

// Get the current scan/match progress status.
export async function getScanStatus() {
    return window.go.main.App.GetScanStatus();
}

// Fetch persisted user settings (currently only { defaultTimezone }).
// Called once at app startup to seed state.defaultTimezone.
export async function getSettings() {
    return window.go.main.App.GetSettings();
}

// Persist the default timezone used to interpret EXIF DateTime for photos
// that lack an OffsetTimeOriginal tag. Accepts an IANA name or "Local".
export async function setDefaultTimezone(tz) {
    return window.go.main.App.SetDefaultTimezone(tz);
}
