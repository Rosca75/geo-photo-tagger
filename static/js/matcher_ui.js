// matcher_ui.js — GPS matching UI (Phase 4)
// Handles the Match All button, threshold selector, and Zone C detail panel.
// After RunMatching completes, re-renders the table with score badges.
// Listens for the 'photo-selected' DOM event fired by table.js.

import { state } from './state.js';
import { runMatching } from './api.js';
import { renderTable } from './table.js';

// initMatcher wires up the Match All button, threshold buttons, and Zone C listener.
// Call once from app.js after DOMContentLoaded.
export function initMatcher() {
    const matchBtn = document.getElementById('btn-match-all');
    if (matchBtn) {
        matchBtn.addEventListener('click', handleMatchAllClick);
    }

    // Wire threshold preset buttons
    document.querySelectorAll('.threshold-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const val = parseInt(btn.dataset.minutes, 10);
            if (!isNaN(val)) setThreshold(val, btn);
        });
    });

    // Listen for row-selection events fired by table.js
    document.addEventListener('photo-selected', e => showPhotoDetail(e.detail.photo));
}

// setThreshold updates state.matchThreshold and marks the active button.
function setThreshold(minutes, activeBtn) {
    state.matchThreshold = minutes;
    document.querySelectorAll('.threshold-btn').forEach(b => b.classList.remove('active'));
    if (activeBtn) activeBtn.classList.add('active');
}

// handleMatchAllClick runs the GPS matching engine and refreshes the UI.
async function handleMatchAllClick() {
    if (state.targetPhotos.length === 0) {
        showZoneMessage('Scan a target folder first.');
        return;
    }
    if (state.referenceFolders.length === 0 && state.gpsTrackFiles.length === 0) {
        showZoneMessage('Add a reference folder or import a GPS track first.');
        return;
    }

    setMatchingIndicator(true);

    try {
        const results = await runMatching({ maxTimeDeltaMinutes: state.matchThreshold });
        state.matchResults = Array.isArray(results) ? results : [];

        // Re-render table so score badges appear
        renderTable(state.targetPhotos);
        updateStatusBarCounts();
    } catch (err) {
        console.error('Matching failed:', err);
        showZoneMessage('Matching failed: ' + String(err));
    } finally {
        setMatchingIndicator(false);
    }
}

// updateStatusBarCounts updates the footer with matched / unmatched counts.
function updateStatusBarCounts() {
    const el = document.getElementById('status-bar-text');
    if (!el || !state.matchResults) return;

    const total = state.targetPhotos.length;
    const matched = state.matchResults.filter(r => r.bestCandidate).length;
    el.textContent = `${total} target photo${total !== 1 ? 's' : ''} \u2502 ${matched} matched \u2502 ${total - matched} unmatched`;
}

// setMatchingIndicator shows/hides a "Matching…" indicator in the scan indicator slot.
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

    // Find the MatchResult for this photo
    const result = state.matchResults
        ? state.matchResults.find(r => r.targetPath === photo.path)
        : null;

    panel.innerHTML = buildDetailHTML(photo, result);
}

// buildDetailHTML returns the inner HTML for the Zone C match detail panel.
function buildDetailHTML(photo, result) {
    const dateStr = formatDate(photo.dateTimeOriginal);
    let html = `
        <div class="detail-header">
            <div class="detail-filename">${escapeHtml(photo.filename)}</div>
            <div class="detail-meta muted">${dateStr}${photo.cameraModel ? ' &middot; ' + escapeHtml(photo.cameraModel) : ''}</div>
        </div>`;

    if (!result || !result.bestCandidate) {
        html += `<p class="muted panel-placeholder" style="margin-top:1rem;">No GPS match found within ${state.matchThreshold} minutes.</p>`;
        return html;
    }

    const best = result.bestCandidate;
    html += `
        <div class="detail-section">
            <div class="detail-section-title">Best Match</div>
            <div class="detail-match-row">
                <span class="badge ${scoreBadgeClass(best.score)}">${best.score}</span>
                <span class="detail-source">${escapeHtml(best.sourceFilename)}</span>
                <span class="muted">${best.timeDeltaFormatted}</span>
                ${best.isInterpolated ? '<span class="chip-format" style="margin-left:4px">interpolated</span>' : ''}
            </div>
            <div class="detail-gps">
                <span class="muted">GPS:</span>
                ${best.gps.latitude.toFixed(6)}, ${best.gps.longitude.toFixed(6)}
            </div>
        </div>`;

    if (result.candidates && result.candidates.length > 1) {
        html += `<div class="detail-section"><div class="detail-section-title">All Candidates (${result.candidates.length})</div>`;
        result.candidates.forEach(c => {
            html += `
                <div class="detail-candidate">
                    <span class="badge ${scoreBadgeClass(c.score)}">${c.score}</span>
                    <span class="detail-source">${escapeHtml(c.sourceFilename)}</span>
                    <span class="muted">${c.timeDeltaFormatted}</span>
                    <span class="muted" style="font-size:0.75rem">${c.source === 'track' ? 'track' : 'photo'}</span>
                </div>`;
        });
        html += '</div>';
    }

    return html;
}

// scoreBadgeClass maps a score to a CSS badge class.
function scoreBadgeClass(score) {
    if (score >= 90) return 'badge-excellent';
    if (score >= 50) return 'badge-matched';
    return 'badge-poor';
}

// showZoneMessage puts a plain message in Zone C.
function showZoneMessage(msg) {
    const panel = document.querySelector('.match-panel');
    if (panel) panel.innerHTML = `<p class="muted panel-placeholder" style="margin-top:2rem;">${escapeHtml(msg)}</p>`;
}

// formatDate converts an ISO 8601 string (from Go time.Time) to locale string.
function formatDate(raw) {
    if (!raw || raw === '0001-01-01T00:00:00Z') return '\u2014';
    return new Date(raw).toLocaleString();
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
