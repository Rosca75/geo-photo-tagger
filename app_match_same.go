package main

// app_match_same.go — Module 3: Same-source matching (phase 7).
// Uses the currently-scanned source folder as BOTH target pool and reference
// pool. Split into its own file to keep app_match.go under 150 lines.

import "fmt"

// RunSameSourceMatching runs the matching engine using the currently-
// scanned source folder as both target pool (photos without GPS) and
// reference pool (photos in the same folder that already have GPS).
//
// Useful when shooting a series at one location with a camera whose GPS
// module is unreliable (e.g. Pentax K-1): photos with GPS taken minutes
// before/after are perfect references for the ones that missed.
//
// Reuses MatchPhotos with no engine changes. lastSourceRecursive is
// honored so subfolders are included only when the user's source scan did.
func (a *App) RunSameSourceMatching(opts MatchOptions) ([]MatchResult, error) {
	if a.targetFolder == "" {
		return nil, fmt.Errorf("no source folder scanned — click Source first")
	}
	if len(a.targetPhotos) == 0 {
		return []MatchResult{}, nil
	}

	a.scanStatus = ScanStatus{
		InProgress: true,
		Phase:      "scanning_references",
		Message:    "Scanning source folder for in-folder references...",
	}

	// Re-scan the source folder specifically for geolocated photos. We
	// deliberately do NOT reuse a.referencePhotos — same-source matches
	// are session-only and the user may switch back to External refs mode
	// later without needing to re-set anything up.
	dateFilter := a.computeTargetDateRange()
	sameSourceRefs, err := ScanForReferencePhotos(a.targetFolder, dateFilter, a.lastSourceRecursive)
	if err != nil {
		a.scanStatus = ScanStatus{Phase: "idle", Message: err.Error()}
		return nil, fmt.Errorf("scanning source folder for refs: %w", err)
	}
	if len(sameSourceRefs) == 0 {
		a.scanStatus = ScanStatus{
			Phase:   "idle",
			Message: "No geolocated photos in the source folder — nothing to match against.",
		}
		return nil, fmt.Errorf("source folder contains no photos with GPS data")
	}

	a.scanStatus = ScanStatus{
		InProgress: true,
		Phase:      "matching",
		Total:      len(a.targetPhotos),
		Message: fmt.Sprintf("Matching %d targets against %d in-folder references...",
			len(a.targetPhotos), len(sameSourceRefs)),
	}

	results := MatchPhotos(a.targetPhotos, sameSourceRefs, nil, opts)

	// Same propagation as RunMatching: update per-photo status and cache.
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
		Message:  fmt.Sprintf("%d of %d photos matched (same source)", matched, len(results)),
	}
	return results, nil
}
