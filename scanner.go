package main

// scanner.go — Filesystem scanning for target and reference photos
// Walks directories recursively and classifies photos by EXIF GPS presence.
// Target photos lack GPS; reference photos have GPS coordinates.
//
// Phase 1: ScanForTargetPhotos (JPG/JPEG/DNG without GPS)
// Phase 2: ScanForReferencePhotos (all formats with GPS)

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// isTargetExtension returns true if ext (lowercase) is a supported
// target photo format. Targets are the files the user wants to geotag.
func isTargetExtension(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".dng":
		return true
	}
	return false
}

// ScanForTargetPhotos walks folderPath recursively and returns every photo
// that is missing GPS EXIF data. Only JPG, JPEG, and DNG files are considered.
//
// Files that already have GPS coordinates are silently skipped — they don't
// need tagging. Files with unreadable EXIF are also skipped with a log warning.
func ScanForTargetPhotos(folderPath string) ([]TargetPhoto, error) {
	var results []TargetPhoto

	// walkFn is invoked by filepath.Walk for every file and directory.
	walkFn := func(path string, info os.FileInfo, err error) error {
		// Skip entries we cannot access (permission error, broken symlink, etc.)
		if err != nil {
			log.Printf("Skipping inaccessible path %q: %v", path, err)
			return nil // returning nil continues the walk
		}

		// filepath.Walk recurses into subdirectories automatically
		if info.IsDir() {
			return nil
		}

		// Filter to supported target extensions (compare lowercase)
		ext := strings.ToLower(filepath.Ext(path))
		if !isTargetExtension(ext) {
			return nil
		}

		// Read EXIF to check for GPS and extract the timestamp
		exifData, err := ReadEXIF(path)
		if err != nil {
			log.Printf("EXIF error for %q: %v — skipping", path, err)
			return nil
		}

		// Photos with GPS already don't need tagging — skip them
		if exifData.HasGPS {
			return nil
		}

		// Build the TargetPhoto record for this file
		photo := TargetPhoto{
			Path:          path,
			Filename:      filepath.Base(path),
			Extension:     ext,
			FileSizeBytes: info.Size(),
			Status:        "unmatched",
			CameraModel:   exifData.CameraModel,
		}

		// Only set DateTimeOriginal if EXIF provided a parseable timestamp.
		// Photos without timestamps will still appear but can't be matched.
		if exifData.HasDateTime {
			photo.DateTimeOriginal = exifData.DateTimeOriginal
		}

		results = append(results, photo)
		return nil
	}

	// Perform the recursive directory walk
	if err := filepath.Walk(folderPath, walkFn); err != nil {
		return nil, fmt.Errorf("walking folder %q: %w", folderPath, err)
	}

	return results, nil
}
