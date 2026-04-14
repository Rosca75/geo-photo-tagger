package main

// exif_writer.go — GPS coordinate injection into photo EXIF data.
// See CLAUDE.md §12 for safety rules: always back up, always verify, keep .bak files.

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	jpegstructure "github.com/dsoprea/go-jpeg-image-structure/v2"
)

// WriteGPS injects GPS coordinates into a photo's EXIF data.
// Backs up the original to <file>.bak, writes GPS, then verifies by re-reading.
// On any failure the backup is restored before the error is returned.
func WriteGPS(targetPath string, lat, lon float64) error {
	// Check if file is writable before creating backup.
	// Creating a .bak of a read-only file is wasted work if we can't write back.
	fi, err := os.Stat(targetPath)
	if err != nil {
		return fmt.Errorf("stat %q: %w", targetPath, err)
	}
	if fi.Mode().Perm()&0200 == 0 {
		return fmt.Errorf("file %q is read-only — cannot write GPS data", targetPath)
	}
	backupPath := targetPath + ".bak"
	if err := copyFile(targetPath, backupPath); err != nil {
		return fmt.Errorf("backup failed: %w", err)
	}
	ext := strings.ToLower(filepath.Ext(targetPath))
	switch ext {
	case ".dng":
		return writeAndVerify(targetPath, backupPath, lat, lon, "DNG", writeGPSToDNG)
	case ".jpg", ".jpeg":
		return writeAndVerify(targetPath, backupPath, lat, lon, "JPEG", writeGPSToJPEG)
	default:
		return fmt.Errorf("unsupported format for GPS write: %s", ext)
	}
}

// writeAndVerify runs a format-specific GPS writer, then re-reads EXIF to confirm
// the coordinates landed within 0.001° (~110 m) — a tolerance that survives DMS
// rounding but catches wrong writes. On any failure the backup is restored.
func writeAndVerify(
	targetPath, backupPath string,
	lat, lon float64,
	label string,
	writer func(string, float64, float64) error,
) error {
	if err := writer(targetPath, lat, lon); err != nil {
		_ = copyFile(backupPath, targetPath)
		return fmt.Errorf("%s GPS write failed: %w", label, err)
	}
	result, err := ReadEXIF(targetPath)
	if err != nil || !result.HasGPS ||
		math.Abs(result.Latitude-lat) > 0.001 ||
		math.Abs(result.Longitude-lon) > 0.001 {
		_ = copyFile(backupPath, targetPath)
		return fmt.Errorf("%s GPS verification failed for %s", label, filepath.Base(targetPath))
	}
	return nil
}

// writeGPSToJPEG sets GPS EXIF tags in a JPEG using the dsoprea pipeline:
// parse JPEG → build IFD tree → update GPS sub-IFD → encode → write to disk.
func writeGPSToJPEG(path string, lat, lon float64) error {
	jmp := jpegstructure.NewJpegMediaParser()
	intfc, err := jmp.ParseFile(path)
	if err != nil {
		return fmt.Errorf("parse JPEG: %w", err)
	}
	sl := intfc.(*jpegstructure.SegmentList) // ParseFile returns MediaContext; cast needed

	rootIb, err := sl.ConstructExifBuilder() // builds IFD tree from the APP1 segment
	if err != nil {
		return fmt.Errorf("construct EXIF builder: %w", err)
	}
	// IFD/GPSInfo is the GPS sub-IFD; GetOrCreateIbFromRootIb creates it if absent.
	gpsIb, err := exif.GetOrCreateIbFromRootIb(rootIb, "IFD/GPSInfo")
	if err != nil {
		return fmt.Errorf("get GPS IFD: %w", err)
	}
	latRats, latRef := decimalToDMS(lat, true)
	lonRats, lonRef := decimalToDMS(lon, false)
	// The 4 mandatory GPS tags: hemisphere refs (ASCII) + DMS coordinates (RATIONAL).
	if err = gpsIb.SetStandardWithName("GPSLatitudeRef", latRef); err != nil {
		return fmt.Errorf("GPSLatitudeRef: %w", err)
	}
	if err = gpsIb.SetStandardWithName("GPSLatitude", latRats); err != nil {
		return fmt.Errorf("GPSLatitude: %w", err)
	}
	if err = gpsIb.SetStandardWithName("GPSLongitudeRef", lonRef); err != nil {
		return fmt.Errorf("GPSLongitudeRef: %w", err)
	}
	if err = gpsIb.SetStandardWithName("GPSLongitude", lonRats); err != nil {
		return fmt.Errorf("GPSLongitude: %w", err)
	}
	if err = sl.SetExif(rootIb); err != nil {
		return fmt.Errorf("set EXIF segments: %w", err)
	}
	var buf bytes.Buffer
	if err = sl.Write(&buf); err != nil {
		return fmt.Errorf("encode JPEG: %w", err)
	}
	return os.WriteFile(path, buf.Bytes(), 0644)
}

// decimalToDMS converts a decimal-degree value to EXIF GPS DMS rational format.
// Returns []Rational{degrees, minutes, seconds} and a ref string (N/S or E/W).
// Seconds uses denominator 10000 to preserve sub-second precision.
func decimalToDMS(decimal float64, isLat bool) ([]exifcommon.Rational, string) {
	ref := "N"
	if isLat && decimal < 0 {
		ref, decimal = "S", -decimal
	} else if !isLat {
		if decimal >= 0 {
			ref = "E"
		} else {
			ref, decimal = "W", -decimal
		}
	}
	deg := int(decimal)
	mf := (decimal - float64(deg)) * 60
	min := int(mf)
	sec := (mf - float64(min)) * 60
	return []exifcommon.Rational{
		{Numerator: uint32(deg), Denominator: 1},
		{Numerator: uint32(min), Denominator: 1},
		{Numerator: uint32(int(sec * 10000)), Denominator: 10000},
	}, ref
}

// UndoGPS restores targetPath from its .bak backup.
// The .bak is preserved so the user can undo again. Returns error if no .bak exists.
func UndoGPS(targetPath string) error {
	backupPath := targetPath + ".bak"
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return fmt.Errorf("no backup found for %s", filepath.Base(targetPath))
	}
	return copyFile(backupPath, targetPath)
}

// ClearBackups recursively deletes all .bak files under folderPath.
// Returns the count of deleted files; unreadable entries are silently skipped.
func ClearBackups(folderPath string) (int, error) {
	count := 0
	walkErr := filepath.WalkDir(folderPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".bak") && os.Remove(path) == nil {
			count++
		}
		return nil
	})
	return count, walkErr
}
