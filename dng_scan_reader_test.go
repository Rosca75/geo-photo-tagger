package main

// dng_scan_reader_test.go — Regression test for the IMGP7911 bug: Pentax
// DNGs with GPS IFDs beyond the 512 KB mark must be correctly classified
// as having GPS during a scan.

import (
	"os"
	"testing"
)

const sampleDNGWithFarGPS = "samples/IMGP7911.DNG"

// Expected coordinates — the file's actual GPS as recorded by the Pentax K-1.
// Tolerance 0.01° is wide enough for any reasonable rational-to-float
// rounding and narrow enough that a wrong file would fail.
const (
	expectedLat    = 49.62
	expectedLon    = 6.13
	coordTolerance = 0.01
)

// TestReadEXIFForScan_DNGWithFarGPS verifies that a Pentax DNG with its
// GPS IFD located beyond the 512 KB LimitReader window is correctly
// recognized as having GPS. Before the dng_scan_reader.go fix, this
// returned HasGPS=false and misclassified the photo.
func TestReadEXIFForScan_DNGWithFarGPS(t *testing.T) {
	if _, err := os.Stat(sampleDNGWithFarGPS); err != nil {
		t.Skipf("%s not present — copy the file into samples/ to enable this test", sampleDNGWithFarGPS)
	}

	result, err := ReadEXIFForScan(sampleDNGWithFarGPS)
	if err != nil {
		t.Fatalf("ReadEXIFForScan returned error: %v", err)
	}
	if !result.HasGPS {
		t.Fatalf("HasGPS=false — the bug is not fixed (GPS IFD sits at offset 0x2C4D84C, past the old 512 KB reader)")
	}
	if absFloat(result.Latitude-expectedLat) > coordTolerance {
		t.Errorf("latitude = %.6f, want ≈ %.2f (tolerance %.2f)", result.Latitude, expectedLat, coordTolerance)
	}
	if absFloat(result.Longitude-expectedLon) > coordTolerance {
		t.Errorf("longitude = %.6f, want ≈ %.2f (tolerance %.2f)", result.Longitude, expectedLon, coordTolerance)
	}
	if !result.HasDateTime {
		t.Errorf("HasDateTime=false — DateTime should be readable even when the old goexif path failed")
	}
}

// absFloat is a local absolute-value helper. Named with the `Float` suffix to
// avoid colliding with any existing `abs` symbol elsewhere in the package.
func absFloat(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
