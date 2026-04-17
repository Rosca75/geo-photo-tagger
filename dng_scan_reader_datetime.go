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

// parseEXIFDateTime parses EXIF's "YYYY:MM:DD HH:MM:SS" format.
func parseEXIFDateTime(s string) (time.Time, error) {
	return time.Parse("2006:01:02 15:04:05", s)
}
