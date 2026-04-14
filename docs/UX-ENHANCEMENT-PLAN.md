# UX Enhancement Plan — GeoPhotoTagger

> **Purpose:** Step-by-step Claude Code implementation instructions for reworking the
> GeoPhotoTagger frontend UX. Every step specifies which files to read, what to change,
> and the expected outcome. Execute steps in order — each builds on the previous one.
>
> **Scope:** Frontend only (HTML, CSS, JS). No Go backend changes unless explicitly noted.
> All existing CLAUDE.md rules remain in force. Test with `wails dev` after each step.

---

## Pre-flight Checklist

Before starting any step:

1. Read `CLAUDE.md` fully — it is the law.
2. Read every file you are about to modify **before** editing it.
3. Run `wails dev` to confirm the app compiles and renders.
4. Keep every JS file under 150 lines, every function under 50 lines.
5. Do **not** touch Go files unless a step explicitly says so.

---

## Architecture of Changes

The current topbar crams five conceptually different zones into one `<header>`:
folder selection, GPS source management, matching controls, filtering, and
status chips. The redesign separates these into clearly labelled groups that
follow the user's actual workflow:

```
┌─────────────────────────────────────────────────────────────────────────┐
│ ZONE A — Top Bar (redesigned into 3 logical groups)                     │
│                                                                         │
│ ┌─── GROUP 1: DATA SOURCES ──────────────────────────────────────────┐  │
│ │ [Source ▾ path... ✕]  [GPS Ref]  [Import Track]                    │  │
│ │ References: [chip] [chip]    GPS Tracks: [chip] [chip]             │  │
│ └────────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│ ┌─── GROUP 2: MATCHING ─────────────────────────────────────────────┐  │
│ │ [Search for GPS match]  Max delta: [10|30|60]  Scanning...         │  │
│ │ Stats: 234 photos │ 156 matched │ 78 unmatched                     │  │
│ └────────────────────────────────────────────────────────────────────┘  │
│                                                                         │
│ ┌─── GROUP 3: VIEW CONTROLS ────────────────────────────────────────┐  │
│ │ Filter: [All|Matched|Unmatched]     [Apply All Accepted]           │  │
│ └────────────────────────────────────────────────────────────────────┘  │
├──────────────────────────────┬──────────────────────────────────────────┤
│ ZONE B — Target Photos       │ ZONE C — Match Details                   │
│ (sortable column headers)    │ (candidate selection + GPS preview)      │
└──────────────────────────────┴──────────────────────────────────────────┘
```

---

## Step 1 — Restructure `index.html` topbar into 3 logical groups

**Goal:** Replace the 5-row topbar with 3 visually separated groups.

**Read first:** `static/index.html`

**Replace** the entire `<header class="topbar">...</header>` block with:

```html
<!-- ZONE A — Top Bar (redesigned: 3 logical groups) -->
<header class="topbar">

    <!-- GROUP 1: Data Sources — folder selection + GPS reference management -->
    <div class="topbar-group" id="group-sources">
        <div class="group-label">Data Sources</div>
        <div class="topbar-row">
            <span class="app-title">GeoPhotoTagger</span>
            <input id="target-folder-path" class="folder-input"
                   type="text" placeholder="No source folder selected..." readonly>
            <button id="btn-browse" class="btn btn-primary"
                    title="Select folder containing photos without GPS data">Source</button>
            <button id="btn-reset-source" class="btn btn-sm btn-secondary btn-icon"
                    title="Clear source folder and reset photo list"
                    style="display:none">&#x2715;</button>
            <div class="btn-separator"></div>
            <button id="btn-add-reference" class="btn btn-secondary"
                    title="Add a folder of geolocated reference photos">GPS Ref</button>
            <button id="btn-import-track" class="btn btn-secondary"
                    title="Import a GPX, KML, or CSV GPS track file">Import Track</button>
        </div>
        <!-- Reference folders chip list -->
        <div id="reference-list-row" class="topbar-row chip-row" style="display:none">
            <span class="label-small">References:</span>
            <div id="reference-list" class="chip-list"></div>
        </div>
        <!-- GPS track files chip list -->
        <div id="track-list-row" class="topbar-row chip-row" style="display:none">
            <span class="label-small">GPS Tracks:</span>
            <div id="track-list" class="chip-list"></div>
        </div>
    </div>

    <!-- GROUP 2: Matching — run match + threshold + stats summary -->
    <div class="topbar-group" id="group-matching">
        <div class="group-label">Matching</div>
        <div class="topbar-row">
            <button id="btn-match-all" class="btn btn-accent"
                    title="Search for GPS matches for all photos without GPS data">Search for GPS match</button>
            <div class="btn-separator"></div>
            <span class="label-small">Max delta:</span>
            <button class="btn btn-sm btn-secondary threshold-btn" data-minutes="10"
                    title="Only match photos taken within 10 minutes of a GPS source">10 min</button>
            <button class="btn btn-sm btn-secondary threshold-btn active" data-minutes="30"
                    title="Only match photos taken within 30 minutes of a GPS source">30 min</button>
            <button class="btn btn-sm btn-secondary threshold-btn" data-minutes="60"
                    title="Only match photos taken within 60 minutes of a GPS source">60 min</button>
            <span id="scan-indicator" class="scan-indicator hidden">Scanning&#8230;</span>
        </div>
        <div class="topbar-row">
            <span id="match-stats" class="match-stats muted">No scan results yet</span>
        </div>
    </div>

    <!-- GROUP 3: View Controls — filter + batch action -->
    <div class="topbar-group" id="group-view">
        <div class="group-label">View</div>
        <div class="topbar-row">
            <span class="label-small">Filter:</span>
            <button class="btn btn-sm btn-secondary filter-btn active" data-filter="all"
                    title="Show all scanned photos">All</button>
            <button class="btn btn-sm btn-secondary filter-btn" data-filter="matched"
                    title="Show only photos with GPS matches">Matched</button>
            <button class="btn btn-sm btn-secondary filter-btn" data-filter="unmatched"
                    title="Show only photos without GPS matches">Unmatched</button>
            <button id="btn-apply-all" class="btn btn-sm btn-primary" style="margin-left:auto"
                    title="Write GPS coordinates to all accepted matches">
                Apply All Accepted
            </button>
        </div>
    </div>

</header>
```

