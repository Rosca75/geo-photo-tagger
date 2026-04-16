package main

// dng_gps_verify.go — Direct binary verification of a newly-written DNG GPS IFD.
//
// After writeGPSToDNG patches the file, we need to confirm lat/lon round-trip
// correctly before deleting (or in phase 3c, not writing) the backup. The naive
// approach — re-decode the full EXIF via goexif — is slow on DNGs (20-80 ms for
// Pentax files with maker notes).
//
// Attempting to use ReadEXIFForScan is not viable either: its 512 KB LimitReader
// cannot reach our appended GPS IFD which lives at the end of a multi-MB DNG.
//
// This file implements the minimal-I/O alternative: follow the patched pointer,
// walk the 5 GPS IFD entries, reconstruct decimal degrees, compare with
// tolerance. No goexif involved.

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// verifyGPSInDNG re-reads ONLY the appended GPS IFD and confirms the lat/lon
// values match expectedLat/expectedLon within 0.001° tolerance. Bounded to
// ~130 bytes of I/O regardless of file size.
//
// Returns nil on match, an error describing what went wrong otherwise.
// Called from writeAndVerify in exif_writer.go as a DNG-specific fast path.
func verifyGPSInDNG(path string, expectedLat, expectedLon float64) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("verify open: %w", err)
	}
	defer f.Close()

	byteOrder, ifd0Offset, err := readTIFFHeader(f)
	if err != nil {
		return fmt.Errorf("verify header: %w", err)
	}
	gpsPointerOffset, err := findGPSInfoIFDPointer(f, byteOrder, ifd0Offset)
	if err != nil {
		return fmt.Errorf("verify locate pointer: %w", err)
	}

	// Read the current GPS IFD offset from the patched pointer.
	if _, err := f.Seek(gpsPointerOffset, io.SeekStart); err != nil {
		return fmt.Errorf("verify seek pointer: %w", err)
	}
	var gpsIFDOffset uint32
	if err := binary.Read(f, byteOrder, &gpsIFDOffset); err != nil {
		return fmt.Errorf("verify read pointer: %w", err)
	}

	lat, lon, err := readGPSCoordsFromIFD(f, byteOrder, int64(gpsIFDOffset))
	if err != nil {
		return fmt.Errorf("verify read coords: %w", err)
	}

	if math.Abs(lat-expectedLat) > 0.001 || math.Abs(lon-expectedLon) > 0.001 {
		return fmt.Errorf("verify mismatch: got (%.6f, %.6f), want (%.6f, %.6f)",
			lat, lon, expectedLat, expectedLon)
	}
	return nil
}

// readGPSCoordsFromIFD walks a GPS IFD at `gpsIFDOffset` and returns decimal
// lat/lon. Expects at minimum the 4 tags we wrote: 0x0001 LatRef, 0x0002 Lat,
// 0x0003 LonRef, 0x0004 Lon.
func readGPSCoordsFromIFD(f *os.File, byteOrder binary.ByteOrder, gpsIFDOffset int64) (lat, lon float64, err error) {
	if _, err = f.Seek(gpsIFDOffset, io.SeekStart); err != nil {
		return 0, 0, fmt.Errorf("seek GPS IFD: %w", err)
	}
	var entryCount uint16
	if err = binary.Read(f, byteOrder, &entryCount); err != nil {
		return 0, 0, fmt.Errorf("read entry count: %w", err)
	}

	var (
		latRefChar, lonRefChar                   byte
		latDeg, latMin, latSec                   float64
		lonDeg, lonMin, lonSec                   float64
		haveLat, haveLon, haveLatRef, haveLonRef bool
	)

	for i := uint16(0); i < entryCount; i++ {
		var tag, typeID uint16
		var count, value uint32
		if err = binary.Read(f, byteOrder, &tag); err != nil {
			return 0, 0, err
		}
		if err = binary.Read(f, byteOrder, &typeID); err != nil {
			return 0, 0, err
		}
		if err = binary.Read(f, byteOrder, &count); err != nil {
			return 0, 0, err
		}
		if err = binary.Read(f, byteOrder, &value); err != nil {
			return 0, 0, err
		}

		// Save position — RATIONAL reads seek elsewhere and come back.
		entryEndPos, _ := f.Seek(0, io.SeekCurrent)

		switch tag {
		case 0x0001: // GPSLatitudeRef: ASCII inline, first byte
			latRefChar = byte(value & 0xFF)
			haveLatRef = true
		case 0x0002: // GPSLatitude: 3×RATIONAL at offset `value`
			latDeg, latMin, latSec, err = readThreeRationals(f, byteOrder, int64(value))
			if err != nil {
				return 0, 0, fmt.Errorf("lat rationals: %w", err)
			}
			haveLat = true
		case 0x0003: // GPSLongitudeRef
			lonRefChar = byte(value & 0xFF)
			haveLonRef = true
		case 0x0004: // GPSLongitude
			lonDeg, lonMin, lonSec, err = readThreeRationals(f, byteOrder, int64(value))
			if err != nil {
				return 0, 0, fmt.Errorf("lon rationals: %w", err)
			}
			haveLon = true
		}

		// Keep typeID/count referenced — we do not use them here but reading
		// them advances the cursor, which is the intent.
		_ = typeID
		_ = count

		if _, err = f.Seek(entryEndPos, io.SeekStart); err != nil {
			return 0, 0, fmt.Errorf("seek back after entry %d: %w", i, err)
		}
	}

	if !(haveLat && haveLon && haveLatRef && haveLonRef) {
		return 0, 0, fmt.Errorf("missing tags: lat=%v lon=%v latRef=%v lonRef=%v",
			haveLat, haveLon, haveLatRef, haveLonRef)
	}

	lat = latDeg + latMin/60 + latSec/3600
	if latRefChar == 'S' {
		lat = -lat
	}
	lon = lonDeg + lonMin/60 + lonSec/3600
	if lonRefChar == 'W' {
		lon = -lon
	}
	return lat, lon, nil
}

// readThreeRationals reads 3 RATIONAL values (6 × uint32 = 24 bytes) starting
// at offset and returns the three floats. Seeks to offset; caller restores.
func readThreeRationals(f *os.File, byteOrder binary.ByteOrder, offset int64) (a, b, c float64, err error) {
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return
	}
	var r [6]uint32
	for i := range r {
		if err = binary.Read(f, byteOrder, &r[i]); err != nil {
			return
		}
	}
	ratio := func(num, den uint32) float64 {
		if den == 0 {
			return 0
		}
		return float64(num) / float64(den)
	}
	return ratio(r[0], r[1]), ratio(r[2], r[3]), ratio(r[4], r[5]), nil
}
