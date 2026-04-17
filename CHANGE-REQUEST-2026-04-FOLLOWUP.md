# Change Request 2026-04 — Follow-up

> **Audience:** Claude Opus 4.7 working on `Rosca75/geo-photo-tagger` on Windows 11.
> **Context:** The previous session (`CHANGE-REQUEST-2026-04.md`) was implemented on branch `claude/implement-change-request-2026-04-6qco3` and merged to `main`. This file covers three follow-up tasks.
>
> **Before touching any file, read `CLAUDE.md` in full.** All its rules apply. If your work in phase B (below) changes CLAUDE.md itself, re-read the updated version before starting phase C.
>
> **Work the phases in order.** A first, then B, then C. Do not start a phase before the previous one is complete.

---

## 0. Summary

| Phase | Area | Size | Output |
|---|---|---|---|
| A | Targeted audit of the 2026-04 merge + self-apply fixes | M | Fix commits + `docs/AUDIT-2026-04.md` |
| B | Refresh `CLAUDE.md` to match the current project state | S | Updated `CLAUDE.md` |
| C | Hover thumbnail for candidates (new feature) | M | Code changes + one feature commit |

---

## Phase A — Targeted audit of the 2026-04 implementation

### A.0 Scope

**Targeted only.** Check that the specific code snippets and behaviors prescribed in `CHANGE-REQUEST-2026-04.md` (the 9 commits' worth of work on branch `claude/implement-change-request-2026-04-6qco3`, now merged to `main`) were implemented correctly. Do **not** go hunting for collateral regressions outside the scope of those 9 phases — that's a broader audit for another session.

### A.1 Remediation authority

You have authority to fix issues directly, within these bounds:

**Auto-fix without asking** if the discrepancy is:
- A missing doc comment on a new Go function
- A typo in a string, tooltip, or log message
- A missing CSS class or rule prescribed in the instruction file
- Style drift (inconsistent indentation, missing trailing newline)
- A missing `go vet` / unused-import cleanup
- A dead code path (e.g. `setThreshold` left in place after phase 4 told you to remove it)

**Do NOT auto-fix; document in `docs/AUDIT-2026-04.md` only** if the discrepancy is:
- Any deviation in `dng_backup.go`, `dng_gps_verify.go`, `dng_gps_writer.go`, or the sidecar format (phase 3 — backup safety is non-negotiable)
- Any public Go method signature change that differs from what the instruction prescribed (affects Wails bindings)
- An architectural deviation (e.g. logic placed in a different file than specified, a missing helper module, a different state-management pattern)
- Anything that looks intentional and reasonable but differs from the plan — the owner decides whether to keep Opus 4.7's call or revert

**When in doubt, don't fix — document.** A too-conservative audit is easier to act on than an auto-fix that papered over a real design choice.

### A.2 How to run the audit

Work from the repo root. Do not re-clone — audit the current working tree, which is already at `main` post-merge.

1. **Identify the pre-change-request baseline.** Run `git log --oneline --all` and find the commit immediately before phase 1 (look for the first commit whose message contains "phase 1" or "CHANGE-REQUEST-2026-04" — the baseline is its parent). Save that SHA; you'll use it to diff.

2. **Produce a file-by-file diff.** `git diff <baseline-sha>..main -- <file>` for each file in the checklist below. Read the diff, compare against the instruction file, note discrepancies.

3. **Compile and test.** `wails build -platform windows/amd64` must succeed. `go test ./...` must pass. `go vet ./...` must be clean. If any of these fail on the merged `main`, stop and document — that's a "do not auto-fix" condition because a broken merge is a bigger signal than a local typo.

4. **For each finding:**
   - If it matches "auto-fix" criteria → fix it, commit with message `audit: fix <short description>`.
   - If it matches "document only" criteria → add an entry to `docs/AUDIT-2026-04.md` (see template in A.4).

5. **One commit per auto-fix.** Do not batch unrelated fixes.

### A.3 Audit checklist

For each phase, verify the listed behaviors. Every item is either ✅ present and correct, ⚠️ present but deviates, or ❌ missing.

**Phase 1 — Quiet logs**
- `app.go` `startup()` calls `setupLogger(os.Getenv("GPT_DEBUG_LOG") == "1")` (not hardcoded `true`)
- `"os"` is in the import block of `app.go`
- `exif_reader.go` `ReadEXIFForScan` no longer contains `slog.Debug("exif_read", …)`
- If `time.Now()` was only used by the deleted Debug line, `"time"` is no longer imported in that file
- Running `wails dev` + scanning 50+ photos produces exactly one `scan_complete` Info line (verify locally if possible)

**Phase 2 — "Loading location…" fix**
- `static/js/state.js` has `geocodeCache: new Map()` and `mapEnabled: false`
- `static/js/geocode.js` exists, exports `getLocationForCoords` and `refreshLocationFor`, under ~80 lines
- `refreshLocationFor` re-queries the container after the `await` (the fix for bug A depends on this)
- `matcher_ui.js` no longer imports `reverseGeocode` from `./api.js`
- `matcher_ui.js` imports `refreshLocationFor` from `./geocode.js`
- `handleCandidateSelect` no longer contains the old `reverseGeocode(lat, lon).then(...)` inline block
- `showPhotoDetail` calls `refreshLocationFor(acc, panel)` after `renderPreview(photo, panel)`

**Phase 3a — Benchmark harness**
- `dng_gps_writer_test.go` exists
- Benchmarks present: `BenchmarkApplyGPS_FullPipeline`, `_BackupOnly`, `_PatchOnly`, `_VerifyFullReader`, `_VerifyFastPath`
- `b.Skipf` guard for missing `samples/IMGP8411.DNG` is present (benchmarks should be skippable, not fail outright)
- `cloneSample` + `cleanupApply` helpers present, cleanup removes both `.bak` and `.bak.json`

**Phase 3b — Fast verify**
- `dng_gps_verify.go` exists
- `verifyGPSInDNG(path, expectedLat, expectedLon)` present with tolerance 0.001
- `readGPSCoordsFromIFD` + `readThreeRationals` helpers present
- `writeAndVerify` in `exif_writer.go` routes DNG through `verifyGPSInDNG` and JPEG through the goexif path
- `dng_gps_writer.go` is under 150 lines (phase 3b instructed to split if needed)

**Phase 3c — Lazy backup + warning dialog**
- `dng_backup.go` exists (may also have `dng_backup_undo.go` if the split was taken)
- `dngBackupSidecar` struct has exactly these fields: `Version`, `GPSPointerOffset`, `OriginalPointerValue`, `OriginalFileSize`, `PreHashHex`
- `Version == 1`, `preApplyHashSize == 64 * 1024`
- `captureDNGBackup`, `loadDNGBackup`, `undoDNGFromSidecar`, `checkDNGTamper`, `SweepOrphanedSidecars` all present
- `WriteGPS` for `.dng` calls `captureDNGBackup` **before** `writeAndVerify` and aborts on backup failure
- `UndoGPS` for `.dng` calls `checkDNGTamper` **before** `undoDNGFromSidecar`
- `ClearBackups` removes both `.bak` and `.bak.json`
- `ScanTargetFolder` in `app.go` calls `SweepOrphanedSidecars` on both old and new folders and logs `orphan_sidecars_cleared`
- `static/js/actions.js` has `buildApplyWarning(photoCount)` helper
- Both `handleApplyAll` and `handleApplySingle` call `showConfirm(buildApplyWarning(...))`
- `showConfirm` renders newlines safely (textContent on `<p>`, not innerHTML — the instruction flagged this explicitly)

**Phase 4 — Subfolder toggle + slider**
- `ScanForTargetPhotosParallel` signature: `(folderPath string, numWorkers int, recursive bool)`
- `ScanForReferencePhotos` signature: `(folderPath string, dateFilter DateRange, recursive bool)`
- `ScanTargetFolder` + `AddReferenceFolder` pass `recursive` through
- `App` struct has `lastSourceRecursive bool` field
- `ScanTargetFolder` assigns `a.lastSourceRecursive = recursive`
- Both scanner callbacks use `fs.SkipDir` pattern for non-root dirs when not recursive
- `static/js/api.js` wrappers updated to pass `recursive`
- `static/index.html` has `chk-source-recursive` and `chk-ref-recursive` checkboxes, both `checked` by default
- `state.js` has `sourceRecursive: true`, `refRecursive: true`
- `scan.js` and `reference.js` wire the checkboxes
- Old `.threshold-btn` buttons removed from HTML
- `#delta-slider` present with `min=5 max=360 step=5 value=30`
- `#delta-slider-value` present
- `matcher_ui.js` has `formatDeltaMinutes(m)` helper
- `input` event updates label; `change` event commits to `state.matchThreshold`
- `setThreshold` removed from `matcher_ui.js`

**Phase 5 — Leaflet mini-map**
- `static/js/map.js` exists, exports `renderMap` and `toggleMap`
- Leaflet URLs pinned to `@1.9.4`
- Map options include `dragging: false`, `scrollWheelZoom: false`, `zoomControl: false`
- OSM attribution string present and not removed
- Previous Leaflet instance `.remove()`'d before creating a new one
- `buildGPSPreview` in `detail_render.js` includes the toggle button and conditional `#gps-mini-map` slot
- `.gps-mini-map` CSS rule present
- `handlePanelClick` routes `.btn-toggle-map` to `toggleMap`
- `showPhotoDetail` calls `renderMap(acc.lat, acc.lon, panel)` when `state.mapEnabled`
- Default `state.mapEnabled === false`

**Phase 6 — Per-photo selection**
- `handleMatchAllClick` auto-populates `state.acceptedMatches` from best candidates
- Same treatment in `handleMatchSingle`
- `table.js` header has `cb-select-all` checkbox
- Rows have `.row-select-cb` checkbox for matched photos only
- `toggleRowSelection` and `toggleAllSelection` helpers present
- Row checkbox click has `stopPropagation` so it doesn't fire the row-level handler
- `.col-select` CSS class styled
- Button label is "Apply GPS data" (not "Apply All Accepted")
- Button tooltip reads "Apply GPS data of the best candidate found"

**Phase 7 — Same-source matching**
- `RunSameSourceMatching` method on `App` in `app_match.go`
- Early return if `a.targetFolder == ""` or `len(a.targetPhotos) == 0`
- Uses `a.lastSourceRecursive` (not hardcoded) when scanning for in-folder refs
- Does NOT mutate `a.referencePhotos`
- `static/js/api.js` has `runSameSourceMatching` wrapper
- `state.matchMode` exists with default `'refs'`
- `static/index.html` has three radio buttons with values `refs`, `track`, `same`
- Default checked = `refs`
- `handleMatchAllClick` routes by mode
- Precondition checks exist for `refs` (needs reference folder) and `track` (needs GPS track file); none for `same`

### A.4 Output: `docs/AUDIT-2026-04.md`

Create this file at the end of the audit. Template:

```markdown
# Audit Report — CHANGE-REQUEST-2026-04

**Auditor:** Claude Opus 4.7
**Baseline commit:** <sha>
**Audited commit:** <sha of main after merge>
**Date:** <YYYY-MM-DD>

## Summary

<One-paragraph verdict: "All phases implemented correctly" / "N discrepancies found, M auto-fixed, K flagged for owner review">

## Auto-fixed findings

| Phase | File | Finding | Fix commit |
|---|---|---|---|
| 1 | exif_reader.go | `time` import left in place after Debug deletion | `<sha>` |
| ... | ... | ... | ... |

## Flagged for owner review

### Finding 1: <short title>
- **Phase:** <n>
- **File:** <path>
- **What the instruction said:** <quote or paraphrase>
- **What was implemented:** <quote or paraphrase>
- **Why flagged:** <e.g. "touches backup safety — out of auto-fix scope">
- **Recommendation:** <accept as-is / revert / discuss>

### Finding 2: ...

## Build + test results

- `wails build -platform windows/amd64`: ✅ / ❌ <output snippet if failed>
- `go test ./...`: ✅ / ❌ <output snippet if failed>
- `go vet ./...`: ✅ / ❌ <output snippet if failed>

## Benchmark numbers (if samples/ present)

<paste `go test -bench BenchmarkApplyGPS -benchmem -benchtime 10x ./...` output>

## Closing

<Any high-level observations about implementation quality, patterns noticed, suggestions for future change requests.>
```

Commit this file with message `docs: add audit report for CHANGE-REQUEST-2026-04`.

### A.5 Phase A exit criteria

Do not proceed to phase B until all of these are true:

- [ ] `docs/AUDIT-2026-04.md` exists on `main`
- [ ] All auto-fix findings have their own commit
- [ ] `wails build -platform windows/amd64` succeeds
- [ ] `go vet ./...` is clean
- [ ] Nothing in the "document only" category has been silently modified

---

## Phase B — Refresh `CLAUDE.md`

### B.0 Context

`CLAUDE.md` documents the rules that govern all work in this repo. Several features were added in 2026-04, some files moved or grew, and the reality of the codebase has drifted from what `CLAUDE.md` describes. Bring it back into alignment.

### B.1 What to update

Read the current `CLAUDE.md` end-to-end first. Then make these specific updates:

**1. Architecture section — frontend file inventory.** The list of `static/js/*.js` files should now include `geocode.js` (phase 2) and `map.js` (phase 5). If the file currently lists them by purpose, add one-line descriptions consistent with the style of existing entries.

**2. Architecture section — Go file inventory.** Add `dng_gps_verify.go` and `dng_backup.go` (plus `dng_backup_undo.go` if the split was taken in phase 3c) with one-line descriptions.

**3. Backup strategy paragraph.** Find whatever currently describes the JPEG `.bak` strategy and extend it to explain the DNG `.bak.json` sidecar strategy. Keep it short — a paragraph, not a treatise. Key points to mention:
   - JPEG: full file copy to `<path>.bak` (unchanged)
   - DNG: lazy sidecar at `<path>.bak.json` with pointer offset, original value, file size, pre-apply SHA-256 of first 64 KB
   - Undo refuses if the pre-apply hash no longer matches (tamper detection)
   - Orphan sidecars swept at `ScanTargetFolder` time

**4. "Modules" or "Matching modes" section.** If CLAUDE.md describes the three matching modes (external refs / GPS track), add "Same source" as module 3 with a one-line description.

**5. Rules list — keep existing rules, add new ones based on patterns from 2026-04.** Add these rules (use whatever numbering convention CLAUDE.md already uses):

   - **Rule (new): Respect the 150-line ceiling — split proactively.** When a file grows past 150 lines, split it before the commit lands. Do not let a file reach 220 lines and say "it's mostly comments." Comments count.
   - **Rule (new): `api.js` is the exclusive call site for `window.go.main.App.*`.** Every new bound Go method gets a matching `api.js` wrapper in the same commit. No exceptions.
   - **Rule (new): New frontend dependencies require CDN pinning.** If a phase introduces a frontend library (as Leaflet 1.9.4 was), pin the exact version in the URL. No `latest`, no range specifiers.
   - **Rule (new): Auto-fixable is not the same as silently-fixable.** Refactoring that goes beyond the instruction's prescribed scope belongs in a follow-up, not mixed into the same commit. If the instruction says "rename X to Y", do only that — don't also rename Z because it would look nicer.
   - **Rule (new): Backup-safety code paths are no-touch except by explicit instruction.** `dng_backup.go`, `dng_gps_verify.go`, `dng_gps_writer.go`, and the `WriteGPS` / `UndoGPS` / `ClearBackups` triplet in `exif_writer.go` may only be modified when a change request names them explicitly. Incidental changes to these files are rejected on review.

**6. Development workflow section.** If CLAUDE.md has a "how to verify changes" section, add:
   - `go test -bench BenchmarkApplyGPS -benchmem -benchtime 10x ./...` for DNG perf work (requires `samples/IMGP8411.DNG`)
   - Note that `GPT_DEBUG_LOG=1 wails dev` restores per-file Debug logging

**7. Known limitations / deferred work section.** If this exists, add:
   - HEIC decoding is read-only (GPS tags only). Rendering a HEIC thumbnail is not yet supported — hover-preview and any future thumbnail feature must short-circuit to "no preview" for HEIC files.

If a section doesn't currently exist but needs to be created for one of these updates, create it using the style and tone of the existing sections.

### B.2 What NOT to change

- Do not rewrite the file end-to-end. The owner's voice is in it; preserve it.
- Do not renumber existing rules. Append new ones at the end of the rules list.
- Do not remove any existing rule unless it's now factually wrong (e.g. it says "JPEG only" in a context where DNG now works).
- Do not expand the document beyond what these updates require. If `CLAUDE.md` is currently 200 lines, aim to finish under 260.

### B.3 Commit

Single commit: `docs: refresh CLAUDE.md for 2026-04 feature set`.

### B.4 Phase B exit criteria

- [ ] `CLAUDE.md` accurately describes the current file inventory
- [ ] Backup strategy paragraph covers both JPEG and DNG
- [ ] Five new rules added to the rules list
- [ ] HEIC limitation noted somewhere findable
- [ ] Diff is minimally invasive (no reorganization, no tone drift)

---

## Phase C — Hover thumbnail on candidate cards (new feature)

### C.0 Feature spec

When the user hovers the **filename or source thumbnail inside a candidate card** (Zone C), a small floating preview (32×32, aspect ratio preserved) appears next to the cursor. It disappears on mouse-leave.

**Supported sources:** photos whose format has an in-Go decoder (JPEG; any other format that existing `thumbnail.go` already handles).

**Unsupported sources — hide the hover entirely, no placeholder, no error icon:**
- GPS track points (no source image exists)
- HEIC candidates (decoder not yet implemented)

"Hide entirely" means: the hover mechanism simply doesn't fire. No empty div, no "no preview" tooltip. The UI should feel like the feature just isn't there for those cases.

### C.1 Design

**Reuse `thumbnail.go` as much as possible.** Before writing anything new, read the existing file and understand what bound method (if any) already generates a thumbnail image for a given file path. Likely candidates: whatever Zone C uses to render the *target* photo's thumbnail today. If that method is already bound and returns base64 or a data URL, use it directly from the hover code — no new Go code needed.

**If `thumbnail.go` exposes a thumbnail generator but it's not bound to Wails,** the lightest-touch change is to add a new `GetCandidateThumbnail(path string) (string, error)` bound method that wraps the existing generator at 32×32. Put this method in `app.go` (or wherever the other `Get…` bound methods live), not in a new file. Add the matching wrapper in `api.js`.

**If the existing thumbnail generator returns a larger size,** do not resize on the Go side for the hover — let CSS handle it via `max-width: 32px; max-height: 32px; object-fit: contain`. Server-side resizing for a 32×32 preview is overkill and adds latency.

**Format detection.** The frontend already knows a candidate's source file extension (it's displayed in the card). Gate the hover in JS: if the extension is `.heic` / `.heif`, or if the candidate is a GPS track point (no sourcePath), don't attach the hover handlers at all. This keeps the Go side simple and avoids round-trips that would just return "unsupported".

