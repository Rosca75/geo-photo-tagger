package main

// matcher.go — Time-based GPS matching engine
// Matches target photos (no GPS) against reference photos and GPS track points
// by comparing EXIF DateTimeOriginal timestamps.
//
// The scoring formula (see CLAUDE.md §6):
//   timeDelta <= 1 min  → score 100 (excellent)
//   timeDelta <= 5 min  → score 90  (very good)
//   timeDelta <= 10 min → score 75  (good)
//   timeDelta <= 30 min → score 50  (fair)
//   timeDelta <= 60 min → score 25  (poor)
//   timeDelta >  60 min → score 0   (no match)
//
// For GPS track data, if two track points bracket the target timestamp,
// coordinates are linearly interpolated between them.
//
// IMPORTANT: All timestamps must be normalized to UTC before comparison.
//
// Key functions:
//   - RunMatching(targets, refs, tracks, opts) → []MatchResult
//   - computeScore(timeDelta) → int
//   - interpolateGPS(before, after, targetTime) → GPSCoord
//
// TODO: Implement in Phase 4
