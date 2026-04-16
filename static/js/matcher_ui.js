// matcher_ui.js — GPS matching UI (Phase 4/5)
// Handles the "Search for GPS match" button, threshold selector, Zone C
// panel rendering, single-photo matching, and candidate radio-selection.
// HTML construction lives in detail_render.js so both files stay small.

import { state } from './state.js';
import { runMatching, runMatchingSingle } from './api.js';
import { renderTable } from './table.js';
import { escapeHtml } from './helpers.js';
import { renderPreview } from './preview.js';
import { updateMatchStats } from './scan.js';
import { buildDetailHTML } from './detail_render.js';
import { refreshLocationFor } from './geocode.js';
import { toggleMap, renderMap } from './map.js';

// formatDeltaMinutes turns a minute count into the compact label shown next
// to the slider: "5 min", "30 min", "2 h", "1 h 30".
function formatDeltaMinutes(m) {
    if (m < 60) return `${m} min`;
    const h = Math.floor(m / 60);
    const rem = m % 60;
    if (rem === 0) return `${h} h`;
    return `${h} h ${rem}`;
}

// initMatcher wires Zone A match controls and Zone C click delegation.
export function initMatcher() {
    const matchBtn = document.getElementById('btn-match-all');
    if (matchBtn) matchBtn.addEventListener('click', handleMatchAllClick);

    const slider = document.getElementById('delta-slider');
    const valueLabel = document.getElementById('delta-slider-value');
    if (slider && valueLabel) {
        slider.value = String(state.matchThreshold);
        valueLabel.textContent = formatDeltaMinutes(state.matchThreshold);
        // Live-update the label on drag; commit to state only on release so
        // scan/match stats don't recompute 60 times/sec while the user drags.
        slider.addEventListener('input', () => {
            valueLabel.textContent = formatDeltaMinutes(parseInt(slider.value, 10));
        });
        slider.addEventListener('change', () => {
            state.matchThreshold = parseInt(slider.value, 10);
        });
    }

    document.addEventListener('photo-selected', e => showPhotoDetail(e.detail.photo));
    const panel = document.querySelector('.match-panel');
    if (panel) panel.addEventListener('click', handlePanelClick);
}

// handleMatchAllClick runs the full GPS matching engine and refreshes Zone B/C.
async function handleMatchAllClick() {
    if (state.targetPhotos.length === 0) { showZoneMessage('Scan a source folder first.'); return; }
    if (state.referenceFolders.length === 0 && state.gpsTrackFiles.length === 0) {
        showZoneMessage('Add a reference folder or import a GPS track first.'); return;
    }
    setMatchingIndicator(true);
    try {
        const results = await runMatching({ maxTimeDeltaMinutes: state.matchThreshold });
        state.matchResults = Array.isArray(results) ? results : [];
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
    if (acc) {
        refreshLocationFor(acc, panel);
    }
    // Render the mini-map only when the user has opted in (phase 5).
    if (acc && state.mapEnabled) {
        renderMap(acc.lat, acc.lon, panel);
    }
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

// handleCandidateSelect implements exclusive radio-style selection.
// Clicking the already-selected candidate deselects it (toggle off).
function handleCandidateSelect(row) {
    const d = row.dataset;
    const targetPath = d.path;
    const lat = parseFloat(d.lat), lon = parseFloat(d.lon);
    const current = state.acceptedMatches.get(targetPath);
    const same = current && current.sourcePath === d.sourcePath && Math.abs(current.lat - lat) < 1e-6;
    if (same) state.acceptedMatches.delete(targetPath);
    else state.acceptedMatches.set(targetPath, {
        lat, lon, score: parseInt(d.score, 10), source: d.source, sourcePath: d.sourcePath
    });
    refreshAfterDecision(targetPath, !same);
}

// refreshAfterDecision updates the Zone B status badge and re-renders Zone C.
function refreshAfterDecision(targetPath, accepted) {
    const row = document.querySelector(`.photo-row[data-path="${CSS.escape(targetPath)}"]`);
    const badge = row && row.querySelector('.col-status .badge');
    if (badge) {
        badge.className = `badge ${accepted ? 'badge-applied' : 'badge-matched'}`;
        badge.textContent = accepted ? 'accepted' : 'matched';
    }
    if (state.selectedPhoto) {
        const photo = state.targetPhotos.find(p => p.path === state.selectedPhoto);
        if (photo) showPhotoDetail(photo);
    }
}
