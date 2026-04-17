// matcher_ui.js — GPS matching UI.
// Handles the "Search for GPS match" button, Zone C panel rendering, and
// single-photo matching. Candidate selection lives in matcher_select.js;
// slider wiring lives in matcher_slider.js; HTML builders live in
// detail_render.js — all split out to keep each file under 150 lines.

import { state } from './state.js';
import { runMatching, runMatchingSingle, runSameSourceMatching } from './api.js';
import { renderTable } from './table.js';
import { escapeHtml } from './helpers.js';
import { renderPreview } from './preview.js';
import { updateMatchStats } from './scan.js';
import { buildDetailHTML } from './detail_render.js';
import { refreshLocationFor } from './geocode.js';
import { toggleMap, renderMap } from './map.js';
import { acceptBestCandidate, handleCandidateSelect } from './matcher_select.js';
import { initDeltaSlider } from './matcher_slider.js';
import { wireCandidateHovers } from './hover_thumbnail.js';

// initMatcher wires Zone A match controls and Zone C click delegation.
export function initMatcher() {
    const matchBtn = document.getElementById('btn-match-all');
    if (matchBtn) matchBtn.addEventListener('click', handleMatchAllClick);

    initDeltaSlider();

    // Phase 7: match-mode radio (External refs / GPS track / Same source).
    document.querySelectorAll('input[name="match-mode"]').forEach(r => {
        r.addEventListener('change', () => {
            if (r.checked) state.matchMode = r.value;
        });
    });

    document.addEventListener('photo-selected', e => showPhotoDetail(e.detail.photo));
    const panel = document.querySelector('.match-panel');
    if (panel) panel.addEventListener('click', handlePanelClick);
}

// handleMatchAllClick runs the full GPS matching engine and refreshes Zone B/C.
// Routes to the correct engine based on state.matchMode ('refs' / 'track' / 'same').
async function handleMatchAllClick() {
    if (state.targetPhotos.length === 0) { showZoneMessage('Scan a source folder first.'); return; }

    // Client-side precondition checks per mode. These are UX, not security —
    // the Go side validates independently.
    if (state.matchMode === 'refs' && state.referenceFolders.length === 0) {
        showZoneMessage('Add a reference folder (or switch to GPS track / Same source).'); return;
    }
    if (state.matchMode === 'track' && state.gpsTrackFiles.length === 0) {
        showZoneMessage('Import a GPS track (or switch to External refs / Same source).'); return;
    }
    // 'same' has no precondition beyond a scanned source folder.

    setMatchingIndicator(true);
    try {
        const opts = { maxTimeDeltaMinutes: state.matchThreshold };
        const results = state.matchMode === 'same'
            ? await runSameSourceMatching(opts)
            : await runMatching(opts);
        state.matchResults = Array.isArray(results) ? results : [];
        // Auto-accept the best candidate for every matched photo. The user can
        // uncheck individual rows in Zone B to exclude them from batch apply, or
        // click a different candidate in Zone C to change the coords.
        state.acceptedMatches.clear();
        state.matchResults.forEach(r => {
            if (r.bestCandidate) acceptBestCandidate(r.targetPath, r.bestCandidate);
        });
        renderTable(state.targetPhotos);
        updateMatchStats();
        document.dispatchEvent(new CustomEvent('match-complete'));
    } catch (err) {
        console.error('Matching failed:', err);
        showZoneMessage('Matching failed: ' + String(err));
    } finally {
        setMatchingIndicator(false);
    }
}

// setMatchingIndicator shows/hides the "Matching…" label in the indicator slot.
function setMatchingIndicator(visible) {
    const el = document.getElementById('scan-indicator');
    if (!el) return;
    el.textContent = visible ? 'Matching\u2026' : 'Scanning\u2026';
    el.classList.toggle('hidden', !visible);
}

// showPhotoDetail renders match details for a selected target photo in Zone C.
// Called by the 'photo-selected' DOM event from table.js.
export function showPhotoDetail(photo) {
    const panel = document.querySelector('.match-panel');
    if (!panel) return;
    const result = state.matchResults
        ? state.matchResults.find(r => r.targetPath === photo.path)
        : null;
    panel.innerHTML = buildDetailHTML(photo, result);
    renderPreview(photo, panel);

    // Kick off (cached) geocoding for the currently-accepted match, if any.
    // This single call replaces all the old scattered reverseGeocode sites
    // and fixes the race where re-renders orphaned previous .then() writes.
    const acc = state.acceptedMatches.get(photo.path);
    if (acc) refreshLocationFor(acc, panel);
    // Render the mini-map only when the user has opted in (phase 5).
    if (acc && state.mapEnabled) renderMap(acc.lat, acc.lon, panel);
    // Attach hover-preview handlers to each candidate. Re-attached on every
    // render; safe because buildDetailHTML replaces the panel's contents.
    wireCandidateHovers(panel);
}

// showZoneMessage puts a plain message in Zone C.
function showZoneMessage(msg) {
    const panel = document.querySelector('.match-panel');
    if (panel) panel.innerHTML = `<p class="muted panel-placeholder" style="margin-top:2rem;">${escapeHtml(msg)}</p>`;
}

// handlePanelClick delegates click events within Zone C. Apply / Undo buttons
// are handled by actions.js; this routes single-match, map-toggle, and
// candidate clicks.
function handlePanelClick(e) {
    const mapToggleBtn = e.target.closest('.btn-toggle-map');
    if (mapToggleBtn) { toggleMap(); return; }
    const matchSingleBtn = e.target.closest('.btn-match-single');
    if (matchSingleBtn) { handleMatchSingle(matchSingleBtn); return; }
    const candidateRow = e.target.closest('.detail-candidate');
    if (candidateRow) { handleCandidateSelect(candidateRow); return; }
}

// handleMatchSingle runs GPS matching for a single selected photo.
async function handleMatchSingle(btn) {
    const targetPath = btn.dataset.path;
    btn.disabled = true;
    btn.textContent = 'Searching\u2026';
    try {
        const result = await runMatchingSingle(targetPath, { maxTimeDeltaMinutes: state.matchThreshold });
        if (!state.matchResults) state.matchResults = [];
        const idx = state.matchResults.findIndex(r => r.targetPath === targetPath);
        if (idx >= 0) state.matchResults[idx] = result; else state.matchResults.push(result);
        // Auto-accept its best candidate so the Zone B checkbox is ticked.
        if (result && result.bestCandidate) acceptBestCandidate(targetPath, result.bestCandidate);
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
