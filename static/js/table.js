// table.js — Target photos table rendering for Zone B
// Builds and updates the data table showing scanned target photos.
// Each row: row number, filename, date/time, camera model, status badge.

import { state } from './state.js';
import { escapeHtml } from './helpers.js';

// renderTable replaces the content of #target-table-container with a fresh
// table built from the given photos array.
export function renderTable(photos) {
    const container = document.getElementById('target-table-container');
    if (!container) return;

    if (!photos || photos.length === 0) {
        container.innerHTML = '<p class="muted panel-placeholder">No photos without GPS found in this folder.</p>';
        return;
    }

    container.innerHTML = '';
    container.appendChild(buildTable(photos));
}

// buildTable creates the full <table> element from the photos array.
function buildTable(photos) {
    const table = document.createElement('table');
    table.className = 'photo-table';
    table.appendChild(buildHeader());

    const tbody = document.createElement('tbody');
    photos.forEach((photo, idx) => tbody.appendChild(buildRow(photo, idx)));
    table.appendChild(tbody);

    return table;
}

// buildHeader returns a <thead> with the column header row.
function buildHeader() {
    const thead = document.createElement('thead');
    thead.innerHTML = `
        <tr>
            <th class="col-num">#</th>
            <th class="col-filename">Filename</th>
            <th class="col-date">Date / Time</th>
            <th class="col-camera">Camera</th>
            <th class="col-score">Score</th>
            <th class="col-status">Status</th>
        </tr>`;
    return thead;
}

// buildRow creates one <tr> for a single target photo.
// Clicking the row selects it, updates state.selectedPhoto, and fires 'photo-selected'.
function buildRow(photo, idx) {
    const tr = document.createElement('tr');
    tr.className = 'photo-row';
    tr.dataset.path = photo.path;

    // Format the EXIF datetime (Go serialises time.Time as ISO 8601 string)
    const rawDate = photo.dateTimeOriginal;
    const dateStr = rawDate && rawDate !== '0001-01-01T00:00:00Z'
        ? new Date(rawDate).toLocaleString()
        : '\u2014';

    // Look up match result for this photo from state.matchResults
    const result = state.matchResults
        ? state.matchResults.find(r => r.targetPath === photo.path)
        : null;
    const best = result && result.bestCandidate ? result.bestCandidate : null;

    // Score badge
    const scoreBadge = best
        ? `<span class="badge ${scoreBadgeClass(best.score)}">${best.score}</span>`
        : '<span class="muted">\u2014</span>';

    // Status badge
    const statusClass = best ? 'badge-matched' : 'badge-unmatched';
    const statusLabel = best ? 'matched' : 'unmatched';

    tr.innerHTML = `
        <td class="col-num">${idx + 1}</td>
        <td class="col-filename" title="${escapeHtml(photo.path)}">${escapeHtml(photo.filename)}</td>
        <td class="col-date">${dateStr}</td>
        <td class="col-camera">${escapeHtml(photo.cameraModel || '\u2014')}</td>
        <td class="col-score">${scoreBadge}</td>
        <td class="col-status"><span class="badge ${statusClass}">${statusLabel}</span></td>`;

    tr.addEventListener('click', () => selectPhoto(photo, tr));
    return tr;
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

    // Notify other modules that a photo was selected
    document.dispatchEvent(new CustomEvent('photo-selected', { detail: { photo } }));
}

