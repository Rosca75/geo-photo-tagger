package main

// scanner_parallel.go — Parallel target photo scanner using a worker pool.
//
// ScanForTargetPhotosParallel is a high-performance replacement for
// ScanForTargetPhotos. It uses the same WalkDir + ReadEXIFForScan building
// blocks but fans EXIF reads out to multiple goroutines so I/O operations
// overlap with each other.
//
// Architecture:
//
//	WalkDir (single goroutine, fast — no per-entry stat)
//	    │
//	    ├─ filters by extension (cheap, no I/O)
//	    │
//	    └─ sends paths to buffered channel ──► Worker Pool (N goroutines)
//	                                               │
//	                                               ├─ ReadEXIFForScan(path)
//	                                               ├─ check HasGPS
//	                                               └─ send TargetPhoto to results channel
//	                                                       │
//	                                                       └──► Collector (single goroutine)
//	                                                                appends to []TargetPhoto
//
// Concurrency level: runtime.NumCPU() workers, capped at 8.
// The cap prevents excessive file-descriptor usage on high-core-count machines.

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// workerJob carries a single file path from the walk goroutine to a worker.
type workerJob struct {
	path string
	ext  string
}

// ScanForTargetPhotosParallel walks folderPath recursively and returns every
// photo that is missing GPS EXIF data, using a parallel worker pool.
//
// numWorkers controls how many goroutines read EXIF concurrently. Pass 0 to
// use the default: min(runtime.NumCPU(), 8). Each worker holds one open file
// at a time, so numWorkers also caps peak file-descriptor usage.
//
// Progress is reported via the app's scanStatus field (updated atomically)
// so the frontend polling GetScanStatus() sees a live progress bar.
func (a *App) ScanForTargetPhotosParallel(folderPath string, numWorkers int) ([]TargetPhoto, error) {
	// Determine the worker count — default to CPU count, cap at 8.
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	if numWorkers > 8 {
		numWorkers = 8
	}

	// --- Phase A: Walk the directory tree and collect candidate paths ---
	// This is single-threaded and fast (WalkDir avoids per-entry stat).
	// We collect all paths first so we know the total count before launching
	// workers, which lets us report accurate percentage progress.

	walkStart := time.Now()

	var candidates []workerJob
	var totalWalked int64 // all entries visited by WalkDir (files + dirs)

	walkErr := filepath.WalkDir(folderPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			slog.Warn("walk_inaccessible", "path", path, "error", err)
			return nil
		}
		atomic.AddInt64(&totalWalked, 1)
		if d.IsDir() {
			return nil
		}
		// Extension filter here — no stat call needed for non-photo files.
		ext := strings.ToLower(filepath.Ext(path))
		if isTargetExtension(ext) {
			candidates = append(candidates, workerJob{path: path, ext: ext})
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("walking folder %q: %w", folderPath, walkErr)
	}

	walkDuration := time.Since(walkStart)
	total := len(candidates)

	// Update scan status so the frontend can show the total count.
	a.scanStatus = ScanStatus{
		InProgress: true,
		Phase:      "reading_exif",
		Progress:   0,
		Total:      total,
		Message:    fmt.Sprintf("Reading EXIF from %d photos...", total),
	}

	slog.Info("walk_complete",
		"folder", folderPath,
		"total_walked", totalWalked,
		"candidates", total,
		"walk_duration_ms", walkDuration.Milliseconds(),
		"workers", numWorkers,
	)

	// --- Phase B: Fan out EXIF reads to the worker pool ---

	exifStart := time.Now()

	// jobCh delivers file paths to workers.
	// Buffer of 100 prevents the walk results from blocking while workers catch up.
	jobCh := make(chan workerJob, 100)

	// resultCh carries TargetPhoto records back to the collector goroutine.
	resultCh := make(chan TargetPhoto, 100)

	// Atomic counters for per-scan statistics and progress reporting.
	var (
		processedCount int64 // files processed so far (for progress bar)
		skippedGPS     int64 // files that had GPS (not targets)
		skippedErrors  int64 // files with unreadable EXIF
	)

	// Launch the worker pool.
	var wg sync.WaitGroup
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Each worker reads jobs until jobCh is closed.
			for job := range jobCh {
				exifData, err := ReadEXIFForScan(job.path)
				if err != nil {
					slog.Warn("exif_read_failed",
						"path", job.path,
						"error", err.Error(),
						"extension", job.ext,
					)
					atomic.AddInt64(&skippedErrors, 1)
					atomic.AddInt64(&processedCount, 1)
					a.updateScanProgress(int(atomic.LoadInt64(&processedCount)), total)
					continue
				}

				// Files with GPS are already tagged — not targets.
				if exifData.HasGPS {
					atomic.AddInt64(&skippedGPS, 1)
					atomic.AddInt64(&processedCount, 1)
					a.updateScanProgress(int(atomic.LoadInt64(&processedCount)), total)
					continue
				}

				// We need the file size now — call Info() only for result files.
				var fileSize int64
				if info, err := os.Stat(job.path); err == nil {
					fileSize = info.Size()
				}

				photo := TargetPhoto{
					Path:          job.path,
					Filename:      filepath.Base(job.path),
					Extension:     job.ext,
					FileSizeBytes: fileSize,
					Status:        "unmatched",
					CameraModel:   exifData.CameraModel,
				}
				if exifData.HasDateTime {
					photo.DateTimeOriginal = exifData.DateTimeOriginal
				}

				resultCh <- photo
				atomic.AddInt64(&processedCount, 1)
				a.updateScanProgress(int(atomic.LoadInt64(&processedCount)), total)
			}
		}()
	}

	// Close resultCh once all workers have finished.
	go func() {
		wg.Wait()
		close(resultCh)
	}()

	// Feed jobs into jobCh from the main goroutine.
	// This runs concurrently with the workers via the goroutine above.
	go func() {
		for _, job := range candidates {
			jobCh <- job
		}
		close(jobCh) // signals workers to drain and exit
	}()

	// --- Phase C: Collect results from all workers ---
	// resultCh is closed when all workers finish (see goroutine above).
	var results []TargetPhoto
	for photo := range resultCh {
		results = append(results, photo)
	}

	exifDuration := time.Since(exifStart)
	totalDuration := time.Since(walkStart)

	slog.Info("scan_complete",
		"folder", folderPath,
		"total_files_walked", totalWalked,
		"candidates_checked", total,
		"targets_found", len(results),
		"skipped_has_gps", atomic.LoadInt64(&skippedGPS),
		"skipped_exif_error", atomic.LoadInt64(&skippedErrors),
		"walk_duration_ms", walkDuration.Milliseconds(),
		"exif_duration_ms", exifDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds(),
		"workers", numWorkers,
	)

	return results, nil
}

// updateScanProgress atomically updates the app's scanStatus progress field.
// Called from worker goroutines — must not hold locks.
func (a *App) updateScanProgress(done, total int) {
	a.scanStatus = ScanStatus{
		InProgress: true,
		Phase:      "reading_exif",
		Progress:   done,
		Total:      total,
		Message:    fmt.Sprintf("Reading EXIF: %d / %d", done, total),
	}
}
