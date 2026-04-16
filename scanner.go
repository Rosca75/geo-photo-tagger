package main

// scanner.go — Filesystem scanning for target and reference photos
// Walks directories recursively and classifies photos by EXIF GPS presence.
// Target photos lack GPS; reference photos have GPS coordinates.
//
// Phase 1: ScanForTargetPhotos (JPG/JPEG/DNG without GPS)
// Phase 2/4: ScanForReferencePhotos (all formats with GPS)

import (
	"fmt"
	"io/fs"
	"log/slog"
	"path/filepath"
	"strings"
	"time"
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
//
// Uses filepath.WalkDir (Go 1.16+) instead of the legacy filepath.Walk.
// WalkDir avoids an extra os.Lstat syscall per entry because fs.DirEntry
// already carries the file type from the directory read. On NAS/SMB mounts,
// each saved stat is a saved network round-trip.
func ScanForTargetPhotos(folderPath string) ([]TargetPhoto, error) {
	var results []TargetPhoto

	walkStart := time.Now()

	// walkFn is invoked by filepath.WalkDir for every file and directory.
	// The fs.DirEntry (d) argument carries type info without a stat syscall.
	walkFn := func(path string, d fs.DirEntry, err error) error {
		// Skip entries we cannot access (permission error, broken symlink, etc.)
		if err != nil {
			slog.Warn("walk_inaccessible", "path", path, "error", err)
			return nil // returning nil continues the walk
		}

		// WalkDir recurses into subdirectories automatically
		if d.IsDir() {
			return nil
		}

		// Filter to supported target extensions (compare lowercase).
		// Doing this before d.Info() avoids a stat syscall for non-photo files.
		ext := strings.ToLower(filepath.Ext(path))
		if !isTargetExtension(ext) {
			return nil
		}

		// Read EXIF using the lightweight scan reader (no maker notes, LimitReader).
		exifData, err := ReadEXIFForScan(path)
		if err != nil {
			slog.Warn("exif_read_failed", "path", path, "error", err.Error(), "extension", ext)
			return nil
		}

		// Photos with GPS already don't need tagging — skip them
		if exifData.HasGPS {
			return nil
		}

		// Call d.Info() only for files that will appear in results.
		// This defers the stat syscall until we know we need the file size.
		info, err := d.Info()
		if err != nil {
			slog.Warn("stat_failed", "path", path, "error", err)
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
		} else {
			slog.Warn("exif_no_datetime", "path", path, "extension", ext)
		}

		results = append(results, photo)
		return nil
	}

	// Perform the recursive directory walk using the efficient WalkDir API
	if err := filepath.WalkDir(folderPath, walkFn); err != nil {
		return nil, fmt.Errorf("walking folder %q: %w", folderPath, err)
	}

	slog.Info("scan_target_complete",
		"folder", folderPath,
		"targets_found", len(results),
		"walk_duration_ms", time.Since(walkStart).Milliseconds(),
	)

	return results, nil
}

// isReferenceExtension returns true if ext (lowercase) is a supported
// reference photo format. Includes RAW formats that may carry GPS.
func isReferenceExtension(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".dng", ".arw", ".heic", ".heif":
		return true
	}
	return false
}

// ScanForReferencePhotos walks folderPath recursively and returns every photo
// that has GPS EXIF data. These become the coordinate sources for matching.
// Only photos with both GPS and a valid DateTimeOriginal are included.
//
// dateFilter optionally restricts which files are considered based on filesystem
// modification time. When non-zero, files whose mod time falls outside the range
// are skipped before EXIF is read — this is the key performance optimisation for
// large reference libraries that span years of photos.
//
// Uses filepath.WalkDir (Go 1.16+) — see ScanForTargetPhotos for rationale.
func ScanForReferencePhotos(folderPath string, dateFilter DateRange, recursive bool) ([]ReferencePhoto, error) {
	var results []ReferencePhoto
	var skippedByDate int

	walkStart := time.Now()

	// walkFn uses fs.DirEntry to avoid the extra stat syscall per entry.
	walkFn := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("walk_inaccessible", "path", path, "error", err)
			return nil
		}
		if d.IsDir() {
			// When non-recursive, skip every directory except the root itself.
			if !recursive && path != folderPath {
				return fs.SkipDir
			}
			return nil
		}

		// Filter by extension before reading EXIF (avoids stat for non-photos)
		ext := strings.ToLower(filepath.Ext(path))
		if !isReferenceExtension(ext) {
			return nil
		}

		// Quick filesystem date check — skip files clearly outside the target date range.
		// Uses the file's mod time as a rough proxy for when the photo was taken.
		// This avoids the expensive EXIF read for files that can't possibly match.
		// Note: file copies change mod time, so we use a generous margin (see
		// computeTargetDateRange). False negatives here are bad; false positives are OK
		// because EXIF-level matching in matcher.go will still discard them.
		if !dateFilter.IsZero() {
			info, err := d.Info()
			if err == nil {
				modTime := info.ModTime()
				if modTime.Before(dateFilter.Start) || modTime.After(dateFilter.End) {
					skippedByDate++
					return nil
				}
			}
		}

		// Read EXIF. HEIC files need the ISOBMFF parser; other formats use the
		// lightweight scan reader (no maker notes, LimitReader).
		var exifData *EXIFData
		if ext == ".heic" || ext == ".heif" {
			exifData, err = ReadHEICExif(path)
		} else {
			exifData, err = ReadEXIFForScan(path)
		}
		if err != nil {
			slog.Warn("exif_read_failed", "path", path, "error", err.Error(), "extension", ext)
			return nil
		}

		// Reference photos must have GPS — that is their entire purpose.
		if !exifData.HasGPS {
			return nil
		}
		// Also skip photos without a timestamp — we need it for matching.
		// Warn so users can see which reference files are being skipped.
		if !exifData.HasDateTime {
			slog.Warn("exif_no_datetime", "path", path, "extension", ext)
			return nil
		}

		results = append(results, ReferencePhoto{
			Path:             path,
			Filename:         filepath.Base(path),
			Extension:        ext,
			DateTimeOriginal: exifData.DateTimeOriginal,
			GPS:              GPSCoord{Latitude: exifData.Latitude, Longitude: exifData.Longitude},
			CameraModel:      exifData.CameraModel,
			SourceFolder:     folderPath,
			IsHEIC:           ext == ".heic" || ext == ".heif",
		})
		return nil
	}

	if err := filepath.WalkDir(folderPath, walkFn); err != nil {
		return nil, fmt.Errorf("walking folder %q: %w", folderPath, err)
	}

	slog.Info("scan_reference_complete",
		"folder", folderPath,
		"references_found", len(results),
		"skipped_by_date", skippedByDate,
		"walk_duration_ms", time.Since(walkStart).Milliseconds(),
	)

	return results, nil
}
