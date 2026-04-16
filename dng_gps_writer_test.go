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
	if _, err := os.Stat(sampleDNG); err != nil {
		b.Skipf("%s not found: %v", sampleDNG, err)
	}
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
	if _, err := os.Stat(sampleDNG); err != nil {
		b.Skipf("%s not found: %v", sampleDNG, err)
	}
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
	if _, err := os.Stat(sampleDNG); err != nil {
		b.Skipf("%s not found: %v", sampleDNG, err)
	}
	path := cloneSample(b)
	defer cleanupApply(path)
	_ = writeGPSToDNG(path, 49.6116, 6.1319)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ReadEXIF(path)
	}
}

// BenchmarkApplyGPS_VerifyFastPath measures the phase-3b direct-binary
// verification (~130 bytes of I/O, no goexif). Compare with VerifyFullReader.
func BenchmarkApplyGPS_VerifyFastPath(b *testing.B) {
	if _, err := os.Stat(sampleDNG); err != nil {
		b.Skipf("%s not found: %v", sampleDNG, err)
	}
	path := cloneSample(b)
	defer cleanupApply(path)
	_ = writeGPSToDNG(path, 49.6116, 6.1319)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = verifyGPSInDNG(path, 49.6116, 6.1319)
	}
}
