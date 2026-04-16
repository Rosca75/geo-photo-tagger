package main

// app_reference.go — Reference folder import/management methods for the App struct.
// Reference folders contain geolocated photos whose GPS coordinates and timestamps
// are used as sources during the matching phase.
//
// Exported methods (callable from JavaScript via window.go.main.App.*):
//   AddReferenceFolder(path)     → (ReferenceFolderInfo, error)
//   GetReferenceFolders()        → []ReferenceFolderInfo
//   RemoveReferenceFolder(path)  → error

import (
	"fmt"
	"time"
)

// ReferenceFolderInfo describes one successfully scanned reference folder.
// Returned to the frontend so it can list what has been added.
type ReferenceFolderInfo struct {
	// Path is the absolute filesystem path to the reference folder.
	Path string `json:"path"`

	// PhotoCount is how many geolocated photos were found inside it.
	PhotoCount int `json:"photoCount"`
}

// AddReferenceFolder scans folderPath for geolocated photos and adds them to
// the shared reference photo pool. If the same folder is added again, its
// previously loaded photos are replaced with fresh data (useful when the folder
// contents have changed on disk).
//
// If target photos have already been scanned, a date range filter is computed
// from their timestamps to skip reference files that clearly can't match —
// this avoids expensive EXIF reads on large libraries spanning many years.
func (a *App) AddReferenceFolder(path string, recursive bool) (ReferenceFolderInfo, error) {
	a.scanStatus = ScanStatus{
		InProgress: true,
		Phase:      "scanning_references",
		Message:    "Scanning reference folder...",
	}

	// Compute a date window from target photos to skip clearly irrelevant files.
	// Returns a zero DateRange (scan everything) if no targets are loaded yet.
	dateFilter := a.computeTargetDateRange()

	photos, err := ScanForReferencePhotos(path, dateFilter, recursive)
	if err != nil {
		a.scanStatus = ScanStatus{Phase: "idle", Message: err.Error()}
		return ReferenceFolderInfo{}, fmt.Errorf("scanning reference folder: %w", err)
	}

	// Replace any photos previously loaded from this folder, then append fresh ones.
	a.referencePhotos = removePhotosFromFolder(a.referencePhotos, path)
	a.referencePhotos = append(a.referencePhotos, photos...)

	info := ReferenceFolderInfo{Path: path, PhotoCount: len(photos)}

	// Update the folder list: replace existing entry or append a new one.
	updated := false
	for i, existing := range a.referenceFolderList {
		if existing.Path == path {
			a.referenceFolderList[i] = info
			updated = true
			break
		}
	}
	if !updated {
		a.referenceFolderList = append(a.referenceFolderList, info)
	}

	a.scanStatus = ScanStatus{
		Phase:   "idle",
		Message: fmt.Sprintf("Found %d geolocated photos in reference folder", len(photos)),
	}

	return info, nil
}

// GetReferenceFolders returns the list of reference folders currently loaded.
// Returns an empty slice (never nil) when no folders have been added.
func (a *App) GetReferenceFolders() []ReferenceFolderInfo {
	if a.referenceFolderList == nil {
		return []ReferenceFolderInfo{}
	}
	return a.referenceFolderList
}

// RemoveReferenceFolder drops all photos from this folder and removes its
// entry from the folder list. The reverse of AddReferenceFolder.
func (a *App) RemoveReferenceFolder(path string) error {
	a.referencePhotos = removePhotosFromFolder(a.referencePhotos, path)

	kept := a.referenceFolderList[:0]
	for _, info := range a.referenceFolderList {
		if info.Path != path {
			kept = append(kept, info)
		}
	}
	a.referenceFolderList = kept
	return nil
}

// removePhotosFromFolder returns a copy of photos with all entries from
// sourceFolder removed. Reuses the backing array to avoid allocation.
func removePhotosFromFolder(photos []ReferencePhoto, sourceFolder string) []ReferencePhoto {
	out := photos[:0]
	for _, p := range photos {
		if p.SourceFolder != sourceFolder {
			out = append(out, p)
		}
	}
	return out
}

// computeTargetDateRange returns a DateRange covering all target photo timestamps,
// expanded by 48 hours on each side to account for timezone drift and file-copy
// scenarios where filesystem mod times deviate from actual capture time.
//
// Returns a zero DateRange (meaning "scan everything") when:
//   - no target photos are loaded, or
//   - none of the loaded targets have a parseable DateTimeOriginal.
func (a *App) computeTargetDateRange() DateRange {
	if len(a.targetPhotos) == 0 {
		return DateRange{}
	}

	var minT, maxT time.Time
	for _, p := range a.targetPhotos {
		if p.DateTimeOriginal.IsZero() {
			continue
		}
		if minT.IsZero() || p.DateTimeOriginal.Before(minT) {
			minT = p.DateTimeOriginal
		}
		if maxT.IsZero() || p.DateTimeOriginal.After(maxT) {
			maxT = p.DateTimeOriginal
		}
	}

	// If no targets had timestamps, fall back to scanning everything.
	if minT.IsZero() {
		return DateRange{}
	}

	// 48-hour margin on each side: generous enough to handle timezone differences
	// between devices and mod-time drift from file copies.
	margin := 48 * time.Hour
	return DateRange{Start: minT.Add(-margin), End: maxT.Add(margin)}
}