**Caching.** Thumbnails cached in a module-level `Map<path, dataURL>` in the new `hover_thumbnail.js` file. Keyed by source path. No eviction — photo libraries browsed in one session rarely exceed a few hundred unique candidates, and 32×32 JPEGs are a few KB each.

**Rendering.** A single `<div id="hover-thumbnail">` element appended to `document.body` on first use. Absolutely positioned, follows the cursor with a small offset (e.g. cursor position + 16px right, + 16px down), pointer-events: none. One shared element across all hover targets — no per-card DOM allocation.

### C.2 Implementation

**File: `static/js/hover_thumbnail.js`** — NEW.

```javascript
// hover_thumbnail.js — Floating 32×32 thumbnail preview next to the cursor
// when the user hovers the filename or source thumbnail inside a candidate
// card in Zone C.
//
// Supported: JPEG (and anything else Go's image package decodes via the
// existing thumbnail.go pipeline). Unsupported (HEIC, GPS track points) is
// handled by never attaching hover handlers to those elements — no error
// state, no placeholder.
//
// Thumbnails are cached in-memory by source path. One shared floating <div>
// is appended to <body> on first use; per-card DOM allocation is avoided.

import { getCandidateThumbnail } from './api.js';

const thumbnailCache = new Map();  // path -> dataURL or '' (negative cache)
let hoverEl = null;
let currentPath = null;

// Extensions we can render. Match lowercase against the path suffix.
// HEIC/HEIF are deliberately excluded — decoding is not implemented.
const SUPPORTED_EXTS = ['.jpg', '.jpeg', '.png', '.gif', '.bmp', '.tiff', '.tif', '.dng'];

// isSupported returns true if path has a format we can thumbnail.
// Callers should also check that path itself is non-empty (GPS track
// points have no sourcePath — attachHover skips those).
export function isSupported(path) {
    if (!path) return false;
    const lower = path.toLowerCase();
    return SUPPORTED_EXTS.some(ext => lower.endsWith(ext));
}

// ensureHoverEl lazily creates the shared floating <div>.
function ensureHoverEl() {
    if (hoverEl) return hoverEl;
    hoverEl = document.createElement('div');
    hoverEl.id = 'hover-thumbnail';
    hoverEl.style.display = 'none';
    document.body.appendChild(hoverEl);
    return hoverEl;
}

// showAt positions the hover element at (clientX+16, clientY+16), clamped
// to the viewport so it never clips off-screen.
function showAt(clientX, clientY) {
    const el = ensureHoverEl();
    const OFFSET = 16;
    const SIZE = 48;  // approx — includes padding/border
    const x = Math.min(clientX + OFFSET, window.innerWidth - SIZE);
    const y = Math.min(clientY + OFFSET, window.innerHeight - SIZE);
    el.style.left = `${x}px`;
    el.style.top = `${y}px`;
    el.style.display = 'block';
}

function hide() {
    if (hoverEl) hoverEl.style.display = 'none';
    currentPath = null;
}

// loadThumbnail fetches the thumbnail for `path` via the bound Go method,
// caches the result (including empty-string misses), and returns it.
async function loadThumbnail(path) {
    if (thumbnailCache.has(path)) return thumbnailCache.get(path);
    try {
        const dataURL = await getCandidateThumbnail(path);
        thumbnailCache.set(path, dataURL || '');
        return dataURL || '';
    } catch {
        thumbnailCache.set(path, '');
        return '';
    }
}

// attachHover wires mouseenter/mousemove/mouseleave on `target` to show a
// thumbnail for `path`. No-op if the path is unsupported. Safe to call
// repeatedly — previous listeners on the same element are NOT tracked, so
// callers should only attach once per element lifetime.
export function attachHover(target, path) {
    if (!target || !isSupported(path)) return;

    target.addEventListener('mouseenter', async (e) => {
        currentPath = path;
        const el = ensureHoverEl();
        const dataURL = await loadThumbnail(path);
        // If the user moved to a different card while we were fetching,
        // this result is stale — drop it.
        if (currentPath !== path) return;
        if (!dataURL) return;
        el.innerHTML = `<img src="${dataURL}" alt="">`;
        showAt(e.clientX, e.clientY);
    });

    target.addEventListener('mousemove', (e) => {
        if (hoverEl && hoverEl.style.display === 'block') {
            showAt(e.clientX, e.clientY);
        }
    });

    target.addEventListener('mouseleave', hide);
}
```

