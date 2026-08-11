package main

// matcher.go — Time-based GPS matching engine
// Matches target photos against reference photos by comparing EXIF
// DateTimeOriginal timestamps (all normalised to UTC).
//
// Scoring (CLAUDE.md §6):
//   delta <= 1 min  → 100 (excellent)
//   delta <= 5 min  → 90  (very good)
//   delta <= 10 min → 75  (good)
//   delta <= 30 min → 50  (fair)
//   delta <= 60 min → 25  (poor)
//   delta >  60 min → 0   (no match)

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"
)

// computeScore converts a time delta to a 0–100 quality score.
func computeScore(delta time.Duration) int {
	m := delta.Minutes()
	switch {
	case m <= 1:
		return 100
	case m <= 5:
		return 90
	case m <= 10:
		return 75
	case m <= 30:
		return 50
	case m <= 60:
		return 25
	default:
		return 0
	}
}

// formatDelta renders a duration as a compact human-readable string.
// Examples: 90s → "1m30s", 3661s → "1h1m", 45s → "45s".
func formatDelta(d time.Duration) string {
	d = d.Round(time.Second)
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm", h, m)
	}
	if m > 0 && s > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%ds", s)
}

// absDuration returns the absolute value of d.
func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// MatchPhotos is the core matching engine.
// For every target photo it finds all reference photos within
// opts.MaxTimeDeltaMinutes, scores them, and returns sorted results.
// All timestamps are normalised to UTC before comparison (CLAUDE.md rule #15).
func MatchPhotos(targets []TargetPhoto, refs []ReferencePhoto, opts MatchOptions) []MatchResult {
	maxDelta := time.Duration(opts.MaxTimeDeltaMinutes) * time.Minute

	results := make([]MatchResult, 0, len(targets))

	for _, target := range targets {
		res := MatchResult{TargetPath: target.Path}

		// Photos without a timestamp cannot be matched by time.
		if target.DateTimeOriginal.IsZero() {
			results = append(results, res)
			continue
		}
		tgt := target.DateTimeOriginal.UTC()

		var cands []MatchCandidate

		// — Reference photo candidates —
		for _, ref := range refs {
			if ref.DateTimeOriginal.IsZero() {
				continue
			}
			delta := absDuration(tgt.Sub(ref.DateTimeOriginal.UTC()))
			if delta > maxDelta {
				continue
			}
			score := computeScore(delta)
			if score == 0 {
				continue
			}
			cands = append(cands, MatchCandidate{
				Source:             "photo",
				SourcePath:         ref.Path,
				SourceFilename:     filepath.Base(ref.Path),
				GPS:                ref.GPS,
				TimeDelta:          delta,
				TimeDeltaFormatted: formatDelta(delta),
				Score:              score,
				IsHEIC:             ref.IsHEIC,
			})
		}

		// Sort: highest score first; tie-break by shortest delta.
		sort.Slice(cands, func(i, j int) bool {
			if cands[i].Score != cands[j].Score {
				return cands[i].Score > cands[j].Score
			}
			return cands[i].TimeDelta < cands[j].TimeDelta
		})

		res.Candidates = cands
		if len(cands) > 0 {
			best := cands[0]
			res.BestCandidate = &best
		}
		results = append(results, res)
	}
	return results
}
