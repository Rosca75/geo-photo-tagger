# Change Request 2026-04 — Hotfix: DNG scan misclassification

> **Audience:** Claude Opus 4.7 working on `Rosca75/geo-photo-tagger` on Windows 11.
> **Context:** A bug was discovered by the owner while scanning a folder containing `IMGP7911.DNG`. The photo has valid GPS data but `ReadEXIFForScan` reports `loading GPS sub-IFD: exif: sub-IFD GPSInfoIFDPointer decode failed: tiff: failed to read IFD tag coungt: EOF`. The photo is misclassified as "no GPS" (or as an EXIF error depending on the current code path) and enters the target pool incorrectly. Applying GPS to such a photo would overwrite legitimate Pentax coordinates with interpolated ones.
>
> **Before touching any file, read `CLAUDE.md` in full.** All its rules apply. This is a small, single-phase change — keep it surgical.
>
> **Scope.** One bug, one fix, one regression test. No refactoring. No opportunistic improvements. If you notice something else looks odd during your work, note it at the bottom of the post-mortem (see section 4) — do not fix it.

---

## 1. Root cause (confirmed)

The Pentax K-1 writes the GPS IFD near end-of-file. On `IMGP7911.DNG`:

- File size: 46,454,974 bytes (~44.3 MB)
- `GPSInfoIFDPointer` in IFD0 → offset `0x2C4D84C` = ~46 MB into the file
- The file and GPS IFD are both structurally valid — confirmed by direct binary parsing

`ReadEXIFForScan` wraps the file in a 512 KB `io.LimitReader` for performance. When goexif follows the `GPSInfoIFDPointer` past offset 524,288, `ReadAt` returns EOF, producing the observed error.

