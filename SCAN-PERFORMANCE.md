# SCAN-PERFORMANCE.md — Target Folder Scan Optimization

> Performance improvement plan for Phase 1 (ScanForTargetPhotos) and Phase 2 (ScanForReferencePhotos).
> The goal: identify photos without GPS data as fast as possible, with instrumented logging
> for ongoing measurement.

---

## 1. Current Bottleneck Analysis

The current `ScanForTargetPhotos` has three sequential cost centres:

| Step | What happens | Cost per file | Notes |
|------|-------------|---------------|-------|
| **Directory walk** | `filepath.Walk` calls `os.Lstat` on every entry | ~1 syscall + stat | Walk is the legacy API |
| **File open** | `os.Open` + full `exif.Decode` | 1 open + read until EXIF parsed | Reads entire APP1 segment (≤64 KB for JPEG) |
| **EXIF decode** | `goexif` parses all IFDs, all tags, all maker notes | CPU-bound | Parses far more than we need (GPS + DateTime + Model) |

For a folder of 10,000 photos at ~5–8 MB each, the scan is **single-threaded** and **I/O-bound**
on the file open + EXIF read. The directory walk itself is a secondary cost (extra `Lstat` per entry).

**Key insight:** we only need three things from each file's EXIF: `HasGPS`, `DateTimeOriginal`,
and `CameraModel`. We do not need to decode the image, read thumbnails, or parse maker notes
at scan time.

---

## 2. Improvement Plan — Three Layers

### Layer 1: Replace `filepath.Walk` with `filepath.WalkDir` (no new dependency)

**Why:** Go 1.16+ introduced `filepath.WalkDir` which uses `fs.DirEntry` instead of `os.FileInfo`.
This avoids calling `os.Lstat` on every visited file and directory. The Go standard library
documentation states that `Walk` is less efficient than `WalkDir` for this reason.

Benchmarks from the Go community show `WalkDir` is ~1.5× faster on local SSD and **up to 3×
faster on network file systems** (NAS/SMB), which is relevant since photographers often store
files on a NAS.

**Change required:**

```go
// BEFORE (current code)
filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
    if info.IsDir() { return nil }
    // ...
})

// AFTER (optimized)
filepath.WalkDir(folderPath, func(path string, d fs.DirEntry, err error) error {
    if d.IsDir() { return nil }
    // Extension filtering happens here — no stat needed
    ext := strings.ToLower(filepath.Ext(path))
    if !isTargetExtension(ext) { return nil }
    // Only now do we open the file for EXIF reading
    // ...
})
```

**Impact:** eliminates one `Lstat` syscall per file/directory visited. On a folder with 10,000
photos + 500 subdirectories, this saves ~10,500 syscalls. On NAS/SMB mounts the saving is
substantial because each stat is a network round-trip.

**Dependency:** none (stdlib `io/fs` and `path/filepath`).

**Risk:** `d.Info()` must be called explicitly if file size is needed (for `FileSizeBytes`).
Call it only for files that pass the extension filter AND the EXIF GPS check — i.e., files
that will actually appear in the results list.

---

### Layer 2: Limit I/O — read only the EXIF header, not the whole file

**Why:** JPEG EXIF data is stored in the APP1 segment, which is limited to 64 KB by the JPEG
specification. The `rwcarlsen/goexif` library's `exif.Decode()` already reads only up to the
EXIF data (it scans for the APP1 marker then reads the TIFF structure), so it does NOT read
the full multi-megabyte file. However, we can still improve things:

1. **Use `io.LimitReader` as a safety net** — wrap the file in `io.LimitReader(f, 128*1024)`
   to guarantee the reader never consumes more than 128 KB even on malformed files or
   large DNG/TIFF files. This prevents pathological cases where a corrupt file causes
   the EXIF decoder to read megabytes.

2. **Skip maker note parsing** — the current `init()` calls `exif.RegisterParsers(mknote.All...)`
   which registers Nikon, Canon, and other maker note parsers. These add CPU cost to parse
   proprietary data we never use. **Remove the `RegisterParsers` call** from the scan path.
   Maker notes are only useful for display purposes (Phase 5+), not for GPS detection.

**Change required in `exif_reader.go`:**