**File: `static/css/components.css`** — append:

```css
#hover-thumbnail {
    position: fixed;
    z-index: 9999;
    pointer-events: none;
    padding: 2px;
    background: var(--background);
    border: 1px solid var(--border);
    border-radius: var(--radius);
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.25);
}
#hover-thumbnail img {
    display: block;
    max-width: 32px;
    max-height: 32px;
    object-fit: contain;
}
```

**File: `static/js/api.js`** — add wrapper:

```javascript
export async function getCandidateThumbnail(path) {
    return window.go.main.App.GetCandidateThumbnail(path);
}
```

**File: `app.go` (or wherever existing `Get…` bound methods live)** — add method. First, read `thumbnail.go` to find the existing thumbnail entry point. Expected shape:

```go
// GetCandidateThumbnail returns a base64 data URL for a 32×32 thumbnail
// of the image at `path`. Used by the hover-preview feature on candidate
// cards in Zone C.
//
// Reuses the existing thumbnail generator from thumbnail.go; do NOT
// duplicate decoding logic. Returns ("", nil) for formats the existing
// generator doesn't support — the caller treats empty string as "no
// preview available" rather than an error.
//
// HEIC/HEIF is explicitly excluded on the frontend (hover handlers are
// never attached for those extensions), so this method does not need
// special-case HEIC handling.
func (a *App) GetCandidateThumbnail(path string) (string, error) {
    // Call the existing thumbnail generator. Adjust this line to match
    // the actual function name and signature in thumbnail.go.
    return generateThumbnailDataURL(path, 32)
}
```

