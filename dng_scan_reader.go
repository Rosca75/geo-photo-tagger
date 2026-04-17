package main

// dng_scan_reader.go — Direct-binary GPS + DateTime probe for DNG files during
// scanning. Bypasses goexif entirely on the scan path.
//
// Problem this solves: ReadEXIFForScan wraps the file in a 512 KB LimitReader
// for performance. Pentax K-1 (and likely other) DNGs put their GPS IFD near
// end-of-file — on IMGP7911.DNG the GPS IFD lives at offset 0x2C4D84C (~46 MB
// in). When goexif follows the pointer past the LimitReader window, it gets
// EOF and reports "sub-IFD GPSInfoIFDPointer decode failed: tiff: failed to
// read IFD tag count: EOF". The photo gets misclassified as "no GPS" and
// enters the target pool incorrectly — a correctness bug that would cause
// the next Apply GPS run to overwrite legitimate camera coordinates.
//
// This file reuses the TIFF-walking helpers in dng_gps_writer.go
// (readTIFFHeader) and the coord parser in dng_gps_verify.go
// (readGPSCoordsFromIFD) so there is exactly one canonical DNG TIFF parser
// in the project. The DateTime / ASCII helpers live in
// dng_scan_reader_datetime.go to keep each file under the 150-line ceiling.

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

	// Walk IFD0 once, collecting both the GPSInfoIFDPointer value and
	// the DateTime value/offset. Single pass, bounded I/O.
	gpsIFDOffset, dateTimeStr, err := scanIFD0ForGPSAndDateTime(f, byteOrder, int64(ifd0Offset))
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
		if t, tErr := parseEXIFDateTime(dateTimeStr); tErr == nil {
			result.HasDateTime = true
			result.DateTimeOriginal = t
		}
	}
	return result, nil
}

// scanIFD0ForGPSAndDateTime walks IFD0 once and returns:
//   - gpsIFDOffset: the value of the GPSInfoIFDPointer tag (0x8825), or 0 if absent
//   - dateTimeStr:  the ASCII value of the DateTime tag (0x0132), or "" if absent
//
// Does not allocate for tags it does not care about. Returns an error only
// on structural problems (truncated count, unreadable entry).
func scanIFD0ForGPSAndDateTime(f *os.File, byteOrder binary.ByteOrder, ifd0Offset int64) (gpsIFDOffset uint32, dateTimeStr string, err error) {
	if _, err = f.Seek(ifd0Offset, io.SeekStart); err != nil {
		return 0, "", fmt.Errorf("seek IFD0: %w", err)
	}
	var entryCount uint16
	if err = binary.Read(f, byteOrder, &entryCount); err != nil {
		return 0, "", fmt.Errorf("read IFD0 entry count: %w", err)
	}

	const tagDateTime = 0x0132
	const tagGPSInfo = 0x8825

	for i := uint16(0); i < entryCount; i++ {
		var tag, typeID uint16
		var count, value uint32
		if err = binary.Read(f, byteOrder, &tag); err != nil {
			return 0, "", fmt.Errorf("entry %d tag: %w", i, err)
		}
		if err = binary.Read(f, byteOrder, &typeID); err != nil {
			return 0, "", fmt.Errorf("entry %d type: %w", i, err)
		}
		if err = binary.Read(f, byteOrder, &count); err != nil {
			return 0, "", fmt.Errorf("entry %d count: %w", i, err)
		}
		if err = binary.Read(f, byteOrder, &value); err != nil {
			return 0, "", fmt.Errorf("entry %d value: %w", i, err)
		}

		switch tag {
		case tagGPSInfo:
			// LONG inline — the value field IS the GPS IFD offset.
			gpsIFDOffset = value
		case tagDateTime:
			// ASCII, typically 20 bytes ("YYYY:MM:DD HH:MM:SS\0") which
			// exceeds 4, so value is an external offset. readASCIITag
			// handles both inline (count<=4) and external cases, so we
			// save and restore the IFD cursor around it.
			current, _ := f.Seek(0, io.SeekCurrent)
			if s, readErr := readASCIITag(f, typeID, count, value, byteOrder); readErr == nil {
				dateTimeStr = s
			}
			if _, err = f.Seek(current, io.SeekStart); err != nil {
				return 0, "", fmt.Errorf("restore after DateTime: %w", err)
			}
		}
	}
	return gpsIFDOffset, dateTimeStr, nil
}