**Key changes from the original:**
- Button text: `Browse` → `Source`, `+ Reference` → `GPS Ref`, `Match All` → `Search for GPS match`
- All main buttons now have `title` attributes for mouseover tooltips
- Sort dropdown removed from topbar (it moves to table headers in Step 4)
- Added `#btn-reset-source` (hidden until a source is selected)
- Reference/track chip rows hidden by default; shown only when items are added
- Match stats line added (`#match-stats`) — replaces relying on the status bar alone
- Three `topbar-group` wrappers with `.group-label` headings

---

## Step 2 — Add CSS for topbar groups and reset button

**Goal:** Style the 3 logical groups, the separator, and the reset button.

**Read first:** `static/css/layout.css`, `static/css/components.css`

**Append to `static/css/layout.css`** (after the existing `.topbar` block):

```css
/* ── Topbar group containers ─────────────────────────────────────── */

.topbar-group {
  padding: var(--space-xs) 0;
  border-bottom: 1px solid var(--border);
}
.topbar-group:last-child {
  border-bottom: none;
}

.group-label {
  font-size: 0.65rem;
  font-weight: var(--font-weight-semi);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  color: var(--text-light);
  margin-bottom: var(--space-xs);
  opacity: 0.7;
}

/* Vertical separator between button groups in the same row */
.btn-separator {
  width: 1px;
  height: 20px;
  background: var(--border);
  margin: 0 var(--space-xs);
}

/* Chip row: hidden by default, revealed by JS when items are present */
.chip-row {
  margin-top: var(--space-xs);
}

/* Match stats summary line */
.match-stats {
  font-size: 0.8rem;
}
```

**Append to `static/css/components.css`** (after the existing `.btn-sm` block):

```css
/* Icon-only button (e.g. reset ✕) — compact square */
.btn-icon {
  padding: var(--space-xs);
  min-width: 28px;
  justify-content: center;
  font-size: 0.75rem;
}
```

---

## Step 3 — Implement source reset button and suppress file listing

**Goal:**
1. Add a reset button that clears the source folder (and optionally reference folders).
2. Stop printing file lists to the console when scanning source or reference folders.

**Read first:** `static/js/scan.js`, `static/js/reference.js`, `static/js/state.js`

### 3a — `static/js/scan.js`

Add the reset button wiring inside `initScan()` and create a `handleResetSource()` handler.

**In `initScan()`**, after the `browseBtn` listener, add:

```javascript
const resetBtn = document.getElementById('btn-reset-source');
if (resetBtn) {
    resetBtn.addEventListener('click', handleResetSource);
}
```

**After `handleBrowseClick()`**, add this new function:

