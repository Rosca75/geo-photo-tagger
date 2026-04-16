// table_row.js — Per-row rendering + selection helpers for the Zone B table.
// Split out of table.js to keep that file under the 150-line limit.

import { state } from './state.js';
import { escapeHtml } from './helpers.js';
import { acceptBestCandidate } from './matcher_select.js';

// buildRow creates one <tr> for a single target photo.
// Clicking the row selects it, updates state.selectedPhoto, and fires
// 'photo-selected' for matcher_ui.js to render Zone C.
export function buildRow(photo, idx) {
    const tr = document.createElement('tr');
    tr.className = 'photo-row';
    tr.dataset.path = photo.path;

    // Format the EXIF datetime (Go serialises time.Time as ISO 8601 string)
    const rawDate = photo.dateTimeOriginal;
    const dateStr = rawDate && rawDate !== '0001-01-01T00:00:00Z'
        ? new Date(rawDate).toLocaleString()
        : '\u2014';

    const result = state.matchResults
        ? state.matchResults.find(r => r.targetPath === photo.path)
        : null;
    const best = result && result.bestCandidate ? result.bestCandidate : null;

    const scoreBadge = best
        ? `<span class="badge ${scoreBadgeClass(best.score)}">${best.score}</span>`
        : '<span class="muted">\u2014</span>';

    const statusClass = best ? 'badge-matched' : 'badge-unmatched';
    const statusLabel = best ? 'matched' : 'unmatched';

    // Row-select checkbox — only shown for matched photos (unmatched rows
    // have nothing to apply). Ticked when the photo is in acceptedMatches.
    const canSelect = !!best;
    const isSelected = state.acceptedMatches.has(photo.path);
    const cbHTML = canSelect
        ? `<input type="checkbox" class="row-select-cb" ${isSelected ? 'checked' : ''}
                  title="Include this photo in the next Apply GPS data batch">`
        : '';

    tr.innerHTML = `
        <td class="col-select">${cbHTML}</td>
        <td class="col-num">${idx + 1}</td>
        <td class="col-filename" title="${escapeHtml(photo.path)}">${escapeHtml(photo.filename)}</td>
        <td class="col-date">${dateStr}</td>
        <td class="col-camera">${escapeHtml(photo.cameraModel || '\u2014')}</td>
        <td class="col-score">${scoreBadge}</td>
        <td class="col-status"><span class="badge ${statusClass}">${statusLabel}</span></td>`;

    const cb = tr.querySelector('.row-select-cb');
    if (cb) {
        // Stop the row-level click handler from firing when the checkbox is clicked.
        cb.addEventListener('click', e => e.stopPropagation());
        cb.addEventListener('change', () => toggleRowSelection(photo, cb.checked));
    }

    tr.addEventListener('click', () => selectPhoto(photo, tr));
    return tr;
}

// toggleRowSelection updates state.acceptedMatches based on the checkbox
// state. Checking a row auto-accepts its best candidate; unchecking removes it.
function toggleRowSelection(photo, checked) {
    if (!checked) { state.acceptedMatches.delete(photo.path); return; }
    const result = state.matchResults &&
        state.matchResults.find(r => r.targetPath === photo.path);
    const best = result && result.bestCandidate;
    if (best) acceptBestCandidate(photo.path, best);
}

// scoreBadgeClass maps a numeric score to a CSS badge class name.
function scoreBadgeClass(score) {
    if (score >= 90) return 'badge-excellent';
    if (score >= 50) return 'badge-matched';
    return 'badge-poor';
}

// selectPhoto marks a row as selected, updates shared state, and fires
// 'photo-selected' so matcher_ui.js can update Zone C without a circular import.
function selectPhoto(photo, tr) {
    document.querySelectorAll('.photo-row.selected')
        .forEach(r => r.classList.remove('selected'));
    tr.classList.add('selected');
    state.selectedPhoto = photo.path;
    document.dispatchEvent(new CustomEvent('photo-selected', { detail: { photo } }));
}
