# CHANGE REQUEST — 2026-04 BATCH 2

> **Scope:** five independent change requests bundled into one document.
> Each CR is self-contained and can be landed in its own commit.
> Follow the order — CR1 → CR5 — unless you have a reason to reorder.
>
> **Read before starting:**
> - `CLAUDE.md` in full (especially §10 rules #3, #4, #9, #10, #14, #17, #18, **#20**).
> - The files named in the "Files to read / touch" line of each CR — read them first, do not assume contents.
> - Rule #20 is load-bearing for CR2: the Go `ClearBackups` / `UndoGPS` / `WriteGPS` triplet and the backup code paths (`dng_backup*.go`, `dng_gps_verify.go`, `dng_gps_writer.go`) are **no-touch**. CR2 only adds *callers*, never modifies those functions.
>
> **Verification after every CR:**
> - `go vet ./...` — clean
> - `go test ./...` — passing
> - `wails dev` — manual smoke test in the native window
>
> Commit boundary: one commit per CR. Do not mix CRs.

---

## CR1 — DNG thumbnails missing in "Same Source" mode

### Symptom
User picks **Same source** radio → clicks **Search for GPS match** → Zone C opens a target DNG. Candidate cards for DNG references show no hover thumbnail. JPG reference candidates work fine in the same view. The target-photo preview at the top of Zone C is also blank for large DNGs.

### Root cause
`thumbnail.go::loadRawEmbeddedPreview` uses `github.com/rwcarlsen/goexif/exif.Decode(f)` on the whole DNG. For Pentax-class DNGs (IMGP* fixture: GPS IFD at offset `0x2C4D84C`, ~46 MB into the file) the same failure mode that forced `dng_scan_reader.go` into existence also breaks the thumbnail path:

1. `goexif` cannot reliably locate the preview thumbnail reference in these DNGs.
2. The fallback `decodeImageFile` uses `golang.org/x/image/tiff`, which does not support the compressed raw image data in a typical DNG.
3. Result: both paths return errors → `GenerateThumbnail` returns `""` → frontend treats it as "no preview" and nothing renders.

Same-source mode exposes this more than External refs because both target *and* candidate are typically DNGs from the same camera — every reference in the candidate list triggers the same broken path.

### Fix — new file `dng_thumbnail_reader.go`

Mirror the direct-binary TIFF approach already proven in `dng_scan_reader.go`. Walk the TIFF IFDs ourselves and extract the largest embedded JPEG preview. Pure Go, no goexif, no full-file scan.

**Reuse, do not duplicate:**
- `readTIFFHeader` — already in `dng_gps_writer.go`.
- Do not add a new dependency. This is the whole point of the direct-binary approach.

**Algorithm (single function, ≤ 50 lines including comments):**

1. Open the file.
2. Call `readTIFFHeader` to get the byte order and IFD0 offset.
3. Walk IFD0:
   - Tag `0x014A` (`SubIFDs`, typeID `LONG`/`IFD`) — collect each sub-IFD offset. SubIFDs are where modern DNGs put their largest previews.
   - Tag `0x0201` (`JPEGInterchangeFormat`) — legacy inline thumbnail offset in IFD0 itself.
   - Tag `0x0202` (`JPEGInterchangeFormatLength`) — matching length.
4. For each SubIFD, read its entries and collect `(JPEGInterchangeFormat, JPEGInterchangeFormatLength)` pairs. Also read `ImageWidth (0x0100)` / `ImageLength (0x0101)` when present so you can pick the **largest** JPEG preview (best quality for the 200 px or 64 px target size).
5. Seek to the chosen offset, read `length` bytes, `jpeg.Decode` them. Return the `image.Image`.
6. Return a typed error when no JPEG preview is found — the caller will fall back to the existing TIFF decode path, which still works for small DNGs that don't hit the goexif bug.

**Wire it into `thumbnail.go`:**

Replace the body of `loadRawEmbeddedPreview` for `.dng` only — keep the existing behaviour for `.arw` (Sony RAW, different structure, current path works). The cleanest split:

```go
// In thumbnail.go, GenerateThumbnail:
if ext == ".dng" {
    img, err = loadDNGEmbeddedPreview(path)       // new — direct TIFF walk
    if err != nil {
        img, err = decodeImageFile(path)          // existing fallback
    }
} else if ext == ".arw" {
    img, err = loadRawEmbeddedPreview(path)       // existing (unchanged)
    if err != nil {
        img, err = decodeImageFile(path)
    }
} else {
    img, err = decodeImageFile(path)
}
```

Keep `loadRawEmbeddedPreview` in place for ARW. Do not delete it.

### Performance note
For a 46 MB Pentax DNG, reading the embedded JPEG preview via direct TIFF walk costs roughly: 8-byte header read + one 512-byte read for IFD0 entries + one 512-byte read per SubIFD (typically 1–3) + one streamed read of the preview bytes themselves (~100–400 KB). That is an order of magnitude less I/O than the current goexif path and does not depend on where the GPS IFD lives.

### Files to read / touch
- **Read:** `dng_scan_reader.go` (reference pattern), `dng_gps_writer.go` (for `readTIFFHeader`), `thumbnail.go` (current pipeline), `types.go` (no changes expected).
- **Create:** `dng_thumbnail_reader.go`.
- **Modify:** `thumbnail.go` only — the `GenerateThumbnail` dispatch block shown above. Nothing else.

### Verification
1. `go vet ./...` clean; `go test ./...` passing.
2. `wails dev`, scan a folder containing large Pentax-class DNGs (>40 MB) without GPS.
3. Click "Same source" → "Search for GPS match".
4. Click a target row that has DNG candidates → Zone C renders, target preview shows a thumbnail, hovering a DNG candidate card shows the 64 px preview.
5. The `samples/IMGP8411.DNG` fixture mentioned in CLAUDE.md §13 is the obvious test file if present.

### Out of scope (do not touch)
- HEIC: CLAUDE.md rule #14 — continue returning `""`, no placeholder round-trip.
- ARW: the existing `loadRawEmbeddedPreview` path is fine. Do not "improve" it in this commit.
- The scan path (`ReadEXIFForScan` → `readDNGScanFields`). CR1 is thumbnails only.

---

## CR2 — `ClearAllBackups` is unreachable from the UI, and backups are never swept on exit

### Symptom
After applying GPS to photos, the app leaves behind `.bak` (JPEG) and `.bak.json` (DNG) sidecar files. Both Go functions exist (`app_write.go::ClearAllBackups`) and are wrapped in `api.js::clearAllBackups`, but nothing in the frontend ever calls it. When the user closes the app, backups stay on disk forever. The user wants:

1. A visible button in the UI that clears all backups for the current source folder.
2. An automatic sweep when the user closes the application.

### Hard constraint — CLAUDE.md §10 rule #20
The backup-safety triplet (`WriteGPS` / `UndoGPS` / `ClearBackups` in `exif_writer.go`, plus `dng_backup.go`, `dng_backup_undo.go`, `dng_gps_verify.go`, `dng_gps_writer.go`) **must not be modified** by this CR. CR2 only adds:

- A new **caller** in the frontend (button).
- A new **caller** in `main.go` / `app.go` (shutdown hook).

If you find yourself editing any file in that triplet, stop and re-read this CR.

### Part A — Frontend button (View zone)

**Location:** Group 3 ("View") in `static/index.html`. Add next to the existing `#btn-apply-all`, but on the left side of the row. The button lives outside the Apply/Undo critical path so accidental clicks on "Clear backups" cannot corrupt a live apply.

**Markup (insert after the three filter buttons, before `#btn-apply-all`):**

```html
<button id="btn-clear-backups" class="btn btn-sm btn-secondary"
        title="Delete all .bak and .bak.json backup files for the current source folder.
This removes the ability to Undo already-applied GPS writes."
        style="margin-left:var(--space-sm)">
    Clear backups
</button>
```

Keep `#btn-apply-all` with `margin-left:auto` so Clear backups sits next to the filters and Apply GPS stays right-aligned.

**Wiring:** in `static/js/actions.js::initActions`, add a second listener:

```js
const clearBtn = document.getElementById('btn-clear-backups');
if (clearBtn) clearBtn.addEventListener('click', handleClearBackups);
```

Add the handler (new function, ≤ 50 lines):

1. Read `state.targetFolder` from `state.js`. If it's empty/null → show a toast "No source folder scanned" and return.
2. Show the existing `showConfirm` dialog with a clear, two-paragraph warning: **"This deletes every .bak and .bak.json file under {folder}. After this, Undo is no longer possible for photos already tagged. Continue?"**
3. On confirm → disable the button, change text to "Clearing…", call `clearAllBackups()` (already wrapped in `api.js`).
4. On success → toast `"Cleared N backup files."` (use the count returned by the Go method).
5. On error → toast error; do not throw; re-enable the button.

Do not add a new confirm helper — reuse the one `handleApplySingle` uses in `actions.js`.

**`actions.js` will exceed 150 lines with this addition — check the final line count.** If it crosses 150, extract `buildApplyWarning`, `showConfirm`, and the toast helper into `static/js/actions_shared.js` and import them — CLAUDE.md rule #3 is non-negotiable.

### Part B — Sweep on exit (Wails shutdown hook)

**Location:** `main.go`.

Wails v2 exposes `OnBeforeClose` (can cancel the close) and `OnShutdown` (fires after the window has closed, no return value). **Use `OnShutdown`.** `OnBeforeClose` runs while the user is waiting for the window to close — any latency there is visible. `OnShutdown` runs asynchronously after the window is gone.

**In `app.go`:** add a new `shutdown` method next to `startup`:

```go
// shutdown is called by Wails when the application window has closed.
// Sweeps backup sidecars for the currently-scanned source folder so users
// do not accumulate stale .bak / .bak.json files across sessions.
// Errors are logged and swallowed — we cannot show UI at this point.
func (a *App) shutdown(ctx context.Context) {
    if a.targetFolder == "" {
        return
    }
    n, err := ClearBackups(a.targetFolder)
    if err != nil {
        slog.Warn("shutdown_clear_backups_failed",
            "folder", a.targetFolder, "error", err.Error())
        return
    }
    if n > 0 {
        slog.Info("shutdown_clear_backups", "folder", a.targetFolder, "count", n)
    }
}
```

Note: this calls the existing `ClearBackups(folder)` helper from `exif_writer.go` — the same one `(*App).ClearAllBackups` wraps. **Do not add a new backup-deletion function.** Rule #20 requires reusing the existing one.

**In `main.go`:** add one line to the `wails.Run` options:

```go
OnStartup:  app.startup,
OnShutdown: app.shutdown,   // NEW
```

### Decision point — does the shutdown sweep need user opt-out?
**No, don't add a toggle.** Three reasons:
1. The user's request explicitly asks for automatic sweep on exit.
2. Users who want to preserve backups across sessions still have the in-session Undo path — the sweep only runs after the window closes, by which time the session is over anyway.
3. Adding a setting would require schema-bumping `settings.go` and is disproportionate for a single boolean.

If the user later changes their mind, adding an opt-out is a one-line conditional against `settings.clearBackupsOnExit`.

### Files to read / touch
- **Read:** `static/index.html` (ZONE A — View group), `static/js/actions.js` (existing patterns), `static/js/api.js` (wrapper already present — line ~114), `static/js/state.js` (for `state.targetFolder`), `main.go`, `app.go`, `app_write.go`.
- **Modify:** `static/index.html`, `static/js/actions.js`, `main.go`, `app.go`.
- **Do NOT modify:** `exif_writer.go`, `app_write.go::ClearAllBackups`, `dng_backup.go`, `dng_backup_undo.go`, `api.js::clearAllBackups`.

### Verification
1. `wails dev` — scan a folder, apply GPS to a few photos → confirm `.bak` / `.bak.json` appear on disk.
2. Click **Clear backups** → confirm dialog appears → click OK → toast shows correct count → `.bak*` files gone.
3. Apply GPS again → close the app window → re-open → the `.bak*` files created in the previous session are gone.
4. Regression check: the existing `SweepOrphanedSidecars` call in `ScanTargetFolder` still runs — make sure you did not accidentally change it.

### Out of scope
- Selective clear (per-photo backup deletion) — not requested.
- Multi-folder backup tracking — `ClearBackups` only knows about `a.targetFolder`. Do not change that contract.
- Confirmation dialog styling — reuse whatever `showConfirm` does today.

---

## CR3 — Modernize the "Default TZ" dropdown

### Symptom
`#select-default-tz` is a naked `<select>` with barely any styling (see `components.css` line 543: `font-size: 0.8rem; padding: 2px 4px;`). Next to the `GPS Ref` and `Import Track` buttons (which use `.btn-secondary`: border, radius, hover state, Inter font, semi-bold) it looks like a Windows 98 relic.

### Fix — pure CSS, keep the native `<select>` element

**Why keep `<select>`:** a custom dropdown would need keyboard handling, outside-click dismissal, ARIA, and CDN-pinned dependencies. None of that is warranted for 8 static options. We can style a native `<select>` to match the buttons using `appearance: none` + a background-image chevron. This is the same technique dedup-photos uses for its filter dropdowns.

**Modify `static/css/components.css`** — replace the existing `.settings-row` / `#select-default-tz` block (lines ~534–546) with:

```css
/* ── Settings row (Default TZ dropdown) ──────────────────────────────── */
.settings-row {
    display: inline-flex;
    align-items: center;
    gap: var(--space-xs);
    font-size: 0.75rem;
    color: var(--text-light);
}
.settings-label { white-space: nowrap; }

/* Match .btn-secondary visual language: bg, border, radius, hover transition.
   appearance:none strips the native arrow; SVG chevron inlined as background. */
#select-default-tz {
    appearance: none;
    -webkit-appearance: none;
    background-color: var(--bg);
    /* Inline chevron — single-color stroke, uses --text-light */
    background-image: url("data:image/svg+xml;charset=utf-8,\
%3Csvg xmlns='http://www.w3.org/2000/svg' width='10' height='6' viewBox='0 0 10 6'%3E\
%3Cpath fill='none' stroke='%236B6B6B' stroke-width='1.5' stroke-linecap='round' \
stroke-linejoin='round' d='M1 1l4 4 4-4'/%3E%3C/svg%3E");
    background-repeat: no-repeat;
    background-position: right var(--space-sm) center;
    background-size: 10px 6px;
    border: 1px solid var(--border);
    border-radius: var(--radius);
    color: var(--text);
    font-family: var(--font);
    font-size: var(--font-size-small);
    font-weight: var(--font-weight-semi);
    padding: var(--space-xs) calc(var(--space-md) + 12px) var(--space-xs) var(--space-sm);
    cursor: pointer;
    transition: all var(--transition);
}

#select-default-tz:hover {
    border-color: var(--primary-light);
    color: var(--primary);
}

#select-default-tz:focus {
    outline: none;
    border-color: var(--primary-light);
    box-shadow: 0 0 0 2px rgba(74, 144, 226, 0.15);
}
```

The chevron colour is hard-coded to `#6B6B6B` (the literal value of `--text-light`). CSS variables cannot be referenced inside `url(data:…)`, which is why we inline the hex. If the palette ever changes, this is the one place to update — document that inline with a comment.

### Decision point — Feather chevron vs inline SVG
The project already imports Feather Icons in `index.html`. You might be tempted to use `<i data-feather="chevron-down">`. **Don't.** Feather injects the icon as an inline SVG *element*, which cannot be used as a CSS `background-image` on a `<select>`. The inline `data:` URI above is the right tool for this job; Feather stays for actual icon elements (see CR4).

### Files to read / touch
- **Read:** `static/css/components.css` (existing button and TZ styles), `static/index.html` (markup is fine, no changes), `static/css/base.css` (to confirm `--space-*`, `--radius`, `--transition`, `--font-size-small`, `--font-weight-semi` all exist and are what we expect).
- **Modify:** `static/css/components.css` only.
- **Do NOT modify:** `static/js/settings.js` (behaviour unchanged), `static/index.html`.

### Verification
1. `wails dev` — the TZ dropdown visually aligns with `GPS Ref` / `Import Track`: same border, same radius, same hover colour shift.
2. The chevron is visible and sits ~8 px from the right edge.
3. Tabbing to the dropdown shows a focus ring in `--primary-light`.
4. Clicking opens the native option list (we kept `<select>`, so this still works on all platforms).
5. Functional regression: changing the selection still fires the `change` handler and shows the "timezone changed" toast — you did not touch the JS.

### Out of scope
- Replacing `<select>` with a custom dropdown component.
- Changing the list of timezones — they stay as-is.

---

## CR4 — Application icon (replace the Wails "W")

### Symptom
Built binary shows the default Wails "W" icon in the taskbar, `.exe` icon, and window title bar. The app is a GPS-photo tagger — the icon should convey that purpose.

### Concept
A simple, recognisable glyph at 16×16 and 32×32 sizes. The strongest candidate: **a map-pin silhouette with a small camera aperture or shutter inside it**. If that's too detailed to read at 16 px, fall back to **a map pin alone** using `--primary` (#1A3A5C) on a transparent background. Both motifs appear in Feather Icons (`map-pin`, `camera`, `aperture`) so there is existing visual language to reference.

**Design decision — leave final artwork to the user.** Claude Code should not auto-generate the PNG. It should:

1. Create the Wails `build/` directory structure if missing (it does not currently exist — `wails.json` does, but `build/` is empty).
2. Drop in **placeholder** source files with clear TODO comments describing exactly what the user needs to replace.
3. Wire `wails.json` and the OS-specific manifest so that once the user swaps in the real artwork, `wails build` picks it up with no further configuration.

### Part A — Create the build scaffold

Wails v2's `wails generate` would do this, but generating inside the repo would pull in a lot of noise. Create just the files needed:

```
build/
├── appicon.png                  (1024×1024 source — used to generate platform icons)
├── README.md                    (explains how to regenerate icons)
├── windows/
│   ├── icon.ico                 (multi-resolution .ico: 16, 24, 32, 48, 64, 256)
│   ├── info.json                (Wails Windows metadata)
│   └── wails.exe.manifest       (Windows application manifest)
└── linux/
    └── (nothing needed — Wails derives Linux icon from appicon.png)
```

**Do not commit binary placeholders to git.** Instead commit a `build/README.md` that explains:

1. Drop a 1024×1024 PNG at `build/appicon.png` (square, transparent background, centred glyph).
2. Generate `build/windows/icon.ico` from that PNG containing the five standard sizes. Recommended tool: https://icoconvert.com (web-based, no install) or ImageMagick `convert appicon.png -define icon:auto-resize=256,64,48,32,24,16 icon.ico` if the user has it installed locally.
3. `wails build` picks up `build/appicon.png` automatically for the Linux build and `build/windows/icon.ico` for the Windows build — no changes to `wails.json` required beyond what is already set.

Commit `build/windows/info.json` with sensible defaults:

```json
{
  "fixed": {
    "file_info": {
      "product_name": "GeoPhotoTagger",
      "comments": "GPS metadata editor for photos"
    }
  },
  "info": {
    "0000": {
      "ProductVersion": "1.0.0",
      "CompanyName": "Rosca75",
      "FileDescription": "GeoPhotoTagger",
      "LegalCopyright": "",
      "ProductName": "GeoPhotoTagger",
      "Comments": "GPS metadata editor for photos"
    }
  }
}
```

Commit `build/windows/wails.exe.manifest` — use the Wails v2 standard template (DPI-awareness + visual styles). If unsure, leave a TODO comment pointing to https://wails.io/docs/reference/options#windows and let the user generate it via `wails generate` on their Windows machine.

### Part B — Generate a placeholder `appicon.png`

Since the user works on a locked-down Windows box without image-editing freedom, create one usable placeholder programmatically. A simple and appropriate choice: a filled `#1A3A5C` map-pin glyph on transparent background, 1024×1024. This can be rendered by a small Go utility script **kept outside the binary** (do not add it to `main`) — e.g. `cmd/genicon/main.go` invoked manually with `go run ./cmd/genicon`. Alternatively, provide an SVG source at `build/appicon.svg` and leave PNG rasterization to the user.

**Recommended path: commit an SVG.** It's text, diff-friendly, easy to tweak, and the user can rasterize it on their PwC machine with any online converter. The SVG should be ~30 lines of hand-written markup using `--primary` and `--primary-light`. Example:

```svg
<!-- build/appicon.svg — GeoPhotoTagger app icon
     Square 1024x1024 canvas, transparent background, centred map-pin
     with an inner camera-aperture motif. Colours match CLAUDE.md §9. -->
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 1024 1024" width="1024" height="1024">
  <!-- Map-pin outline -->
  <path d="M512 128 C 352 128 224 256 224 416
           C 224 608 512 896 512 896
           C 512 896 800 608 800 416
           C 800 256 672 128 512 128 Z"
        fill="#1A3A5C"/>
  <!-- Inner camera lens (aperture) -->
  <circle cx="512" cy="400" r="140" fill="#FFFFFF"/>
  <circle cx="512" cy="400" r="90"  fill="#4A90E2"/>
  <!-- Aperture blades (simplified) -->
  <circle cx="512" cy="400" r="30" fill="#FFFFFF"/>
</svg>
```

Then `build/README.md` instructs the user to rasterize to PNG via any online SVG→PNG tool (e.g. https://cloudconvert.com/svg-to-png) and save it as `build/appicon.png`.

### Files to read / touch
- **Read:** `main.go`, `wails.json` — neither should need changes beyond what Wails already supports; `wails.json` does not need an `icon` field.
- **Create:** `build/README.md`, `build/appicon.svg`, `build/windows/info.json`, `build/windows/wails.exe.manifest`.
- **Do NOT create placeholder binaries in git.** `.ico` / `.png` are for the user to generate and commit once artwork is finalized.
- **Add to `.gitignore` (optional):** nothing — once `appicon.png` and `icon.ico` are committed they should stay in the repo.

### Verification
1. Follow `build/README.md` — drop in `appicon.png` (1024×1024) and `icon.ico` (multi-res).
2. `wails build -platform windows/amd64` on the user's machine.
3. The resulting `build/bin/geo-photo-tagger.exe` shows the new icon in File Explorer, in the taskbar when running, and in the top-left of the app window.
4. `wails build -tags webkit2_41 -platform linux/amd64` on CI → icon appears when the binary runs on a Linux desktop.

### Out of scope
- macOS icon (`.icns`) — the project does not ship for macOS.
- Splash screen — Wails v2 does not use one by default.

---

## CR5 — Build workflow already exists; align it with dedup-photos conventions

### Symptom
User says "make sure there is a workflow to build a binary for Windows and Linux" and asks to leverage the dedup-photos workflow. **Workflows already exist** (`.github/workflows/ci.yml` and `release.yml`) and functionally cover both platforms. The task is therefore alignment / cleanup, not creation.

### Decision
Two changes:

1. **Leave `ci.yml` essentially as-is.** It is already better than dedup-photos' `ci.yml` (builds on both Windows *and* Linux with Wails, uploads artifacts — dedup-photos only runs `go build` / `go test` without actually producing a Wails binary). Just bring it in line with Go module version and light cleanup.

2. **Refactor `release.yml`** to the three-job structure dedup-photos uses (`build-windows` → `build-linux` → `release`). The current matrix-based approach works but gives uglier release output — softprops/action-gh-release runs twice and race-creates the release. The three-job structure creates the release once, attaches both binaries.

### Changes to `.github/workflows/ci.yml`

Minimal — just bump Go to match `go.mod` (`go 1.25.0`) and add a `gofmt` check matching dedup-photos:

- In **both** jobs, change `go-version: '1.24'` → `go-version: '1.25'`.
- After `go vet ./...` in the `build-linux` job (only; Windows gofmt CRLF noise is not worth chasing), add:

```yaml
      - name: Check formatting
        run: |
          if [ -n "$(gofmt -l .)" ]; then
            echo "The following files are not formatted:"
            gofmt -l .
            exit 1
          fi
```

- Add `go test ./...` step after the vet step on both OSes (matches dedup-photos and is a free correctness net).

Do not change the artifact upload paths — they are already correct.

### Changes to `.github/workflows/release.yml`

**Replace the current matrix job with three separate jobs.** Full new file content:

```yaml
name: Release

# Triggers on version tag push (e.g. git tag v1.0.0 && git push origin v1.0.0)
on:
  push:
    tags:
      - 'v*'

jobs:

  # ──────────────────────────────────────────────────────────────
  # Windows build — produces geo-photo-tagger.exe
  # Must run on windows-latest: Wails embeds WebView2 which
  # requires Windows SDK headers at compile time.
  # ──────────────────────────────────────────────────────────────
  build-windows:
    runs-on: windows-latest
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Set up Node.js (required by Wails to bundle the frontend)
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install Wails CLI
        run: go install github.com/wailsapp/wails/v2/cmd/wails@latest

      - name: Build Windows binary
        run: wails build -platform windows/amd64 -o geo-photo-tagger.exe

      - name: Upload Windows binary
        uses: actions/upload-artifact@v4
        with:
          name: windows-binary
          path: build/bin/geo-photo-tagger.exe

  # ──────────────────────────────────────────────────────────────
  # Linux build — produces geo-photo-tagger-linux
  # Ubuntu 24.04 ships webkit2gtk-4.1 (not 4.0). Wails' built-in
  # 4.1 support is activated via -tags webkit2_41 per CLAUDE.md §14.
  # ──────────────────────────────────────────────────────────────
  build-linux:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Set up Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'

      - name: Set up Node.js
        uses: actions/setup-node@v4
        with:
          node-version: '20'

      - name: Install Linux system dependencies
        run: |
          sudo apt-get update
          sudo apt-get install -y libgtk-3-dev libwebkit2gtk-4.1-dev pkg-config

      - name: Install Wails CLI
        run: go install github.com/wailsapp/wails/v2/cmd/wails@latest

      - name: Build Linux binary
        run: wails build -tags webkit2_41 -platform linux/amd64 -o geo-photo-tagger-linux

      - name: Upload Linux binary
        uses: actions/upload-artifact@v4
        with:
          name: linux-binary
          path: build/bin/geo-photo-tagger-linux

  # ──────────────────────────────────────────────────────────────
  # Release — waits for both builds, creates the GitHub Release,
  # and attaches both binaries as downloadable assets.
  # ──────────────────────────────────────────────────────────────
  release:
    needs: [build-windows, build-linux]
    runs-on: ubuntu-latest
    permissions:
      contents: write

    steps:
      - name: Download Windows binary
        uses: actions/download-artifact@v4
        with:
          name: windows-binary
          path: artifacts/

      - name: Download Linux binary
        uses: actions/download-artifact@v4
        with:
          name: linux-binary
          path: artifacts/

      - name: Create GitHub Release
        uses: softprops/action-gh-release@v2
        with:
          files: |
            artifacts/geo-photo-tagger.exe
            artifacts/geo-photo-tagger-linux
          generate_release_notes: true
```

### Interaction with CR4
Once the icon is in place (`build/windows/icon.ico`, `build/appicon.png`), Wails will automatically bundle it during `wails build` in this workflow. No workflow changes needed for that — Wails reads from `build/` by convention.

### Files to read / touch
- **Read:** `.github/workflows/ci.yml`, `.github/workflows/release.yml`, `go.mod`, `CLAUDE.md` §14 (Linux build notes).
- **Modify:** `.github/workflows/ci.yml` (Go version bump + fmt + test steps).
- **Replace:** `.github/workflows/release.yml` (full rewrite per spec above).

### Verification
1. Push a throwaway commit to a PR branch → `ci.yml` runs both jobs → both green; artifacts are downloadable from the run page; `gofmt` check passes (run `gofmt -w .` locally first if needed).
2. Push a tag `v0.0.1-test` → `release.yml` runs; both builds succeed; `release` job creates a draft release with both binaries attached; auto-generated release notes appear.
3. Delete the test release and tag after verification.

### Out of scope
- macOS build — user has not asked for it; adding it multiplies CI minutes and the app has not been tested on darwin.
- Code signing / notarization — not requested; the installer is self-built and unsigned, consistent with dedup-photos.
- Version number injection — can be a follow-up CR if the user wants `--version` on the binary.

---

## Final commit order

| Commit | Title | Touches |
|--------|-------|---------|
| 1 | `CR1: direct-binary DNG thumbnail preview extractor` | new `dng_thumbnail_reader.go`, modified `thumbnail.go` |
| 2 | `CR2: expose ClearAllBackups in UI + sweep on shutdown` | `index.html`, `actions.js`, `main.go`, `app.go` |
| 3 | `CR3: modernize Default TZ dropdown to match button styles` | `components.css` only |
| 4 | `CR4: app icon scaffold (replaces Wails W placeholder)` | new `build/` tree |
| 5 | `CR5: align CI+release workflows with dedup-photos conventions` | `.github/workflows/*.yml` |

Each commit must pass `go vet ./...` and `go test ./...` independently. If any commit in the sequence breaks the build for the next, stop and fix it before moving on — do not batch-fix at the end.
