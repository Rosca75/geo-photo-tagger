# DNG GPS Write — Implementation Plan

> **Purpose:** Claude Code instruction file for adding GPS write support to DNG files
> in GeoPhotoTagger. Based on binary analysis of real Pentax K-1 DNG files and research
> into the TIFF/EXIF specification and Go EXIF library ecosystem.
>
> **Scope:** Go backend only (`exif_writer.go` + new `dng_gps_writer.go`). No frontend changes.
> The existing JPEG GPS write path remains untouched.

---

## Executive Summary

DNG files are TIFF containers. Unlike JPEG (where EXIF lives in a discrete APP1 segment
that can be rebuilt independently), TIFF stores IFDs as linked structures scattered across
the file. Rebuilding the entire TIFF structure just to add GPS tags would risk corrupting
the 44 MB of raw sensor data, SubIFDs, MakerNotes, and proprietary Pentax metadata.

**The chosen approach is a surgical binary patch:**

1. Build a standalone GPS IFD blob (entries + RATIONAL value data) from scratch using
   `encoding/binary` — no third-party library needed for the write path.
2. Append this blob at the end of the DNG file.
3. Patch the 4-byte GPSInfoIFD pointer in IFD0 (at a known, computed byte offset)
   to point to the appended blob's file offset.

This is safe because the TIFF spec explicitly states that IFD offsets "may point anywhere
in the file, even after the image data." No other offsets in the file are modified.

---

## Research Findings

### DNG file structure (from real file analysis)

Both test files (Pentax K-1 Mark II DNG, ~44 MB) share this structure:

```
Offset 0:       TIFF header (8 bytes, little-endian, magic 42)
Offset 8:       IFD0 (39 entries, 482 bytes) — includes tag 34853 (GPSInfoIFD)
Offset ~646:    SubIFD pointers, DNG color matrices, maker data
Offset ~160878: SubIFD[0] (thumbnail IFD)
Offset ~161284: SubIFD[1] (raw sensor data IFD)
Offset ~161642: Exif IFD (22 entries)
Offset ~161984: GPS IFD
                  - WITH GPS (8108): 20 entries, ~246 bytes + value data
                  - WITHOUT GPS (8411): 1 entry (GPSVersionID only), 18 bytes
Offset ~162016+: Thumbnail strip data, then raw sensor data (~4.4 MB at end)
```

**Critical observation:** Both files already have a GPSInfoIFD pointer (tag 34853) in
IFD0 entry [23], with the value field at **byte offset 294**. Even the no-GPS file
points to a minimal GPS IFD containing just GPSVersionID. This means:
- We never need to *add* a new IFD0 entry — the pointer already exists.
- We only need to *update* its 4-byte value to point to our new GPS IFD.

### Why "append at EOF + patch pointer" is the right approach

| Approach | Risk | Verdict |
|----------|------|---------|
| Rebuild entire TIFF via dsoprea/go-exif | Extremely high: would re-serialize ALL IFDs, potentially corrupting SubIFDs, MakerNotes, DNG-specific tags, and 44 MB of raw data | **Rejected** |
| Overwrite existing GPS IFD in-place | No-GPS files have only 32 bytes of GPS IFD space; our minimal GPS IFD needs ~114 bytes; would overflow into thumbnail strip data | **Rejected** |
| Append GPS IFD at EOF + patch pointer | Only 2 writes: append ~120 bytes at EOF + patch 4 bytes at offset 294; zero risk to any existing data | **Chosen** |

### Why no third-party library is needed for the write path

The GPS IFD is a simple, well-specified TIFF structure: a 2-byte entry count, N×12-byte
entries (sorted by tag ID), a 4-byte next-IFD pointer (always 0), plus RATIONAL value
data immediately after. We control the exact byte layout. Using `encoding/binary` directly
is both safer and simpler than pulling the file through dsoprea's IFD builder (which was
designed for JPEG's self-contained EXIF segment, not for patching large TIFF files).

The *read* path (`ReadEXIF` via `rwcarlsen/goexif`) already works for DNG — no changes needed.

---

## Pre-flight Checklist