```javascript
// handleResetSource clears the source folder, all target photos, and match results.
// Resets the UI to its initial empty state.
async function handleResetSource() {
    state.targetFolder = null;
    state.targetPhotos = [];
    state.matchResults = null;
    state.selectedPhoto = null;
    state.acceptedMatches.clear();

    const pathInput = document.getElementById('target-folder-path');
    if (pathInput) pathInput.value = '';

    const resetBtn = document.getElementById('btn-reset-source');
    if (resetBtn) resetBtn.style.display = 'none';

    renderTable([]);
    updateMatchStats();
    setStatusBar('Ready \u2014 select a source folder to begin');

    // Clear Zone C
    const panel = document.querySelector('.match-panel');
    if (panel) panel.innerHTML = '<p class="muted panel-placeholder">Select a photo from the list to see match details.</p>';
}
```

**In `runScan()`**, after `state.targetFolder = path;` is set (inside `handleBrowseClick`), show the reset button:

```javascript
const resetBtn = document.getElementById('btn-reset-source');
if (resetBtn) resetBtn.style.display = '';
```

Also, add and export `updateMatchStats()` in `scan.js`:

```javascript
// updateMatchStats refreshes the #match-stats summary in the topbar.
export function updateMatchStats() {
    const el = document.getElementById('match-stats');
    if (!el) return;
    const total = state.targetPhotos.length;
    if (total === 0) {
        el.textContent = 'No scan results yet';
        return;
    }
    const matched = state.matchResults
        ? state.matchResults.filter(r => r.bestCandidate).length
        : 0;
    el.textContent = `${total} photo${total !== 1 ? 's' : ''} \u2502 ${matched} matched \u2502 ${total - matched} unmatched`;
}
```

**Important:** Remove any `console.log` or visible-to-user file listing that prints every file found during scan. The scan result should only populate the table, not dump file paths anywhere. Check `runScan()` and `handleBrowseClick()` — if either logs the full `photos` array, remove that log line.

### 3b — `static/js/reference.js`

**In `renderReferenceList()`**: After rendering chips, show or hide the chip row:

```javascript
const row = document.getElementById('reference-list-row');
if (row) row.style.display = state.referenceFolders.length > 0 ? '' : 'none';
```

Remove any `console.log` calls that dump the full list of reference photos found.

### 3c — `static/js/track.js`

Apply the same chip-row visibility logic in `renderTrackList()`:

```javascript
const row = document.getElementById('track-list-row');
if (row) row.style.display = state.gpsTrackFiles.length > 0 ? '' : 'none';
```

---

## Step 4 — Move sorting from dropdown into clickable table column headers

**Goal:** Remove the `<select id="sort-select">` dropdown (already removed from HTML in Step 1)
and make each `<th>` in the target photos table a clickable sort toggle. Clicking a column
header sorts ascending; clicking it again reverses to descending.

**Read first:** `static/js/table.js`, `static/js/filters.js`, `static/css/table.css`

### 4a — `static/js/table.js`

Replace `buildHeader()` with a version that adds sort-toggle `data-sort` attributes
and appends a sort-arrow `<span>`:

```javascript
// Module-level sort state for column header toggling.
let sortColumn = 'filename';
let sortDirection = 'asc'; // 'asc' or 'desc'

// buildHeader creates the <thead> with clickable sortable column headers.
function buildHeader() {
    const thead = document.createElement('thead');
    const tr = document.createElement('tr');

    const columns = [
        { key: 'num',      label: '#',           sortable: false, cls: 'col-num' },
        { key: 'filename', label: 'Filename',     sortable: true,  cls: 'col-filename' },
        { key: 'date',     label: 'Date / Time',  sortable: true,  cls: 'col-date' },
        { key: 'camera',   label: 'Camera',       sortable: true,  cls: 'col-camera' },
        { key: 'score',    label: 'Score',         sortable: true,  cls: 'col-score' },
        { key: 'status',   label: 'Status',        sortable: true,  cls: 'col-status' },
    ];

    columns.forEach(col => {
        const th = document.createElement('th');
        th.className = col.cls;
        if (col.sortable) {
            th.dataset.sort = col.key;
            th.title = `Sort by ${col.label}`;
            const arrow = col.key === sortColumn
                ? (sortDirection === 'asc' ? ' \u25B2' : ' \u25BC')
                : '';
            th.innerHTML = `${col.label}<span class="sort-arrow">${arrow}</span>`;
            if (col.key === sortColumn) {
                th.classList.add(sortDirection === 'asc' ? 'sort-asc' : 'sort-desc');
            }
            th.addEventListener('click', () => handleHeaderSort(col.key));
        } else {
            th.textContent = col.label;
        }
        tr.appendChild(th);
    });

    thead.appendChild(tr);
    return thead;
}
```

Add a `handleHeaderSort()` function in `table.js`:

