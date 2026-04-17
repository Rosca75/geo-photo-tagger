package main

// dng_scan_reader.go — Direct-binary GPS + DateTime probe for DNG files during
// scanning. Bypasses goexif entirely on the scan path.
//
// Problem this solves: ReadEXIFForScan wraps the file in a 512 KB LimitReader
// for performance, but Pentax K-1 (and similar) DNGs put their GPS IFD near
// end-of-file (IMGP7911.DNG: offset 0x2C4D84C, ~46 MB in). goexif hits EOF
// past the window and reports the photo as "no GPS" — a correctness bug
// that would cause Apply GPS to overwrite legitimate camera coordinates.
//
// Reuses readTIFFHeader (dng_gps_writer.go) and readGPSCoordsFromIFD
// (dng_gps_verify.go) so there is exactly one canonical DNG TIFF parser.
// DateTime / ASCII helpers live in dng_scan_reader_datetime.go.

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// readDNGScanFields returns the minimum EXIF fields the scanner needs:
// HasGPS + Latitude + Longitude + HasDateTime + DateTimeOriginal. Errors
// indicate structural problems with the file (not a valid TIFF, truncated
// IFD0, etc.) — the caller should fall back to the same empty-struct
// behavior ReadEXIFForScan uses for JPEG goexif errors.
//
// A missing GPSInfoIFDPointer is NOT an error — it means "no GPS", which
// is expected for un-geotagged photos. Same for DateTime.
func readDNGScanFields(path string) (*EXIFData, error) {
	result := &EXIFData{}

	f, err := os.Open(path)
	if err != nil {
		return result, fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	byteOrder, ifd0Offset, err := readTIFFHeader(f)
	if err != nil {
		return result, fmt.Errorf("tiff header: %w", err)
	}

	// Walk IFD0 once, collecting the GPSInfoIFDPointer, the DateTime, and
	// the OffsetTimeOriginal (consulting the Exif sub-IFD if IFD0 did not
	// carry 0x9011 directly). Single pass, bounded I/O.
	gpsIFDOffset, dateTimeStr, offsetStr, err := scanIFD0ForGPSAndDateTime(f, byteOrder, int64(ifd0Offset))
	if err != nil {
		return result, fmt.Errorf("walking IFD0: %w", err)
	}

	if gpsIFDOffset != 0 {
		lat, lon, gpsErr := readGPSCoordsFromIFD(f, byteOrder, int64(gpsIFDOffset))
		if gpsErr == nil {
			result.HasGPS = true
			result.Latitude = lat
			result.Longitude = lon
		}
		// If the GPS IFD exists but is malformed, we treat it as "no GPS"
		// silently — same behavior as the JPEG path when goexif returns a
		// non-critical error. The scan log will still show total counts.
	}

	if dateTimeStr != "" {
		if t, tErr := parseEXIFDateTimeToUTC(dateTimeStr, offsetStr); tErr == nil {
			result.HasDateTime = true
			result.DateTimeOriginal = t
		}
	}
	return result, nil
}

// scanIFD0ForGPSAndDateTime walks IFD0 once and returns:
//   - gpsIFDOffset: the value of the GPSInfoIFDPointer tag (0x8825), or 0 if absent
//   - dateTimeStr:  the ASCII value of the DateTime tag (0x0132), or "" if absent
//   - offsetStr:    the ASCII value of the OffsetTimeOriginal tag (0x9011), or "" if absent
//
// OffsetTimeOriginal conventionally lives in the Exif sub-IFD, not IFD0. If
// we capture an ExifIFDPointer (0x8769) during the walk and still haven't
// seen 0x9011 by the end, we do a focused second pass into the sub-IFD via
// scanExifIFDForOffset.
//
// Does not allocate for tags it does not care about. Returns an error only
// on structural problems (truncated count, unreadable entry).
func scanIFD0ForGPSAndDateTime(f *os.File, byteOrder binary.ByteOrder, ifd0Offset int64) (gpsIFDOffset uint32, dateTimeStr, offsetStr string, err error) {
	if _, err = f.Seek(ifd0Offset, io.SeekStart); err != nil {
		return 0, "", "", fmt.Errorf("seek IFD0: %w", err)
	}
	var entryCount uint16
	if err = binary.Read(f, byteOrder, &entryCount); err != nil {
		return 0, "", "", fmt.Errorf("read IFD0 entry count: %w", err)
	}

	const tagDateTime = 0x0132
	const tagGPSInfo = 0x8825
	const tagExifIFDPointer = 0x8769
	const tagOffsetTimeOriginal = 0x9011

	var exifIFDOffset uint32

	for i := uint16(0); i < entryCount; i++ {
		var tag, typeID uint16
		var count, value uint32
		if err = binary.Read(f, byteOrder, &tag); err != nil {
			return 0, "", "", fmt.Errorf("entry %d tag: %w", i, err)
		}
		if err = binary.Read(f, byteOrder, &typeID); err != nil {
			return 0, "", "", fmt.Errorf("entry %d type: %w", i, err)
		}
		if err = binary.Read(f, byteOrder, &count); err != nil {
			return 0, "", "", fmt.Errorf("entry %d count: %w", i, err)
		}
		if err = binary.Read(f, byteOrder, &value); err != nil {
			return 0, "", "", fmt.Errorf("entry %d value: %w", i, err)
		}

		switch tag {
		case tagGPSInfo:
			// LONG inline — the value field IS the GPS IFD offset.
			gpsIFDOffset = value
		case tagExifIFDPointer:
			// LONG inline — remember the pointer so we can walk the Exif
			// sub-IFD for OffsetTimeOriginal if IFD0 did not carry it.
			exifIFDOffset = value
		case tagDateTime, tagOffsetTimeOriginal:
			// ASCII, typically 20 bytes ("YYYY:MM:DD HH:MM:SS\0") which
			// exceeds 4, so value is an external offset. readASCIITag
			// handles both inline (count<=4) and external cases, so we
			// save and restore the IFD cursor around it.
			current, _ := f.Seek(0, io.SeekCurrent)
			if s, readErr := readASCIITag(f, typeID, count, value, byteOrder); readErr == nil {
				if tag == tagDateTime {
					dateTimeStr = s
				} else {
					offsetStr = s
				}
			}
			if _, err = f.Seek(current, io.SeekStart); err != nil {
				return 0, "", "", fmt.Errorf("restore after ASCII tag: %w", err)
			}
		}
	}

	// Fallback: OffsetTimeOriginal normally lives in the Exif sub-IFD.
	if offsetStr == "" && exifIFDOffset != 0 {
		offsetStr = scanExifIFDForOffset(f, byteOrder, int64(exifIFDOffset))
	}
	return gpsIFDOffset, dateTimeStr, offsetStr, nil
}
