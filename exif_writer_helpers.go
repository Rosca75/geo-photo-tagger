package main

// exif_writer_helpers.go — low-level file I/O helpers for exif_writer.go.

import (
	"io"
	"os"
)

// copyFile copies the file at src to dst, preserving the source file mode.
// dst is created or overwritten. Used by WriteGPS (backup) and UndoGPS (restore).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	// Stat the source so we can replicate its file permissions on the destination.
	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