**The exact function name `generateThumbnailDataURL` is a placeholder.** Replace it with whatever `thumbnail.go` actually exposes. If the existing generator returns `([]byte, error)` instead of a data URL, the new method must base64-encode + prefix with `data:image/jpeg;base64,` before returning. If the existing generator doesn't take a size parameter, pass through at whatever default size it produces — the CSS `max-width: 32px` will downscale on display.

**If `thumbnail.go` doesn't currently expose a function that fits this shape,** do the minimum: add a small wrapper function in `thumbnail.go` that adapts the existing generator to return `(string, error)` where the string is a data URL. Comment the wrapper and add `// Introduced for the hover-preview feature (C.2).` near the top.

**Do NOT add HEIC detection on the Go side.** The frontend is authoritative for "is this format supported." Keeping that decision in one place (the JS `SUPPORTED_EXTS` list) avoids drift.

**File: `static/js/detail_render.js`** — modify `buildCandidateCard` (or whatever the current function is called that renders a single candidate inside Zone C). The goal is to add a data attribute to the filename/thumbnail element that identifies the source path, so `matcher_ui.js` can attach the hover handler. Minimum change:

- Find the element that renders the candidate's filename + source thumbnail.
- Add `data-candidate-source-path="${candidate.sourcePath || ''}"` and class `candidate-hover-target`.

