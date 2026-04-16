package main

// exif_writer.go — GPS coordinate injection into photo EXIF data.
// See CLAUDE.md §12 for safety rules: always back up, always verify, keep .bak files.

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// WriteGPS injects GPS coordinates into a photo's EXIF data.
//
// DNG backup strategy: a lazy sidecar (dng_backup.go) stores only the 4-byte
// pointer value and file size — ~300 bytes instead of 44 MB. Undo truncates
// the file and restores the pointer. A pre-apply hash guards against silent
// corruption if another tool edited the file between apply and undo.
//
// JPEG backup strategy: unchanged — full file copy to <path>.bak.
func WriteGPS(targetPath string, lat, lon float64) error {
	fi, err := os.Stat(targetPath)
	if err != nil {
		return fmt.Errorf("stat %q: %w", targetPath, err)
	}
	if fi.Mode().Perm()&0200 == 0 {
		return fmt.Errorf("file %q is read-only — cannot write GPS data", targetPath)
	}

	ext := strings.ToLower(filepath.Ext(targetPath))
	switch ext {
	case ".dng":
		// Capture pre-apply state to the sidecar BEFORE any file modification.
		if err := captureDNGBackup(targetPath); err != nil {
			return fmt.Errorf("DNG backup capture failed: %w", err)
		}
		return writeAndVerify(targetPath, sidecarPath(targetPath), lat, lon, "DNG", writeGPSToDNG)

	case ".jpg", ".jpeg":
		backupPath := targetPath + ".bak"
		if err := copyFile(targetPath, backupPath); err != nil {
			return fmt.Errorf("backup failed: %w", err)
		}
		return writeAndVerify(targetPath, backupPath, lat, lon, "JPEG", writeGPSToJPEG)

	default:
		return fmt.Errorf("unsupported format for GPS write: %s", ext)
	}
}

// restoreFromBackup is used by writeAndVerify's failure path. It handles
// both the legacy JPEG .bak (file copy) and the new DNG .bak.json (sidecar).
func restoreFromBackup(targetPath, backupPath string) error {
	// DNG sidecar case
	if strings.HasSuffix(backupPath, ".bak.json") {
		sc, err := loadDNGBackup(targetPath)
		if err != nil {
			return fmt.Errorf("load sidecar for restore: %w", err)
		}
		return undoDNGFromSidecar(targetPath, sc)
	}
	// JPEG full-copy case
	return copyFile(backupPath, targetPath)
}

// writeAndVerify runs a format-specific GPS writer, then verifies the result.
// For DNG it uses the direct-binary fast path in verifyGPSInDNG (~130 bytes
// of I/O, file-size-independent). For JPEG it keeps the goexif round-trip
// which has always worked. On any failure the backup is restored.
func writeAndVerify(
	targetPath, backupPath string,
	lat, lon float64,
	label string,
	writer func(string, float64, float64) error,
) error {
	if err := writer(targetPath, lat, lon); err != nil {
		_ = restoreFromBackup(targetPath, backupPath)
		return fmt.Errorf("%s GPS write failed: %w", label, err)
	}

	var verifyErr error
	if label == "DNG" {
		verifyErr = verifyGPSInDNG(targetPath, lat, lon)
	} else {
		result, err := ReadEXIF(targetPath)
		if err != nil || !result.HasGPS ||
			math.Abs(result.Latitude-lat) > 0.001 ||
			math.Abs(result.Longitude-lon) > 0.001 {
			verifyErr = fmt.Errorf("EXIF verification failed")
		}
	}
	if verifyErr != nil {
		_ = restoreFromBackup(targetPath, backupPath)
		return fmt.Errorf("%s GPS verification failed for %s: %w",
			label, filepath.Base(targetPath), verifyErr)
	}
	return nil
}

// UndoGPS restores targetPath from its backup (DNG sidecar or JPEG .bak).
// For DNG, a pre-apply hash fingerprint in the sidecar is checked BEFORE
// the undo runs — if the hash doesn't match, undo refuses and returns a
// clear error. This catches the case where another tool (Lightroom, Bridge)
// edited the DNG between apply and undo.
func UndoGPS(targetPath string) error {
	ext := strings.ToLower(filepath.Ext(targetPath))
	if ext == ".dng" {
		sc, err := loadDNGBackup(targetPath)
		if err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("no backup sidecar found for %s", filepath.Base(targetPath))
			}
			return err
		}
		if err := checkDNGTamper(targetPath, sc); err != nil {
			return err
		}
		return undoDNGFromSidecar(targetPath, sc)
	}

	// JPEG path (unchanged)
	backupPath := targetPath + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("no backup found for %s", filepath.Base(targetPath))
	}
	return copyFile(backupPath, targetPath)
}

// ClearBackups recursively deletes .bak files AND .bak.json sidecars under
// folderPath. Returns the total count of deleted files.
func ClearBackups(folderPath string) (int, error) {
	count := 0
	walkErr := filepath.WalkDir(folderPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if (strings.HasSuffix(path, ".bak") || strings.HasSuffix(path, ".bak.json")) &&
			os.Remove(path) == nil {
			count++
		}
		return nil
	})
	return count, walkErr
}
