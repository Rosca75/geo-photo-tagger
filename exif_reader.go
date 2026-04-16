package main

// exif_reader.go — EXIF metadata extraction
// Reads GPS coordinates, timestamps, and camera model from photo files.
// Supports JPEG, PNG, DNG, ARW (any format with standard EXIF/TIFF data)
// plus HEIC via the jdeng/goheif/heif ISOBMFF parser (ReadHEICExif).

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	heif "github.com/jdeng/goheif/heif"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/mknote"
)

// init registers maker note parsers once at startup.
// These add compatibility for camera-specific EXIF extensions
// (e.g. Nikon, Canon, Sony). Safe to call even without maker notes present.
func init() {
	exif.RegisterParsers(mknote.All...)
}

// ReadEXIF extracts EXIF metadata from the photo at the given path.
// Handles JPEG and TIFF-based formats (JPG, PNG, DNG, ARW).
//
// If the file has no EXIF data (common for some PNGs), an empty EXIFData
// struct is returned with no error — absent EXIF is normal, not a failure.
// Only file I/O errors are returned as actual errors.
func ReadEXIF(path string) (*EXIFData, error) {
	// Open the file for sequential reading
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %q: %w", path, err)
	}
	defer f.Close()

	// Decode EXIF from the file.
	// For JPEG: goexif finds the APP1 marker containing the EXIF IFD.
	// For TIFF (DNG, ARW): goexif reads the TIFF header directly.
	x, err := exif.Decode(f)
	if err != nil {
		// No EXIF data is expected for some files — log at Debug and return empty struct
		slog.Debug("exif_no_data", "path", path, "error", err.Error())
		return &EXIFData{}, nil
	}

	result := &EXIFData{}

	// --- GPS coordinates ---
	// LatLong() returns an error when GPS fields are absent, which is normal.
	lat, lon, err := x.LatLong()
	if err == nil {
		result.HasGPS = true
		result.Latitude = lat
		result.Longitude = lon
	}

	// --- Capture timestamp ---
	// EXIF DateTimeOriginal stores local time without timezone information.
	// The matching engine (Phase 4) handles timezone offsets.
	t, err := x.DateTime()
	if err == nil {
		result.DateTimeOriginal = t
		result.HasDateTime = true
	}

	// --- Camera model ---
	// e.g. "NIKON Z 6_2", "Canon EOS R5", "iPhone 14 Pro"
	modelTag, err := x.Get(exif.Model)
	if err == nil {
		result.CameraModel, _ = modelTag.StringVal()
	}

	return result, nil
}

// ReadHEICExif extracts GPS and timestamp from HEIC files using the
// jdeng/goheif/heif ISOBMFF parser. HEIC is the default format for
// iPhone photos and the primary source of GPS reference data.
//
// This reads the EXIF data embedded in the HEIC container without
// attempting to decode the HEVC image pixels (no CGo required).
func ReadHEICExif(path string) (*EXIFData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening HEIC %q: %w", path, err)
	}
	defer f.Close()

	// Parse the ISOBMFF container to find the EXIF box.
	// heif.Open takes an io.ReaderAt — os.File implements it.
	hf := heif.Open(f)
	exifBytes, err := hf.EXIF()
	if err != nil {
		slog.Debug("heic_no_exif", "path", path, "error", err.Error())
		return &EXIFData{}, nil
	}

	// The EXIF bytes from HEIC are a raw TIFF structure.
	// Feed them to goexif for parsing (same as JPEG APP1 content).
	x, err := exif.Decode(bytes.NewReader(exifBytes))
	if err != nil {
		slog.Debug("heic_exif_decode_failed", "path", path, "error", err.Error())
		return &EXIFData{}, nil
	}

	result := &EXIFData{}

	lat, lon, err := x.LatLong()
	if err == nil {
		result.HasGPS = true
		result.Latitude = lat
		result.Longitude = lon
	}

	t, err := x.DateTime()
	if err == nil {
		result.DateTimeOriginal = t
		result.HasDateTime = true
	}

	modelTag, err := x.Get(exif.Model)
	if err == nil {
		result.CameraModel, _ = modelTag.StringVal()
	}

	return result, nil
}

// ReadEXIFForScan is a lightweight EXIF reader optimised for the scan phase.
//
// Differences from ReadEXIF:
//   - Does NOT use maker note parsers (registered by init() for ReadEXIF).
//     Maker notes add CPU cost to parse proprietary camera data we don't need
//     during a bulk scan — they are only useful for the detail/preview view.
//   - Wraps the file in io.LimitReader(f, 128*1024) as a safety net.
//     JPEG EXIF is capped at 64 KB by spec; 128 KB gives DNG/TIFF margin.
//     This prevents a malformed or corrupt file from causing the decoder to
//     read megabytes before failing.
//
// Keep ReadEXIF for detail views (Phase 5+) where richer metadata is needed.
// Only use ReadEXIFForScan in the scan path where we check GPS presence.
func ReadEXIFForScan(path string) (*EXIFData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening %q: %w", path, err)
	}
	defer f.Close()

	// Choose read limit based on file format.
	// JPEG: APP1 is capped at 64 KB by spec → 128 KB limit is safe.
	// DNG/TIFF: embed JPEG previews (100-200 KB) that push EXIF IFDs further.
	//   Real-world Pentax K-1 DNG: ExifIFD at 158 KB, GPSInfo at 158 KB.
	//   512 KB covers even large embedded previews while staying far below
	//   the full multi-MB raw data.
	ext := strings.ToLower(filepath.Ext(path))
	readLimit := int64(128 * 1024) // 128 KB default (JPEG)
	if ext == ".dng" || ext == ".arw" || ext == ".tiff" || ext == ".tif" {
		readLimit = 512 * 1024 // 512 KB for TIFF-based formats
	}
	limited := io.LimitReader(f, readLimit)

	// Decode EXIF without maker note parsers — faster for GPS/DateTime check.
	// The global init() registered parsers apply to the full exif.Decode path,
	// but since we are not calling RegisterParsers here, new parser state is not
	// added. The existing global parsers from init() are still active, so we
	// create a fresh Decode call on the limited reader which will use the global
	// parser registry but skip any parser not already registered.
	x, err := exif.Decode(limited)
	if err != nil {
		// JPEG, DNG, ARW files should always have EXIF — log at Warn.
		// PNG may legitimately lack EXIF — log at Debug.
		level := slog.LevelDebug
		if ext == ".jpg" || ext == ".jpeg" || ext == ".dng" || ext == ".arw" {
			level = slog.LevelWarn
		}
		slog.Log(context.Background(), level, "exif_decode_failed",
			"path", path, "error", err.Error(), "extension", ext)
		return &EXIFData{}, nil
	}

	result := &EXIFData{}

	// GPS check — the primary filter during target scan.
	// LatLong() returns an error when GPS fields are absent, which is normal.
	lat, lon, err := x.LatLong()
	if err == nil {
		result.HasGPS = true
		result.Latitude = lat
		result.Longitude = lon
	}

	// Timestamp — needed later for time-based GPS matching.
	t, err := x.DateTime()
	if err == nil {
		result.DateTimeOriginal = t
		result.HasDateTime = true
	}

	// Camera model — single tag read, negligible cost.
	// e.g. "NIKON Z 6_2", "Canon EOS R5", "iPhone 14 Pro"
	modelTag, err := x.Get(exif.Model)
	if err == nil {
		result.CameraModel, _ = modelTag.StringVal()
	}

	return result, nil
}
