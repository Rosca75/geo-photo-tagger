# Change Request 2026-04 — GeoPhotoTagger

> **Audience:** Claude Opus 4.7 working on `Rosca75/geo-photo-tagger` on Windows 11.
> **Author of this plan:** Claude Opus 4.7 (different session), after reading the full codebase + `docs/DNG-GPS-WRITE-PLAN.md`.
>
> **Before touching any file, read `CLAUDE.md` in full.** Every rule in it applies. In particular: rules #1 (read before writing), #3 (no file >150 lines), #8 (state lives in `state.js`), #9 (Wails calls live in `api.js`), #11 (comment all Go code), #13 (no CGo / no external binaries).
>
> **Prescriptive vs. judgment.** This plan is prescriptive for code-heavy bits (exact Go function shapes, HTML snippets, CSS). For architectural choices — how to split files when they approach 150 lines, commit ordering inside a phase, edge-case handling — exercise judgment. If in doubt, match the existing codebase's style (small files, one concern per file, heavy comments).
>
> **Work through the phases in order.** Each phase compiles and runs independently. Do not start a phase before the previous one passes its verification.

---

## 0. Summary of all 8 change requests

| # | Area | Size | Phase |
|---|---|---|---|
| 1 | Quiet the scan logging — summary only, no per-file spam | S | 1 |
| 2 | Add reverse-geocoded city/region/country + mini world map to Zone C, with on/off toggle | L | 5 |
| 3 | DNG GPS-apply performance: lazy-backup, fast verify, benchmark harness | L | 3 |
| 4 | Bug: "Loading location…" never resolves (especially after DNG apply) | M | 2 |
| 5 | Include-subfolders toggle for Source button and GPS Ref button | M | 4 |
| 6 | Replace max-delta preset buttons with a range slider (5 min – 6 hr) | S | 4 |
| 7 | Per-photo selection + rename "Apply All Accepted" → "Apply GPS data" | M | 6 |
| 8 | Module 3: same-source matching (source folder IS the reference folder) | L | 7 |

---

## 1. Global rules for this change request

