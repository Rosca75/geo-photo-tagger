package main

// dng_thumbnail_reader.go — Direct-binary DNG JPEG-preview extractor.
// For large Pentax-class DNGs, goexif cannot reliably locate the embedded
// preview, and golang.org/x/image/tiff cannot decode compressed DNG raw
// data. Walking the TIFF IFDs ourselves (as dng_scan_reader.go does for
// GPS) pulls the embedded JPEG out directly. Reuses readTIFFHeader from
// dng_gps_writer.go.

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"os"
)

// dngPreview ranks candidate JPEG previews by pixels (width*height; 0 when
// dimensions are not declared).
type dngPreview struct {
	offset uint32
	length uint32
	pixels uint64
}

// loadDNGEmbeddedPreview walks IFD0 + SubIFDs, collects every
// (JPEGInterchangeFormat, JPEGInterchangeFormatLength) pair, picks the
// largest, seeks to it and JPEG-decodes. Returns an error on no preview
// so the caller can fall back.
func loadDNGEmbeddedPreview(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	byteOrder, ifd0Offset, err := readTIFFHeader(f)
	if err != nil {
		return nil, err
	}

	previews, subIFDs, err := collectPreviewsFromIFD(f, byteOrder, int64(ifd0Offset))
	if err != nil {
		return nil, fmt.Errorf("walk IFD0: %w", err)
	}
	// Modern DNGs put their largest preview in a SubIFD (no recursion
	// into sub-sub-IFDs — not observed in consumer DNGs).
	for _, sub := range subIFDs {
		ps, _, subErr := collectPreviewsFromIFD(f, byteOrder, int64(sub))
		if subErr != nil {
			continue
		}
		previews = append(previews, ps...)
	}

	if len(previews) == 0 {
		return nil, fmt.Errorf("no JPEG preview in DNG %s", path)
	}
	best := previews[0]
	for _, p := range previews[1:] {
		if p.pixels > best.pixels {
			best = p
		}
	}
	if _, err := f.Seek(int64(best.offset), io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek to preview: %w", err)
	}
	// jpeg.Decode stops at EOI; LimitReader guards a bogus length.
	return jpeg.Decode(io.LimitReader(f, int64(best.length)))
}

// collectPreviewsFromIFD reads one IFD in a single pass. Returns up to one
// dngPreview (when both 0x0201 and 0x0202 are present) and the list of
// SubIFD offsets (0x014A).
func collectPreviewsFromIFD(f *os.File, bo binary.ByteOrder, ifdOffset int64) ([]dngPreview, []uint32, error) {
	if _, err := f.Seek(ifdOffset, io.SeekStart); err != nil {
		return nil, nil, fmt.Errorf("seek IFD: %w", err)
	}
	var entryCount uint16
	if err := binary.Read(f, bo, &entryCount); err != nil {
		return nil, nil, fmt.Errorf("entry count: %w", err)
	}
	var (
		subs    []uint32
		preview dngPreview
		w, h    uint32
	)
	for i := uint16(0); i < entryCount; i++ {
		var entry [12]byte
		if _, err := io.ReadFull(f, entry[:]); err != nil {
			return nil, nil, fmt.Errorf("entry %d: %w", i, err)
		}
		tag := bo.Uint16(entry[0:2])
		typeID := bo.Uint16(entry[2:4])
		count := bo.Uint32(entry[4:8])
		val := bo.Uint32(entry[8:12])
		switch tag {
		case 0x0100:
			w = shortOrLong(bo, entry[8:], typeID, val)
		case 0x0101:
			h = shortOrLong(bo, entry[8:], typeID, val)
		case 0x0201:
			preview.offset = val
		case 0x0202:
			preview.length = val
		case 0x014A:
			cur, _ := f.Seek(0, io.SeekCurrent)
			subs = append(subs, readSubIFDList(f, bo, count, val)...)
			if _, err := f.Seek(cur, io.SeekStart); err != nil {
				return nil, nil, fmt.Errorf("restore after SubIFDs: %w", err)
			}
		}
	}
	if preview.offset != 0 && preview.length != 0 {
		preview.pixels = uint64(w) * uint64(h)
		return []dngPreview{preview}, subs, nil
	}
	return nil, subs, nil
}

// shortOrLong returns a single-count SHORT (typeID 3, first 2 bytes of the
// value field) or LONG (4, all 4 bytes) in the file's byte order.
func shortOrLong(bo binary.ByteOrder, v []byte, typeID uint16, val uint32) uint32 {
	if typeID == 3 {
		return uint32(bo.Uint16(v[0:2]))
	}
	return val
}

// readSubIFDList reads offsets pointed to by tag 0x014A. count==1 means
// value IS the offset; otherwise value points to an array of count LONGs.
func readSubIFDList(f *os.File, bo binary.ByteOrder, count, value uint32) []uint32 {
	if count == 0 {
		return nil
	}
	if count == 1 {
		return []uint32{value}
	}
	if _, err := f.Seek(int64(value), io.SeekStart); err != nil {
		return nil
	}
	offs := make([]uint32, count)
	for i := range offs {
		if err := binary.Read(f, bo, &offs[i]); err != nil {
			return offs[:i]
		}
	}
	return offs
}
