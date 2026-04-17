package main

// exif_reader_offset.go — Extracts OffsetTimeOriginal (EXIF tag 0x9011) and
// the raw DateTimeOriginal string from a goexif-decoded *exif.Exif. The
// goexif library does not expose 0x9011 via its known-tag map, so we walk
// the Exif sub-IFD ourselves using the raw TIFF bytes it preserves on the
// decoded object.
//
// Paired with parseEXIFDateTimeToUTC to replace the naive x.DateTime() call
// on the JPEG + HEIC scan paths. See dng_scan_reader_datetime.go for the
// DNG-path equivalent.

import (
	"bytes"
	"strings"

	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"
)

// extractDateTimeAndOffset returns the raw EXIF DateTimeOriginal string
// ("YYYY:MM:DD HH:MM:SS") and its matching OffsetTimeOriginal ("+HH:MM" /
// "-HH:MM"). Either value may be "" if the tag is absent.
//
// Falls back to DateTime (tag 0x0132, IFD0) when DateTimeOriginal is
// missing — same preference order goexif's x.DateTime() applies.
func extractDateTimeAndOffset(x *exif.Exif) (dateTimeStr, offsetStr string) {
	dateTimeStr = getASCIITag(x, exif.DateTimeOriginal)
	if dateTimeStr == "" {
		dateTimeStr = getASCIITag(x, exif.DateTime)
	}
	offsetStr = readOffsetTimeOriginal(x)
	return dateTimeStr, offsetStr
}

// getASCIITag fetches an ASCII tag value from the goexif main table,
// returning "" if the tag is missing or not string-shaped. Trims the
// trailing NUL that EXIF ASCII always carries.
func getASCIITag(x *exif.Exif, name exif.FieldName) string {
	tag, err := x.Get(name)
	if err != nil {
		return ""
	}
	s, err := tag.StringVal()
	if err != nil {
		return ""
	}
	return strings.TrimRight(s, "\x00")
}

// readOffsetTimeOriginal walks the Exif sub-IFD in x.Raw to find tag
// 0x9011 (OffsetTimeOriginal). goexif's known-field map does not include
// 0x9011, so the tag is never loaded into x.main; we mirror what
// loadSubDir() does internally but only for this one tag.
//
// Returns "" on any structural issue — callers treat that as "no offset
// present" and fall back to the default timezone.
func readOffsetTimeOriginal(x *exif.Exif) string {
	if x == nil || x.Tiff == nil {
		return ""
	}
	ptrTag, err := x.Get(exif.ExifIFDPointer)
	if err != nil {
		return ""
	}
	offset, err := ptrTag.Int64(0)
	if err != nil {
		return ""
	}
	r := bytes.NewReader(x.Raw)
	if _, err := r.Seek(offset, 0); err != nil {
		return ""
	}
	subDir, _, err := tiff.DecodeDir(r, x.Tiff.Order)
	if err != nil {
		return ""
	}
	const tagOffsetTimeOriginal = 0x9011
	for _, t := range subDir.Tags {
		if t.Id != tagOffsetTimeOriginal {
			continue
		}
		s, err := t.StringVal()
		if err != nil {
			return ""
		}
		return strings.TrimRight(s, "\x00")
	}
	return ""
}
