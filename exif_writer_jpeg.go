package main

// exif_writer_jpeg.go — JPEG-specific GPS write helpers.
// Split from exif_writer.go to keep each file under the 150-line limit
// (CLAUDE.md rule #3).

import (
	"bytes"
	"fmt"
	"os"

	exif "github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
	jpegstructure "github.com/dsoprea/go-jpeg-image-structure/v2"
)

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
