package main

// app_write.go — Wails-bound methods for writing GPS data to photos.
// Split from app.go to keep file lengths under 150 lines (see CLAUDE.md §10).
// All methods here call exif_writer.go functions; app.go handles read operations.

import "fmt"

// ApplyGPS writes GPS coordinates to a single target photo's EXIF data.
// Creates a .bak backup before modifying the file (CLAUDE.md §12, rule 1).
// Returns a status map for the JS frontend: {"status":"applied"} or {"status":"error"}.
func (a *App) ApplyGPS(targetPath string, lat float64, lon float64) (map[string]interface{}, error) {
	if err := WriteGPS(targetPath, lat, lon); err != nil {
		return map[string]interface{}{"status": "error", "message": err.Error()}, err
	}

	// Update the in-memory status so GetScanStatus and the table reflect the change.
	for i := range a.targetPhotos {
		if a.targetPhotos[i].Path == targetPath {
			a.targetPhotos[i].Status = "applied"
			break
		}
	}
	return map[string]interface{}{"status": "applied"}, nil
}

// ApplyBatchGPS batch-writes GPS coordinates to multiple photos in one call.
// The frontend sends the full list of accepted matches (collected from state.acceptedMatches).
// Each entry in matches is a GPSApplyRequest{TargetPath, Lat, Lon}.
// Returns a summary map: {"applied": N, "errors": N, "messages": [...]}.
func (a *App) ApplyBatchGPS(matches []GPSApplyRequest) (map[string]interface{}, error) {
	applied := 0
	var errs []string

	for _, m := range matches {
		if err := WriteGPS(m.TargetPath, m.Lat, m.Lon); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", m.TargetPath, err))
			continue
		}
		applied++
		// Update the in-memory status so Zone B badges reflect the apply.
		for i := range a.targetPhotos {
			if a.targetPhotos[i].Path == m.TargetPath {
				a.targetPhotos[i].Status = "applied"
				break
			}
		}
	}

	result := map[string]interface{}{
		"applied":  applied,
		"errors":   len(errs),
		"messages": errs,
	}
	// Return a non-nil error only when every single write failed.
	if applied == 0 && len(errs) > 0 {
		return result, fmt.Errorf("all %d GPS writes failed", len(errs))
	}
	return result, nil
}

// UndoGPS restores a target photo from its .bak backup.
// The backup file is preserved so the user can undo again later.
func (a *App) UndoGPS(targetPath string) (map[string]interface{}, error) {
	if err := UndoGPS(targetPath); err != nil {
		return map[string]interface{}{"status": "error", "message": err.Error()}, err
	}

	// Reset the in-memory status back to "matched" so the badge updates.
	for i := range a.targetPhotos {
		if a.targetPhotos[i].Path == targetPath {
			a.targetPhotos[i].Status = "matched"
			break
		}
	}
	return map[string]interface{}{"status": "restored"}, nil
}

// ClearAllBackups deletes all .bak files in the target folder scanned this session.
// Returns the count of deleted files and any walk error.
func (a *App) ClearAllBackups() (int, error) {
	if a.targetFolder == "" {
		return 0, fmt.Errorf("no target folder scanned in this session")
	}
	return ClearBackups(a.targetFolder)
}
