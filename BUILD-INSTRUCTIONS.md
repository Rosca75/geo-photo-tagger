# BUILD-INSTRUCTIONS.md — GeoPhotoTagger

> Step-by-step implementation phases. Run each phase in Claude Code as a separate task.
> Each phase must compile and run before moving to the next.
> Always read CLAUDE.md first — it contains all architectural rules and constraints.

---

## Phase 0 — Project Scaffolding

**Goal:** Wails project compiles and opens an empty window.

### Steps

1. Initialize the Go module:
   ```bash
   go mod init github.com/Rosca75/geo-photo-tagger
   ```

2. Install Wails CLI (if not installed):
   ```bash
   go install github.com/wailsapp/wails/v2/cmd/wails@latest
   ```

3. Create `main.go`:
   - Import `embed` and `wails` packages
   - `//go:embed all:static` to embed the frontend
   - `wails.Run()` with window 1400×900px, minimum 1000×700px
   - Bind `App` struct

4. Create `app.go`:
   - Define `App` struct with `ctx context.Context`
   - `startup(ctx)` method to store context
   - Stub all public methods from CLAUDE.md §5 (return empty/placeholder values)

5. Create `types.go`:
   - Define all shared types: `TargetPhoto`, `ReferencePhoto`, `GPSCoord`,
     `MatchResult`, `MatchCandidate`, `MatchOptions`, `ScanStatus`

6. Create placeholder `static/index.html` with a "GeoPhotoTagger" title and
   a single `<h1>` element.

7. Create `wails.json`:
   ```json
   {
     "name": "GeoPhotoTagger",
     "outputfilename": "geo-photo-tagger",
     "frontend:install": "",
     "frontend:build": "",
     "frontend:dev:watcher": "",
     "frontend:dev:serverUrl": "",
     "author": {
       "name": "Rosca75"
     }
   }
   ```

8. Verify: `wails dev` opens a window showing "GeoPhotoTagger".

---

## Phase 1 — Target Folder Scanning

**Goal:** User can browse for a folder, scan it, and see a list of photos without GPS data.

### Steps

1. Implement `scanner.go`:
   - `ScanForTargetPhotos(folderPath string) ([]TargetPhoto, error)`
   - Recursive walk, filter by extension: `.jpg`, `.jpeg`, `.dng` (case-insensitive)
   - For each file: read EXIF, check if GPS latitude/longitude are present
   - If no GPS → add to results as `TargetPhoto`
   - Extract `DateTimeOriginal` from EXIF for each file
   - **Comment every step heavily**

2. Implement `exif_reader.go`:
   - `ReadEXIF(path string) (*EXIFData, error)` — returns struct with:
     - `HasGPS bool`
     - `Latitude, Longitude float64` (zero if no GPS)
     - `DateTimeOriginal time.Time`
     - `CameraModel string`
   - Handle missing EXIF gracefully (log warning, skip file)

3. Implement `thumbnail.go`:
   - `GenerateThumbnail(path string, maxSize int) (string, error)`
   - Decode JPG/PNG natively, DNG/ARW via `x/image/tiff`
   - Resize to fit within `maxSize × maxSize` (default 200px)
   - Encode as JPEG, return base64 string
   - For unsupported formats (HEIC), return `""` (empty string)

4. Wire up in `app.go`:
   - `OpenFolderDialog()` → use `runtime.OpenDirectoryDialog()`
   - `ScanTargetFolder(path)` → call scanner, store results in App state
   - `GetThumbnail(path)` → call thumbnail generator
   - `GetScanStatus()` → return progress info

5. Build minimal frontend:
   - `static/index.html` — basic layout with top bar + table area
   - `static/js/app.js` — entry point
   - `static/js/state.js` — state object
   - `static/js/api.js` — wrap `App.OpenFolderDialog`, `App.ScanTargetFolder`, `App.GetThumbnail`
   - `static/js/scan.js` — browse button handler, trigger scan, poll status
   - `static/js/table.js` — render target photos in a table (filename, date, status)

6. Verify: Browse → select folder → see table of photos without GPS.

---

## Phase 2 — Reference Folder Scanning

**Goal:** User can add one or more reference folders containing geolocated photos.

### Steps