```javascript
// handleHeaderSort toggles sort direction when the same column is clicked,
// or switches to ascending for a new column. Dispatches 'sort-changed'
// so filters.js re-applies the sort.
function handleHeaderSort(column) {
    if (sortColumn === column) {
        sortDirection = sortDirection === 'asc' ? 'desc' : 'asc';
    } else {
        sortColumn = column;
        sortDirection = 'asc';
    }
    document.dispatchEvent(new CustomEvent('sort-changed', {
        detail: { column: sortColumn, direction: sortDirection }
    }));
}
```

Export `sortColumn` and `sortDirection` getters (or export the variables directly) so
`filters.js` can read them.

### 4b — `static/js/filters.js`

- Remove the `let currentSort = 'filename';` module-level variable.
- Remove the `<select id="sort-select">` event listener from `initFilters()`.
- Import `sortColumn` and `sortDirection` from `table.js` (or listen for the `sort-changed` event).
- In `initFilters()`, add:

```javascript
document.addEventListener('sort-changed', () => applyFilters());
```

- In `applyFilters()`, replace the sort step to use `sortColumn` / `sortDirection` from
  `table.js` instead of the old `currentSort`. The sort comparators remain the same, but
  multiply by `-1` when `sortDirection === 'desc'`.

The updated sort block:

```javascript
import { getSortState } from './table.js';

// In applyFilters():
const { column, direction } = getSortState();
const dir = direction === 'desc' ? -1 : 1;

filtered = [...filtered].sort((a, b) => {
    let cmp = 0;
    if (column === 'date') {
        const ta = a.dateTimeOriginal || '';
        const tb = b.dateTimeOriginal || '';
        cmp = ta.localeCompare(tb);
    } else if (column === 'score') {
        const sa = bestMap.has(a.path) ? bestMap.get(a.path).score : -1;
        const sb = bestMap.has(b.path) ? bestMap.get(b.path).score : -1;
        cmp = sa - sb; // ascending by default; dir handles reversal
    } else if (column === 'status') {
        const ha = bestMap.has(a.path) ? 0 : 1;
        const hb = bestMap.has(b.path) ? 0 : 1;
        cmp = ha !== hb ? ha - hb : (a.filename || '').localeCompare(b.filename || '');
    } else if (column === 'camera') {
        cmp = (a.cameraModel || '').localeCompare(b.cameraModel || '');
    } else {
        // Default: filename
        cmp = (a.filename || '').localeCompare(b.filename || '');
    }
    return cmp * dir;
});
```

Export a `getSortState()` helper from `table.js`:

```javascript
// getSortState returns the current sort column and direction for external use.
export function getSortState() {
    return { column: sortColumn, direction: sortDirection };
}
```

---

## Step 5 — Single-photo matching ("Match Selected")

**Goal:** Allow the user to match one selected photo instead of always running a full batch.
When a single photo is selected in Zone B and the user clicks a context-aware button,
only that photo is matched.

**This step requires a small Go backend addition.**

### 5a — Go: Add `RunMatchingSingle` to `app_match.go`

**Read first:** `app_match.go`, `matcher.go`

Add a new exported method on `*App`:

```go
// RunMatchingSingle runs the GPS matching engine for a single target photo,
// identified by its absolute path. Returns the MatchResult for that photo only.
// If the photo is not found in the loaded target list, returns an error.
//
// This allows the user to match one photo at a time from the frontend,
// without re-matching the entire set.
func (a *App) RunMatchingSingle(targetPath string, opts MatchOptions) (MatchResult, error) {
    // Find the target photo in the loaded list.
    var target *TargetPhoto
    for i := range a.targetPhotos {
        if a.targetPhotos[i].Path == targetPath {
            target = &a.targetPhotos[i]
            break
        }
    }
    if target == nil {
        return MatchResult{}, fmt.Errorf("target photo not found: %s", targetPath)
    }

    // Require at least one GPS source.
    if len(a.referencePhotos) == 0 && len(a.gpsTrackPoints) == 0 {
        return MatchResult{}, fmt.Errorf("no GPS sources loaded")
    }

    // Match just this one photo by passing a single-element slice.
    results := MatchPhotos([]TargetPhoto{*target}, a.referencePhotos, a.gpsTrackPoints, opts)

    if len(results) == 0 {
        return MatchResult{TargetPath: targetPath}, nil
    }

    result := results[0]

    // Update this photo in app state.
    if result.BestCandidate != nil {
        target.BestMatch = result.BestCandidate
        target.Status = "matched"
    } else {
        target.Status = "unmatched"
    }

    // Merge into matchResults: replace existing entry or append.
    found := false
    for i := range a.matchResults {
        if a.matchResults[i].TargetPath == targetPath {
            a.matchResults[i] = result
            found = true
            break
        }
    }
    if !found {
        a.matchResults = append(a.matchResults, result)
    }

    return result, nil
}
```