1. **No new Go dependencies.** Everything uses stdlib or already-approved packages.
2. **One frontend dep is added (Leaflet) in phase 5 only.** CDN import consistent with the existing Feather / Google Fonts pattern. No npm install.
3. **Every new Go function gets a doc comment.** The owner is not a Go expert (CLAUDE.md rule #11).
4. **Keep files under 150 lines.** Split proactively as you approach the limit.
5. **`api.js` is the only place `window.go.main.App.*` is called.** If you add a new Go bound method, add a matching wrapper in `api.js` in the same commit.
6. **Preserve existing behavior for users who don't touch the new controls.** Default recursive=true, default delta=30, default map=off.
7. **Don't refactor opportunistically.** Out-of-scope improvements go in a follow-up, not mixed into these commits.

---

## Phase 1 — Quiet the logs (req #1)

### Current state

- `app.go` `startup()` calls `setupLogger(true)` unconditionally → slog at Debug in every build.
- `exif_reader.go` `ReadEXIFForScan` emits one `slog.Debug("exif_read", …)` per photo. 500 photos = 500 log lines.
- `scanner_parallel.go` already emits a clean Info summary (`walk_complete`, `scan_complete`). Keep these.
- `scanner.go` (sequential) has its own summary — keep.

### What "good" looks like

One Info line per scan of the Source folder:

```
time=… level=INFO msg=scan_complete folder=... total_files_walked=… candidates_checked=… targets_found=… skipped_has_gps=… skipped_exif_error=… walk_duration_ms=… exif_duration_ms=… total_duration_ms=…
```

One Info line per Reference folder scan (`scan_reference_complete`).

Warn lines for broken files stay — those should be visible.

### Changes

**File: `app.go`** (line ~63, inside `startup`)

Replace:

```go
setupLogger(true)
```

with:

```go
// Debug-level logging is opt-in via the GPT_DEBUG_LOG env var so typical
// wails dev sessions are not flooded with one log line per photo.
// Set GPT_DEBUG_LOG=1 before launching wails dev to restore per-file timing.
setupLogger(os.Getenv("GPT_DEBUG_LOG") == "1")
```

Add `"os"` to the imports block if not present.

**File: `exif_reader.go`** (inside `ReadEXIFForScan`, line ~218)

Delete the per-file Debug block:

```go
// DELETE:
slog.Debug("exif_read",
    "path", path,
    "has_gps", result.HasGPS,
    "has_datetime", result.HasDateTime,
    "duration_us", time.Since(start).Microseconds(),
)
```

Also delete `start := time.Now()` at the top of the function (line ~152) if no longer referenced. Run `go vet` — if `"time"` becomes unused in this file, remove the import too.

**Nothing else changes.** The Info-level summaries in `scanner_parallel.go` already report everything req #1 asks for (`candidates_checked`, `targets_found`, `skipped_has_gps`, `total_duration_ms`). Do not add new fields.

### Verification

1. `wails dev`.
2. Click Source on a folder with 50+ photos. Terminal shows exactly one `scan_complete` line plus any Warn lines for broken files. No `exif_read` lines.
3. Same for Ref: one `scan_reference_complete` line.
4. Set `GPT_DEBUG_LOG=1` before `wails dev` → per-file Debug lines return. Sanity check only.

---

## Phase 2 — Fix the "Loading location…" bug (req #4)

### Root cause (two separate bugs, both fixed here)

**Bug A — race on candidate click.** `handleCandidateSelect` in `matcher_ui.js`:
1. Fires `reverseGeocode(lat, lon).then(...)` targeting `#gps-location-info`.
2. Calls `refreshAfterDecision` → `showPhotoDetail` → re-renders the entire Zone C panel via `buildDetailHTML`, creating a brand-new `#gps-location-info`.
3. Promise resolves later and writes into the new element — but if the user clicks a different candidate first, the second re-render orphans the first geocode's write target. Result: flicker, stuck "Loading…", or wrong city for the selected candidate.

**Bug B — apply doesn't re-fire geocoding.** `handleApplySingle` in `actions.js` calls `reRenderDetail(path)` → dispatches `photo-selected` → `showPhotoDetail` → `buildDetailHTML`. Because `acceptedMatches` still has the entry, `buildGPSPreviewWrapper` re-renders `#gps-location-info` with "Loading location…", but `handleCandidateSelect` is never called again. No geocode fires. "Loading location…" stays forever. Visible on both JPG and DNG; user noticed it on DNG because DNG apply is slower.

### Fix strategy

1. Move geocoding into a single idempotent function that any render path can call.
2. Cache results by rounded coordinates so repeat clicks are instant and Nominatim's 1 req/sec limit is respected.
3. Always call it after rendering the preview.

### Changes

**File: `static/js/state.js`** — add:

```javascript
// Cache: "lat5,lon5" → "City, Region, Country" (rounded to 5 decimals ≈ 1.1 m).
// Prevents repeat Nominatim calls when the user clicks back and forth
// between candidates with the same coordinates. Nominatim's usage policy
// is 1 request/second; the cache is what makes that survivable.
geocodeCache: new Map(),

// Whether the mini world map is visible in Zone C (populated in phase 5).
// Default false: no Leaflet CDN download until the user opts in.
mapEnabled: false,
```

Do not persist these — they reset on app restart, which is correct.

**File: `static/js/geocode.js`** — NEW, under 80 lines.

```javascript
// geocode.js — Reverse-geocoding with in-memory caching.
// Wraps api.reverseGeocode() with a Map cache keyed on rounded coordinates.
// Nominatim's usage policy is 1 request/second; the cache is what makes
// that survivable when the user clicks between candidates quickly.

import { state } from './state.js';
import { reverseGeocode as apiReverseGeocode } from './api.js';

// cacheKey rounds coordinates to 5 decimals (~1.1 m precision) so that near-
// duplicate photos share a cache entry. The key format is deterministic and
// canonical — do not change it without clearing existing caches.
function cacheKey(lat, lon) {
    return `${lat.toFixed(5)},${lon.toFixed(5)}`;
}

// getLocationForCoords returns a location string for (lat, lon), using the
// cache if possible. Returns "" on any failure — callers should fall back
// to showing raw coordinates only.
export async function getLocationForCoords(lat, lon) {
    const key = cacheKey(lat, lon);
    if (state.geocodeCache.has(key)) {
        return state.geocodeCache.get(key);
    }
    try {
        const loc = await apiReverseGeocode(lat, lon);
        // Cache empty strings too — no point retrying a Nominatim miss.
        state.geocodeCache.set(key, loc || '');
        return loc || '';
    } catch {
        return '';
    }
}

// refreshLocationFor updates the #gps-location-info element inside
// containerEl for the given accepted match. Safe to call repeatedly — the
// cache absorbs duplicate calls. Writes "Loading location…" first so the
// user sees something change before the (possibly cached) resolution.
//
// Re-queries the container after the await so that if the panel re-rendered
// while we were waiting, we write into whichever element is currently in
// the DOM — this is what kills the race bug in matcher_ui.js.
export async function refreshLocationFor(acc, containerEl) {
    if (!acc || !containerEl) return;
    const el = containerEl.querySelector('#gps-location-info');
    if (!el) return;
    el.textContent = 'Loading location\u2026';
    const loc = await getLocationForCoords(acc.lat, acc.lon);
    const stillThere = containerEl.querySelector('#gps-location-info');
    if (stillThere) {
        stillThere.textContent = loc || 'Location not available';
    }
}
```

**File: `static/js/matcher_ui.js`**

1. At the top, replace `import { runMatching, runMatchingSingle, reverseGeocode } from './api.js';` with `import { runMatching, runMatchingSingle } from './api.js';` and add `import { refreshLocationFor } from './geocode.js';`.

2. In `handleCandidateSelect`, delete the `reverseGeocode(...).then(...)` block entirely:

```javascript
// DELETE:
if (!same) {
    reverseGeocode(lat, lon).then(location => {
        const locEl = document.getElementById('gps-location-info');
        if (locEl) locEl.textContent = location || 'Location not available';
    }).catch(() => { /* silent — raw coords still visible */ });
}
```

The re-render path (step 3 below) now handles this.

3. In `showPhotoDetail`, after `renderPreview(photo, panel);`, add:

```javascript
// Kick off (cached) geocoding for the currently-accepted match, if any.
// This single call replaces all the old scattered reverseGeocode sites
// and fixes the race where re-renders orphaned previous .then() writes.
const acc = state.acceptedMatches.get(photo.path);
if (acc) {
    refreshLocationFor(acc, panel);
}
```

**File: `static/js/actions.js`** — no code change. `handleApplySingle` and `handleUndo` already call `reRenderDetail(path)` which dispatches `photo-selected`, and that path now fires `refreshLocationFor` automatically. Verify this works during testing.

### Verification

1. Click a candidate → "Loading location…" appears briefly → city name resolves within ~1.5 s.
2. Click a different candidate → either fast (new coords) or instant (cache hit).
3. Click the same candidate twice → second click is instant (cache hit, no Nominatim request in DevTools Network tab).
4. Apply GPS on a DNG → Undo button appears → location text stays as the city name, never reverts to "Loading location…".
5. Apply GPS on a JPG → same behavior.
6. DevTools Network tab: exactly one Nominatim request per unique rounded-coord pair across the entire session.

---

## Phase 3 — DNG write performance (req #3)

### Approach

Three parallel wins, land one commit each:

1. **Phase 3a — benchmark harness.** Measurable baseline so the owner can validate each subsequent change on their machine.
2. **Phase 3b — fast verify via direct binary readback.** Replaces the goexif full-decoder verify with a ~130-byte binary round-trip. File-size-independent.
3. **Phase 3c — lazy backup via sidecar metadata.** Replaces the 44 MB `.bak` copy with a ~300-byte JSON sidecar containing everything needed to reverse the patch. Includes a hash-based fingerprint so undo refuses to restore if the file was modified externally between apply and undo.

### Phase 3a — Benchmark harness

The owner needs measurable numbers on their Windows SSD to validate 3b and 3c.

**Prerequisite:** copy `IMGP8411.DNG` (no GPS) and `IMGP8108.DNG` (with GPS) into `samples/` before running benchmarks.

**File: `dng_gps_writer_test.go`** — NEW, ~130 lines.

```go
package main

// dng_gps_writer_test.go — Benchmarks for the DNG GPS apply pipeline.
//
// Requires two files in samples/:
//   samples/IMGP8411.DNG — Pentax K-1 DNG without GPS (target)
//   samples/IMGP8108.DNG — Pentax K-1 DNG with GPS (reference; not written)
//
// Run:
//   go test -bench BenchmarkApplyGPS -benchmem -benchtime 10x ./...
//
// The -benchtime 10x cap is intentional — each iteration copies a 44 MB DNG
// to a temp file, so 10 iterations is plenty to get stable numbers without
// burning through SSD write cycles.

import (
    "os"
    "testing"
)

const sampleDNG = "samples/IMGP8411.DNG"

// cloneSample copies the read-only sample DNG to a fresh writable temp
// path. Returns the temp path; caller must delete both <path> and its
// <path>.bak (or <path>.bak.json after phase 3c).
func cloneSample(tb testing.TB) string {
    tb.Helper()
    tmp, err := os.CreateTemp("", "geophototagger-bench-*.DNG")
    if err != nil {
        tb.Fatalf("create temp: %v", err)
    }
    tmp.Close()
    if err := copyFile(sampleDNG, tmp.Name()); err != nil {
        tb.Fatalf("clone sample: %v", err)
    }
    return tmp.Name()
}

func cleanupApply(path string) {
    os.Remove(path)
    os.Remove(path + ".bak")
    os.Remove(path + ".bak.json")
}

// BenchmarkApplyGPS_FullPipeline measures the entire WriteGPS call: stat,
// backup, DNG binary patch, verify. This is the baseline the user feels.
func BenchmarkApplyGPS_FullPipeline(b *testing.B) {
    if _, err := os.Stat(sampleDNG); err != nil {
        b.Skipf("%s not found: %v", sampleDNG, err)
    }
    for i := 0; i < b.N; i++ {
        path := cloneSample(b)
        b.StartTimer()
        if err := WriteGPS(path, 49.6116, 6.1319); err != nil {
            b.Fatalf("WriteGPS: %v", err)
        }
        b.StopTimer()
        cleanupApply(path)
    }
}

// BenchmarkApplyGPS_BackupOnly isolates the cost of the backup step.
// After phase 3c this should drop by ~3 orders of magnitude.
func BenchmarkApplyGPS_BackupOnly(b *testing.B) {
    for i := 0; i < b.N; i++ {
        path := cloneSample(b)
        b.StartTimer()
        _ = copyFile(path, path+".bak")
        b.StopTimer()
        cleanupApply(path)
    }
}

// BenchmarkApplyGPS_PatchOnly isolates the pure binary patch.
// Expected to be the smallest component, dominated by disk sync.
func BenchmarkApplyGPS_PatchOnly(b *testing.B) {
    for i := 0; i < b.N; i++ {
        path := cloneSample(b)
        b.StartTimer()
        _ = writeGPSToDNG(path, 49.6116, 6.1319)
        b.StopTimer()
        cleanupApply(path)
    }
}

// BenchmarkApplyGPS_VerifyFullReader measures the current (phase-0) verify
// path: full goexif decoder with maker notes, no LimitReader.
func BenchmarkApplyGPS_VerifyFullReader(b *testing.B) {
    path := cloneSample(b)
    defer cleanupApply(path)
    _ = writeGPSToDNG(path, 49.6116, 6.1319)
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = ReadEXIF(path)
    }
}

// BenchmarkApplyGPS_VerifyFastPath measures the phase-3b direct-binary
// verification (~130 bytes of I/O, no goexif). This benchmark is only
// meaningful after phase 3b lands.
func BenchmarkApplyGPS_VerifyFastPath(b *testing.B) {
    path := cloneSample(b)
    defer cleanupApply(path)
    _ = writeGPSToDNG(path, 49.6116, 6.1319)
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _ = verifyGPSInDNG(path, 49.6116, 6.1319)
    }
}
```

**How the owner runs it** (from Windows cmd, outside wails dev — local-only, no proxy involvement):

```
cd C:\path\to\geo-photo-tagger
go test -bench BenchmarkApplyGPS -benchmem -benchtime 10x ./...
```

Capture the numbers after each of 3a/3b/3c and drop them into the commit message.

### Phase 3b — Fast verify via direct binary readback

**Approach:** read back the 4-byte GPS pointer, follow it, and parse out the RATIONAL values directly using `encoding/binary`. Bounded to ~130 bytes of I/O regardless of file size — `goexif`'s 512 KB `LimitReader` would be unable to reach the GPS IFD that `writeGPSToDNG` appends at EOF in multi-MB DNGs.

**File: `dng_gps_verify.go`** — NEW (keep `dng_gps_writer.go` under 150 lines by splitting). ~110 lines.

```go
package main

// dng_gps_verify.go — Direct binary verification of a newly-written DNG GPS IFD.
//
// After writeGPSToDNG patches the file, we need to confirm lat/lon round-trip
// correctly before deleting (or in phase 3c, not writing) the backup. The naive
// approach — re-decode the full EXIF via goexif — is slow on DNGs (20-80 ms for
// Pentax files with maker notes).
//
// Attempting to use ReadEXIFForScan is not viable either: its 512 KB LimitReader
// cannot reach our appended GPS IFD which lives at the end of a multi-MB DNG.
//
// This file implements the minimal-I/O alternative: follow the patched pointer,
// walk the 5 GPS IFD entries, reconstruct decimal degrees, compare with
// tolerance. No goexif involved.

import (
    "encoding/binary"
    "fmt"
    "io"
    "math"
    "os"
)

// verifyGPSInDNG re-reads ONLY the appended GPS IFD and confirms the lat/lon
// values match expectedLat/expectedLon within 0.001° tolerance. Bounded to
// ~130 bytes of I/O regardless of file size.
//
// Returns nil on match, an error describing what went wrong otherwise.
// Called from writeAndVerify in exif_writer.go as a DNG-specific fast path.
func verifyGPSInDNG(path string, expectedLat, expectedLon float64) error {
    f, err := os.Open(path)
    if err != nil {
        return fmt.Errorf("verify open: %w", err)
    }
    defer f.Close()

    byteOrder, ifd0Offset, err := readTIFFHeader(f)
    if err != nil {
        return fmt.Errorf("verify header: %w", err)
    }
    gpsPointerOffset, err := findGPSInfoIFDPointer(f, byteOrder, ifd0Offset)
    if err != nil {
        return fmt.Errorf("verify locate pointer: %w", err)
    }

    // Read the current GPS IFD offset from the patched pointer.
    if _, err := f.Seek(gpsPointerOffset, io.SeekStart); err != nil {
        return fmt.Errorf("verify seek pointer: %w", err)
    }
    var gpsIFDOffset uint32
    if err := binary.Read(f, byteOrder, &gpsIFDOffset); err != nil {
        return fmt.Errorf("verify read pointer: %w", err)
    }

    lat, lon, err := readGPSCoordsFromIFD(f, byteOrder, int64(gpsIFDOffset))
    if err != nil {
        return fmt.Errorf("verify read coords: %w", err)
    }

    if math.Abs(lat-expectedLat) > 0.001 || math.Abs(lon-expectedLon) > 0.001 {
        return fmt.Errorf("verify mismatch: got (%.6f, %.6f), want (%.6f, %.6f)",
            lat, lon, expectedLat, expectedLon)
    }
    return nil
}

// readGPSCoordsFromIFD walks a GPS IFD at `gpsIFDOffset` and returns decimal
// lat/lon. Expects at minimum the 4 tags we wrote: 0x0001 LatRef, 0x0002 Lat,
// 0x0003 LonRef, 0x0004 Lon.
func readGPSCoordsFromIFD(f *os.File, byteOrder binary.ByteOrder, gpsIFDOffset int64) (lat, lon float64, err error) {
    if _, err = f.Seek(gpsIFDOffset, io.SeekStart); err != nil {
        return 0, 0, fmt.Errorf("seek GPS IFD: %w", err)
    }
    var entryCount uint16
    if err = binary.Read(f, byteOrder, &entryCount); err != nil {
        return 0, 0, fmt.Errorf("read entry count: %w", err)
    }

    var (
        latRefChar, lonRefChar                   byte
        latDeg, latMin, latSec                   float64
        lonDeg, lonMin, lonSec                   float64
        haveLat, haveLon, haveLatRef, haveLonRef bool
    )

    for i := uint16(0); i < entryCount; i++ {
        var tag, typeID uint16
        var count, value uint32
        if err = binary.Read(f, byteOrder, &tag); err != nil { return 0, 0, err }
        if err = binary.Read(f, byteOrder, &typeID); err != nil { return 0, 0, err }
        if err = binary.Read(f, byteOrder, &count); err != nil { return 0, 0, err }
        if err = binary.Read(f, byteOrder, &value); err != nil { return 0, 0, err }

        // Save position — RATIONAL reads seek elsewhere and come back.
        entryEndPos, _ := f.Seek(0, io.SeekCurrent)

        switch tag {
        case 0x0001: // GPSLatitudeRef: ASCII inline, first byte
            latRefChar = byte(value & 0xFF)
            haveLatRef = true
        case 0x0002: // GPSLatitude: 3×RATIONAL at offset `value`
            latDeg, latMin, latSec, err = readThreeRationals(f, byteOrder, int64(value))
            if err != nil { return 0, 0, fmt.Errorf("lat rationals: %w", err) }
            haveLat = true
        case 0x0003: // GPSLongitudeRef
            lonRefChar = byte(value & 0xFF)
            haveLonRef = true
        case 0x0004: // GPSLongitude
            lonDeg, lonMin, lonSec, err = readThreeRationals(f, byteOrder, int64(value))
            if err != nil { return 0, 0, fmt.Errorf("lon rationals: %w", err) }
            haveLon = true
        }

        if _, err = f.Seek(entryEndPos, io.SeekStart); err != nil {
            return 0, 0, fmt.Errorf("seek back after entry %d: %w", i, err)
        }
    }

    if !(haveLat && haveLon && haveLatRef && haveLonRef) {
        return 0, 0, fmt.Errorf("missing tags: lat=%v lon=%v latRef=%v lonRef=%v",
            haveLat, haveLon, haveLatRef, haveLonRef)
    }

    lat = latDeg + latMin/60 + latSec/3600
    if latRefChar == 'S' { lat = -lat }
    lon = lonDeg + lonMin/60 + lonSec/3600
    if lonRefChar == 'W' { lon = -lon }
    return lat, lon, nil
}

// readThreeRationals reads 3 RATIONAL values (6 × uint32 = 24 bytes) starting
// at offset and returns the three floats. Seeks to offset; caller restores.
func readThreeRationals(f *os.File, byteOrder binary.ByteOrder, offset int64) (a, b, c float64, err error) {
    if _, err = f.Seek(offset, io.SeekStart); err != nil { return }
    var r [6]uint32
    for i := range r {
        if err = binary.Read(f, byteOrder, &r[i]); err != nil { return }
    }
    ratio := func(num, den uint32) float64 {
        if den == 0 { return 0 }
        return float64(num) / float64(den)
    }
    return ratio(r[0], r[1]), ratio(r[2], r[3]), ratio(r[4], r[5]), nil
}
```

**File: `exif_writer.go`** — route DNG through the fast verifier. Replace `writeAndVerify`:

```go
// writeAndVerify runs a format-specific GPS writer, then verifies the result.
// For DNG it uses the direct-binary fast path in verifyGPSInDNG (~130 bytes
// of I/O). For JPEG it keeps the goexif round-trip which has always worked.
// On any failure the backup is restored.
func writeAndVerify(
    targetPath, backupPath string,
    lat, lon float64,
    label string,
    writer func(string, float64, float64) error,
) error {
    if err := writer(targetPath, lat, lon); err != nil {
        _ = restoreFromBackup(targetPath, backupPath)
        return fmt.Errorf("%s GPS write failed: %w", label, err)
    }

    var verifyErr error
    if label == "DNG" {
        verifyErr = verifyGPSInDNG(targetPath, lat, lon)
    } else {
        result, err := ReadEXIF(targetPath)
        if err != nil || !result.HasGPS ||
            math.Abs(result.Latitude-lat) > 0.001 ||
            math.Abs(result.Longitude-lon) > 0.001 {
            verifyErr = fmt.Errorf("EXIF verification failed")
        }
    }
    if verifyErr != nil {
        _ = restoreFromBackup(targetPath, backupPath)
        return fmt.Errorf("%s GPS verification failed for %s: %w",
            label, filepath.Base(targetPath), verifyErr)
    }
    return nil
}
```

Note the new `restoreFromBackup` helper — this is introduced in phase 3c below. For phase 3b alone, leave the existing `copyFile(backupPath, targetPath)` calls in place.

### Phase 3c — Lazy backup via sidecar metadata

**Goal:** replace the 44 MB `copyFile` backup with a ~300-byte JSON sidecar. Undo reverses the patch by truncating the file back to its original size and writing the original 4-byte pointer value. A hash fingerprint of the pre-apply state guarantees undo refuses to run if the file was modified externally between apply and undo.

**Fingerprint design.** Hash the first 64 KB of the file (covers TIFF header, full IFD0, and SubIFD pointers — the regions we care about). SHA-256, stored hex in the sidecar. 64 KB is plenty to detect tampering while keeping the hash fast (<1 ms on any SSD).

**File: `dng_backup.go`** — NEW, ~180 lines (may need splitting; if it crosses 150 lines, split the sidecar types + capture into `dng_backup.go` and put undo/tamper into `dng_backup_undo.go`).

```go
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
// Sidecar format: JSON at <targetPath>.bak.json. ClearAllBackups and the
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
```

**File: `exif_writer.go`** — rework `WriteGPS`, `UndoGPS`, `ClearBackups`, and add `restoreFromBackup`:

```go
// WriteGPS injects GPS coordinates into a photo's EXIF data.
//
// DNG backup strategy: a lazy sidecar (dng_backup.go) stores only the 4-byte
// pointer value and file size — ~300 bytes instead of 44 MB. Undo truncates
// the file and restores the pointer. A pre-apply hash guards against silent
// corruption if another tool edited the file between apply and undo.
//
// JPEG backup strategy: unchanged — full file copy to <path>.bak.
func WriteGPS(targetPath string, lat, lon float64) error {
    fi, err := os.Stat(targetPath)
    if err != nil {
        return fmt.Errorf("stat %q: %w", targetPath, err)
    }
    if fi.Mode().Perm()&0200 == 0 {
        return fmt.Errorf("file %q is read-only — cannot write GPS data", targetPath)
    }

    ext := strings.ToLower(filepath.Ext(targetPath))
    switch ext {
    case ".dng":
        // Capture pre-apply state to the sidecar BEFORE any file modification.
        if err := captureDNGBackup(targetPath); err != nil {
            return fmt.Errorf("DNG backup capture failed: %w", err)
        }
        return writeAndVerify(targetPath, sidecarPath(targetPath), lat, lon, "DNG", writeGPSToDNG)

    case ".jpg", ".jpeg":
        backupPath := targetPath + ".bak"
        if err := copyFile(targetPath, backupPath); err != nil {
            return fmt.Errorf("backup failed: %w", err)
        }
        return writeAndVerify(targetPath, backupPath, lat, lon, "JPEG", writeGPSToJPEG)

    default:
        return fmt.Errorf("unsupported format for GPS write: %s", ext)
    }
}

// restoreFromBackup is used by writeAndVerify's failure path. It handles
// both the legacy JPEG .bak (file copy) and the new DNG .bak.json (sidecar).
func restoreFromBackup(targetPath, backupPath string) error {
    // DNG sidecar case
    if strings.HasSuffix(backupPath, ".bak.json") {
        sc, err := loadDNGBackup(targetPath)
        if err != nil {
            return fmt.Errorf("load sidecar for restore: %w", err)
        }
        return undoDNGFromSidecar(targetPath, sc)
    }
    // JPEG full-copy case
    return copyFile(backupPath, targetPath)
}

// UndoGPS restores targetPath from its backup (DNG sidecar or JPEG .bak).
// For DNG, a pre-apply hash fingerprint in the sidecar is checked BEFORE
// the undo runs — if the hash doesn't match, undo refuses and returns a
// clear error. This catches the case where another tool (Lightroom, Bridge)
// edited the DNG between apply and undo.
func UndoGPS(targetPath string) error {
    ext := strings.ToLower(filepath.Ext(targetPath))
    if ext == ".dng" {
        sc, err := loadDNGBackup(targetPath)
        if err != nil {
            if os.IsNotExist(err) {
                return fmt.Errorf("no backup sidecar found for %s", filepath.Base(targetPath))
            }
            return err
        }
        if err := checkDNGTamper(targetPath, sc); err != nil {
            return err
        }
        return undoDNGFromSidecar(targetPath, sc)
    }

    // JPEG path (unchanged)
    backupPath := targetPath + ".bak"
    if _, err := os.Stat(backupPath); os.IsNotExist(err) {
        return fmt.Errorf("no backup found for %s", filepath.Base(targetPath))
    }
    return copyFile(backupPath, targetPath)
}

// ClearBackups recursively deletes .bak files AND .bak.json sidecars under
// folderPath. Returns the total count of deleted files.
func ClearBackups(folderPath string) (int, error) {
    count := 0
    walkErr := filepath.WalkDir(folderPath, func(path string, d os.DirEntry, err error) error {
        if err != nil || d.IsDir() {
            return nil
        }
        if (strings.HasSuffix(path, ".bak") || strings.HasSuffix(path, ".bak.json")) &&
            os.Remove(path) == nil {
            count++
        }
        return nil
    })
    return count, walkErr
}
```

**File: `app.go`** — wire the orphan sweep into `ScanTargetFolder`, right before `a.targetFolder = path`:

```go
// Clean orphaned backup sidecars from the previously-scanned folder (if any)
// and from the folder we're about to scan. Keeps pending undos intact; only
// removes sidecars whose DNG was deleted.
if a.targetFolder != "" && a.targetFolder != path {
    if n := SweepOrphanedSidecars(a.targetFolder); n > 0 {
        slog.Info("orphan_sidecars_cleared", "folder", a.targetFolder, "count", n)
    }
}
if n := SweepOrphanedSidecars(path); n > 0 {
    slog.Info("orphan_sidecars_cleared", "folder", path, "count", n)
}
```

### Warning in the confirmation dialog (safety UX)

Add a "close other apps that may have the DNG open" warning inside the apply confirmation dialog — and extend that confirmation to single applies too (they don't have one today).

**File: `static/js/actions.js`**

1. Add a shared helper above `handleApplyAll`:

```javascript
// buildApplyWarning returns the standard warning text shown in apply
// confirmation dialogs. Reminds the user to close tools like Lightroom or
// Bridge that may hold the DNG file open — on Windows an open file handle
// will cause the GPS write to fail mid-flight, and the lazy-backup undo
// depends on a clean pre-apply state.
function buildApplyWarning(photoCount) {
    const plural = photoCount !== 1 ? 's' : '';
    return `Apply GPS data to ${photoCount} photo${plural}?

Please close any app (Lightroom, Photoshop, Bridge, Explorer preview)
that may have these photos open — they must not be locked by another
process, or the write will fail.`;
}
```

2. Update `handleApplyAll`:

```javascript
const ok = await showConfirm(buildApplyWarning(count));
```

3. Add a confirm to `handleApplySingle`:

```javascript
async function handleApplySingle(btn) {
    const { path, lat, lon } = btn.dataset;
    const ok = await showConfirm(buildApplyWarning(1));
    if (!ok) return;
    btn.disabled = true;
    btn.textContent = 'Applying\u2026';
    // ... rest unchanged
}
```

Note: `showConfirm` currently renders its message with `innerHTML`. Our warning has newlines — either split on `\n` and wrap each line in `<p>`, or change the rendering path to use `textContent` on a `<p>` element. Prefer the latter (simpler + XSS-safe). Comment the change when you make it.

### Phase 3 verification

1. `go test -bench BenchmarkApplyGPS -benchmem -benchtime 10x ./...` — capture numbers after each of 3a, 3b, 3c. Paste into commit messages.
2. `wails dev`, apply GPS to a DNG → ExifTool or Windows Explorer → Details tab confirms GPS tag.
3. Apply → Undo → byte-for-byte equal to pre-apply (compare with `fc /b` on Windows or `cmp` on Linux CI).
4. Tamper test: apply GPS, manually edit the first 1 KB of the DNG in a hex editor, then click Undo → expect a clear error message about external modification. File should remain as user left it.
5. Orphan sweep: apply GPS to a DNG, delete the DNG manually, re-scan the folder → check that the `.bak.json` is gone and the log line `orphan_sidecars_cleared` fired.
6. Confirm dialog: Apply GPS data → warning text visible, mentions Lightroom/Bridge/Explorer. Single apply also shows the dialog.
7. `ClearBackups` removes both `.bak` and `.bak.json`.

---

## Phase 4 — Subfolder toggle + max-delta slider (reqs #5 and #6)

### Subfolder toggles (req #5)

Today `filepath.WalkDir` always recurses. Thread a `recursive bool` through.

**File: `scanner_parallel.go`** — change signature:

```go
func (a *App) ScanForTargetPhotosParallel(folderPath string, numWorkers int, recursive bool) ([]TargetPhoto, error) {
```

In the WalkDir callback, at the top of the `d.IsDir()` branch:

```go
if d.IsDir() {
    // When non-recursive, skip every directory except the root itself.
    if !recursive && path != folderPath {
        return fs.SkipDir
    }
    return nil
}
```

**File: `scanner.go`** — same for `ScanForReferencePhotos`:

```go
func ScanForReferencePhotos(folderPath string, dateFilter DateRange, recursive bool) ([]ReferencePhoto, error) {
```

Same `fs.SkipDir` pattern in its walk callback.

**File: `app.go`** — update `ScanTargetFolder`:

```go
func (a *App) ScanTargetFolder(path string, recursive bool) ([]TargetPhoto, error) {
    // ...
    a.lastSourceRecursive = recursive  // needed by phase 7 RunSameSourceMatching
    photos, err := a.ScanForTargetPhotosParallel(path, 0, recursive)
    // ...
}
```

Add `lastSourceRecursive bool` to the `App` struct.

**File: `app_reference.go`** — update `AddReferenceFolder`:

```go
func (a *App) AddReferenceFolder(path string, recursive bool) (ReferenceFolderInfo, error) {
    // ...
    photos, err := ScanForReferencePhotos(path, dateFilter, recursive)
    // ...
}
```

**File: `static/js/api.js`** — update wrappers:

```javascript
export async function scanTargetFolder(path, recursive) {
    return window.go.main.App.ScanTargetFolder(path, recursive);
}
export async function addReferenceFolder(path, recursive) {
    return window.go.main.App.AddReferenceFolder(path, recursive);
}
```

**File: `static/index.html`** — add inline checkbox controls in the Data Sources group. After `<button id="btn-reset-source" ...>`:

```html
<label class="subfolder-toggle" title="Include photos in subfolders when scanning">
    <input id="chk-source-recursive" type="checkbox" checked>
    <span>Include subfolders</span>
</label>
```

After `<button id="btn-import-track" ...>`:

```html
<label class="subfolder-toggle" title="Include photos in subfolders when adding a reference folder">
    <input id="chk-ref-recursive" type="checkbox" checked>
    <span>Include subfolders</span>
</label>
```

**File: `static/css/components.css`** — append:

```css
.subfolder-toggle {
    display: inline-flex;
    align-items: center;
    gap: var(--space-xs);
    font-size: 0.75rem;
    color: var(--text-light);
    cursor: pointer;
    user-select: none;
}
.subfolder-toggle input[type="checkbox"] {
    margin: 0;
    cursor: pointer;
}
```

**File: `static/js/state.js`** — add:

```javascript
// Whether Source scans recurse into subfolders. Default true preserves
// pre-phase-4 behavior.
sourceRecursive: true,

// Whether Ref scans recurse into subfolders. Default true.
refRecursive: true,
```

**File: `static/js/scan.js`** — wire the source checkbox. In `initScan`:

```javascript
const chk = document.getElementById('chk-source-recursive');
if (chk) {
    chk.checked = state.sourceRecursive;
    chk.addEventListener('change', () => { state.sourceRecursive = chk.checked; });
}
```

In `runScan`, change:

```javascript
const photos = await scanTargetFolder(path);
```

to:

```javascript
const photos = await scanTargetFolder(path, state.sourceRecursive);
```

**File: `static/js/reference.js`** — symmetric. In `initReference`:

```javascript
const chk = document.getElementById('chk-ref-recursive');
if (chk) {
    chk.checked = state.refRecursive;
    chk.addEventListener('change', () => { state.refRecursive = chk.checked; });
}
```

In `handleAddReferenceClick`, change `await addReferenceFolder(path)` to `await addReferenceFolder(path, state.refRecursive)`.

### Max-delta slider (req #6)

**File: `static/index.html`** — in the Matching group, replace the three `.threshold-btn` buttons and the "Max delta:" label with:

```html
<label class="delta-slider-label" for="delta-slider">Max delta:</label>
<input id="delta-slider" class="delta-slider" type="range"
       min="5" max="360" step="5" value="30"
       title="Maximum time difference between a photo and a GPS source for a match to be considered">
<span id="delta-slider-value" class="delta-slider-value">30 min</span>
```

**File: `static/css/components.css`** — append:

```css
.delta-slider-label { font-size: 0.75rem; color: var(--text-light); }
.delta-slider { width: 160px; accent-color: var(--primary); cursor: pointer; }
.delta-slider-value {
    font-size: 0.8rem;
    font-weight: var(--font-weight-semi);
    color: var(--primary);
    min-width: 58px;
    text-align: left;
}
```

**File: `static/js/matcher_ui.js`** — in `initMatcher`, replace the `document.querySelectorAll('.threshold-btn').forEach(...)` block with:

```javascript
const slider = document.getElementById('delta-slider');
const valueLabel = document.getElementById('delta-slider-value');
if (slider && valueLabel) {
    slider.value = String(state.matchThreshold);
    valueLabel.textContent = formatDeltaMinutes(state.matchThreshold);
    // Live-update the label on drag; commit to state only on release so
    // scan/match stats don't recompute 60 times/sec while the user drags.
    slider.addEventListener('input', () => {
        valueLabel.textContent = formatDeltaMinutes(parseInt(slider.value, 10));
    });
    slider.addEventListener('change', () => {
        state.matchThreshold = parseInt(slider.value, 10);
    });
}
```

Add a helper near the top of the file (above `initMatcher`):

```javascript
// formatDeltaMinutes turns a minute count into the compact label shown next
// to the slider: "5 min", "30 min", "2 h", "1 h 30".
function formatDeltaMinutes(m) {
    if (m < 60) return `${m} min`;
    const h = Math.floor(m / 60);
    const rem = m % 60;
    if (rem === 0) return `${h} h`;
    return `${h} h ${rem}`;
}
```

Remove `setThreshold` — nothing calls it anymore.

### Verification

1. Uncheck "Include subfolders" next to Source → re-scan a nested-folder source → only root-level photos appear.
2. Same for Ref.
3. Slider defaults to 30, label tracks while dragging, state updates on release.
4. Match with delta at 5 min: few matches. At 6 h: lots.

---

## Phase 5 — Mini world map + reverse-geocoded location (req #2)

### Design

- Load Leaflet 1.9.x + OSM tiles from CDN **only on first use**. No download while the toggle is off.
- "Show map" button in Zone C's coordinate strip. Default off.
- 100% wide × 180 px tall, zoom 10, **non-interactive** (no drag, no zoom controls, no scroll-wheel-zoom). It's a "you are here" preview, not a navigator — keeps CPU cost predictable when the user clicks through candidates.
- Reuse `state.geocodeCache` from phase 2.

### Changes

**File: `static/js/map.js`** — NEW, ~120 lines.

```javascript
// map.js — Leaflet mini-map lazy loader + marker renderer for Zone C.
//
// Leaflet CSS+JS is loaded on first use only — zero network cost when the
// map toggle is off. Subsequent renders reuse the already-loaded library.
//
// The map is deliberately non-interactive: no drag, no zoom controls, no
// scroll-wheel zoom, no double-click-to-zoom. This is a "you are here"
// preview, not a navigator — and it keeps CPU cost predictable when the
// user clicks rapidly through candidates.

import { state } from './state.js';

const LEAFLET_CSS = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.css';
const LEAFLET_JS  = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.js';

let leafletLoadPromise = null;

// loadLeaflet lazily injects Leaflet's CSS+JS on first call. Returns a
// promise resolving to the global L object. Subsequent callers wait on
// the same cached promise.
function loadLeaflet() {
    if (typeof window.L !== 'undefined') return Promise.resolve(window.L);
    if (leafletLoadPromise) return leafletLoadPromise;

    leafletLoadPromise = new Promise((resolve, reject) => {
        const link = document.createElement('link');
        link.rel = 'stylesheet';
        link.href = LEAFLET_CSS;
        document.head.appendChild(link);

        const script = document.createElement('script');
        script.src = LEAFLET_JS;
        script.async = true;
        script.onload = () => resolve(window.L);
        script.onerror = () => reject(new Error('Leaflet CDN load failed'));
        document.head.appendChild(script);
    });
    return leafletLoadPromise;
}

// renderMap renders a mini-map inside the #gps-mini-map container for the
// given coordinates. Safe to call repeatedly — previous Leaflet instances
// are torn down before creating a new one (Leaflet does not garbage-collect
// on its own, so explicit .remove() is important).
export async function renderMap(lat, lon, containerEl) {
    if (!containerEl) return;
    try {
        const L = await loadLeaflet();
        const target = containerEl.querySelector('#gps-mini-map');
        if (!target) return;
        if (target._leafletInstance) {
            target._leafletInstance.remove();
            target._leafletInstance = null;
        }
        const map = L.map(target, {
            zoomControl: false,
            dragging: false,
            scrollWheelZoom: false,
            doubleClickZoom: false,
            boxZoom: false,
            keyboard: false,
            attributionControl: true,
        }).setView([lat, lon], 10);

        L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
            maxZoom: 18,
            // OSM tile usage requires attribution — do not remove.
            attribution: '&copy; OpenStreetMap contributors',
        }).addTo(map);

        L.marker([lat, lon]).addTo(map);
        target._leafletInstance = map;
    } catch (err) {
        console.warn('Map render failed:', err);
    }
}

// toggleMap flips state.mapEnabled and re-renders Zone C via the
// photo-selected event. Session-only — resets on app restart.
export function toggleMap() {
    state.mapEnabled = !state.mapEnabled;
    if (state.selectedPhoto) {
        const photo = state.targetPhotos.find(p => p.path === state.selectedPhoto);
        if (photo) {
            document.dispatchEvent(new CustomEvent('photo-selected', { detail: { photo } }));
        }
    }
}
```

**File: `static/js/detail_render.js`** — modify `buildGPSPreview`:

```javascript
export function buildGPSPreview(acc) {
    const mapToggleLabel = state.mapEnabled ? 'Hide map' : 'Show map';
    const mapSlot = state.mapEnabled
        ? '<div id="gps-mini-map" class="gps-mini-map"></div>'
        : '';
    return `
        <div class="gps-preview-card">
            <div class="gps-coords">
                <span class="label-small">Coordinates:</span>
                <span class="gps-value">${acc.lat.toFixed(6)}, ${acc.lon.toFixed(6)}</span>
                <button class="btn btn-sm btn-secondary btn-toggle-map"
                        title="Show or hide the mini map preview">${mapToggleLabel}</button>
            </div>
            <div id="gps-location-info" class="gps-location muted">Loading location\u2026</div>
            ${mapSlot}
        </div>`;
}
```

**File: `static/css/components.css`** — append:

```css
.gps-mini-map {
    width: 100%;
    height: 180px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    margin-top: var(--space-sm);
    overflow: hidden;
}
```

**File: `static/js/matcher_ui.js`** — add imports at top:

```javascript
import { toggleMap, renderMap } from './map.js';
```

Add a route in `handlePanelClick`:

```javascript
function handlePanelClick(e) {
    const mapToggleBtn = e.target.closest('.btn-toggle-map');
    if (mapToggleBtn) { toggleMap(); return; }
    // ... existing routing unchanged
}
```

In `showPhotoDetail`, after the `refreshLocationFor(acc, panel)` line from phase 2, add:

```javascript
if (acc && state.mapEnabled) {
    renderMap(acc.lat, acc.lon, panel);
}
```

### Verification

1. Toggle off by default. DevTools Network tab: no Leaflet request on startup.
2. Click a candidate → location resolves (phase 2 still works). Coords row has a "Show map" button.
3. Click "Show map" → Leaflet loads once (one CSS + one JS request), map appears with marker, button flips to "Hide map".
4. Click a different candidate → map re-renders at new coords smoothly.
5. Click "Hide map" → map disappears; Leaflet is NOT re-downloaded on next toggle (promise cached).
6. Reload the app → map defaults back to off (session-only).

---

## Phase 6 — Per-photo apply selection + rename (req #7)

### Model

One source of truth: `state.acceptedMatches` (already a Map) reflects the user's intent for the next batch apply. When a match is found, every matched photo is auto-added. The checkbox column in Zone B toggles membership. Clicking a non-best candidate in Zone C overwrites the entry for that photo.

### Changes

**File: `static/js/matcher_ui.js`** — in `handleMatchAllClick`, after `state.matchResults = Array.isArray(results) ? results : [];`, add:

```javascript
// Auto-accept the best candidate for every matched photo. The user can
// uncheck individual rows in Zone B to exclude them from batch apply, or
// click a different candidate in Zone C to change the coords.
state.acceptedMatches.clear();
state.matchResults.forEach(r => {
    if (r.bestCandidate) {
        const best = r.bestCandidate;
        state.acceptedMatches.set(r.targetPath, {
            lat: best.gps.latitude,
            lon: best.gps.longitude,
            score: best.score,
            source: best.source,
            sourcePath: best.sourcePath,
        });
    }
});
```

Same treatment in `handleMatchSingle` — after `state.matchResults[idx] = result;` (and the equivalent `.push(result)` branch), auto-accept its best candidate.

**File: `static/js/table.js`** — add a select column as the first column. In `buildHeader`'s `cols` array, prepend:

```javascript
['select', '', false, 'col-select'],
```

In `buildHeader`'s loop, handle the select column specially (it's a checkbox header, not a text label):

```javascript
if (key === 'select') {
    th.className = cls;
    th.innerHTML = '<input type="checkbox" id="cb-select-all" title="Select/deselect all matched photos">';
    tr.appendChild(th);
    // Wire the handler on next tick so the element is in the DOM.
    setTimeout(() => {
        const selAll = document.getElementById('cb-select-all');
        if (selAll) {
            selAll.addEventListener('change', () => toggleAllSelection(selAll.checked));
        }
    }, 0);
    return;
}
```

In `buildRow`, prepend the row checkbox `<td>`:

```javascript
const result = state.matchResults
    ? state.matchResults.find(r => r.targetPath === photo.path)
    : null;
const best = result && result.bestCandidate ? result.bestCandidate : null;

const canSelect = !!best;
const isSelected = state.acceptedMatches.has(photo.path);
const cbHTML = canSelect
    ? `<input type="checkbox" class="row-select-cb" ${isSelected ? 'checked' : ''}
              title="Include this photo in the next Apply GPS data batch">`
    : '';

// inside tr.innerHTML, BEFORE <td class="col-num">:
// <td class="col-select">${cbHTML}</td>
```

After setting `tr.innerHTML`, wire the checkbox:

```javascript
const cb = tr.querySelector('.row-select-cb');
if (cb) {
    // Stop the row-level click handler from firing when the checkbox is clicked.
    cb.addEventListener('click', e => e.stopPropagation());
    cb.addEventListener('change', () => toggleRowSelection(photo, cb.checked));
}
```

Add the helpers at the bottom of `table.js`:

```javascript
// toggleRowSelection updates state.acceptedMatches based on the checkbox
// state for one row. When checking, auto-accept the photo's best candidate
// (which is the meaning of the checkbox: "include this one in batch apply").
function toggleRowSelection(photo, checked) {
    if (checked) {
        const result = state.matchResults &&
            state.matchResults.find(r => r.targetPath === photo.path);
        const best = result && result.bestCandidate;
        if (!best) return;
        state.acceptedMatches.set(photo.path, {
            lat: best.gps.latitude,
            lon: best.gps.longitude,
            score: best.score,
            source: best.source,
            sourcePath: best.sourcePath,
        });
    } else {
        state.acceptedMatches.delete(photo.path);
    }
}

// toggleAllSelection flips every visible row checkbox to `checked` and
// fires the change handler on each so state.acceptedMatches updates.
function toggleAllSelection(checked) {
    document.querySelectorAll('.row-select-cb').forEach(cb => {
        if (cb.checked !== checked) {
            cb.checked = checked;
            cb.dispatchEvent(new Event('change'));
        }
    });
}
```

**File: `static/css/table.css`** — append:

```css
.col-select { width: 32px; text-align: center; }
.col-select input[type="checkbox"] { cursor: pointer; margin: 0; }
```

**File: `static/index.html`** — rename the batch button:

Replace:

```html
<button id="btn-apply-all" class="btn btn-sm btn-primary" style="margin-left:auto"
        title="Write GPS coordinates to all accepted matches">
    Apply All Accepted
</button>
```

with:

```html
<button id="btn-apply-all" class="btn btn-sm btn-primary" style="margin-left:auto"
        title="Apply GPS data of the best candidate found">
    Apply GPS data
</button>
```

**File: `static/js/actions.js`** — no logic change. `handleApplyAll` already iterates `state.acceptedMatches`, and with phase 6 that Map now reflects user selection exactly. The warning dialog from phase 3 already shows for both single and batch apply.

### Interaction with Zone C candidate clicks

When the user clicks a non-best candidate in Zone C, `handleCandidateSelect` overwrites the Map entry for that photo (existing behavior). The row checkbox stays ticked. If the user unticks then re-clicks a candidate in Zone C, the checkbox auto-ticks via `handleCandidateSelect`'s Map.set. Document this in a comment in `handleCandidateSelect` — it's intentional, not a bug.

### Verification

1. Run a match → every matched row's checkbox is ticked. Unmatched rows have no checkbox at all.
2. Header checkbox toggles all (only the matched ones that have row checkboxes).
3. Uncheck a row → click "Apply GPS data" → that row is skipped in the batch.
4. Click a non-best candidate in Zone C for some row → row stays ticked, new coords used on apply.
5. Tooltip on "Apply GPS data" reads "Apply GPS data of the best candidate found".

---

## Phase 7 — Module 3: Same-source matching (req #8)

### Design

Source folder serves as BOTH targets (photos without GPS) AND references (photos in the same folder with GPS). No changes to `matcher.go` — the engine is agnostic about where references came from.

UI: a segmented radio control with three modes: **External refs** / **GPS track** / **Same source**. Default "External refs" preserves pre-phase-7 behavior. 'External refs' and 'GPS track' both dispatch to existing `RunMatching` (which uses whichever of `a.referencePhotos` or `a.gpsTrackPoints` is populated — intentionally flexible for users who have both). 'Same source' dispatches to a new `RunSameSourceMatching`.

### Changes

**File: `app_match.go`** — add method (note field name: `lastSourceRecursive` was added in phase 4):

```go
// RunSameSourceMatching runs the matching engine using the currently-
// scanned source folder as both target pool (photos without GPS) and
// reference pool (photos in the same folder that already have GPS).
//
// Useful when shooting a series at one location with a camera whose GPS
// module is unreliable (e.g. Pentax K-1): photos with GPS taken minutes
// before/after are perfect references for the ones that missed.
//
// Reuses MatchPhotos with no engine changes.
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
    // are session-only and the user may switch back to External refs
    // mode later without needing to re-set anything up.
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

    // Same propagation as RunMatching.
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
```

**File: `static/js/api.js`** — add:

```javascript
export async function runSameSourceMatching(opts) {
    return window.go.main.App.RunSameSourceMatching(opts);
}
```

**File: `static/js/state.js`** — add:

```javascript
// Which source feeds the next match run:
//   'refs'   — external reference folders (module 1)
//   'track'  — imported GPX/KML/CSV (module 2)
//   'same'   — photos inside the source folder with GPS (module 3)
// Default 'refs' preserves pre-phase-7 behavior.
matchMode: 'refs',
```

**File: `static/index.html`** — inside the Matching group, immediately before `<button id="btn-match-all">`, add:

```html
<div class="match-mode-group" role="radiogroup" aria-label="Match source">
    <label title="Match against external reference folders (module 1)">
        <input type="radio" name="match-mode" value="refs" checked>
        <span>External refs</span>
    </label>
    <label title="Match against an imported GPS track file (module 2)">
        <input type="radio" name="match-mode" value="track">
        <span>GPS track</span>
    </label>
    <label title="Match against other photos in the same source folder (module 3)">
        <input type="radio" name="match-mode" value="same">
        <span>Same source</span>
    </label>
</div>
<div class="btn-separator"></div>
```

**File: `static/css/components.css`** — append:

```css
.match-mode-group {
    display: inline-flex;
    align-items: center;
    gap: var(--space-sm);
    font-size: 0.75rem;
    color: var(--text-light);
}
.match-mode-group label {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    cursor: pointer;
    user-select: none;
}
.match-mode-group input[type="radio"] {
    margin: 0;
    cursor: pointer;
}
```

**File: `static/js/matcher_ui.js`** — add import at top:

```javascript
import { runMatching, runMatchingSingle, runSameSourceMatching } from './api.js';
```

In `initMatcher`, add the mode-radio listeners:

```javascript
document.querySelectorAll('input[name="match-mode"]').forEach(r => {
    r.addEventListener('change', () => {
        if (r.checked) state.matchMode = r.value;
    });
});
```

Rewrite `handleMatchAllClick` to route by mode:

```javascript
async function handleMatchAllClick() {
    if (state.targetPhotos.length === 0) {
        showZoneMessage('Scan a source folder first.');
        return;
    }

    // Client-side precondition checks per mode. These are UX, not security —
    // the Go side validates independently.
    if (state.matchMode === 'refs' && state.referenceFolders.length === 0) {
        showZoneMessage('Add a reference folder (or switch to GPS track / Same source).');
        return;
    }
    if (state.matchMode === 'track' && state.gpsTrackFiles.length === 0) {
        showZoneMessage('Import a GPS track (or switch to External refs / Same source).');
        return;
    }
    // 'same' has no precondition beyond a scanned source folder.

    setMatchingIndicator(true);
    try {
        const opts = { maxTimeDeltaMinutes: state.matchThreshold };
        const results = state.matchMode === 'same'
            ? await runSameSourceMatching(opts)
            : await runMatching(opts);
        state.matchResults = Array.isArray(results) ? results : [];

        // Auto-accept best candidates (phase 6)
        state.acceptedMatches.clear();
        state.matchResults.forEach(r => {
            if (r.bestCandidate) {
                const best = r.bestCandidate;
                state.acceptedMatches.set(r.targetPath, {
                    lat: best.gps.latitude, lon: best.gps.longitude,
                    score: best.score, source: best.source, sourcePath: best.sourcePath,
                });
            }
        });

        renderTable(state.targetPhotos);
        updateMatchStats();
        document.dispatchEvent(new CustomEvent('match-complete'));
    } catch (err) {
        console.error('Matching failed:', err);
        showZoneMessage('Matching failed: ' + String(err));
    } finally {
        setMatchingIndicator(false);
    }
}
```

### Verification

1. Source = folder containing some photos with GPS and some without (your Pentax K-1 case).
2. Select "Same source" radio → click "Search for GPS match" → matches found using the geolocated photos in the same folder.
3. Apply GPS → original geolocated photos keep their GPS; target photos get their matched coords.
4. Switch back to "External refs" → works as before.
5. In "Same source" mode with a folder that has NO geolocated photos → clear error message, no crash.

---

## Full testing checklist

Before the final merge, run through this in `wails dev`:

1. [ ] Scan a 200+ photo source folder. Exactly one `scan_complete` Info line in the terminal. No `exif_read` Debug lines.
2. [ ] `GPT_DEBUG_LOG=1 wails dev` — per-file Debug lines return.
3. [ ] Uncheck "Include subfolders" next to Source, re-scan → only root-level photos appear.
4. [ ] Same for Ref.
5. [ ] Slider: drag from 5 to 360 → label tracks. On release, matcher uses the new value.
6. [ ] Run a match → every matched row has a ticked checkbox. Select-all in header works.
7. [ ] Click a candidate in Zone C → location resolves within ~1.5 s, cached on repeat click (no second Nominatim request in DevTools).
8. [ ] Click "Show map" → Leaflet loads once (one CSS + one JS request), map appears with marker.
9. [ ] Apply GPS on a JPG → "Applied" badge; location text stays on the resolved city, no "Loading…" regression.
10. [ ] Apply GPS on a DNG → write succeeds; ExifTool confirms GPS tag; location text stays resolved.
11. [ ] Undo a DNG apply → byte-for-byte equal to pre-apply state (`fc /b` on Windows).
12. [ ] Tamper test: apply GPS, edit DNG in a hex editor, Undo → clear error message, file unchanged.
13. [ ] `ClearAllBackups` removes both `.bak` and `.bak.json` under the source folder.
14. [ ] Apply with a DNG open in another app (simulate: hold it open with PowerShell `Get-Content -Wait`) → clean error, no partial state. User can retry after closing the other app.
15. [ ] Orphan sweep: apply GPS, delete the DNG manually, scan a different folder → `orphan_sidecars_cleared` log line fires for the old folder.
16. [ ] "Same source" radio mode → match against photos in the same source folder works.
17. [ ] Uncheck one row before "Apply GPS data" → that row is skipped.
18. [ ] Tooltip on "Apply GPS data" button reads "Apply GPS data of the best candidate found".
19. [ ] Confirm dialog before every apply (single + batch) shows the "close other apps" warning.
20. [ ] `go test -bench BenchmarkApplyGPS -benchmem -benchtime 10x ./...` — numbers captured before and after phase 3b+3c and attached to commit messages.

---

## Commit hygiene

One commit per phase. Each commit must pass `wails build -platform windows/amd64` cleanly and its phase verification steps.

Suggested commit titles:

1. `phase 1: quiet scan logs — summary only, debug gated by env var`
2. `phase 2: fix stuck "Loading location…" + add geocode cache`
3. `phase 3a: add DNG apply benchmark harness`
4. `phase 3b: direct-binary DNG verify (no goexif)`
5. `phase 3c: lazy DNG backup via sidecar metadata with tamper detection`
6. `phase 4: recursive toggle for Source/Ref + delta slider`
7. `phase 5: lazy-loaded Leaflet mini-map with toggle`
8. `phase 6: per-photo apply selection + rename button`
9. `phase 7: module 3 — same-source matching`

Paste the benchmark numbers into the phase 3a/3b/3c commit messages so the owner can track the perf delta over time.
