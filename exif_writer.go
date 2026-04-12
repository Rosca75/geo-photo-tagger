package main

// exif_writer.go — GPS coordinate injection into photo EXIF data
// Writes GPS latitude/longitude into target photos that lack geolocation.
//
// SAFETY RULES (see CLAUDE.md §12):
//   1. Always create a .bak backup before modifying any file
//   2. Verify the write by re-reading EXIF after writing
//   3. Never modify DateTimeOriginal or any non-GPS EXIF field
//   4. Batch operations must be interruptible (check context)
//   5. Keep .bak files until the user explicitly clears them
//
// Key functions:
//   - WriteGPS(targetPath, lat, lon) → error
//   - UndoGPS(targetPath) → error      — restore from .bak
//   - ClearBackups(folderPath) → error  — delete all .bak files
//
// TODO: Implement in Phase 6