Leave the rest of the card markup untouched.

**File: `static/js/matcher_ui.js`** — attach handlers after rendering. Add import:

```javascript
import { attachHover } from './hover_thumbnail.js';
```

In `showPhotoDetail`, after the panel HTML is rendered (same location where the existing candidate click handlers are wired), add:

```javascript
// Attach hover-thumbnail handlers to each candidate. attachHover itself
// short-circuits for HEIC files and empty paths (GPS track points), so
// we don't need to filter here.
panel.querySelectorAll('.candidate-hover-target').forEach(el => {
    const path = el.dataset.candidateSourcePath;
    attachHover(el, path);
});
```

Note: because `showPhotoDetail` is called on every re-render, and `attachHover` doesn't track listeners, repeated calls will stack listeners on the same element. This is fine because the re-render destroys and recreates the panel HTML — `panel.querySelectorAll(...)` matches fresh elements each time. But if you change `showPhotoDetail` to do partial updates instead of full re-renders, this logic needs revisiting. Leave a comment reflecting this.

### C.3 Edge cases

- **GPS track point candidates:** `candidate.sourcePath` is empty. `isSupported('')` returns `false`. `attachHover` no-ops. ✅
- **HEIC candidates:** `isSupported('photo.heic')` returns `false`. `attachHover` no-ops. ✅
- **Network/IO failure in `getCandidateThumbnail`:** `loadThumbnail` catches, caches empty string, subsequent hovers hit the cache and show nothing. User sees no preview — same as unsupported. ✅
- **User hovers rapidly between candidates:** `currentPath` check prevents stale fetches from rendering into the hover element.
- **User scrolls with the mouse outside the card:** `mouseleave` fires → `hide()` → hover disappears. ✅

