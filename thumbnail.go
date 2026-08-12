package main

// thumbnail.go — Thumbnail generation for photo previews
// Produces small base64-encoded JPEG strings used in img.src data URLs.
//
// Supported formats:
//   - JPG/JPEG — stdlib image/jpeg decoder
//   - PNG      — stdlib image/png decoder (registered via blank import)
//   - DNG      — embedded JPEG preview via goexif, falls back to TIFF decode
//   - ARW      — same as DNG (Sony RAW is TIFF-based)
//   - HEIC     — WASM-based libheif decoder via heic_thumbnail.go

import (
	"fmt"
	"image"
	_ "image/png" // registers PNG decoder for image.Decode
	"os"
	"path/filepath"
	"strings"

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

	// Cache lookup. The key folds in the file's mtime and size, so an edit on
	// disk (e.g. ApplyGPS rewriting EXIF) misses the stale entry and re-decodes.
	// A hit skips the expensive decode entirely — the main win for HEIC, which
	// runs a WASM decoder, and for any re-render/hover of the same file.
	key, haveKey := thumbCacheKey2(path, maxSize)
	if haveKey {
		if v, ok := thumbCache.get(key); ok {
			return v, nil
		}
	}

	ext := strings.ToLower(filepath.Ext(path))

	// HEIC/HEIF: delegate to the WASM-based decoder in heic_thumbnail.go.
	// Returns "" on failure so the frontend can show a fallback placeholder.
	if ext == ".heic" || ext == ".heif" {
		b64, err := generateHEICThumbnail(path, maxSize)
		// Cache both successes and empty-string misses so a HEIC that cannot be
		// decoded is not retried on every subsequent hover.
		if haveKey && err == nil {
			thumbCache.put(key, b64)
		}
		return b64, err
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
	b64, err := encodeJPEGBase64(scaled)
	if err != nil {
		return "", err
	}
	// Store the finished thumbnail so repeat requests are served from memory.
	if haveKey {
		thumbCache.put(key, b64)
	}
	return b64, nil
}

// thumbCacheKey2 stats the file at path and builds the composite cache key.
// Returns ok=false when the file cannot be stat'd (e.g. it was removed), in
// which case the caller skips caching and just decodes directly.
func thumbCacheKey2(path string, maxSize int) (string, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return "", false
	}
	return thumbCacheKey(path, maxSize, info.ModTime().UnixNano(), info.Size()), true
}
