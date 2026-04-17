package main

// dng_scan_reader_datetime.go — ASCII + DateTime helpers used by
// dng_scan_reader.go. Split out so each file stays under the 150-line
// ceiling defined in CLAUDE.md §10.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// readASCIITag reads an ASCII TIFF tag value. If count <= 4 the bytes are
// packed inline into the 4-byte value field; otherwise `value` is an
// absolute file offset to `count` bytes of ASCII data. Callers are
// responsible for restoring the IFD cursor after this function seeks.
func readASCIITag(f *os.File, typeID uint16, count, value uint32, byteOrder binary.ByteOrder) (string, error) {
	if typeID != 2 { // EXIF ASCII
		return "", errors.New("not ASCII")
	}
	if count == 0 {
		return "", nil
	}
	if count <= 4 {
		// Inline: reconstruct the 4 value-field bytes in the declared byte order.
		buf := make([]byte, 4)
		byteOrder.PutUint32(buf, value)
		return trimASCII(buf[:count]), nil
	}
	if _, err := f.Seek(int64(value), io.SeekStart); err != nil {
		return "", fmt.Errorf("seek ASCII value: %w", err)
	}
	buf := make([]byte, count)
	if _, err := io.ReadFull(f, buf); err != nil {
		return "", fmt.Errorf("read ASCII value: %w", err)
	}
	return trimASCII(buf), nil
}

// trimASCII drops a trailing NUL terminator if present (EXIF ASCII includes it).
func trimASCII(b []byte) string {
	if n := len(b); n > 0 && b[n-1] == 0 {
		return string(b[:n-1])
	}
	return string(b)
}

// parseEXIFDateTimeToUTC parses an EXIF "YYYY:MM:DD HH:MM:SS" timestamp and
// returns a UTC-anchored time.Time.
//
// If offsetStr is non-empty and valid (e.g. "-05:00", "+01:00"), the offset
// is applied directly. Otherwise the timestamp is interpreted in the
// user-configured default timezone (see GetDefaultTimezone), which defaults
// to the OS local timezone.
//
// This is the single canonical EXIF DateTime parser for the scan path.
// Every scanned photo flows through here so matcher comparisons operate on
// correctly-anchored UTC values.
func parseEXIFDateTimeToUTC(dateTimeStr, offsetStr string) (time.Time, error) {
	const layout = "2006:01:02 15:04:05"

	if offsetStr != "" {
		// Offset format is "+HH:MM" or "-HH:MM". time.Parse with a layout
		// that includes "-07:00" handles both signs.
		combined := dateTimeStr + " " + offsetStr
		if t, err := time.Parse(layout+" -07:00", combined); err == nil {
			return t.UTC(), nil
		}
		// Fall through to default-tz path if offset string is malformed.
	}

	loc := GetDefaultTimezone()
	t, err := time.ParseInLocation(layout, dateTimeStr, loc)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// scanExifIFDForOffset seeks to the given Exif sub-IFD offset and walks its
// entries looking only for OffsetTimeOriginal (0x9011). Returns "" if the
// tag is absent or any structural error occurs — callers treat "" as
// "no offset present" and fall back to the default timezone.
//
// Keeps the walk minimal (no GPS / DateTime collection) because this is a
// second pass invoked only when IFD0 did not already contain the offset.
func scanExifIFDForOffset(f *os.File, byteOrder binary.ByteOrder, ifdOffset int64) string {
	if _, err := f.Seek(ifdOffset, io.SeekStart); err != nil {
		return ""
	}
	var entryCount uint16
	if err := binary.Read(f, byteOrder, &entryCount); err != nil {
		return ""
	}
	const tagOffsetTimeOriginal = 0x9011
	for i := uint16(0); i < entryCount; i++ {
		var tag, typeID uint16
		var count, value uint32
		if err := binary.Read(f, byteOrder, &tag); err != nil {
			return ""
		}
		if err := binary.Read(f, byteOrder, &typeID); err != nil {
			return ""
		}
		if err := binary.Read(f, byteOrder, &count); err != nil {
			return ""
		}
		if err := binary.Read(f, byteOrder, &value); err != nil {
			return ""
		}
		if tag != tagOffsetTimeOriginal {
			continue
		}
		current, _ := f.Seek(0, io.SeekCurrent)
		s, readErr := readASCIITag(f, typeID, count, value, byteOrder)
		if _, err := f.Seek(current, io.SeekStart); err != nil {
			return ""
		}
		if readErr != nil {
			return ""
		}
		return s
	}
	return ""
}
