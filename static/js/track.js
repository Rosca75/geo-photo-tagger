// track.js — GPS track file import UI (Phase 3)
// Handles the [Import GPS Track] button: opens a native file picker, calls the
// Go parser, and renders the imported track list with per-track remove buttons.
//
// State management: imported track descriptors (GPSTrackFile objects) are stored
// in state.gpsTrackFiles. This module owns reads and writes to that array.

import { state } from './state.js';
import { openFileDialog, importGPSTrack, removeGPSTrack } from './api.js';

// initTrack wires up the Import GPS Track button.
// Call once from app.js after DOMContentLoaded.
export function initTrack() {
    const btn = document.getElementById('btn-import-track');
    if (btn) {
        btn.addEventListener('click', handleImportTrackClick);
    }
    // Render an empty list on startup (shows "No GPS tracks imported" placeholder)
    renderTrackList();
}

// handleImportTrackClick opens the file picker and imports the chosen track file.
async function handleImportTrackClick() {
    // openFileDialog opens the native OS file picker filtered to .gpx/.kml/.csv
    const path = await openFileDialog();
    if (!path) return; // user cancelled — do nothing

    try {
        // importGPSTrack calls Go's ImportGPSTrack(path) → returns GPSTrackFile
        const trackFile = await importGPSTrack(path);
        if (!trackFile || !trackFile.path) return;

        // Replace existing entry (if re-imported) or push new one
        const idx = state.gpsTrackFiles.findIndex(t => t.path === trackFile.path);
        if (idx >= 0) {
            state.gpsTrackFiles[idx] = trackFile;
        } else {
            state.gpsTrackFiles.push(trackFile);
        }
        renderTrackList();
        updateStatusBar();
    } catch (err) {
        console.error('Import GPS track failed:', err);
    }
}

// renderTrackList rebuilds the #track-list element from state.gpsTrackFiles.
// Hides the whole chip row when no tracks have been imported so the topbar
// stays compact until the user actually adds something.
export function renderTrackList() {
    const container = document.getElementById('track-list');
    if (!container) return;

    container.innerHTML = '';
    state.gpsTrackFiles.forEach(tf => {
        container.appendChild(buildTrackChip(tf));
    });

    const row = document.getElementById('track-list-row');
    if (row) row.style.display = state.gpsTrackFiles.length > 0 ? '' : 'none';
}

// buildTrackChip creates a single track chip element showing filename + point count.
function buildTrackChip(tf) {
    const div = document.createElement('div');
    div.className = 'folder-chip';

    // Format label includes the format badge (GPX / KML / CSV)
    const fmtLabel = tf.format ? tf.format.toUpperCase() : '';
    div.innerHTML = `
        <span class="chip-format">${escapeHtml(fmtLabel)}</span>
        <span class="chip-name" title="${escapeHtml(tf.path)}">${escapeHtml(tf.filename)}</span>
        <span class="chip-count">${tf.pointCount.toLocaleString()} pts</span>
        <button class="chip-remove" title="Remove this track" aria-label="Remove">&#x2715;</button>`;

    div.querySelector('.chip-remove').addEventListener('click', () => handleRemoveTrack(tf.path));
    return div;
}

// handleRemoveTrack calls Go to remove the track's points, then updates state and UI.
async function handleRemoveTrack(path) {
    try {
        await removeGPSTrack(path);
        state.gpsTrackFiles = state.gpsTrackFiles.filter(t => t.path !== path);
        renderTrackList();
        updateStatusBar();
    } catch (err) {
        console.error('Remove GPS track failed:', err);
    }
}

// updateStatusBar refreshes the footer status line to reflect track import counts.
function updateStatusBar() {
    const el = document.getElementById('status-bar-text');
    if (!el) return;

    const totalPts = state.gpsTrackFiles.reduce((sum, t) => sum + t.pointCount, 0);
    const fileCount = state.gpsTrackFiles.length;

    if (fileCount === 0) {
        // Fall back to the standard scan-based status if no tracks
        return;
    }
    const trackSuffix = ` \u2502 ${fileCount} track file${fileCount !== 1 ? 's' : ''} (${totalPts.toLocaleString()} pts)`;
    // Only append if there's existing text, otherwise set it
    const current = el.textContent;
    if (current && !current.includes('track file')) {
        el.textContent = current + trackSuffix;
    } else if (!current || current === 'Ready \u2014 select a target folder to begin') {
        el.textContent = `${fileCount} track file${fileCount !== 1 ? 's' : ''} \u2502 ${totalPts.toLocaleString()} pts`;
    }
}

// escapeHtml prevents XSS when inserting user-controlled strings into innerHTML.
function escapeHtml(str) {
    if (!str) return '';
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}
