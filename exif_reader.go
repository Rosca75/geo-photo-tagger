package main

// exif_reader.go — EXIF metadata extraction
// Reads GPS coordinates, timestamps, and camera model from photo files.
// Supports JPEG, PNG, DNG, ARW (any format with standard EXIF/TIFF data).
// HEIC support (ReadHEICExif) will be added in Phase 2.

import (
	"fmt"
	"log"
	"os"

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
		// No EXIF data is expected for some files — log and return empty struct
		log.Printf("No EXIF in %q: %v", path, err)
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