```go
// NEW: Lightweight EXIF reader for scan phase only.
// Does NOT register maker note parsers — faster for GPS/DateTime check.
func ReadEXIFForScan(path string) (*EXIFData, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, fmt.Errorf("opening %q: %w", path, err)
    }
    defer f.Close()

    // Safety limit: EXIF cannot exceed 64 KB in JPEG (use 128 KB for DNG margin)
    limited := io.LimitReader(f, 128*1024)

    x, err := exif.Decode(limited)
    if err != nil {
        return &EXIFData{}, nil
    }

    result := &EXIFData{}

    // GPS check — the only thing that matters for target scan filtering
    lat, lon, err := x.LatLong()
    if err == nil {
        result.HasGPS = true
        result.Latitude = lat
        result.Longitude = lon
    }

    // Timestamp — needed for matching later
    t, err := x.DateTime()
    if err == nil {
        result.DateTimeOriginal = t
        result.HasDateTime = true
    }

    // Camera model — lightweight, single tag read
    modelTag, err := x.Get(exif.Model)
    if err == nil {
        result.CameraModel, _ = modelTag.StringVal()
    }

    return result, nil
}
```

**Keep `ReadEXIF` (with maker notes) for the detail/preview path** where richer metadata is
displayed to the user. The scan path only needs `ReadEXIFForScan`.

**Impact:** ~10–30% CPU reduction per file by skipping maker note parsing. The `LimitReader`
prevents edge-case slowdowns on corrupt/oversized files.

**Note on DNG files:** DNG is TIFF-based and its EXIF is typically in the first IFD within the
first few KB. The 128 KB limit is generous enough for any real DNG header. If a DNG has its
GPS IFD beyond 128 KB (extremely unlikely for standard cameras), the file will be treated as
"no EXIF" and show up as a target — which is a safe failure mode (user sees the file, can
manually inspect).

---

### Layer 3: Concurrent EXIF reading with worker pool

**Why:** The scan is I/O-bound (file open + read). Modern SSDs handle parallel reads well, and
even HDDs benefit from OS readahead when files are scattered across the disk. On NAS/SMB,
parallelism is essential because each file open is a network round-trip.

The `charlievieth/fastwalk` library claims ~4× speedup on Linux by parallelizing directory
reads, but it is an external dependency. Instead, we use a **stdlib-only approach**: walk the
directory tree sequentially (cheap with `WalkDir`) to collect candidate file paths, then fan
out EXIF reads to a bounded worker pool.

**Architecture:**

```
WalkDir (single goroutine, fast)
    │
    ├─ filters by extension
    │
    └─ sends paths to buffered channel ──► Worker Pool (N goroutines)
                                               │
                                               ├─ ReadEXIFForScan(path)
                                               ├─ check HasGPS
                                               └─ send TargetPhoto to results channel
                                                       │
                                                       └──► Collector (single goroutine)
                                                                appends to []TargetPhoto
```

**Concurrency level:** `runtime.NumCPU()` workers, capped at 8 maximum. The cap prevents
excessive file descriptor usage on machines with many cores. Each worker holds one open file
at a time.

**Why not `fastwalk` or `MichaelTJones/walk`?** The CLAUDE.md rule "Adding a new dependency
requires explicit approval" applies. The stdlib `WalkDir` + goroutine pool achieves most of
the benefit without a new dependency. If future benchmarks show the walk phase (not EXIF) is
the bottleneck, `fastwalk` can be reconsidered — but for photo folders (thousands of files,
not millions), the walk phase is fast enough.

**Change required — new `scanner_parallel.go`:**

```go
// ScanForTargetPhotosParallel is the high-performance version of ScanForTargetPhotos.
// It walks the directory tree with WalkDir, then fans out EXIF reads to a worker pool.
func ScanForTargetPhotosParallel(folderPath string, numWorkers int) ([]TargetPhoto, error) {
    // Phase A: collect candidate paths (fast, single-threaded)
    // Phase B: fan out EXIF reads to workers (parallel, I/O-bound)
    // Phase C: collect results
    // ...
}
```

**Impact:** on an 8-core machine with SSD, expect ~3–5× speedup for EXIF reading. On NAS,
expect ~4–8× due to network latency hiding. The walk phase adds negligible overhead.

