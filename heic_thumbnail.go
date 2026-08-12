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

// initHEIC configures the HEIC decoder at startup and kicks off a background
// pre-warm so the first *user-triggered* thumbnail doesn't pay the one-time
// ~200-400 ms WASM compile.
//
// If a dynamic libheif is found but its version is below 1.18, WASM mode is
// forced for full compatibility with HDR/tmap-brand HEICs (iPhone 12+).
//
// Important: simply configuring the decoder does NOT compile the WASM module —
// the underlying fork compiles lazily (sync.OnceFunc) on the first real decode
// call. To move that cost off the critical path we call prewarmHEIC, which
// forces the compile in a goroutine at startup.
func initHEIC() {
	if heic.Dynamic() != nil {
		// A dynamic libheif is available; no WASM compile to pre-warm.
		return
	}
	if !heicDynamicVersionAtLeast(1, 18) {
		heic.ForceWasmMode = true
		log.Println("[heic] Dynamic libheif < 1.18; switching to WASM decoder")
	}
	prewarmHEIC()
}

// prewarmHEIC forces the WASM libheif module to compile ahead of the first user
// request. It runs in a background goroutine so app startup is not blocked.
//
// The trick: the fork's decodeWASM path runs its one-time initialize() (which
// calls CompileModule — the expensive step) at the very start of the call,
// before it reads any pixels. So a DecodeThumbnail on a tiny throwaway buffer
// triggers the compile even though the decode itself returns an error. We
// deliberately ignore that error.
func prewarmHEIC() {
	go func() {
		// A few bytes are enough — the decode fails, but the WASM compile that
		// runs first is exactly what we want warmed.
		_, _ = heic.DecodeThumbnail(bytes.NewReader([]byte{0, 0, 0, 0}))
	}()
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
