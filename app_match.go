package main

// app_match.go — GPS matching engine wiring for the App struct.
// Exposes RunMatching and GetMatchResults to the Wails/JS frontend.
//
// RunMatching executes synchronously (the JS await blocks until it completes).
// For typical datasets (< 5 000 photos) this takes < 1 second.
// Large-dataset background matching with progress streaming is planned for Phase 9.

import "fmt"

// RunMatching executes the GPS matching engine against all loaded target photos,
// reference photos, and GPS track points using the given options.
// Stores results in app state and returns them directly so the frontend can
// render them without a separate GetMatchResults call.
//
// Also updates each TargetPhoto in the stored list with its best match and
// new status ("matched" / "unmatched"), so subsequent GetScanStatus calls
// reflect the correct counts.
func (a *App) RunMatching(opts MatchOptions) ([]MatchResult, error) {
	// Require at least one photo to match.
	if len(a.targetPhotos) == 0 {
		return []MatchResult{}, nil
	}
	// Require at least one source of GPS data.
	if len(a.referencePhotos) == 0 && len(a.gpsTrackPoints) == 0 {
		return nil, fmt.Errorf("no GPS sources loaded — add a reference folder or import a GPS track first")
	}

	a.scanStatus = ScanStatus{
		InProgress: true,
		Phase:      "matching",
		Total:      len(a.targetPhotos),
		Message:    "Matching photos against GPS sources...",
	}

	// Call the engine from matcher.go.
	results := MatchPhotos(a.targetPhotos, a.referencePhotos, a.gpsTrackPoints, opts)

	// Build a path→result lookup so we can update targetPhotos in O(1) per photo.
	byPath := make(map[string]*MatchResult, len(results))
	for i := range results {
		byPath[results[i].TargetPath] = &results[i]
	}

	matched := 0
	for i := range a.targetPhotos {
		r, ok := byPath[a.targetPhotos[i].Path]
		if ok && r.BestCandidate != nil {
			a.targetPhotos[i].BestMatch = r.BestCandidate
			a.targetPhotos[i].Status = "matched"
			matched++
		} else {
			a.targetPhotos[i].Status = "unmatched"
		}
	}

	a.matchResults = results
	a.scanStatus = ScanStatus{
		Phase:    "idle",
		Progress: matched,
		Total:    len(results),
		Message:  fmt.Sprintf("%d of %d photos matched", matched, len(results)),
	}

	return results, nil
}

// GetMatchResults returns the most recent matching results.
// Returns an empty slice (never nil) before RunMatching has been called.
func (a *App) GetMatchResults() []MatchResult {
	if a.matchResults == nil {
		return []MatchResult{}
	}
	return a.matchResults
}
