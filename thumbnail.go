package main

// thumbnail.go — Thumbnail generation for photo previews
// Produces small base64-encoded JPEG strings used in img.src data URLs.
//
// Supported formats:
//   - JPG/JPEG — stdlib image/jpeg decoder
//   - PNG      — stdlib image/png decoder (registered via blank import)
//   - DNG      — embedded JPEG preview via goexif, falls back to TIFF decode
//   - ARW      — same as DNG (Sony RAW is TIFF-based)
//   - HEIC     — returns "" (no pure-Go HEVC decoder; frontend shows placeholder)

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // registers PNG decoder for image.Decode
	"os"
	"path/filepath"
	"strings"

	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/tiff" // registers TIFF decoder for DNG/ARW
)

// GenerateThumbnail decodes the image at path and returns a base64 JPEG
// thumbnail no larger than maxSize × maxSize pixels (aspect ratio preserved).
// Returns "" with no error for HEIC/HEIF files (unsupported format by design).
// Returns "" on any decode error so the frontend can show a placeholder.
func GenerateThumbnail(path string, maxSize int) (string, error) {
	if maxSize <= 0 {
		maxSize = 200
	}

	ext := strings.ToLower(filepath.Ext(path))

	// HEIC/HEIF: no pure-Go pixel decoder available without CGo. Per CLAUDE.md
	// hard constraint #1, we return empty string and let the frontend show an icon.
	if ext == ".heic" || ext == ".heif" {
		return "", nil
	}

	var img image.Image
	var err error

	// DNG: goexif fails on large Pentax-class DNGs (see dng_thumbnail_reader.go).
	// Walk the TIFF IFDs directly to pull the embedded JPEG preview, and fall
	// back to decodeImageFile (TIFF decoder) for small DNGs that lack one.
	// ARW: goexif still works, so keep the existing path unchanged.
	if ext == ".dng" {
		img, err = loadDNGEmbeddedPreview(path)
		if err != nil {
			img, err = decodeImageFile(path)
		}
	} else if ext == ".arw" {
		img, err = loadRawEmbeddedPreview(path)
		if err != nil {
			img, err = decodeImageFile(path)
		}
	} else {
		img, err = decodeImageFile(path)
	}

	if err != nil {
		return "", fmt.Errorf("loading %q: %w", path, err)
	}

	scaled := scaleToFit(img, maxSize)
	return encodeJPEGBase64(scaled)
}

// loadRawEmbeddedPreview extracts the JPEG thumbnail that most RAW files
// (DNG, ARW) embed in their EXIF data. This is a small, quick-to-read preview
// rather than the full-resolution RAW image data.
func loadRawEmbeddedPreview(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	x, err := exif.Decode(f)
	if err != nil {
		return nil, err
	}

	jpegBytes, err := x.JpegThumbnail()
	if err != nil {
		return nil, err
	}

	return jpeg.Decode(bytes.NewReader(jpegBytes))
}

// decodeImageFile opens and decodes an image file using Go's registered decoders.
// Supports JPEG (image/jpeg), PNG (image/png), and TIFF (golang.org/x/image/tiff).
// DNG and ARW are TIFF-based and handled by the TIFF decoder.
func decodeImageFile(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	return img, err
}

// scaleToFit resizes img so its longest dimension equals maxSize,
// maintaining aspect ratio. Returns the original if it already fits.
func scaleToFit(img image.Image, maxSize int) image.Image {
	w := img.Bounds().Dx()
	h := img.Bounds().Dy()

	if w <= maxSize && h <= maxSize {
		return img
	}

	// Calculate new dimensions preserving aspect ratio
	var newW, newH int
	if w > h {
		newW = maxSize
		newH = (h * maxSize) / w
	} else {
		newH = maxSize
		newW = (w * maxSize) / h
	}
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	// BiLinear scaling gives good quality for downscaling
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)
	return dst
}

// encodeJPEGBase64 JPEG-encodes img and returns the bytes as a base64 string.
// The result can be used directly in: img.src = "data:image/jpeg;base64," + result
func encodeJPEGBase64(img image.Image) (string, error) {
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		return "", fmt.Errorf("jpeg encode: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}