**Progress reporting:** the worker pool updates `ScanStatus.Progress` atomically via
`sync/atomic`. The frontend polls `GetScanStatus()` to show the progress bar.

---

## 3. Logging Strategy for Performance Measurement

All timing measurements use `log/slog` (stdlib, Go 1.21+). Go 1.25 (current `go.mod`) fully
supports it. No external logging dependency needed.

### 3.1 Why `log/slog`

The Go standard library introduced `log/slog` in Go 1.21 as a structured, leveled logger.
Benchmarks show it allocates only ~40 bytes per log call (comparable to zerolog) with fewer
allocations per operation than zap. For a desktop application logging scan metrics, `slog` is
more than sufficient. It is in the stdlib, so it satisfies the "no unapproved dependency" rule.

The current codebase uses `log.Printf` — this will be migrated to `slog` for structured fields.

### 3.2 Log Points

Each log point uses structured key-value fields so they can be parsed programmatically later.

**Scan-level timing (one log entry per scan):**

```go
slog.Info("scan_complete",
    "folder", folderPath,
    "total_files_walked", totalWalked,      // all files seen by WalkDir
    "candidates_checked", candidatesChecked, // files with target extension
    "targets_found", len(results),           // files without GPS
    "skipped_has_gps", skippedGPS,           // files that had GPS (not targets)
    "skipped_exif_error", skippedErrors,     // files with unreadable EXIF
    "walk_duration_ms", walkDuration.Milliseconds(),
    "exif_duration_ms", exifDuration.Milliseconds(),
    "total_duration_ms", totalDuration.Milliseconds(),
    "workers", numWorkers,
)
```

**Per-file timing (only at `slog.LevelDebug`, disabled by default):**

```go
slog.Debug("exif_read",
    "path", path,
    "has_gps", exifData.HasGPS,
    "has_datetime", exifData.HasDateTime,
    "duration_us", elapsed.Microseconds(),
    "file_size_bytes", fileSize,
)
```

**Error logging (replaces current `log.Printf`):**

```go
slog.Warn("exif_read_failed",
    "path", path,
    "error", err.Error(),
    "extension", ext,
)
```

### 3.3 Logger Initialization

Add a `logger.go` file (or a section in `app.go`) that initializes `slog`:

```go
// setupLogger configures the global slog logger.
// In dev mode (wails dev), log at Debug level for full per-file timing.
// In production builds, log at Info level (scan summaries only).
func setupLogger(debug bool) {
    level := slog.LevelInfo
    if debug {
        level = slog.LevelDebug
    }
    handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
        Level: level,
    })
    slog.SetDefault(slog.New(handler))
}
```

**Future extension:** when a file-based log is needed (e.g., for user-submitted bug reports),
swap `os.Stderr` for an `io.MultiWriter(os.Stderr, logFile)`. The structured format makes
grep/jq analysis trivial.

### 3.4 Timing Instrumentation Pattern

Use a consistent helper to measure durations:

```go
// timeOp returns a function that, when called, logs the elapsed time.
// Usage: defer timeOp("scan_walk", slog.String("folder", path))()
func timeOp(operation string, attrs ...slog.Attr) func() {
    start := time.Now()
    return func() {
        elapsed := time.Since(start)
        allAttrs := append(attrs, slog.Int64("duration_ms", elapsed.Milliseconds()))
        slog.LogAttrs(context.Background(), slog.LevelInfo, operation, allAttrs...)
    }
}
```

---

## 4. Implementation Order

Execute these changes in this exact order. Each step must compile and pass manual testing
before proceeding to the next.

### Step 1: Add `log/slog` infrastructure

- [ ] Create `logger.go` with `setupLogger()` function
- [ ] Call `setupLogger(true)` in `app.go` `startup()` (debug mode for now)
- [ ] Replace all existing `log.Printf` calls in `scanner.go` and `exif_reader.go` with `slog`
- [ ] Verify: `wails dev` → scan a folder → see structured log output in terminal

### Step 2: Switch to `filepath.WalkDir`

- [ ] In `scanner.go`, replace `filepath.Walk` with `filepath.WalkDir` in both
      `ScanForTargetPhotos` and `ScanForReferencePhotos`