### 5b — JS: Add `runMatchingSingle` to `api.js`

**Read first:** `static/js/api.js`

Append:

```javascript
// Run GPS matching for a single target photo identified by its absolute path.
// Returns a MatchResult for that photo only.
export async function runMatchingSingle(targetPath, opts) {
    return window.go.main.App.RunMatchingSingle(targetPath, opts);
}
```

### 5c — JS: Wire "Match Selected" into `matcher_ui.js`

**Read first:** `static/js/matcher_ui.js`

In `showPhotoDetail()` (which renders Zone C), add a "Search for GPS match" button
for the selected photo when it has no match result yet (or has been left unmatched).

After the `buildDetailHTML()` call in `showPhotoDetail()`, append a match-single button
section at the top of the detail if no match exists:

**Inside `buildDetailHTML()`**, when `!result || !result.bestCandidate`, replace the
"No GPS match found" placeholder with:

```javascript
if (!result || !result.bestCandidate) {
    return html + `
        <div class="detail-section" style="margin-top:1rem;">
            <p class="muted">No GPS match found within ${state.matchThreshold} minutes.</p>
            <button class="btn btn-sm btn-accent btn-match-single"
                data-path="${escapeHtml(photo.path)}"
                title="Search for GPS matches for this photo only"
                style="margin-top:var(--space-sm)">Search for GPS match</button>
        </div>`;
}
```

**In `handlePanelClick()`**, add a handler for `.btn-match-single`:

```javascript
const matchSingleBtn = e.target.closest('.btn-match-single');
if (matchSingleBtn) {
    handleMatchSingle(matchSingleBtn);
    return;
}
```

**Add the handler function:**

```javascript
// handleMatchSingle runs GPS matching for a single selected photo.
async function handleMatchSingle(btn) {
    const targetPath = btn.dataset.path;
    btn.disabled = true;
    btn.textContent = 'Searching\u2026';

    try {
        const result = await runMatchingSingle(targetPath, {
            maxTimeDeltaMinutes: state.matchThreshold
        });

        // Merge result into state.matchResults
        if (!state.matchResults) state.matchResults = [];
        const idx = state.matchResults.findIndex(r => r.targetPath === targetPath);
        if (idx >= 0) {
            state.matchResults[idx] = result;
        } else {
            state.matchResults.push(result);
        }

        // Re-render the table row and Zone C
        renderTable(state.targetPhotos);
        updateMatchStats();
        const photo = state.targetPhotos.find(p => p.path === targetPath);
        if (photo) showPhotoDetail(photo);
    } catch (err) {
        console.error('Single match failed:', err);
        btn.disabled = false;
        btn.textContent = 'Search for GPS match';
    }
}
```

Import `runMatchingSingle` from `api.js` and `updateMatchStats` from `scan.js` at the top of `matcher_ui.js`.

---

## Step 6 — Redesign Zone C candidate selection with exclusive radio-style pick

**Goal:** When a match is found with one or more candidates, the user can:
1. Click a candidate row to preview its GPS data (coordinates, reverse-geocoded location).
2. Select exactly one candidate (radio-button style — selecting one deselects any other).
3. The selected candidate becomes the accepted match for that photo.

**Read first:** `static/js/matcher_ui.js`, `static/css/components.css`

### 6a — Replace candidate row rendering in `buildDetailHTML()`

Replace the candidates section in `buildDetailHTML()` with:

```javascript
if (result.candidates && result.candidates.length > 0) {
    html += `<div class="detail-section">
        <div class="detail-section-title">Candidates (${result.candidates.length}) — click to select</div>`;

    result.candidates.forEach((c, idx) => {
        // Determine if this candidate is the currently accepted one
        const isSelected = acc
            && acc.sourcePath === c.sourcePath
            && Math.abs(acc.lat - c.gps.latitude) < 1e-6;

        html += `
            <div class="detail-candidate${isSelected ? ' selected' : ''}"
                 data-candidate-idx="${idx}"
                 data-path="${escapeHtml(photo.path)}"
                 data-lat="${c.gps.latitude}"
                 data-lon="${c.gps.longitude}"
                 data-score="${c.score}"
                 data-source="${escapeHtml(c.source || '')}"
                 data-source-path="${escapeHtml(c.sourcePath || '')}"
                 data-source-filename="${escapeHtml(c.sourceFilename || '')}"
                 data-time-delta="${escapeHtml(c.timeDeltaFormatted || '')}"
                 title="Click to select this candidate as the GPS source">
                <span class="candidate-radio">${isSelected ? '&#x25C9;' : '&#x25CB;'}</span>
                <span class="badge ${scoreBadgeClass(c.score)}">${c.score}</span>
                <span class="detail-source">${escapeHtml(c.sourceFilename)}</span>
                <span class="muted">${c.timeDeltaFormatted}</span>
                <span class="muted" style="font-size:0.75rem">${c.source === 'track' ? 'track' : 'photo'}</span>
                ${c.isInterpolated ? '<span class="chip-format" style="margin-left:4px">interpolated</span>' : ''}
            </div>`;
    });
    html += '</div>';

    // GPS detail preview area — populated when a candidate is clicked
    html += `<div id="candidate-gps-preview" class="detail-section candidate-preview">
        ${isSelected(acc) ? buildGPSPreview(acc) : '<p class="muted" style="font-size:0.85rem;">Click a candidate above to preview GPS data.</p>'}
    </div>`;
}
```