1. Extend `scanner.go`:
   - `ScanForReferencePhotos(folderPath string) ([]ReferencePhoto, error)`
   - Recursive walk, filter: `.jpg`, `.jpeg`, `.png`, `.dng`, `.arw`, `.heic`, `.heif`
   - For each file: read EXIF, require GPS to be present
   - If GPS present → add to results as `ReferencePhoto` with coords + timestamp
   - For HEIC: use `goheif/heif` ISOBMFF parser to extract EXIF GPS only
   - **Do not attempt to decode HEIC image pixels**

2. Implement HEIC EXIF extraction in `exif_reader.go`:
   - `ReadHEICExif(path string) (*EXIFData, error)`
   - Parse ISOBMFF boxes to find EXIF data
   - Extract GPS and DateTimeOriginal
   - Return error if no GPS found (skip file)

3. Wire up in `app.go`:
   - `AddReferenceFolder(path)` → scan folder, append results to reference list
   - Support multiple reference folders (accumulate, don't replace)

4. Update frontend:
   - Add "Reference Folders" section in top bar
   - Button: `[+ Add Reference Folder]`
   - Show list of added reference folders with photo counts
   - Allow removing a reference folder from the list

5. Verify: Add 1–2 reference folders → see count of geolocated reference photos.

---

## Phase 3 — GPS Track Import (Option #2)

**Goal:** User can import GPX, KML, or CSV files as GPS data sources.

### Steps

1. Implement `gpx_parser.go`:
   - `ParseGPXFile(path string) ([]GPSTrackPoint, error)` — XML parse
   - `ParseKMLFile(path string) ([]GPSTrackPoint, error)` — XML parse
   - `ParseCSVFile(path string) ([]GPSTrackPoint, error)` — expect columns: timestamp, lat, lon
   - `GPSTrackPoint { Time time.Time; Lat, Lon float64 }`
   - Handle timezone parsing carefully (GPX uses ISO 8601 / UTC)

2. Wire up in `app.go`:
   - `ImportGPSTrack(path)` → detect format by extension, parse, store track points
   - `OpenFileDialog()` → native file picker filtered to `.gpx`, `.kml`, `.csv`

3. Update frontend:
   - Button: `[Import GPS Track]`
   - Show list of imported tracks with point counts
   - Allow removing a track from the list

4. Verify: Import a GPX file → see track point count displayed.

---

## Phase 4 — GPS Matching Engine

**Goal:** Match target photos against reference photos and track points by timestamp.

### Steps

1. Implement `matcher.go`:
   - `RunMatching(targets []TargetPhoto, refs []ReferencePhoto, tracks []GPSTrackPoint, opts MatchOptions) []MatchResult`
   - For each target photo:
     a. Find all reference photos within `opts.MaxTimeDelta`
     b. Find all track points within `opts.MaxTimeDelta`
     c. Score each candidate (see CLAUDE.md §6)
     d. Sort candidates by score descending
     e. Build `MatchResult` with best match + all candidates
   - **Normalize all timestamps to UTC before comparison**
   - For track data: if two track points bracket the target time, **interpolate GPS coordinates**
     using linear interpolation between the two closest points

2. Wire up in `app.go`:
   - `RunMatching(opts)` → launch matching in background goroutine
   - `GetMatchResults()` → return current results (poll-friendly)
   - Progress reporting: emit percentage during matching

3. Update frontend:
   - `[Match All]` button in top bar
   - Threshold selector: `[10 min] [30 min] [60 min] [Custom]`
   - Progress bar during matching
   - Update table: add score column, color-coded badges
   - Click a row → show match details in Zone C

4. Verify: Scan target + reference → Match → see scored results.

---

## Phase 5 — Match Review UI

**Goal:** User can review each match, see candidate details, accept/reject.

### Steps

1. Implement `static/js/matcher_ui.js`:
   - Right panel (Zone C) shows details for selected target photo:
     - Target photo thumbnail + metadata
     - Best match: thumbnail + GPS coords + score + time delta
     - All candidates: ranked list with accept/reject buttons
   - HEIC candidates show placeholder icon + "(HEIC — no preview)" label

2. Implement `static/js/preview.js`:
   - Large thumbnail preview (400px max)
   - Metadata overlay: filename, date, camera model
   - For matched photos: show GPS coordinates

3. Implement `static/js/filters.js`:
   - Filter results by: matched/unmatched/all
   - Filter by score threshold (slider or buttons)
   - Filter by time distance
   - Sort by: filename, date, score, status

4. Update `state.js`:
   - `acceptedMatches` Map tracking user decisions
   - Track per-photo accept/reject status

5. Verify: Click through photos, accept/reject matches, filters work.

---

## Phase 6 — GPS Write + Undo

**Goal:** Apply GPS coordinates to accepted target photos.

### Steps

1. Implement `exif_writer.go`:
   - `WriteGPS(targetPath string, lat, lon float64) error`
   - Create `.bak` backup before any modification
   - Write GPS IFD into EXIF (GPSLatitude, GPSLongitude, GPSLatitudeRef, GPSLongitudeRef)
   - Re-read EXIF after write to verify GPS was written correctly
   - **Never modify DateTimeOriginal or any non-GPS EXIF field**

2. Wire up in `app.go`:
   - `ApplyGPS(targetPath, lat, lon)` → single file write
   - `ApplyAllMatches()` → batch write all accepted matches
   - `UndoGPS(targetPath)` → restore from `.bak` file
   - `ClearBackups()` → delete all `.bak` files (with confirmation)
   - Batch apply must be cancellable (check context between files)

3. Update frontend:
   - `[Apply GPS]` button per match in Zone C
   - `[Apply All Accepted]` button in top bar
   - `[Undo]` button per applied photo
   - Status badges: "Applied ✓", "Pending", "No match"
   - Progress bar for batch apply
   - Confirmation dialog before batch apply

4. Verify: Apply GPS to a single photo → verify with ExifTool or similar.
   Apply batch → verify. Undo → verify backup restored.

---

## Phase 7 — Polish + Full CSS

**Goal:** Professional UI matching dedup-photos dark theme.

### Steps

1. `static/css/base.css` — design tokens from CLAUDE.md §9, reset, typography
2. `static/css/layout.css` — CSS Grid 3-zone layout
3. `static/css/table.css` — data table with alternating rows, score badges
4. `static/css/components.css` — buttons, toasts, dialogs, progress bars, folder chips

5. Score badges with color coding:
   - Score ≥ 90: purple badge (excellent)
   - Score 50–89: cyan badge (good)
   - Score < 50: orange badge (poor)

6. HEIC placeholder styling: grey box with camera icon + "HEIC" label

7. Responsive resize handles between Zone B and Zone C

8. Verify: Full visual review, all interactions smooth.

---

## Phase 8 — GitHub Actions CI/CD

**Goal:** Automated builds and releases.

### Steps

1. `.github/workflows/ci.yml`:
   - Trigger: push to `main`, pull requests
   - Matrix: `windows/amd64`, `linux/amd64`
   - Linux: install `libwebkit2gtk-4.1-dev`, build with `-tags webkit2_41`
   - Windows: standard `wails build`
   - Upload build artifacts

2. `.github/workflows/release.yml`:
   - Trigger: tag push (`v*`)
   - Build both platforms
   - Create GitHub release with binaries
   - Name binaries: `geo-photo-tagger-windows-amd64.exe`, `geo-photo-tagger-linux-amd64`

3. Verify: Push a tag → release created with both binaries.

---

## Phase 9 — Testing + Edge Cases

**Goal:** Handle real-world messiness.

### Steps

1. Timezone edge cases:
   - Camera set to local time (no timezone in EXIF)
   - Phone photos in UTC
   - Travel across timezones mid-shoot
   - Add a "timezone offset" option for manual correction

2. DNG/ARW specifics:
   - Some DNG files have huge embedded previews — use those for thumbnails
   - ARW files may have multiple IFDs — find the right one

3. Large dataset handling:
   - 10,000+ target photos
   - 50,000+ reference photos
   - Progress reporting with ETA
   - Efficient matching (sort references by time, use binary search)

4. Error handling:
   - Corrupted EXIF
   - Read-only files (can't write GPS)
   - Missing DateTimeOriginal (skip with warning)
   - Disk full during backup creation
