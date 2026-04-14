package main

// dng_gps_writer.go — DNG GPS write via surgical binary patch.
//
// Strategy (see DNG-GPS-WRITE-PLAN.md for the full rationale):
//   1. Parse the TIFF header to find IFD0 and locate the GPSInfoIFD pointer
//      (tag 0x8825). Every camera DNG already has this entry in IFD0.
//   2. Build a self-contained 114-byte GPS IFD blob (entries + RATIONAL value data).
//   3. Append the blob at the end of the file (word-aligned).
//   4. Patch the 4-byte GPSInfoIFD pointer in IFD0 to reference the appended blob.
//
// The TIFF spec explicitly allows IFD offsets to point anywhere in the file — including
// after the raw image data — so this approach never disturbs SubIFDs, MakerNotes, color
// matrices, or the 44 MB of sensor data. Only two regions of the file are written: the
// new blob at EOF and the 4-byte pointer in IFD0. The blob construction logic lives in
// dng_gps_blob.go so this file stays focused on file I/O.

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// writeGPSToDNG writes GPS latitude/longitude into a DNG file in-place.
// The caller is responsible for backup/restore — this function only mutates the file.
func writeGPSToDNG(path string, lat, lon float64) error {
	// Open for in-place read+write. We never truncate — only append and patch.
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open DNG: %w", err)
	}
	defer f.Close()

	byteOrder, ifd0Offset, err := readTIFFHeader(f)
	if err != nil {
		return err
	}

	gpsPointerOffset, err := findGPSInfoIFDPointer(f, byteOrder, ifd0Offset)
	if err != nil {
		return err
	}

	appendOffset, err := alignedAppendOffset(f)
	if err != nil {
		return err
	}

	// Build the blob and append it at the end of the file.
	blob := buildGPSIFDBlob(lat, lon, appendOffset, byteOrder)
	if _, err := f.Write(blob); err != nil {
		return fmt.Errorf("write GPS IFD blob: %w", err)
	}

	// Patch the 4-byte GPSInfoIFD pointer in IFD0 to reference the new blob.
	if _, err := f.Seek(gpsPointerOffset, io.SeekStart); err != nil {
		return fmt.Errorf("seek to GPS pointer: %w", err)
	}
	if err := binary.Write(f, byteOrder, uint32(appendOffset)); err != nil {
		return fmt.Errorf("write GPS pointer: %w", err)
	}
	return nil
}

// readTIFFHeader reads and validates the 8-byte TIFF header and returns the byte
// order and IFD0 offset. Layout: 2 bytes order (II/MM), 2 bytes magic (42), 4 bytes
// IFD0 offset.
func readTIFFHeader(f *os.File) (binary.ByteOrder, uint32, error) {
	var header [8]byte
	if _, err := io.ReadFull(f, header[:]); err != nil {
		return nil, 0, fmt.Errorf("read TIFF header: %w", err)
	}
	var byteOrder binary.ByteOrder
	switch string(header[0:2]) {
	case "II":
		byteOrder = binary.LittleEndian
	case "MM":
		byteOrder = binary.BigEndian
	default:
		return nil, 0, fmt.Errorf("invalid TIFF byte order: %x %x", header[0], header[1])
	}
	magic := byteOrder.Uint16(header[2:4])
	if magic != 42 {
		return nil, 0, fmt.Errorf("invalid TIFF magic: %d (expected 42)", magic)
	}
	return byteOrder, byteOrder.Uint32(header[4:8]), nil
}

// findGPSInfoIFDPointer walks IFD0 and returns the absolute file offset of the
// 4-byte value field of the GPSInfoIFD entry (tag 0x8825). Each IFD entry is
// 12 bytes: tag(2) + type(2) + count(4) + value_or_offset(4).
func findGPSInfoIFDPointer(f *os.File, byteOrder binary.ByteOrder, ifd0Offset uint32) (int64, error) {
	if _, err := f.Seek(int64(ifd0Offset), io.SeekStart); err != nil {
		return 0, fmt.Errorf("seek to IFD0: %w", err)
	}
	var entryCount uint16
	if err := binary.Read(f, byteOrder, &entryCount); err != nil {
		return 0, fmt.Errorf("read IFD0 entry count: %w", err)
	}
	for i := uint16(0); i < entryCount; i++ {
		entryOffset := int64(ifd0Offset) + 2 + int64(i)*12
		if _, err := f.Seek(entryOffset, io.SeekStart); err != nil {
			return 0, fmt.Errorf("seek to IFD0 entry %d: %w", i, err)
		}
		var tag uint16
		if err := binary.Read(f, byteOrder, &tag); err != nil {
			return 0, fmt.Errorf("read IFD0 tag %d: %w", i, err)
		}
		if tag == 0x8825 { // GPSInfoIFD
			// The 4-byte value field sits at bytes 8–11 of the 12-byte entry.
			return entryOffset + 8, nil
		}
	}
	return 0, fmt.Errorf("DNG file has no GPSInfoIFD tag (0x8825) in IFD0")
}

// alignedAppendOffset seeks to EOF, pads with one zero byte if needed to restore
// 2-byte TIFF word alignment, and returns the resulting append offset. Also guards
// against the theoretical 4 GB limit of 32-bit TIFF offsets — consumer DNGs are
// 20–60 MB so this is only defensive.
func alignedAppendOffset(f *os.File) (int64, error) {
	fileEnd, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, fmt.Errorf("seek to EOF: %w", err)
	}
	if fileEnd%2 != 0 {
		if _, err := f.Write([]byte{0x00}); err != nil {
			return 0, fmt.Errorf("write alignment pad: %w", err)
		}
		fileEnd++
	}
	if fileEnd > math.MaxUint32 {
		return 0, fmt.Errorf("DNG file too large for GPS write (%d bytes)", fileEnd)
	}
	return fileEnd, nil
}