Remove the old per-candidate Accept/Reject buttons. The new UX is: click a candidate row
to select it (radio-style). Click it again to deselect.

**Helper function** to check if there's an accepted match:

```javascript
function isSelected(acc) {
    return acc && acc.lat !== undefined;
}
```

**Helper function** to build the GPS preview panel:

```javascript
// buildGPSPreview renders coordinates and location info for the selected candidate.
function buildGPSPreview(acc) {
    return `
        <div class="gps-preview-card">
            <div class="gps-coords">
                <span class="label-small">Coordinates:</span>
                <span class="gps-value">${acc.lat.toFixed(6)}, ${acc.lon.toFixed(6)}</span>
            </div>
            <div id="gps-location-info" class="gps-location muted">
                Loading location...
            </div>
        </div>`;
}
```

### 6b — Handle candidate click in `handlePanelClick()`

Replace the old `handleAccept` / `handleReject` routing with:

```javascript
function handlePanelClick(e) {
    const candidateRow = e.target.closest('.detail-candidate');
    const matchSingleBtn = e.target.closest('.btn-match-single');

    if (matchSingleBtn) {
        handleMatchSingle(matchSingleBtn);
        return;
    }

    if (candidateRow) {
        handleCandidateSelect(candidateRow);
        return;
    }
}
```

**New `handleCandidateSelect()`:**

```javascript
// handleCandidateSelect implements exclusive radio-style candidate selection.
// Clicking the already-selected candidate deselects it (toggle off).
function handleCandidateSelect(row) {
    const d = row.dataset;
    const targetPath = d.path;
    const currentAcc = state.acceptedMatches.get(targetPath);

    // Toggle: if this candidate is already selected, deselect it
    const isAlreadySelected = currentAcc
        && currentAcc.sourcePath === d.sourcePath
        && Math.abs(currentAcc.lat - parseFloat(d.lat)) < 1e-6;

    if (isAlreadySelected) {
        state.acceptedMatches.delete(targetPath);
    } else {
        state.acceptedMatches.set(targetPath, {
            lat: parseFloat(d.lat),
            lon: parseFloat(d.lon),
            score: parseInt(d.score, 10),
            source: d.source,
            sourcePath: d.sourcePath
        });
    }

    // Re-render Zone C to reflect the new selection
    refreshAfterDecision(targetPath, !isAlreadySelected);
}
```

### 6c — Add CSS for candidate selection styling

**Append to `static/css/components.css`:**

```css
/* ── Candidate radio-select styling ──────────────────────────────── */

.detail-candidate {
    cursor: pointer;
    transition: background var(--transition), border-left var(--transition);
    padding-left: var(--space-sm);
}
.detail-candidate:hover {
    background: rgba(74, 144, 226, 0.05);
}

/* Selected candidate — green left border + light green background */
.detail-candidate.selected {
    background: rgba(80, 200, 120, 0.08);
    border-left: 3px solid var(--success);
    padding-left: calc(var(--space-sm) - 3px);
}

/* Radio indicator */
.candidate-radio {
    font-size: 1rem;
    color: var(--text-light);
    flex-shrink: 0;
    width: 20px;
    text-align: center;
}
.detail-candidate.selected .candidate-radio {
    color: var(--success);
}

/* GPS preview card */
.candidate-preview {
    margin-top: var(--space-sm);
    padding: var(--space-sm);
    background: var(--bg-subtle);
    border-radius: var(--radius);
    border: 1px solid var(--border);
}

.gps-preview-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-xs);
}

.gps-coords {
    display: flex;
    align-items: center;
    gap: var(--space-sm);
}

.gps-value {
    font-family: 'Courier New', monospace;
    font-size: 0.85rem;
    color: var(--primary);
    font-weight: var(--font-weight-semi);
}

.gps-location {
    font-size: 0.8rem;
}
```

