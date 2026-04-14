package main

// dng_gps_blob.go — pure functions that construct a standalone GPS IFD blob
// for appending into a DNG/TIFF file. No filesystem I/O here — this makes the
// blob builder trivial to unit-test and keeps dng_gps_writer.go focused on I/O.

import (
	"bytes"
	"encoding/binary"
)

// buildGPSIFDBlob produces a self-contained 114-byte GPS IFD with 5 entries:
// GPSVersionID, GPSLatitudeRef, GPSLatitude, GPSLongitudeRef, GPSLongitude.
//
// Layout (byte offsets relative to the start of the blob):
//
//	0..1    entry count = 5
//	2..13   entry 0 GPSVersionID      (BYTE  ×4, inline value 2,3,0,0)
//	14..25  entry 1 GPSLatitudeRef    (ASCII ×2, inline "N\0" or "S\0")
//	26..37  entry 2 GPSLatitude       (RATIONAL ×3, value = absolute file offset)
//	38..49  entry 3 GPSLongitudeRef   (ASCII ×2, inline "E\0" or "W\0")
//	50..61  entry 4 GPSLongitude      (RATIONAL ×3, value = absolute file offset)
//	62..65  next IFD pointer = 0
//	66..89  latitude  RATIONAL data (3 × 8 bytes)
//	90..113 longitude RATIONAL data (3 × 8 bytes)
//
// Entries are sorted by tag ID ascending as required by the TIFF spec.
// `appendOffset` is the absolute file offset where the blob will be written;
// it is needed because RATIONAL value-field offsets are absolute file offsets,
// not offsets relative to the IFD.
func buildGPSIFDBlob(lat, lon float64, appendOffset int64, byteOrder binary.ByteOrder) []byte {
	buf := new(bytes.Buffer)

	// Entry count: 5.
	_ = binary.Write(buf, byteOrder, uint16(5))

	// Small helper: write one 12-byte IFD entry (tag, type, count, value).
	writeEntry := func(tag, typeID uint16, count uint32, value uint32) {
		_ = binary.Write(buf, byteOrder, tag)
		_ = binary.Write(buf, byteOrder, typeID)
		_ = binary.Write(buf, byteOrder, count)
		_ = binary.Write(buf, byteOrder, value)
	}

	// IFD header occupies: 2 (count) + 5*12 (entries) + 4 (next-IFD) = 66 bytes.
	// Latitude rationals begin at blob offset 66, longitude at 90.
	latDataOffset := uint32(appendOffset) + 66
	lonDataOffset := uint32(appendOffset) + 90

	// Split latitude/longitude into hemisphere reference + absolute magnitude.
	latRef, absLat := hemisphereRef(lat, 'N', 'S')
	lonRef, absLon := hemisphereRef(lon, 'E', 'W')

	// Entry 0: GPSVersionID — BYTE ×4, inline bytes 02 03 00 00 (EXIF GPS 2.3.0.0).
	// We pack the inline value into a uint32 respecting byte order so the bytes
	// land on disk as 02 03 00 00 regardless of LE/BE.
	var versionValue uint32
	if byteOrder == binary.LittleEndian {
		versionValue = 0x00000302
	} else {
		versionValue = 0x02030000
	}
	writeEntry(0x0000, 1, 4, versionValue) // type 1 = BYTE

	// Entry 1: GPSLatitudeRef — ASCII ×2, inline "N\0" or "S\0".
	writeEntry(0x0001, 2, 2, asciiInlineValue(latRef, byteOrder))
	// Entry 2: GPSLatitude — RATIONAL ×3, value = absolute file offset.
	writeEntry(0x0002, 5, 3, latDataOffset)
	// Entry 3: GPSLongitudeRef — ASCII ×2, inline "E\0" or "W\0".
	writeEntry(0x0003, 2, 2, asciiInlineValue(lonRef, byteOrder))
	// Entry 4: GPSLongitude — RATIONAL ×3, value = absolute file offset.
	writeEntry(0x0004, 5, 3, lonDataOffset)

	// Next-IFD pointer: 0 (no chained IFD).
	_ = binary.Write(buf, byteOrder, uint32(0))

	// RATIONAL value data: 3 deg/min/sec pairs per coordinate.
	writeRationals(buf, byteOrder, absLat)
	writeRationals(buf, byteOrder, absLon)

	return buf.Bytes()
}

// hemisphereRef returns a 4-byte ASCII inline value ("N\0\0\0" / "S\0\0\0" or "E"/"W")
// and the absolute magnitude of the coordinate. `pos` is the positive hemisphere letter
// (N or E), `neg` is the negative one (S or W).
func hemisphereRef(decimal float64, pos, neg byte) ([4]byte, float64) {
	if decimal < 0 {
		return [4]byte{neg, 0, 0, 0}, -decimal
	}
	return [4]byte{pos, 0, 0, 0}, decimal
}

// asciiInlineValue packs a 4-byte ASCII inline value into a uint32 in the file's byte
// order so that when the uint32 is written with `byteOrder`, the resulting bytes on disk
// read exactly as b[0], b[1], b[2], b[3].
func asciiInlineValue(b [4]byte, byteOrder binary.ByteOrder) uint32 {
	if byteOrder == binary.LittleEndian {
		return uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
	}
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}

// writeRationals writes 3 RATIONAL values (degrees, minutes, seconds) for a decimal-degree
// value. Each RATIONAL is a numerator/denominator uint32 pair. Seconds use denominator 10000
// for sub-second precision, matching the JPEG path in exif_writer.go.
func writeRationals(buf *bytes.Buffer, byteOrder binary.ByteOrder, decimal float64) {
	deg := int(decimal)
	mf := (decimal - float64(deg)) * 60
	min := int(mf)
	sec := (mf - float64(min)) * 60

	// Degrees: integer / 1.
	_ = binary.Write(buf, byteOrder, uint32(deg))
	_ = binary.Write(buf, byteOrder, uint32(1))
	// Minutes: integer / 1.
	_ = binary.Write(buf, byteOrder, uint32(min))
	_ = binary.Write(buf, byteOrder, uint32(1))
	// Seconds: scaled by 10000 / 10000.
	_ = binary.Write(buf, byteOrder, uint32(int(sec*10000)))
	_ = binary.Write(buf, byteOrder, uint32(10000))
}
