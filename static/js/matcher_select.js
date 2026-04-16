// matcher_select.js — Accept helpers + Zone C candidate radio-selection.
// Split out of matcher_ui.js to keep both files under the 150-line limit.

import { state } from './state.js';

// acceptBestCandidate writes the photo's best match into acceptedMatches
// so its Zone B checkbox is ticked and batch apply picks it up.
// Used right after a match run (both all-photos and single-photo modes).
export function acceptBestCandidate(targetPath, best) {
    state.acceptedMatches.set(targetPath, {
        lat: best.gps.latitude,
        lon: best.gps.longitude,
        score: best.score,
        source: best.source,
        sourcePath: best.sourcePath,
    });
}

// handleCandidateSelect implements exclusive radio-style selection.
// Clicking the already-selected candidate deselects it (toggle off).
// Clicking any other candidate sets it as the accepted match, which also
// re-ticks the Zone B checkbox if the user had unticked it. That is
// intentional: "I want this one instead" implies "include this photo".
export function handleCandidateSelect(row) {
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
// Uses the 'photo-selected' event to avoid a circular import with matcher_ui.js.
function refreshAfterDecision(targetPath, accepted) {
    const row = document.querySelector(`.photo-row[data-path="${CSS.escape(targetPath)}"]`);
    const badge = row && row.querySelector('.col-status .badge');
    if (badge) {
        badge.className = `badge ${accepted ? 'badge-applied' : 'badge-matched'}`;
        badge.textContent = accepted ? 'accepted' : 'matched';
    }
    // Also keep the Zone B row checkbox in sync with the accepted-map state.
    const cb = row && row.querySelector('.row-select-cb');
    if (cb) cb.checked = accepted;

    if (state.selectedPhoto) {
        const photo = state.targetPhotos.find(p => p.path === state.selectedPhoto);
        if (photo) document.dispatchEvent(new CustomEvent('photo-selected', { detail: { photo } }));
    }
}
