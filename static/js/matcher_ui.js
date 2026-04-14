// matcher_ui.js — GPS matching UI (Phase 4/5)
// Handles the "Search for GPS match" button, threshold selector, Zone C
// panel rendering, single-photo matching, and candidate radio-selection.
// HTML construction lives in detail_render.js so both files stay small.

import { state } from './state.js';
import { runMatching, runMatchingSingle, reverseGeocode } from './api.js';
import { renderTable } from './table.js';
import { escapeHtml } from './helpers.js';
import { renderPreview } from './preview.js';
import { updateMatchStats } from './scan.js';
import { buildDetailHTML } from './detail_render.js';

// initMatcher wires Zone A match controls and Zone C click delegation.
export function initMatcher() {
    const matchBtn = document.getElementById('btn-match-all');
    if (matchBtn) matchBtn.addEventListener('click', handleMatchAllClick);
    document.querySelectorAll('.threshold-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const val = parseInt(btn.dataset.minutes, 10);
            if (!isNaN(val)) setThreshold(val, btn);
        });
    });
    document.addEventListener('photo-selected', e => showPhotoDetail(e.detail.photo));
    const panel = document.querySelector('.match-panel');
    if (panel) panel.addEventListener('click', handlePanelClick);
}

// setThreshold updates state.matchThreshold and marks the active button.
function setThreshold(minutes, activeBtn) {
    state.matchThreshold = minutes;
    document.querySelectorAll('.threshold-btn').forEach(b => b.classList.remove('active'));
    if (activeBtn) activeBtn.classList.add('active');
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
}

// showZoneMessage puts a plain message in Zone C.
function showZoneMessage(msg) {
    const panel = document.querySelector('.match-panel');
    if (panel) panel.innerHTML = `<p class="muted panel-placeholder" style="margin-top:2rem;">${escapeHtml(msg)}</p>`;
}

// handlePanelClick delegates click events within Zone C. Apply / Undo buttons
// are handled by actions.js; this routes single-match and candidate clicks.
function handlePanelClick(e) {
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
    // Fire-and-forget reverse geocoding for the newly-selected candidate.
    if (!same) {
        reverseGeocode(lat, lon).then(location => {
            const locEl = document.getElementById('gps-location-info');
            if (locEl) locEl.textContent = location || 'Location not available';
        }).catch(() => { /* silent — raw coords still visible */ });
    }
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