- [ ] Defer the `d.Info()` call (for `FileSizeBytes`) until after extension filter + EXIF check
- [ ] Add walk-phase timing log (`walk_duration_ms`)
- [ ] Verify: same scan results as before, check log for timing

### Step 3: Add `ReadEXIFForScan` lightweight reader

- [ ] Add `ReadEXIFForScan()` to `exif_reader.go` (no maker notes, with `LimitReader`)
- [ ] Keep existing `ReadEXIF()` unchanged (used by detail views, Phase 5+)
- [ ] Update `ScanForTargetPhotos` and `ScanForReferencePhotos` to call `ReadEXIFForScan`
- [ ] Add per-file debug logging (`exif_read` entries)
- [ ] Verify: same results, compare timing in logs vs. before

### Step 4: Implement parallel worker pool

- [ ] Create `scanner_parallel.go` with `ScanForTargetPhotosParallel()`
- [ ] Two-phase approach: sequential `WalkDir` → parallel EXIF reads
- [ ] Bounded worker pool (`runtime.NumCPU()`, max 8)
- [ ] Atomic progress counter for `ScanStatus`
- [ ] Add scan-level summary log (`scan_complete` entry with all counters)
- [ ] Wire into `app.go` as the new default scan function
- [ ] Verify: correct results, faster timing, progress bar works

### Step 5: Benchmark & validate

- [ ] Prepare a test folder with known contents (e.g., 500 JPGs, 100 DNGs, mix of GPS/no-GPS)
- [ ] Run three scans: note `total_duration_ms` from logs
- [ ] Compare against pre-optimization baseline (manually timed or from earlier logs)
- [ ] Document results in this file (section 6 below)

---

## 5. Files Modified

| File | Change |
|------|--------|
| `logger.go` | **NEW** — slog setup + timing helper |
| `scanner.go` | `filepath.Walk` → `filepath.WalkDir`, call `ReadEXIFForScan`, add slog |
| `scanner_parallel.go` | **NEW** — parallel scan with worker pool |
| `exif_reader.go` | Add `ReadEXIFForScan()`, keep `ReadEXIF()` intact |
| `app.go` | Call `setupLogger()`, wire `ScanForTargetPhotosParallel` |
| `types.go` | No changes expected |

**No new dependencies.** All changes use stdlib: `log/slog`, `io`, `sync`, `sync/atomic`,
`runtime`, `io/fs`, `path/filepath`, `context`.

---

## 6. Benchmark Results

> **To be filled in after implementation.**
>
> | Metric | Before | After Step 2 | After Step 3 | After Step 4 |
> |--------|--------|-------------|-------------|-------------|
> | Walk time (ms) | — | — | — | — |
> | EXIF read time (ms) | — | — | — | — |
> | Total scan time (ms) | — | — | — | — |
> | Files scanned | — | — | — | — |
> | Workers used | 1 | 1 | 1 | — |
> | Test folder | — | — | — | — |

---

## 7. Future Considerations (Out of Scope for Now)

- **EXIF result cache:** persist a `path → {modTime, size, hasGPS, dateTime}` map to disk
  (JSON or bolt DB) to skip re-reading files on subsequent scans of the same folder. Only
  re-read files whose `modTime` or `size` changed. This turns a 10-second scan into a
  sub-second scan for repeat visits.

- **`fastwalk` library:** if benchmarks show the `WalkDir` phase is >30% of total time
  (unlikely for photo folders), consider `github.com/charlievieth/fastwalk` which parallelizes
  directory reads themselves. Requires dependency approval per CLAUDE.md §11.

- **`evanoberholster/imagemeta`:** a performance-focused Go EXIF library that claims zero
  allocations and supports JPEG, HEIC, TIFF, and RAW formats. If `rwcarlsen/goexif` becomes a
  bottleneck, this could be a drop-in replacement for the scan path. Requires dependency
  approval.

- **Early termination for GPS check:** a custom minimal EXIF parser that only looks for the
  `GPSInfoIFDPointer` (tag 0x8825) in IFD0 could determine GPS presence by reading ~200 bytes
  of EXIF instead of parsing all tags. This is a micro-optimization worth ~50% per-file CPU
  reduction, but requires writing a custom TIFF/IFD parser. Consider only if 10,000+ file
  scans are still too slow after Steps 1–4.
