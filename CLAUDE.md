# CLAUDE.md — GeoPhotoTagger

> This file is the single source of truth for Claude Code working on this project.
> Read it fully before making any change. Follow every rule without exception.

---

## 1. Project Overview

**GeoPhotoTagger** is a Go-based GPS metadata editor for photos, packaged as a **native desktop
application** using [Wails v2](https://wails.io). It opens a native Windows window (via WebView2),
helps the user tag photos that lack GPS coordinates by cross-referencing them against geolocated
images from phones/tablets or against imported GPS track data.

**Owner profile:**

* Running on **Windows 11**, Go installed via `winget install GoLang.Go`
* Comfortable with Python, TypeScript/JS, web frontends — not a Go expert
* **Go code must be heavily commented** — explain every function and non-obvious block
* Build command: `wails build -platform windows/amd64`
* Dev mode (live reload): `wails dev`
* Prerequisites: Go 1.21+, Wails CLI v2 (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`),
  Node.js 16+ (required by Wails toolchain), WebView2 (pre-installed on Windows 10/11)

**Core use case:**
The user shoots with a digital camera (DSLR/mirrorless) that has no GPS module. Meanwhile,
phones/tablets carried by the user or family members are recording geolocated photos. This app
cross-references timestamps to find the best GPS match and write coordinates into the camera photos.

---

## 2. Supported Formats

### Target photos (no GPS — the files to be tagged)
| Format | Thumbnail | GPS write | Notes |
|--------|-----------|-----------|-------|
| JPG    | ✅        | ✅        | Primary use case |
| DNG    | ✅        | ✅        | Adobe RAW format |

### Reference photos (with GPS — used as coordinate sources)
| Format | Thumbnail | GPS read | Notes |
|--------|-----------|----------|-------|
| JPG    | ✅        | ✅       | |
| PNG    | ✅        | ✅       | PNG rarely has GPS but supported |
| DNG    | ✅        | ✅       | |
| ARW    | ✅        | ✅       | Sony RAW format |
| HEIC   | ❌        | ✅       | **GPS data only — no thumbnail/preview** |

**HEIC limitation:** Go has no pure-Go HEVC decoder, and CGo-based solutions cause build issues
on Windows. HEIC files are read for EXIF GPS data only. The UI will show a placeholder icon
instead of a thumbnail for HEIC reference images.

### GPS data import (Option #2)
| Format | Notes |
|--------|-------|
| GPX    | Standard GPS Exchange Format |
| KML    | Google Earth / Maps export |
| CSV    | timestamp, latitude, longitude columns |

---

## 3. Hard Constraints

1. **No CGo.** The project must cross-compile cleanly. No C libraries.
2. **No ImageMagick.** No external binary dependencies at runtime.
3. **No external process spawning** for image operations.
4. **Single binary.** Everything embedded via `//go:embed`.
5. **Pure Go only** for all image decoding and EXIF manipulation.

---

## 4. Repository Structure

```
geo-photo-tagger/
├── main.go              Wails app entry point (wails.Run)
├── app.go               App struct — all public methods bound to JS frontend
├── scanner.go           Filesystem walk: scan target folder + reference folders
├── exif_reader.go       EXIF extraction: GPS coords, timestamps, camera model
├── exif_writer.go       GPS coordinate injection into target files
├── matcher.go           Time-based GPS matching engine + scoring
├── thumbnail.go         Thumbnail generation for JPG, PNG, DNG, ARW (not HEIC)
├── gpx_parser.go        GPX/KML/CSV track file parsing
├── types.go             Shared type definitions (no logic)
├── logger.go            slog-based structured logging setup
├── scanner_parallel.go  Parallel scan with worker pool
├── app_write.go         GPS write/undo/batch Wails-bound methods
├── exif_writer_helpers.go   decimal-to-DMS conversion for GPS write
├── wails.json           Wails config (name, version, author)
├── go.mod / go.sum
├── CLAUDE.md            This file — development guidelines
├── BUILD-INSTRUCTIONS.md  Step-by-step implementation phases
├── README.md
├── LICENSE
├── .gitignore
├── .github/
│   └── workflows/
│       ├── ci.yml       Build verification on push/PR
│       └── release.yml  Cross-platform binary release on tag push
└── static/              ← active frontend (embedded by main.go via //go:embed all:static)
    ├── index.html
    ├── css/
    │   ├── base.css       CSS variables, reset, typography
    │   ├── layout.css     Grid layout
    │   ├── table.css      Data table styles
    │   └── components.css Buttons, badges, toast, dialogs
    └── js/
        ├── app.js         Entry point — imports modules, wires init()
        ├── state.js       Shared state object (single source of truth)
        ├── api.js         All window.go.main.App.* calls (isolation layer)
        ├── components.js  showToast(), showConfirm(), placeholder icons
        ├── scan.js        Folder scanning orchestration
        ├── browse.js      Native folder picker wrapper
        ├── matcher_ui.js  GPS match results display + scoring filters
        ├── table.js       Results data table (target photos + matched GPS)
        ├── helpers.js     escapeHtml, formatDate shared utilities
        ├── preview.js     Thumbnail preview card (Zone C header)
        ├── filters.js     Match result filtering and sorting
        └── actions.js     GPS apply, batch apply, undo handlers
```

---

## 5. Architecture — Wails v2 Desktop App

### How it works

Wails replaces any `net/http` server entirely. There is no TCP port, no `localhost`,
no `fetch()` calls. Instead:

1. `main.go` embeds the `static/` directory with `//go:embed all:static`
2. Wails opens a native Windows window and loads `static/index.html` inside it
3. Wails injects `window.go` into the page — a JS object with one method per bound Go function
4. The frontend calls `window.go.main.App.MethodName(args)` which returns a **Promise**
5. Go return values (structs, maps) are automatically serialised to JS objects

### Go ↔ JavaScript bridge

| JS call | Go method | Purpose |
|---------|-----------|---------|
| `App.OpenFolderDialog()` | `(a *App) OpenFolderDialog()` | Native OS folder picker |
| `App.ScanTargetFolder(path)` | `(a *App) ScanTargetFolder(path string)` | Scan for photos without GPS |
| `App.AddReferenceFolder(path)` | `(a *App) AddReferenceFolder(path string)` | Add GPS reference source |
| `App.ImportGPSTrack(path)` | `(a *App) ImportGPSTrack(path string)` | Load GPX/KML/CSV file |
| `App.RunMatching(opts)` | `(a *App) RunMatching(opts MatchOptions)` | Execute GPS matching |
| `App.GetMatchResults()` | `(a *App) GetMatchResults()` | Poll matching progress |
| `App.GetThumbnail(path)` | `(a *App) GetThumbnail(path string)` | Base64 JPEG thumbnail |
| `App.ApplyGPS(targetPath, lat, lon)` | `(a *App) ApplyGPS(...)` | Write GPS to a single file |
| `App.ApplyAllMatches()` | `(a *App) ApplyAllMatches()` | Batch-apply all accepted matches |
| `App.GetScanStatus()` | `(a *App) GetScanStatus()` | Progress during scan/match |

### Thumbnails

* `GetThumbnail(path)` returns a base64-encoded JPEG string.
* Frontend sets: `img.src = "data:image/jpeg;base64," + result`
* For HEIC files, return an empty string — the frontend shows a placeholder icon.
* Supported for thumbnails: JPG, PNG, DNG, ARW only.

---

## 6. GPS Matching Algorithm

### Time-based scoring

The core matching logic compares the `DateTimeOriginal` EXIF field of each target photo
against the timestamps of all reference photos (and GPS track points).

**Scoring formula:**
```
timeDelta = abs(targetTime - referenceTime)

if timeDelta <= 1 min   → score = 100  (excellent)
if timeDelta <= 5 min   → score = 90   (very good)
if timeDelta <= 10 min  → score = 75   (good)
if timeDelta <= 30 min  → score = 50   (fair)
if timeDelta <= 60 min  → score = 25   (poor)
if timeDelta >  60 min  → score = 0    (no match)
```

### User-configurable thresholds

The UI exposes a **maximum time distance** filter with presets:
* 10 minutes (strict)
* 30 minutes (moderate — default)
* 60 minutes (relaxed)
* Custom value

### Match result structure

Each target photo gets a list of candidate matches, sorted by score descending.
The user can accept/reject each match before applying GPS data.

---

## 7. Frontend Architecture

### Module structure

`static/index.html` loads `js/app.js` via `<script type="module" src="/js/app.js">`.
CSS is split into 4 files loaded via `<link>` tags.

**`api.js` is the isolation layer** — it wraps all `window.go.main.App.*` calls;
no other module touches `window.go` directly.

### State object

```javascript
export const state = {
    targetFolder: null,        // Path to folder with untagged photos
    referenceFolders: [],      // Array of paths to GPS reference folders
    gpsTrackFiles: [],         // Array of imported GPX/KML/CSV paths
    targetPhotos: [],          // Scanned photos without GPS
    matchResults: null,        // Matching results from RunMatching()
    scanInProgress: false,     // Whether scan/match is running
    matchThreshold: 30,        // Max time distance in minutes
    selectedPhoto: null,       // Currently selected target photo
    acceptedMatches: new Map() // targetPath → { lat, lon, score, source }
};
```

---

## 8. UI Layout — 3-Zone Interface

```
┌──────────────────────────────────────────────────────────────────┐
│  ZONE A — Top Bar (full width, fixed)                            │
│  [Target Folder] [Browse] [+ Reference] [Import Track]          │
│  [Match All]  Threshold: [10|30|60|Custom]    [Apply Selected]  │
│  [━━━━━━━━━━━━━━━ progress bar (during scan/match) ━━━━━━━━━━] │
├────────────────────────────────┬─────────────────────────────────┤
│                                │                                 │
│  ZONE B — Target Photos        │  ZONE C — Match Details          │
│  (left panel, ~45% width)      │  (right panel, ~55% width)      │
│                                │                                 │
│  Table: filename, date/time,   │  Selected photo preview          │
│  status (matched/unmatched),   │  Best match thumbnail + score    │
│  score badge                   │  GPS coordinates                 │
│                                │  Map preview (optional)          │
│  Click row → show matches      │  Candidate list (ranked)         │
│  in Zone C                     │  [Accept] [Reject] per candidate │
│                                │                                 │
├────────────────────────────────┴─────────────────────────────────┤
│  STATUS BAR: 234 target photos │ 156 matched │ 78 unmatched     │
└──────────────────────────────────────────────────────────────────┘
```

---

## 9. Design Tokens

Light professional theme — same palette as dedup-photos for visual consistency.
Reference implementation: https://github.com/Rosca75/dedup-photos/tree/main/static/css

css
:root {
    /* Primary */
    --primary:          #1A3A5C;   /* Deep blue — buttons, headings, strong UI */
    --primary-light:    #4A90E2;   /* Sky blue  — accents, focus rings, links  */

    /* Status */
    --success:          #50C878;   /* Mint green — GPS applied, confirmed       */
    --danger:           #E74C3C;   /* Red        — errors, destructive actions  */
    --warning:          #F5A623;   /* Orange     — low-confidence matches       */

    /* Neutrals */
    --text:             #2D2D2D;   /* Dark grey  — main body text              */
    --text-light:       #6B6B6B;   /* Medium grey — secondary / muted text     */
    --border:           #E0E0E0;   /* Light grey — dividers, input borders     */
    --bg-subtle:        #F5F5F5;   /* Very light grey — alternating rows, cards */
    --bg:               #FFFFFF;   /* White — main page background             */
    --black:            #121212;   /* Deep black — headings                    */

    /* Score-quality badges */
    --excellent:        #1A3A5C;   /* Score >= 90 — deep blue (primary)        */
    --good:             #4A90E2;   /* Score 50-89 — sky blue (primary-light)   */
    --poor:             #F5A623;   /* Score < 50 — orange (warning)            */

    /* Typography */
    --font: 'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif;
}
```

---

## 10. Key Development Rules

1. **Read before writing.** Before modifying any file, read it first. Never assume its current contents.
2. **One file at a time.** Change one module, verify it is correct, then move on.
3. **No file over 150 lines.** If a file approaches this limit, split it.
4. **No function over 50 lines.** Extract helpers when functions grow.
5. **No HTTP, no `fetch()`.** All Go ↔ JS communication goes through `window.go.main.App.*` Promises.
6. **Asset paths have no `/static/` prefix.** Wails embeds `static/` and strips the prefix via `fs.Sub`. A file at `static/css/base.css` loads as `/css/base.css`.
7. **`static/` is the active frontend directory.** All frontend work happens here. There is no `frontend/` directory.
8. **State lives in `state.js` only.** Never store shared state as module-level variables in other files.
9. **Wails calls live in `api.js` only.** No other module calls `window.go.*` directly.
10. **Avoid modifying business logic files** (`matcher.go`, `exif_writer.go`, `gpx_parser.go`) unless the change is explicitly scoped and described in an improvement plan.
11. **Comment all Go code.** The owner is not a Go expert. Explain every non-obvious construct.
12. **Test after every change.** Run `wails dev` and verify in the native window.
13. **No CGo, no ImageMagick, no external binaries.** Pure Go only. This is a hard constraint.
14. **HEIC = GPS data only.** Never attempt to decode HEIC pixels or generate HEIC thumbnails. Read EXIF GPS only.
15. **Timestamps are sacred.** All time comparisons must account for timezone differences between devices. Normalize to UTC before comparing.

---

## 11. Go Packages — Approved Dependencies

| Purpose | Package | Notes |
|---------|---------|-------|
| EXIF read | `github.com/rwcarlsen/goexif/exif` | Standard pure-Go EXIF reader |
| EXIF write | `github.com/dsoprea/go-exif/v3` | GPS injection into JPEG/DNG |
| JPEG decode | `image/jpeg` (stdlib) | Thumbnail generation |
| PNG decode | `image/png` (stdlib) | Thumbnail generation |
| DNG/TIFF decode | `golang.org/x/image/tiff` | DNG is TIFF-based |
| ARW decode | `golang.org/x/image/tiff` | Sony ARW is TIFF-based |
| HEIC EXIF | `github.com/jdeng/goheif/heif` | ISOBMFF parsing for EXIF extraction only |
| Image resize | `golang.org/x/image/draw` | Thumbnail downscaling |
| GPX parsing | `encoding/xml` (stdlib) | GPX is XML |
| KML parsing | `encoding/xml` (stdlib) | KML is XML |
| CSV parsing | `encoding/csv` (stdlib) | CSV track import |

**Adding a new dependency requires explicit approval.** Do not `go get` without asking first.

---

## 12. EXIF Write Safety

Writing GPS data to photos is a **destructive operation**. Safety rules:

1. **Always create a backup** before modifying any file. Copy original to `<filename>.bak` in the same directory.
2. **Verify the write** by re-reading EXIF after write and confirming GPS matches expected values.
3. **Never modify the original timestamp.** Only GPS fields are written.
4. **Batch apply must be interruptible.** The user can cancel mid-batch.
5. **Undo support:** Keep `.bak` files until the user explicitly clears them.

---

## 13. Linux Build Notes (for CI)

Linux CI builds require the webkit2gtk-4.1 development library:

```bash
sudo apt-get install -y libwebkit2gtk-4.1-dev libgtk-3-dev
```

The Wails build command for Linux must include the webkit2_41 build tag:

```bash
wails build -tags webkit2_41 -platform linux/amd64
```

This is the fix for Ubuntu 24.04+ which ships webkit2gtk-4.1 instead of 4.0.
