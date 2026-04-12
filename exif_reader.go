package main

// exif_reader.go — EXIF metadata extraction
// Reads GPS coordinates, timestamps, and camera model from photo EXIF data.
// Supports standard JPEG/TIFF-based formats natively and HEIC via ISOBMFF parsing.
//
// Key functions:
//   - ReadEXIF(path) → EXIFData    — for JPG, PNG, DNG, ARW
//   - ReadHEICExif(path) → EXIFData — for HEIC files (GPS + timestamp only)
//
// TODO: Implement in Phase 1
