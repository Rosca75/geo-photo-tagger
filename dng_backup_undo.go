package main

// dng_backup_undo.go — DNG undo + tamper detection.
//
// Companion to dng_backup.go. Split out so both files stay under the
// CLAUDE.md 150-line limit. See dng_backup.go for the sidecar schema
// and capture logic.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// undoDNGFromSidecar reverses a writeGPSToDNG patch using the sidecar.
// Caller should invoke checkDNGTamper first for the pre-apply safety check.
func undoDNGFromSidecar(targetPath string, sc *dngBackupSidecar) error {
	f, err := os.OpenFile(targetPath, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open for undo: %w", err)
	}
	defer f.Close()

	// Detect byte order again — cheap and robust; the sidecar doesn't store it.
	byteOrder, _, err := readTIFFHeader(f)
	if err != nil {
		return fmt.Errorf("undo header: %w", err)
	}

	// Step 1: restore the original 4-byte pointer value in IFD0.
	if _, err := f.Seek(sc.GPSPointerOffset, io.SeekStart); err != nil {
		return fmt.Errorf("undo seek pointer: %w", err)
	}
	if err := binary.Write(f, byteOrder, sc.OriginalPointerValue); err != nil {
		return fmt.Errorf("undo write pointer: %w", err)
	}

	// Step 2: truncate the file back to its original size, discarding
	// the appended GPS IFD blob.
	if err := f.Truncate(sc.OriginalFileSize); err != nil {
		return fmt.Errorf("undo truncate: %w", err)
	}
	return nil
}

// checkDNGTamper verifies the file at targetPath still matches the sidecar's
// pre-apply hash. Returns nil if the file looks untouched (aside from our own
// patch), or an error describing the tamper condition.
//
// The first 64 KB covers TIFF header, IFD0, and SubIFD pointers — the regions
// our patch does NOT touch, so they should be byte-identical to the pre-apply
// state regardless of whether the GPS write happened.
func checkDNGTamper(targetPath string, sc *dngBackupSidecar) error {
	f, err := os.Open(targetPath)
	if err != nil {
		return fmt.Errorf("tamper check open: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.CopyN(h, f, preApplyHashSize); err != nil && err != io.EOF {
		return fmt.Errorf("tamper check hash: %w", err)
	}
	if hex.EncodeToString(h.Sum(nil)) != sc.PreHashHex {
		return fmt.Errorf(
			"%s appears to have been modified externally since GPS was applied — refusing to undo to prevent corruption",
			filepath.Base(targetPath))
	}
	return nil
}