### C.4 Verification

1. `wails dev`. Scan a source folder with JPEGs, add a reference folder with JPEGs, run a match.
2. Click a target photo → candidates appear in Zone C.
3. Hover the filename of a JPEG candidate → floating 32×32 thumbnail appears near the cursor. Move cursor → thumbnail follows. Leave the element → thumbnail disappears.
4. Hover a HEIC candidate → nothing happens. No placeholder, no console error.
5. Switch match mode to "GPS track" (if a track is imported) → hover over a track-point candidate → nothing happens.
6. Hover the same JPEG candidate twice in a row → second hover is instant (cache hit). Verify in DevTools Network tab: only one request per unique path.
7. Hover near the right or bottom edge of the window → thumbnail stays on-screen (clamped).
8. Click a different target photo → hover handlers re-attach on the new panel's candidates. Old panel's handlers don't leak (those elements no longer exist in the DOM).

### C.5 Commit

Single commit: `feat: hover thumbnail preview for candidate cards`.

### C.6 Phase C exit criteria

- [ ] JPEG hover works
- [ ] HEIC hover silently no-ops
- [ ] GPS track point hover silently no-ops
- [ ] No new Go dependencies
- [ ] `thumbnail.go` was not duplicated — existing generator reused
- [ ] `wails build -platform windows/amd64` passes
- [ ] `CLAUDE.md` (updated in phase B) accurately describes the new feature's files

---

## Final checklist

Before handing back to the owner:

- [ ] Phase A: `docs/AUDIT-2026-04.md` committed, all auto-fixes committed individually
- [ ] Phase B: `CLAUDE.md` updated in a single commit
- [ ] Phase C: hover thumbnail feature committed in a single commit
- [ ] `main` builds cleanly on Windows
- [ ] `go vet ./...` is clean
- [ ] `go test ./...` passes
- [ ] No file exceeds 150 lines (including new `hover_thumbnail.js` and updated `CLAUDE.md`-referenced files)

If any exit criterion cannot be met — for example, the audit uncovers something that requires owner input before it can be fixed — stop at that phase, document the blocker at the top of `docs/AUDIT-2026-04.md`, and return control. Do not skip ahead to the next phase with unresolved issues behind you.