Before starting, read these files fully:
- `CLAUDE.md` (especially rules #1, #2, #5, #13)
- `exif_writer.go` (current JPEG GPS write pipeline)
- `exif_writer_helpers.go` (copyFile helper)
- `types.go` (EXIFData, GPSCoord)
- `exif_reader.go` (ReadEXIF — used for verification)

---

## Step 1 — Create `dng_gps_writer.go`

**Goal:** A new file containing the pure-Go DNG GPS write function.

**File:** `dng_gps_writer.go`

### Function: `writeGPSToDNG(path string, lat, lon float64) error`

This function performs 3 operations on the file:

#### Operation 1 — Read the TIFF header and locate the GPSInfoIFD pointer

```
1. Open the file for read+write (os.OpenFile with O_RDWR).
2. Read bytes 0–1: confirm byte order is 'II' (little-endian) or 'MM' (big-endian).
   Store as binary.LittleEndian or binary.BigEndian.
3. Read bytes 2–3: confirm TIFF magic = 42.
4. Read bytes 4–7: get IFD0 offset.
5. Seek to IFD0 offset. Read 2-byte entry count (N).
6. Walk N entries (each 12 bytes). For each entry, read the tag ID (first 2 bytes).
   When tag == 34853 (0x8825, GPSInfoIFD):
     - Record the byte offset of this entry's value field:
       gpsPointerOffset = ifd0Offset + 2 + (entryIndex * 12) + 8
     - Read the current 4-byte value (current GPS IFD offset) — for logging only.
     - Break out of the loop.
7. If tag 34853 was not found → return error: "DNG has no GPSInfoIFD tag in IFD0"
```

#### Operation 2 — Build the GPS IFD blob

Build a byte buffer containing a complete, self-contained GPS IFD with 5 entries:

| Entry | Tag | Type | Count | Value |
|-------|-----|------|-------|-------|
| 0 | 0x0000 GPSVersionID | BYTE (1) | 4 | [2, 3, 0, 0] (inline) |
| 1 | 0x0001 GPSLatitudeRef | ASCII (2) | 2 | "N\0" or "S\0" (inline) |
| 2 | 0x0002 GPSLatitude | RATIONAL (5) | 3 | → offset to 3×RATIONAL (24 bytes) |
| 3 | 0x0003 GPSLongitudeRef | ASCII (2) | 2 | "E\0" or "W\0" (inline) |
| 4 | 0x0004 GPSLongitude | RATIONAL (5) | 3 | → offset to 3×RATIONAL (24 bytes) |

**Layout in the blob:**

```
Bytes 0–1:    Entry count = 5 (uint16)
Bytes 2–13:   Entry 0 (GPSVersionID) — value inline: 02 03 00 00
Bytes 14–25:  Entry 1 (GPSLatitudeRef) — value inline: 'N' 00 00 00
Bytes 26–37:  Entry 2 (GPSLatitude) — value = offset to lat rationals
Bytes 38–49:  Entry 3 (GPSLongitudeRef) — value inline: 'E' 00 00 00
Bytes 50–61:  Entry 4 (GPSLongitude) — value = offset to lon rationals
Bytes 62–65:  Next IFD pointer = 0 (no next IFD)
Bytes 66–89:  Latitude RATIONAL data (3 × 8 bytes = 24 bytes)
Bytes 90–113: Longitude RATIONAL data (3 × 8 bytes = 24 bytes)
Total: 114 bytes
```

**Critical detail for RATIONAL offsets:** The value field of entries 2 and 4 must contain
the *absolute file offset* of the RATIONAL data, not an offset relative to the IFD.
Since we're appending the blob at the end of the file:

```
appendOffset = current file size (before appending)
latRationalsFileOffset  = appendOffset + 66
lonRationalsFileOffset  = appendOffset + 90
```

These absolute offsets are written into the value fields of entries 2 and 4.

**DMS conversion:** Reuse the existing `decimalToDMS()` function from `exif_writer.go`
to convert lat/lon to RATIONAL arrays. The function already returns
`[]exifcommon.Rational` — extract the Numerator/Denominator uint32 pairs from each.

**Important:** IFD entries MUST be sorted by tag ID in ascending order. The layout above
already satisfies this (0x0000, 0x0001, 0x0002, 0x0003, 0x0004).

#### Operation 3 — Append blob + patch pointer

```
1. Seek to end of file. Record the current position as appendOffset.
   Verify appendOffset is even (TIFF word-alignment). If odd, write one 0x00 pad byte
   and increment appendOffset by 1.
2. Write the 114-byte GPS IFD blob at appendOffset.
3. Seek to gpsPointerOffset (computed in Operation 1).
4. Write appendOffset as a 4-byte uint32 in the file's byte order.
5. Close the file. Sync/flush is handled by os.File.Close().
```

### Helper: `buildGPSIFDBlob(lat, lon float64, appendOffset int64, byteOrder binary.ByteOrder) []byte`

Separate the blob construction into a pure function that returns a `[]byte`.
This makes it easy to unit-test without touching the filesystem.

Parameters:
- `lat, lon`: decimal degrees
- `appendOffset`: the file offset where this blob will be written (needed to compute
  absolute offsets for RATIONAL data)
- `byteOrder`: the TIFF file's byte order

Returns: the complete 114-byte blob ready to be appended.

### Byte order awareness

Both test files are little-endian, but the code must handle big-endian too.
Use `binary.Write` with the detected `byteOrder` for all multi-byte values.

---

## Step 2 — Modify `exif_writer.go` to route DNG to the new writer

**Goal:** Replace the `"GPS write for DNG files not yet supported"` error with a call
to `writeGPSToDNG`.

**Read first:** `exif_writer.go`

**In `WriteGPS()`**, replace:

```go
if ext == ".dng" {
    return fmt.Errorf("GPS write for DNG files not yet supported")
}
```

With:

```go
if ext == ".dng" {
    if err := writeGPSToDNG(targetPath, lat, lon); err != nil {
        // Restore from backup on failure
        _ = copyFile(backupPath, targetPath)
        return fmt.Errorf("DNG GPS write failed: %w", err)
    }
    // Verify: re-read EXIF and confirm GPS matches
    result, err := ReadEXIF(targetPath)
    if err != nil || !result.HasGPS ||
        math.Abs(result.Latitude-lat) > 0.001 ||
        math.Abs(result.Longitude-lon) > 0.001 {
        _ = copyFile(backupPath, targetPath)
        return fmt.Errorf("DNG GPS verification failed for %s", filepath.Base(targetPath))
    }
    return nil
}
```

**Key points:**
- The backup has already been created (line 33 in the current code).
- The verification step re-reads EXIF using the existing `ReadEXIF()` which already
  works with DNG/TIFF files via `rwcarlsen/goexif`.
- On any failure, the backup is restored before returning the error.
- The tolerance of 0.001° (~110 m) is the same as for JPEG — it accounts for
  DMS rounding.

---

## Step 3 — Detailed implementation of `buildGPSIFDBlob`

Here is the exact byte construction logic. This is the core of the feature.

```go
func buildGPSIFDBlob(lat, lon float64, appendOffset int64, byteOrder binary.ByteOrder) []byte {
    buf := new(bytes.Buffer)

    // Entry count: 5 entries
    binary.Write(buf, byteOrder, uint16(5))

    // --- Helper to write one 12-byte IFD entry ---
    writeEntry := func(tag, typeID uint16, count uint32, value uint32) {
        binary.Write(buf, byteOrder, tag)
        binary.Write(buf, byteOrder, typeID)
        binary.Write(buf, byteOrder, count)
        binary.Write(buf, byteOrder, value)
    }

    // Compute absolute file offsets for the RATIONAL value data.
    // The IFD header is: 2 (count) + 5*12 (entries) + 4 (next-IFD) = 66 bytes.
    // Lat rationals start at blob offset 66, lon at 90.
    latDataOffset := uint32(appendOffset) + 66
    lonDataOffset := uint32(appendOffset) + 90

    // Determine hemisphere references
    latRef := [4]byte{'N', 0, 0, 0}
    absLat := lat
    if lat < 0 {
        latRef = [4]byte{'S', 0, 0, 0}
        absLat = -lat
    }
    lonRef := [4]byte{'E', 0, 0, 0}
    absLon := lon
    if lon < 0 {
        lonRef = [4]byte{'W', 0, 0, 0}
        absLon = -lon
    }

    // --- Entry 0: GPSVersionID (tag 0, BYTE, count 4, value inline) ---
    // Value: 02 03 00 00 — EXIF GPS version 2.3.0.0
    // Inline: 4 bytes fit in the value field.
    // Encode as uint32 with correct byte order.
    var versionValue uint32
    if byteOrder == binary.LittleEndian {
        versionValue = 0x00000302  // bytes: 02 03 00 00
    } else {
        versionValue = 0x02030000
    }
    writeEntry(0x0000, 1, 4, versionValue) // BYTE=1

    // --- Entry 1: GPSLatitudeRef (tag 1, ASCII, count 2, value inline) ---
    var latRefVal uint32
    if byteOrder == binary.LittleEndian {
        latRefVal = uint32(latRef[0]) | uint32(latRef[1])<<8
    } else {
        latRefVal = uint32(latRef[0])<<24 | uint32(latRef[1])<<16
    }
    writeEntry(0x0001, 2, 2, latRefVal) // ASCII=2

    // --- Entry 2: GPSLatitude (tag 2, RATIONAL, count 3, value = offset) ---
    writeEntry(0x0002, 5, 3, latDataOffset) // RATIONAL=5

    // --- Entry 3: GPSLongitudeRef (tag 3, ASCII, count 2, value inline) ---
    var lonRefVal uint32
    if byteOrder == binary.LittleEndian {
        lonRefVal = uint32(lonRef[0]) | uint32(lonRef[1])<<8
    } else {
        lonRefVal = uint32(lonRef[0])<<24 | uint32(lonRef[1])<<16
    }
    writeEntry(0x0003, 2, 2, lonRefVal) // ASCII=2

    // --- Entry 4: GPSLongitude (tag 4, RATIONAL, count 3, value = offset) ---
    writeEntry(0x0004, 5, 3, lonDataOffset) // RATIONAL=5

    // --- Next IFD pointer: 0 (no next IFD) ---
    binary.Write(buf, byteOrder, uint32(0))

    // --- RATIONAL value data for latitude (3 × 2 uint32 = 24 bytes) ---
    writeRationals(buf, byteOrder, absLat)

    // --- RATIONAL value data for longitude (3 × 2 uint32 = 24 bytes) ---
    writeRationals(buf, byteOrder, absLon)

    return buf.Bytes()
}

// writeRationals writes 3 RATIONAL values (deg, min, sec) for a decimal-degree value.
// Each RATIONAL is a numerator/denominator uint32 pair.
// Seconds use denominator 10000 for sub-second precision (same as JPEG path).
func writeRationals(buf *bytes.Buffer, byteOrder binary.ByteOrder, decimal float64) {
    deg := int(decimal)
    mf := (decimal - float64(deg)) * 60
    min := int(mf)
    sec := (mf - float64(min)) * 60

    // Degrees: exact integer / 1
    binary.Write(buf, byteOrder, uint32(deg))
    binary.Write(buf, byteOrder, uint32(1))
    // Minutes: exact integer / 1
    binary.Write(buf, byteOrder, uint32(min))
    binary.Write(buf, byteOrder, uint32(1))
    // Seconds: scaled by 10000 for precision / 10000
    binary.Write(buf, byteOrder, uint32(int(sec*10000)))
    binary.Write(buf, byteOrder, uint32(10000))
}
```

---

## Step 4 — Implement `writeGPSToDNG`

```go
func writeGPSToDNG(path string, lat, lon float64) error {
    // Open the file for in-place read+write.
    f, err := os.OpenFile(path, os.O_RDWR, 0)
    if err != nil {
        return fmt.Errorf("open DNG: %w", err)
    }
    defer f.Close()

    // --- Read TIFF header ---
    var header [8]byte
    if _, err := io.ReadFull(f, header[:]); err != nil {
        return fmt.Errorf("read TIFF header: %w", err)
    }

    var byteOrder binary.ByteOrder
    switch string(header[0:2]) {
    case "II":
        byteOrder = binary.LittleEndian
    case "MM":
        byteOrder = binary.BigEndian
    default:
        return fmt.Errorf("invalid TIFF byte order: %x %x", header[0], header[1])
    }

    magic := byteOrder.Uint16(header[2:4])
    if magic != 42 {
        return fmt.Errorf("invalid TIFF magic: %d (expected 42)", magic)
    }

    ifd0Offset := byteOrder.Uint32(header[4:8])

    // --- Walk IFD0 to find the GPSInfoIFD pointer ---
    if _, err := f.Seek(int64(ifd0Offset), io.SeekStart); err != nil {
        return fmt.Errorf("seek to IFD0: %w", err)
    }

    var entryCount uint16
    if err := binary.Read(f, byteOrder, &entryCount); err != nil {
        return fmt.Errorf("read IFD0 entry count: %w", err)
    }

    gpsPointerOffset := int64(-1)
    for i := uint16(0); i < entryCount; i++ {
        entryOffset := int64(ifd0Offset) + 2 + int64(i)*12
        if _, err := f.Seek(entryOffset, io.SeekStart); err != nil {
            return fmt.Errorf("seek to IFD0 entry %d: %w", i, err)
        }
        var tag uint16
        if err := binary.Read(f, byteOrder, &tag); err != nil {
            return fmt.Errorf("read IFD0 tag %d: %w", i, err)
        }
        if tag == 0x8825 { // GPSInfoIFD
            gpsPointerOffset = entryOffset + 8 // value field is at bytes 8-11 of the entry
            break
        }
    }
    if gpsPointerOffset < 0 {
        return fmt.Errorf("DNG file has no GPSInfoIFD tag (0x8825) in IFD0")
    }

    // --- Compute append offset (must be word-aligned) ---
    fileEnd, err := f.Seek(0, io.SeekEnd)
    if err != nil {
        return fmt.Errorf("seek to EOF: %w", err)
    }
    appendOffset := fileEnd
    if appendOffset%2 != 0 {
        // Write one padding byte for TIFF word-alignment
        if _, err := f.Write([]byte{0x00}); err != nil {
            return fmt.Errorf("write alignment pad: %w", err)
        }
        appendOffset++
    }

    // --- Build and append the GPS IFD blob ---
    blob := buildGPSIFDBlob(lat, lon, appendOffset, byteOrder)
    if _, err := f.Write(blob); err != nil {
        return fmt.Errorf("write GPS IFD blob: %w", err)
    }

    // --- Patch the GPSInfoIFD pointer in IFD0 ---
    if _, err := f.Seek(gpsPointerOffset, io.SeekStart); err != nil {
        return fmt.Errorf("seek to GPS pointer: %w", err)
    }
    if err := binary.Write(f, byteOrder, uint32(appendOffset)); err != nil {
        return fmt.Errorf("write GPS pointer: %w", err)
    }

    return nil
}
```

**Required imports for `dng_gps_writer.go`:**
```go
import (
    "bytes"
    "encoding/binary"
    "fmt"
    "io"
    "os"
)
```

---

## Step 5 — Verification and edge cases

### Verification strategy

After `writeGPSToDNG` completes, `WriteGPS` in `exif_writer.go` calls `ReadEXIF(path)`
which uses `rwcarlsen/goexif`. This library opens the file as a standard TIFF, follows
the GPSInfoIFD pointer (now pointing to our appended blob), and reads the GPS tags.
If lat/lon don't match within 0.001° tolerance, the backup is restored.

**This verification path already exists and works for DNG files** — no changes needed.

### Edge cases to handle

1. **File has no GPSInfoIFD tag at all:** Return a clear error. This would mean the DNG
   was produced by very unusual software. All standard camera DNGs include this tag.

2. **File is read-only:** Already handled by the existing permission check in `WriteGPS()`
   (line 29–31 in current `exif_writer.go`).

3. **File > 4 GB:** The append offset would overflow a uint32. DNG files from consumer
   cameras are typically 20–60 MB, so this is theoretical. Add a guard:
   ```go
   if appendOffset > math.MaxUint32 {
       return fmt.Errorf("DNG file too large for GPS write (%d bytes)", appendOffset)
   }
   ```

4. **Repeat writes:** If the user applies GPS, then applies different GPS, the file grows
   by ~114 bytes each time (old GPS IFD blob becomes orphaned but harmless — the pointer
   is updated to the newest). This is identical to how ExifTool handles TIFF GPS writes.
   The backup/restore mechanism handles undo.

5. **Big-endian DNG files:** Uncommon but possible (Leica, Phase One). The code uses the
   detected `byteOrder` throughout, so this is handled automatically.

---

## Step 6 — Update `CLAUDE.md`

Add to the **Go ↔ JavaScript bridge** table:
- No new bridge methods needed — `ApplyGPS` already calls `WriteGPS` which now handles DNG.

Update section 2 (**Supported Formats**) to confirm DNG GPS write is implemented:
- Change the DNG target row from "GPS write: ✅" with a note to just "GPS write: ✅"
  (remove any "not yet supported" caveat if present in comments).

Update section 4 (**Repository Structure**):
- Add `dng_gps_writer.go` with description: "DNG GPS write — binary TIFF patch (append IFD at EOF)"

---

## File Change Summary

| File | Action | Description |
|------|--------|-------------|
| `dng_gps_writer.go` | **CREATE** | New file: `writeGPSToDNG`, `buildGPSIFDBlob`, `writeRationals` |
| `exif_writer.go` | **MODIFY** | Replace DNG error stub with call to `writeGPSToDNG` + verify |
| `CLAUDE.md` | **MODIFY** | Update format table, add file to repo structure |

**No new dependencies.** Only `encoding/binary`, `bytes`, `io`, `os`, `fmt`, `math` (all stdlib).

---

## Testing Procedure

1. `wails dev` — confirm the app compiles.
2. Use IMGP8411.DNG (no GPS) as target, IMGP8108.DNG (with GPS) as reference.
3. Run matching → should find a match for IMGP8411 based on timestamp proximity.
4. Accept the match and click "Apply GPS".
5. **Expected:** Toast shows "GPS applied: IMGP8411.DNG". No error.
6. **Verify externally:** Open the modified IMGP8411.DNG in any EXIF viewer (Windows
   properties → Details tab, or ExifTool) and confirm GPS coordinates are present.
7. Test "Undo" — should restore from .bak file.
8. Test with a JPEG file to confirm the existing path still works.
