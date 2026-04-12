package main

// thumbnail.go — Thumbnail generation for photo previews
// Decodes images and generates small JPEG thumbnails returned as base64 strings.
//
// Supported formats for thumbnails:
//   - JPG/JPEG — via image/jpeg (stdlib)
//   - PNG      — via image/png (stdlib)
//   - DNG      — via golang.org/x/image/tiff (DNG is TIFF-based)
//   - ARW      — via golang.org/x/image/tiff (Sony ARW is TIFF-based)
//
// NOT supported (returns empty string):
//   - HEIC — no pure-Go HEVC decoder exists; frontend shows placeholder icon
//
// Key functions:
//   - GenerateThumbnail(path, maxSize) → (base64String, error)
//
// TODO: Implement in Phase 1