**Note:** The `.detail-candidate.accepted` class from the old CSS should be removed and
replaced by `.detail-candidate.selected` as shown above. Clean up the old `.accepted` rule
in `components.css`.

---

## Step 7 — Reverse geocoding for GPS preview (optional enhancement)

**Goal:** When the user clicks a candidate, show the country/region/city for the GPS
coordinates. This requires a new Go backend method.

### 7a — Go: Add `ReverseGeocode` to `app.go`

This is an **offline** reverse-geocoding approach using a simple nearest-city lookup.
For a lightweight solution without external API calls, use the coordinate to provide
a basic description.

**Option A (minimal — no network):** Just display the raw coordinates. Skip this step
entirely and the GPS preview already works from Step 6.

**Option B (with network — recommended):** Add a Go method that calls the free
Nominatim OpenStreetMap API for reverse geocoding.

```go
// ReverseGeocode returns a human-readable location string for the given coordinates.
// Uses the OpenStreetMap Nominatim API (free, no API key required).
// Returns an empty string on error rather than failing the whole operation.
func (a *App) ReverseGeocode(lat, lon float64) string {
    url := fmt.Sprintf("https://nominatim.openstreetmap.org/reverse?lat=%f&lon=%f&format=json&zoom=10",
        lat, lon)

    client := &http.Client{Timeout: 5 * time.Second}
    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return ""
    }
    req.Header.Set("User-Agent", "GeoPhotoTagger/1.0")

    resp, err := client.Do(req)
    if err != nil {
        return ""
    }
    defer resp.Body.Close()

    var result struct {
        DisplayName string `json:"display_name"`
        Address     struct {
            City    string `json:"city"`
            Town    string `json:"town"`
            Village string `json:"village"`
            County  string `json:"county"`
            State   string `json:"state"`
            Country string `json:"country"`
        } `json:"address"`
    }

    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return ""
    }

    // Build a concise location string: City/Town, State, Country
    city := result.Address.City
    if city == "" { city = result.Address.Town }
    if city == "" { city = result.Address.Village }
    if city == "" { city = result.Address.County }

    parts := []string{}
    if city != "" { parts = append(parts, city) }
    if result.Address.State != "" { parts = append(parts, result.Address.State) }
    if result.Address.Country != "" { parts = append(parts, result.Address.Country) }

    if len(parts) == 0 {
        return result.DisplayName
    }
    return strings.Join(parts, ", ")
}
```

Add `"net/http"`, `"encoding/json"`, and `"strings"` to the imports.

### 7b — JS: Add `reverseGeocode` to `api.js`

```javascript
// Reverse-geocode GPS coordinates to a human-readable location string.
// Returns a string like "Paris, Île-de-France, France" or "" on error.
export async function reverseGeocode(lat, lon) {
    return window.go.main.App.ReverseGeocode(lat, lon);
}
```

### 7c — JS: Call reverse geocoding in `handleCandidateSelect()`

After the `refreshAfterDecision()` call, when a candidate is selected (not deselected),
trigger reverse geocoding:

```javascript
if (!isAlreadySelected) {
    // Fire-and-forget reverse geocoding
    reverseGeocode(parseFloat(d.lat), parseFloat(d.lon)).then(location => {
        const locEl = document.getElementById('gps-location-info');
        if (locEl) {
            locEl.textContent = location || 'Location not available';
        }
    });
}
```

Import `reverseGeocode` from `api.js` at the top of `matcher_ui.js`.

---

## Step 8 — Update `matcher_ui.js` match-stats integration

**Goal:** Make sure `updateMatchStats()` is called after every match operation
so the topbar stats line stays current.

**Read first:** `static/js/matcher_ui.js`, `static/js/scan.js`

In `matcher_ui.js`, import `updateMatchStats` from `scan.js`:

```javascript
import { updateMatchStats } from './scan.js';
```

In `handleMatchAllClick()`, after `renderTable(state.targetPhotos)`, call:

```javascript
updateMatchStats();
```

Remove the old `updateStatusBarCounts()` function from `matcher_ui.js` — its job is now
done by `updateMatchStats()` in `scan.js`. Keep the footer status bar for simple contextual
messages (scanning, errors), but the primary count display is now `#match-stats`.

---

## Step 9 — Tooltips on all remaining buttons

**Goal:** Ensure every interactive button in the UI has a `title` attribute.

Most buttons already got tooltips in Step 1 (HTML restructure). This step covers
dynamically-rendered buttons that are created in JS:

**In `table.js`:** The column headers already have `title` from Step 4.

**In `reference.js` / `track.js`:** The chip remove buttons already have
`title="Remove this folder"` / `title="Remove this track"`. Verify they are present.

**In `matcher_ui.js`:** The "Apply GPS" and "Undo" buttons rendered in `buildDetailHTML()`:

