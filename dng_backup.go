package main

// dng_backup.go — Lazy DNG backup via sidecar metadata.
//
// Instead of copying the entire 44 MB DNG to a .bak file before every GPS
// write, this strategy stores only the minimum needed to reverse the patch:
//   - the absolute file offset of the 4-byte GPSInfoIFD pointer in IFD0
//   - the original 4-byte value at that offset (the old GPS IFD offset)
//   - the original file size (so we can truncate back after undo)
//   - a SHA-256 hash of the first 64 KB of the pre-apply file
//
// The hash fingerprint is what makes undo safe: if another tool modified the
// DNG between apply and undo, the hash won't match and UndoGPS refuses to
// restore — protecting the user from silent corruption.
//
// Sidecar format: JSON at <targetPath>.bak.json. ClearBackups and the
// orphan sweep both handle these files alongside the legacy .bak.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// dngBackupSidecar holds all state needed to reverse a writeGPSToDNG patch.
// Serialised as JSON to <targetPath>.bak.json.
type dngBackupSidecar struct {
	// Version of the sidecar schema. Bump if the format changes.
	Version int `json:"version"`

	// GPSPointerOffset is the absolute file offset of the 4-byte value field
	// of the GPSInfoIFD entry in IFD0.
	GPSPointerOffset int64 `json:"gpsPointerOffset"`

	// OriginalPointerValue is the 4-byte value at GPSPointerOffset before
	// the patch — typically points to the camera's pre-existing minimal GPS IFD.
	OriginalPointerValue uint32 `json:"originalPointerValue"`

	// OriginalFileSize is the file's size before the GPS IFD blob was appended.
	// Undo truncates back to this size.
	OriginalFileSize int64 `json:"originalFileSize"`

	// PreHashHex is the SHA-256 of the first 64 KB of the pre-apply file.
	// Used to detect external modification between apply and undo.
	PreHashHex string `json:"preHashHex"`
}

// preApplyHashSize controls how many bytes of the file head are hashed for
// the tamper-detection fingerprint. 64 KB covers TIFF header, IFD0, and
// most SubIFD pointers — enough to catch any meaningful modification.
const preApplyHashSize = 64 * 1024

// sidecarPath returns the path of the backup sidecar for a given target DNG.
func sidecarPath(targetPath string) string {
	return targetPath + ".bak.json"
}

// captureDNGBackup computes pre-apply metadata and writes the sidecar JSON.
// Must be called BEFORE writeGPSToDNG modifies the file. Returns error only
// on unrecoverable I/O failure — callers must abort the GPS write if this
// fails, since without the sidecar undo would be impossible.
func captureDNGBackup(targetPath string) error {
	f, err := os.Open(targetPath)
	if err != nil {
		return fmt.Errorf("open for backup: %w", err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return fmt.Errorf("stat for backup: %w", err)
	}

	byteOrder, ifd0Offset, err := readTIFFHeader(f)
	if err != nil {
		return fmt.Errorf("header for backup: %w", err)
	}
	gpsPointerOffset, err := findGPSInfoIFDPointer(f, byteOrder, ifd0Offset)
	if err != nil {
		return fmt.Errorf("pointer for backup: %w", err)
	}

	// Read the original 4-byte pointer value.
	if _, err := f.Seek(gpsPointerOffset, io.SeekStart); err != nil {
		return fmt.Errorf("seek pointer for backup: %w", err)
	}
	var origValue uint32
	if err := binary.Read(f, byteOrder, &origValue); err != nil {
		return fmt.Errorf("read pointer for backup: %w", err)
	}

	// Hash the first 64 KB for tamper detection.
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("seek for hash: %w", err)
	}
	h := sha256.New()
	if _, err := io.CopyN(h, f, preApplyHashSize); err != nil && err != io.EOF {
		return fmt.Errorf("hash head: %w", err)
	}

	sidecar := dngBackupSidecar{
		Version:              1,
		GPSPointerOffset:     gpsPointerOffset,
		OriginalPointerValue: origValue,
		OriginalFileSize:     fi.Size(),
		PreHashHex:           hex.EncodeToString(h.Sum(nil)),
	}

	data, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sidecar: %w", err)
	}
	return os.WriteFile(sidecarPath(targetPath), data, 0644)
}

// loadDNGBackup reads and parses the sidecar JSON for targetPath.
// Returns an error if the sidecar is missing, malformed, or an unknown version.
func loadDNGBackup(targetPath string) (*dngBackupSidecar, error) {
	data, err := os.ReadFile(sidecarPath(targetPath))
	if err != nil {
		return nil, err
	}
	var sc dngBackupSidecar
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("parse sidecar: %w", err)
	}
	if sc.Version != 1 {
		return nil, fmt.Errorf("unsupported sidecar version: %d", sc.Version)
	}
	return &sc, nil
}

// SweepOrphanedSidecars removes .bak.json files whose corresponding DNG no
// longer exists. Called at ScanTargetFolder time so dangling metadata from
// deleted photos doesn't linger. Does NOT touch sidecars with a living
// target — those are legitimate pending-undo state that must survive restart.
// Returns the count of removed orphans.
func SweepOrphanedSidecars(folderPath string) int {
	if folderPath == "" {
		return 0
	}
	count := 0
	_ = filepath.WalkDir(folderPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".bak.json") {
			return nil
		}
		targetPath := strings.TrimSuffix(path, ".bak.json")
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			if os.Remove(path) == nil {
				count++
			}
		}
		return nil
	})
	return count
}
