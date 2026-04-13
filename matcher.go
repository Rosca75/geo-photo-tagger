package main

// matcher.go — Time-based GPS matching engine
// Matches target photos against reference photos and GPS track points by
// comparing EXIF DateTimeOriginal timestamps (all normalised to UTC).
//
// Scoring (CLAUDE.md §6):
//   delta <= 1 min  → 100 (excellent)
//   delta <= 5 min  → 90  (very good)
//   delta <= 10 min → 75  (good)
//   delta <= 30 min → 50  (fair)
//   delta <= 60 min → 25  (poor)
//   delta >  60 min → 0   (no match)
//
// When GPS track points bracket the target time, coordinates are linearly
// interpolated between the two surrounding points (see interpolateGPS).

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

// interpolateGPS computes the GPS coordinates at targetTime by linearly
// interpolating between two surrounding track points.
// fraction = (targetTime - before) / (after - before), clamped to [0,1].
func interpolateGPS(before, after GPSTrackPoint, targetTime time.Time) GPSCoord {
	total := after.Time.Sub(before.Time).Seconds()
	if total <= 0 {
		return before.GPS // degenerate case: points at the same time
	}
	frac := targetTime.Sub(before.Time).Seconds() / total
	return GPSCoord{
		Latitude:  before.GPS.Latitude + frac*(after.GPS.Latitude-before.GPS.Latitude),
		Longitude: before.GPS.Longitude + frac*(after.GPS.Longitude-before.GPS.Longitude),
	}
}

// absDuration returns the absolute value of d.
func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

// MatchPhotos is the core matching engine.
// For every target photo it finds all reference photos and track points
// within opts.MaxTimeDeltaMinutes, scores them, and returns sorted results.
// All timestamps are normalised to UTC before comparison (CLAUDE.md rule #15).
func MatchPhotos(targets []TargetPhoto, refs []ReferencePhoto, tracks []GPSTrackPoint, opts MatchOptions) []MatchResult {
	maxDelta := time.Duration(opts.MaxTimeDeltaMinutes) * time.Minute

	// Pre-sort track points by time so matchFromTracks can binary-search.
	sorted := make([]GPSTrackPoint, len(tracks))
	copy(sorted, tracks)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Time.Before(sorted[j].Time) })

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

		// — GPS track candidates —
		cands = append(cands, matchFromTracks(tgt, sorted, maxDelta)...)

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

// matchFromTracks returns at most one MatchCandidate from the GPS track data.
// Prefers interpolated coordinates when the target time falls between two points.
// Falls back to the nearest single track point when no bracket exists.
func matchFromTracks(targetTime time.Time, tracks []GPSTrackPoint, maxDelta time.Duration) []MatchCandidate {
	if len(tracks) == 0 {
		return nil
	}

	// Binary search for the first track point at or after targetTime.
	pos := sort.Search(len(tracks), func(i int) bool {
		return !tracks[i].Time.Before(targetTime)
	})

	// Case 1: two surrounding points exist — interpolate between them.
	if pos > 0 && pos < len(tracks) {
		before := tracks[pos-1]
		after := tracks[pos]
		dBefore := targetTime.Sub(before.Time) // always ≥ 0
		dAfter := after.Time.Sub(targetTime)   // always ≥ 0
		if dBefore <= maxDelta && dAfter <= maxDelta {
			// Score on the closer bracket; rewards dense tracks.
			minDelta := dBefore
			if dAfter < minDelta {
				minDelta = dAfter
			}
			score := computeScore(minDelta)
			if score > 0 {
				return []MatchCandidate{{
					Source:             "track",
					SourcePath:         before.SourceFile,
					SourceFilename:     filepath.Base(before.SourceFile),
					GPS:                interpolateGPS(before, after, targetTime),
					TimeDelta:          minDelta,
					TimeDeltaFormatted: formatDelta(minDelta),
					Score:              score,
					IsInterpolated:     true,
				}}
			}
		}
	}

	// Case 2: no bracket — use the nearest single point within maxDelta.
	bestIdx, bestDelta := -1, maxDelta+1
	for _, i := range []int{pos - 1, pos} {
		if i < 0 || i >= len(tracks) {
			continue
		}
		d := absDuration(targetTime.Sub(tracks[i].Time))
		if d < bestDelta {
			bestDelta, bestIdx = d, i
		}
	}
	if bestIdx < 0 {
		return nil
	}
	score := computeScore(bestDelta)
	if score == 0 {
		return nil
	}
	pt := tracks[bestIdx]
	return []MatchCandidate{{
		Source:             "track",
		SourcePath:         pt.SourceFile,
		SourceFilename:     filepath.Base(pt.SourceFile),
		GPS:                pt.GPS,
		TimeDelta:          bestDelta,
		TimeDeltaFormatted: formatDelta(bestDelta),
		Score:              score,
		IsInterpolated:     false,
	}}
}