```javascript
// Apply GPS button
title="Write these GPS coordinates to the photo file"

// Undo button
title="Restore the photo from its backup file"
```

**In `actions.js`:** The confirm dialog buttons:

```javascript
// Confirm button
title="Proceed with applying GPS data"

// Cancel button
title="Cancel this operation"
```

---

## Step 10 — Final cleanup and validation

**Goal:** Ensure all changes are consistent and the app works end-to-end.

### Checklist

1. **Run `wails dev`** and verify the app compiles without errors.

2. **Test the user workflow** in order:
   - Click "Source" → native folder picker opens → target photos appear in Zone B table.
   - The reset ✕ button appears next to the source path.
   - Click ✕ → everything resets to initial state.
   - Click "GPS Ref" → reference folder chip appears in Group 1.
   - Click "Import Track" → track chip appears in Group 1.
   - Click "Search for GPS match" → all photos are matched → stats update in Group 2.
   - Click a table column header → table sorts by that column → click again to reverse.
   - Select an unmatched photo → Zone C shows "Search for GPS match" button for that photo.
   - Click per-photo match button → single photo is matched → Zone C updates.
   - With a matched photo selected, click candidate rows → radio-style selection works.
   - Only one candidate can be selected at a time (previous selection auto-clears).
   - GPS coordinates and location info appear in the preview panel.
   - "Apply All Accepted" processes all accepted matches.

3. **Verify no console spam:**
   - Scanning source folder does NOT print file paths to console.
   - Scanning reference folder does NOT print file paths to console.
   - Only meaningful log messages (`GeoPhotoTagger initialising...`, `GeoPhotoTagger ready.`)
     appear in the console.

4. **Verify tooltips:**
   - Hover over every button in Zone A — tooltip text appears.
   - Hover over column headers in Zone B — tooltip text appears.
   - Hover over candidate rows in Zone C — tooltip text appears.

5. **Verify file sizes:** No JS file exceeds 150 lines. If `matcher_ui.js` has grown
   past 150 lines after Step 6, extract the GPS preview logic into a new
   `static/js/gps_preview.js` module:
   - Export `buildGPSPreview(acc)` and the reverse-geocoding trigger.
   - Import in `matcher_ui.js`.

6. **Update `CLAUDE.md`:**
   - Update section 8 (UI Layout) to reflect the 3-group topbar.
   - Update the file listing in section 4 if any new JS modules were created.
   - Add `RunMatchingSingle` to the Go ↔ JavaScript bridge table in section 5.
   - Add `ReverseGeocode` to the bridge table if Step 7 was implemented.

---

## Summary of All File Changes

| File | Action | Step |
|------|--------|------|
| `static/index.html` | Replace topbar HTML | 1 |
| `static/css/layout.css` | Append group/separator styles | 2 |
| `static/css/components.css` | Append icon-btn, candidate, GPS preview styles | 2, 6c |
| `static/js/scan.js` | Add reset handler, updateMatchStats, suppress logs | 3a |
| `static/js/reference.js` | Chip-row visibility, suppress logs | 3b |
| `static/js/track.js` | Chip-row visibility | 3c |
| `static/js/table.js` | Clickable sort headers, export getSortState | 4a |
| `static/js/filters.js` | Use table sort state, remove dropdown | 4b |
| `app_match.go` | Add RunMatchingSingle method | 5a |
| `static/js/api.js` | Add runMatchingSingle wrapper | 5b |
| `static/js/matcher_ui.js` | Single-match button, candidate radio-select, GPS preview | 5c, 6, 8 |
| `app.go` | Add ReverseGeocode method (optional) | 7a |
| `static/js/api.js` | Add reverseGeocode wrapper (optional) | 7b |
| `CLAUDE.md` | Update layout diagram, bridge table, file list | 10 |

---

## Implementation Order Rationale

The steps are sequenced to minimize rework:

1. **HTML structure first** (Step 1) — all subsequent JS changes reference the new element IDs.
2. **CSS second** (Step 2) — styles must exist before JS renders into the new containers.
3. **Core JS wiring** (Steps 3–4) — reset, chip visibility, and sorting are independent of matching logic.
4. **Backend addition** (Step 5a) — needed before the frontend can call single-photo matching.
5. **Frontend matching** (Steps 5b–5c) — uses the new Go method.
6. **Candidate UX** (Step 6) — redesigns Zone C; depends on matching being functional.
7. **Reverse geocoding** (Step 7) — optional layer on top of candidate selection.
8. **Integration** (Steps 8–9) — wires stats and tooltips across all modules.
9. **Validation** (Step 10) — end-to-end testing after all changes are in place.
