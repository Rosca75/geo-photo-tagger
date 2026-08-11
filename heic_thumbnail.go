package main

// heic_thumbnail.go — HEIC/HEIF thumbnail generation using WASM-based libheif.
//
// This module adds thumbnail support for HEIC/HEIF reference photos.
// It uses github.com/Rosca75/heic which embeds a libheif/libde265 binary
// compiled to WASM and run via tetratelabs/wazero — no CGo, no external binaries.
//
// Fast path: only the first 192 KB of each HEIC file is read. iPhone-style
// HEICs store the ISOBMFF header + embedded thumbnail tile within ~128 KB;
// the extra headroom handles edge cases. Full-file fallback is used on failure.

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"log"
	"os"

	"github.com/Rosca75/heic"
)

// heicHeaderReadSize is the byte range used by the HEIC fast path.
// 192 KB covers the ftyp + meta + iloc + thumbnail tile on all iPhone HEIC
// samples tested. Files whose thumbnail tile sits beyond this window fall
// back to a full read.
const heicHeaderReadSize = 192 * 1024

// initHEIC configures the HEIC decoder at startup.
// Calling this before the first thumbnail request pre-compiles the WASM
// module, avoiding a 200-400 ms delay on the first HEIC decode.
// If a dynamic libheif is found but its version is below 1.18, WASM mode
// is forced for full compatibility with HDR/tmap-brand HEICs (iPhone 12+).
func initHEIC() {
	if heic.Dynamic() != nil {
		// No dynamic libheif available; the package will use WASM automatically.
		return
	}
	if !heicDynamicVersionAtLeast(1, 18) {
		heic.ForceWasmMode = true
		log.Println("[heic] Dynamic libheif < 1.18; switching to WASM decoder")
	}
}

// readHEICHeader reads up to heicHeaderReadSize bytes from the file at path.
// Returns the bytes actually read (may be fewer for small files) and any
// non-EOF I/O error.
func readHEICHeader(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, heicHeaderReadSize)
	n, err := io.ReadFull(f, buf)
	// io.ReadFull returns ErrUnexpectedEOF when the file is shorter than the
	// buffer -- that is normal; we just use however many bytes we got.
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return nil, err
	}
	return buf[:n], nil
}

// decodeHEICImage extracts a decodable image from a HEIC file.
// Strategy (in order):
//  1. Try DecodeThumbnail from first 192 KB (fastest -- embedded JPEG tile).
//  2. Try Decode (primary image) from first 192 KB.
//  3. Fall back to full file read and retry both operations.
func decodeHEICImage(path string) (image.Image, error) {
	header, err := readHEICHeader(path)
	if err != nil {
		return nil, err
	}

	// Fast path attempts on the header buffer.
	if img, thumbErr := heic.DecodeThumbnail(bytes.NewReader(header)); thumbErr == nil {
		return img, nil
	}
	if img, decErr := heic.Decode(bytes.NewReader(header)); decErr == nil {
		return img, nil
	}

	// Fallback: full file read (for files whose thumbnail tile is past 192 KB).
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if img, thumbErr := heic.DecodeThumbnail(bytes.NewReader(data)); thumbErr == nil {
		return img, nil
	}
	if img, decErr := heic.Decode(bytes.NewReader(data)); decErr == nil {
		return img, nil
	}
	return nil, fmt.Errorf("heic: could not decode thumbnail or primary image from %q", path)
}

// generateHEICThumbnail decodes a HEIC file and returns a base64-encoded JPEG
// thumbnail at most maxSize x maxSize pixels (aspect ratio preserved).
// Returns "" on any error so the frontend can display a fallback.
func generateHEICThumbnail(path string, maxSize int) (string, error) {
	img, err := decodeHEICImage(path)
	if err != nil {
		// Non-fatal: return empty string so the frontend shows a placeholder.
		return "", fmt.Errorf("heic thumbnail %q: %w", path, err)
	}
	scaled := scaleToFit(img, maxSize)
	return encodeJPEGBase64(scaled)
}