This is the same root cause as [goexif2 issue #12](https://github.com/xor-gate/goexif2/issues/12). The upstream fix (detect + ignore invalid IFD offsets) is in the fork but not in the `rwcarlsen/goexif` the project depends on.

**Why earlier testing missed it.** The GPS IFD's exact offset is scene/file dependent. Previously tested Pentax files (`IMGP8108.DNG`, `IMGP8411.DNG`) have GPS IFDs at offsets within the 512 KB window. `IMGP7911.DNG` is the first file where the GPS IFD landed beyond it.

**Why phase 3b didn't cover this.** Phase 3b fixed `verifyGPSInDNG` (the write-verify path) using direct binary reads — noting explicitly in its design that `ReadEXIFForScan`'s `LimitReader` cannot reach EOF-appended GPS IFDs. The same bug in the **scan-read** path was not addressed because no failing file had surfaced at the time.

---

## 2. The fix

**Strategy: direct-binary GPS probe for DNG in the scan path.**

`ReadEXIFForScan` currently uses goexif for all formats. For DNG we bypass goexif entirely on the scan path and answer the only two questions the scanner actually needs:

1. **Does this DNG have GPS?** (sets `HasGPS`, `Latitude`, `Longitude`)
2. **What's the DateTime?** (sets `HasDateTime`, `DateTime`)

Both can be answered by reading a few hundred bytes at known offsets using the same TIFF-walking helpers `dng_gps_writer.go` and `dng_gps_verify.go` already use. No `LimitReader`, no goexif, no EOF-at-46MB issue.

**JPEG is untouched** — the existing `ReadEXIFForScan` path with its 512 KB `LimitReader` keeps working as-is.

### 2.1 New file: `dng_scan_reader.go`

Location: repo root alongside the other DNG files. Target length ~130–150 lines; if it crosses 150, split the DateTime reader into `dng_scan_reader_datetime.go`.

```go
package main

// dng_scan_reader.go — Direct-binary GPS + DateTime probe for DNG files during
// scanning. Bypasses goexif entirely on the scan path.
//
// Problem this solves: ReadEXIFForScan wraps the file in a 512 KB LimitReader
// for performance. Pentax K-1 (and likely other) DNGs put their GPS IFD near
// end-of-file — on IMGP7911.DNG the GPS IFD lives at offset 0x2C4D84C (~46 MB
// in). When goexif follows the pointer past the LimitReader window, it gets
// EOF and reports "sub-IFD GPSInfoIFDPointer decode failed: tiff: failed to
// read IFD tag count: EOF". The photo gets misclassified as "no GPS" and
// enters the target pool incorrectly — a correctness bug that would cause
// the next Apply GPS run to overwrite legitimate camera coordinates.
//
// This file reuses the TIFF-walking helpers in dng_gps_writer.go
// (readTIFFHeader, findGPSInfoIFDPointer) and the coord parser in
// dng_gps_verify.go (readGPSCoordsFromIFD) so there is exactly one
// canonical DNG TIFF parser in the project.

import (
    "encoding/binary"
    "errors"
    "fmt"
    "io"
    "os"
    "time"
)

// readDNGScanFields returns the minimum EXIF fields the scanner needs:
// HasGPS + Latitude + Longitude + HasDateTime + DateTime. Errors indicate
// structural problems with the file (not a valid TIFF, truncated IFD0,
// etc.) and should be treated the same way the existing ReadEXIFForScan
// treats goexif errors for JPEG.
//
// A missing GPSInfoIFDPointer is NOT an error — it means "no GPS", which
// is expected for un-geotagged photos. Same for DateTime.
func readDNGScanFields(path string) (result ExifResult, err error) {
    f, err := os.Open(path)
    if err != nil {
        return result, fmt.Errorf("open: %w", err)
    }
    defer f.Close()

    byteOrder, ifd0Offset, err := readTIFFHeader(f)
    if err != nil {
        return result, fmt.Errorf("tiff header: %w", err)
    }

    // Walk IFD0 once, collecting both the GPSInfoIFDPointer value and
    // the DateTime value/offset. Single pass, bounded I/O.
    gpsIFDOffset, dateTimeStr, err := scanIFD0ForGPSAndDateTime(f, byteOrder, ifd0Offset)
    if err != nil {
        return result, fmt.Errorf("walking IFD0: %w", err)
    }

    if gpsIFDOffset != 0 {
        lat, lon, gpsErr := readGPSCoordsFromIFD(f, byteOrder, int64(gpsIFDOffset))
        if gpsErr == nil {
            result.HasGPS = true
            result.Latitude = lat
            result.Longitude = lon
        }
        // If the GPS IFD exists but is malformed, we treat it as "no GPS"
        // silently — same behavior as the JPEG path when goexif returns a
        // non-critical error. The scan log will still show total counts.
    }

    if dateTimeStr != "" {
        if t, tErr := parseEXIFDateTime(dateTimeStr); tErr == nil {
            result.HasDateTime = true
            result.DateTime = t
        }
    }
    return result, nil
}

// scanIFD0ForGPSAndDateTime walks IFD0 once and returns:
//   - gpsIFDOffset: the value of the GPSInfoIFDPointer tag (0x8825), or 0 if absent
//   - dateTimeStr:  the ASCII value of the DateTime tag (0x0132), or "" if absent
//
// Does not allocate for tags it does not care about. Returns an error only
// on structural problems (truncated count, unreadable entry).
func scanIFD0ForGPSAndDateTime(f *os.File, byteOrder binary.ByteOrder, ifd0Offset int64) (gpsIFDOffset uint32, dateTimeStr string, err error) {
    if _, err = f.Seek(ifd0Offset, io.SeekStart); err != nil {
        return 0, "", fmt.Errorf("seek IFD0: %w", err)
    }
    var entryCount uint16
    if err = binary.Read(f, byteOrder, &entryCount); err != nil {
        return 0, "", fmt.Errorf("read IFD0 entry count: %w", err)
    }

    const tagDateTime = 0x0132
    const tagGPSInfo = 0x8825

    for i := uint16(0); i < entryCount; i++ {
        var tag, typeID uint16
        var count, value uint32
        if err = binary.Read(f, byteOrder, &tag); err != nil {
            return 0, "", fmt.Errorf("entry %d tag: %w", i, err)
        }
        if err = binary.Read(f, byteOrder, &typeID); err != nil {
            return 0, "", fmt.Errorf("entry %d type: %w", i, err)
        }
        if err = binary.Read(f, byteOrder, &count); err != nil {
            return 0, "", fmt.Errorf("entry %d count: %w", i, err)
        }
        if err = binary.Read(f, byteOrder, &value); err != nil {
            return 0, "", fmt.Errorf("entry %d value: %w", i, err)
        }

        switch tag {
        case tagGPSInfo:
            // LONG inline — the value field IS the GPS IFD offset.
            gpsIFDOffset = value
        case tagDateTime:
            // ASCII, typically 20 bytes ("YYYY:MM:DD HH:MM:SS\0") which
            // exceeds 4, so value is an external offset. But guard anyway
            // in case a malformed file has count <= 4.
            current, _ := f.Seek(0, io.SeekCurrent)
            s, readErr := readASCIITag(f, typeID, count, value, byteOrder)
            if readErr == nil {
                dateTimeStr = s
            }
            // Restore position after the external read.
            if _, err = f.Seek(current, io.SeekStart); err != nil {
                return 0, "", fmt.Errorf("restore after DateTime: %w", err)
            }
        }
    }
    return gpsIFDOffset, dateTimeStr, nil
}

// readASCIITag reads an ASCII TIFF tag value. If count <= 4 the bytes are
// packed inline into the 4-byte value field; otherwise `value` is an
// absolute file offset to `count` bytes of ASCII data.
func readASCIITag(f *os.File, typeID uint16, count, value uint32, byteOrder binary.ByteOrder) (string, error) {
    if typeID != 2 { // EXIF ASCII
        return "", errors.New("not ASCII")
    }
    if count == 0 {
        return "", nil
    }
    if count <= 4 {
        // Inline: reconstruct the 4 value-field bytes in the declared byte order.
        buf := make([]byte, 4)
        byteOrder.PutUint32(buf, value)
        return trimASCII(buf[:count]), nil
    }
    if _, err := f.Seek(int64(value), io.SeekStart); err != nil {
        return "", fmt.Errorf("seek ASCII value: %w", err)
    }
    buf := make([]byte, count)
    if _, err := io.ReadFull(f, buf); err != nil {
        return "", fmt.Errorf("read ASCII value: %w", err)
    }
    return trimASCII(buf), nil
}

// trimASCII drops a trailing NUL terminator if present (EXIF ASCII includes it).
func trimASCII(b []byte) string {
    if n := len(b); n > 0 && b[n-1] == 0 {
        return string(b[:n-1])
    }
    return string(b)
}

// parseEXIFDateTime parses EXIF's "YYYY:MM:DD HH:MM:SS" format.
func parseEXIFDateTime(s string) (time.Time, error) {
    return time.Parse("2006:01:02 15:04:05", s)
}
```

### 2.2 Modify `exif_reader.go`

**Only the dispatch in `ReadEXIFForScan` changes.** Find the top of `ReadEXIFForScan` (right after any path validation). Add extension dispatch:

```go
// Fast scan path for DNG: bypass goexif entirely. The 512 KB LimitReader
// used below cannot reach GPS IFDs that Pentax (and similar cameras)
// append near end-of-file on large DNGs — see dng_scan_reader.go.
ext := strings.ToLower(filepath.Ext(path))
if ext == ".dng" {
    return readDNGScanFields(path)
}
```

Do NOT delete the existing JPEG path. Do NOT touch `ReadEXIF` (the non-scan version). If `"strings"` or `"path/filepath"` are not imported in `exif_reader.go`, add them.

**Verify the `ExifResult` struct fields.** The exact field names (`HasGPS`, `Latitude`, `Longitude`, `HasDateTime`, `DateTime`) must match what `ExifResult` currently declares. If `types.go` uses different names, adjust `readDNGScanFields` accordingly — do not rename the struct.

### 2.3 What NOT to change

- `ReadEXIF` (the full read path) stays goexif-based. It's used elsewhere (JPEG verify, possibly preview) and has no `LimitReader` issue.
- `verifyGPSInDNG` stays as-is. It's the write-verify path and already solved.
- `dng_gps_writer.go`, `dng_gps_verify.go`, `dng_backup.go` — no changes. Reuse their helpers (`readTIFFHeader`, `findGPSInfoIFDPointer`, `readGPSCoordsFromIFD`, `readThreeRationals`), do not duplicate them. If any of those helpers is package-private to a file you can't import from, make it a free function in the same package (which it already is — all these files are `package main`).
- `scanner_parallel.go`, `scanner.go` — no changes. They call `ReadEXIFForScan` and get a correct result transparently.

---

## 3. Regression test

### 3.1 Sample file setup (owner-side, one-time)

The file `IMGP7911.DNG` (44.3 MB) must not be committed to Git — it exceeds GitHub's 25 MB blob limit, and big binary fixtures bloat the repo anyway. Keep it local.

**Add `samples/` to `.gitignore`** if not already there:

```
# Local-only DNG test fixtures — too big for Git, owner keeps these
samples/
```

Owner action (one time): copy `IMGP7911.DNG` into `<repo>/samples/`. The test detects presence and skips itself if absent, so fresh clones and CI stay green.

### 3.2 New test file: `dng_scan_reader_test.go`

```go
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
    expectedLat = 49.62
    expectedLon = 6.13
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
    if abs(result.Latitude-expectedLat) > coordTolerance {
        t.Errorf("latitude = %.6f, want ≈ %.2f (tolerance %.2f)", result.Latitude, expectedLat, coordTolerance)
    }
    if abs(result.Longitude-expectedLon) > coordTolerance {
        t.Errorf("longitude = %.6f, want ≈ %.2f (tolerance %.2f)", result.Longitude, expectedLon, coordTolerance)
    }
    if !result.HasDateTime {
        t.Errorf("HasDateTime=false — DateTime should be readable even when the old goexif path failed")
    }
}

func abs(x float64) float64 {
    if x < 0 {
        return -x
    }
    return x
}
```

Adjust `expectedLat` / `expectedLon` if the actual values differ from the approximate Luxembourg coordinates assumed above — the owner can confirm by running ExifTool on the file or by letting the test fail the first time and reading the actual value from the error.

---

## 4. Verification

### 4.1 Build + unit tests

From repo root:

```
go vet ./...
go test ./... -run TestReadEXIFForScan
wails build -platform windows/amd64
```

All three must succeed. The new regression test must PASS (not skip) when `samples/IMGP7911.DNG` is present.

### 4.2 End-to-end check in the app

1. `wails dev`
2. Scan the folder containing IMGP7911.DNG as the Source.
3. Confirm the scan log no longer emits `exif_decode_failed` for that file.
4. Confirm IMGP7911.DNG appears in the **"skipped — already has GPS"** count, NOT in the target pool.
5. Run a match with any reference → confirm IMGP7911.DNG is not proposed as a target for GPS write.

### 4.3 Misclassification risk note (for the owner)

**Before this fix landed, IMGP7911.DNG (and potentially any other Pentax DNG whose GPS IFD offset exceeds 524288 bytes) was likely misclassified as "no GPS" during scans.** If such a photo was ever included in a batch Apply GPS operation, its original Pentax coordinates would have been overwritten with interpolated coordinates from the matching process.

Opus 4.7: at the end of this change request, add a note to the commit message (or a short section in the post-mortem) reminding the owner to:

- Review any previously applied batches that included Pentax DNGs
- Check for sidecar `.bak.json` files under previously scanned folders — an Apply from before phase 3c would have no backup, but an Apply from after phase 3c can be reversed via Undo if the sidecar still exists and its tamper hash still matches
- Consider re-scanning previously processed folders now that the scanner correctly identifies GPS presence

Do NOT write a migration script or automated recovery tool. This is a manual review the owner performs.

---

## 5. Commit

Single commit: `fix: DNG scan correctly reads GPS IFDs past 512 KB`.

Suggested commit body:

```
Pentax K-1 (and similar cameras) write the GPS IFD near end-of-file in
large DNGs. On IMGP7911.DNG the GPS IFD sits at offset 0x2C4D84C (~46 MB
in), which is past the 512 KB io.LimitReader that ReadEXIFForScan used
for performance. goexif would follow the GPSInfoIFDPointer and hit EOF,
producing "sub-IFD GPSInfoIFDPointer decode failed: tiff: failed to read
IFD tag count: EOF". The photo was misclassified as "no GPS" and could
be incorrectly targeted by Apply GPS.

Fix: bypass goexif entirely on the DNG scan path. New dng_scan_reader.go
walks IFD0 with ~200 bytes of I/O, reads GPS (reusing helpers from
dng_gps_writer.go + dng_gps_verify.go) and DateTime, and returns the
minimum ExifResult the scanner needs. JPEG path is untouched.

Regression test on IMGP7911.DNG requires the file be placed locally at
samples/IMGP7911.DNG (too big for Git, samples/ is .gitignored). The
test skips cleanly if the file is absent.

Owner follow-up: previously scanned Pentax DNGs may have been
misclassified — review any past Apply GPS batches that included DNGs.
```

---

## 6. Exit criteria

- [ ] `dng_scan_reader.go` created, under 150 lines (split if needed)
- [ ] `ReadEXIFForScan` dispatches `.dng` to `readDNGScanFields`
- [ ] `dng_scan_reader_test.go` created and passes when `samples/IMGP7911.DNG` is present
- [ ] `samples/` is in `.gitignore`
- [ ] `go vet ./...` clean
- [ ] `wails build -platform windows/amd64` succeeds
- [ ] No changes to `ReadEXIF`, `verifyGPSInDNG`, `dng_gps_writer.go`, `dng_backup.go`, or any scanner file
- [ ] Misclassification risk note surfaced to the owner in the commit message
