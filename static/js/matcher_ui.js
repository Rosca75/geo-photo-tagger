// matcher_ui.js — GPS matching UI (Phase 4/5)
// Handles the Match All button, threshold selector, and Zone C detail panel.
// After RunMatching completes, re-renders the table with score badges and
// dispatches 'match-complete' so filters.js can auto-apply.
// Listens for the 'photo-selected' DOM event fired by table.js.

import { state } from './state.js';
import { runMatching } from './api.js';
import { renderTable } from './table.js';
import { escapeHtml, formatDate } from './helpers.js';
import { renderPreview } from './preview.js';

// initMatcher wires up the Match All button, threshold buttons, Zone C listener,
// and accept/reject event delegation on the match panel.
export function initMatcher() {
    const matchBtn = document.getElementById('btn-match-all');
    if (matchBtn) matchBtn.addEventListener('click', handleMatchAllClick);

    // Threshold preset buttons (10 / 30 / 60 min)
    document.querySelectorAll('.threshold-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            const val = parseInt(btn.dataset.minutes, 10);
            if (!isNaN(val)) setThreshold(val, btn);
        });
    });

    // Listen for row-selection events fired by table.js
    document.addEventListener('photo-selected', e => showPhotoDetail(e.detail.photo));

    // Delegate accept/reject button clicks within Zone C
    const panel = document.querySelector('.match-panel');
    if (panel) panel.addEventListener('click', handlePanelClick);
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
        showZoneMessage('Scan a target folder first.'); return;
    }
    if (state.referenceFolders.length === 0 && state.gpsTrackFiles.length === 0) {
        showZoneMessage('Add a reference folder or import a GPS track first.'); return;
    }
    setMatchingIndicator(true);
    try {
        const results = await runMatching({ maxTimeDeltaMinutes: state.matchThreshold });
        state.matchResults = Array.isArray(results) ? results : [];
        renderTable(state.targetPhotos);
        updateStatusBarCounts();
        // Notify filters.js to auto-apply after matching
        document.dispatchEvent(new CustomEvent('match-complete'));
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
    const total   = state.targetPhotos.length;
    const matched = state.matchResults.filter(r => r.bestCandidate).length;
    el.textContent = `${total} target photo${total !== 1 ? 's' : ''} \u2502 ${matched} matched \u2502 ${total - matched} unmatched`;
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
    // Render the text detail first, then prepend the thumbnail card
    panel.innerHTML = buildDetailHTML(photo, result);
    renderPreview(photo, panel);
}

// buildDetailHTML returns the inner HTML for the Zone C match detail panel.
// Accept/reject buttons are rendered per candidate so the user can choose
// which GPS source to apply to this photo.
function buildDetailHTML(photo, result) {
    const acc = state.acceptedMatches.get(photo.path);
    let html = `
        <div class="detail-header">
            <div class="detail-filename">${escapeHtml(photo.filename)}</div>
            <div class="detail-meta muted">${formatDate(photo.dateTimeOriginal)}${photo.cameraModel ? ' &middot; ' + escapeHtml(photo.cameraModel) : ''}</div>
        </div>`;

    if (!result || !result.bestCandidate) {
        return html + `<p class="muted panel-placeholder" style="margin-top:1rem;">No GPS match found within ${state.matchThreshold} minutes.</p>`;
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
            <div class="detail-gps"><span class="muted">GPS:</span> ${best.gps.latitude.toFixed(6)}, ${best.gps.longitude.toFixed(6)}</div>
        </div>`;

    if (result.candidates && result.candidates.length > 0) {
        html += `<div class="detail-section"><div class="detail-section-title">Candidates (${result.candidates.length})</div>`;
        result.candidates.forEach(c => {
            // Mark this row as accepted if its coords match the stored accepted entry
            const isAcc = acc
                && acc.sourcePath === c.sourcePath
                && Math.abs(acc.lat - c.gps.latitude) < 1e-6;
            html += `
                <div class="detail-candidate${isAcc ? ' accepted' : ''}">
                    <span class="badge ${scoreBadgeClass(c.score)}">${c.score}</span>
                    <span class="detail-source">${escapeHtml(c.sourceFilename)}</span>
                    <span class="muted">${c.timeDeltaFormatted}</span>
                    <span class="muted" style="font-size:0.75rem">${c.source === 'track' ? 'track' : 'photo'}</span>
                    <button class="btn btn-sm btn-primary btn-accept"
                        data-path="${escapeHtml(photo.path)}"
                        data-lat="${c.gps.latitude}" data-lon="${c.gps.longitude}"
                        data-score="${c.score}" data-source="${escapeHtml(c.source || '')}"
                        data-source-path="${escapeHtml(c.sourcePath || '')}">Accept</button>
                    <button class="btn btn-sm btn-secondary btn-reject"
                        data-path="${escapeHtml(photo.path)}">Reject</button>
                </div>`;
        });
        html += '</div>';
    }

    // Apply GPS / Undo section — shown below the candidates list.
    // "applied" status: show Undo button + Applied badge.
    // Accepted (but not yet applied): show Apply GPS button.
    if (photo.status === 'applied') {
        html += `
        <div class="detail-section" style="margin-top:var(--space-sm)">
            <button class="btn btn-sm btn-secondary btn-undo"
                data-path="${escapeHtml(photo.path)}">Undo</button>
            <span class="badge badge-applied" style="margin-left:var(--space-xs)">Applied \u2713</span>
        </div>`;
    } else if (acc) {
        html += `
        <div class="detail-section" style="margin-top:var(--space-sm)">
            <button class="btn btn-sm btn-primary btn-apply-single"
                data-path="${escapeHtml(photo.path)}"
                data-lat="${acc.lat}" data-lon="${acc.lon}">Apply GPS</button>
        </div>`;
    }

    return html;
}

// scoreBadgeClass maps a numeric score to a CSS badge class name.
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

// handlePanelClick delegates click events within Zone C to the correct handler.
function handlePanelClick(e) {
    const acceptBtn = e.target.closest('.btn-accept');
    const rejectBtn = e.target.closest('.btn-reject');
    if (acceptBtn)      handleAccept(acceptBtn);
    else if (rejectBtn) handleReject(rejectBtn);
}

// handleAccept stores the chosen candidate in state.acceptedMatches and refreshes.
function handleAccept(btn) {
    const d = btn.dataset;
    state.acceptedMatches.set(d.path, {
        lat: parseFloat(d.lat), lon: parseFloat(d.lon),
        score: parseInt(d.score, 10), source: d.source, sourcePath: d.sourcePath
    });
    refreshAfterDecision(d.path, true);
}

// handleReject removes the accepted entry from state and refreshes.
function handleReject(btn) {
    state.acceptedMatches.delete(btn.dataset.path);
    refreshAfterDecision(btn.dataset.path, false);
}

// refreshAfterDecision updates the table-row status badge and re-renders Zone C.
function refreshAfterDecision(targetPath, accepted) {
    // Update the status badge in the table without a full re-render
    const row = document.querySelector(`.photo-row[data-path="${CSS.escape(targetPath)}"]`);
    if (row) {
        const badge = row.querySelector('.col-status .badge');
        if (badge) {
            badge.className = `badge ${accepted ? 'badge-applied' : 'badge-matched'}`;
            badge.textContent = accepted ? 'accepted' : 'matched';
        }
    }
    // Re-render the detail panel so the accepted row gets the highlight class
    if (state.selectedPhoto) {
        const photo = state.targetPhotos.find(p => p.path === state.selectedPhoto);
        if (photo) showPhotoDetail(photo);
    }
}
