package main

// thumbnail_encode.go — Decode/scale/encode helpers shared by GenerateThumbnail.
// Split out of thumbnail.go to keep both files under the 150-line ceiling.
// These are pure image helpers with no caching or format-dispatch logic.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"os"

	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/draw"
)

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
