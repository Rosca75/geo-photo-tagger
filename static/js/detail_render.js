// detail_render.js — HTML builders for the Zone C match detail panel.
// Kept separate from matcher_ui.js to keep both files under the 150-line limit.
// These functions only produce HTML strings; click handling is wired in
// matcher_ui.js via event delegation on the .match-panel element.

import { state } from './state.js';
import { escapeHtml, formatDate } from './helpers.js';

// scoreBadgeClass maps a numeric score to the badge CSS class used in Zone B and Zone C.
export function scoreBadgeClass(score) {
    if (score >= 90) return 'badge-excellent';
    if (score >= 50) return 'badge-matched';
    return 'badge-poor';
}

// buildDetailHTML returns the inner HTML for the Zone C match detail panel.
// `photo` is the selected TargetPhoto; `result` is its MatchResult (or null).
// Candidate rows are selected radio-style — clicking one picks it as the
// accepted GPS source, clicking it again toggles it off.
export function buildDetailHTML(photo, result) {
    const acc = state.acceptedMatches.get(photo.path);

    let html = `
        <div class="detail-header">
            <div class="detail-filename">${escapeHtml(photo.filename)}</div>
            <div class="detail-meta muted">${formatDate(photo.dateTimeOriginal)}${photo.cameraModel ? ' &middot; ' + escapeHtml(photo.cameraModel) : ''}</div>
        </div>`;

    // No match yet → offer a per-photo "Search for GPS match" button.
    if (!result || !result.bestCandidate) {
        return html + buildNoMatchSection(photo);
    }

    html += buildBestMatchSection(result.bestCandidate);

    if (result.candidates && result.candidates.length > 0) {
        html += buildCandidatesSection(photo, result.candidates, acc);
        html += buildGPSPreviewWrapper(acc);
    }

    html += buildApplySection(photo, acc);
    return html;
}

// buildNoMatchSection renders the per-photo "Search for GPS match" button
// shown when no match exists for the selected photo.
function buildNoMatchSection(photo) {
    return `
        <div class="detail-section" style="margin-top:1rem;">
            <p class="muted">No GPS match found within ${state.matchThreshold} minutes.</p>
            <button class="btn btn-sm btn-accent btn-match-single"
                data-path="${escapeHtml(photo.path)}"
                title="Search for GPS matches for this photo only"
                style="margin-top:var(--space-sm)">Search for GPS match</button>
        </div>`;
}

// buildBestMatchSection renders the "Best Match" summary block at the top.
function buildBestMatchSection(best) {
    return `
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
}

// buildCandidatesSection renders the list of candidate rows with radio-style
// selection markers. Clicking a row is handled by matcher_ui.handleCandidateSelect.
function buildCandidatesSection(photo, candidates, acc) {
    let html = `<div class="detail-section">
        <div class="detail-section-title">Candidates (${candidates.length}) &mdash; click to select</div>`;

    candidates.forEach((c) => {
        const isSelected = acc
            && acc.sourcePath === c.sourcePath
            && Math.abs(acc.lat - c.gps.latitude) < 1e-6;

        html += `
            <div class="detail-candidate candidate-hover-target${isSelected ? ' selected' : ''}"
                 data-path="${escapeHtml(photo.path)}"
                 data-lat="${c.gps.latitude}"
                 data-lon="${c.gps.longitude}"
                 data-score="${c.score}"
                 data-source="${escapeHtml(c.source || '')}"
                 data-source-path="${escapeHtml(c.sourcePath || '')}"
                 data-candidate-source-path="${escapeHtml(c.sourcePath || '')}"
                 title="Click to select this candidate as the GPS source">
                <span class="candidate-radio">${isSelected ? '&#x25C9;' : '&#x25CB;'}</span>
                <span class="badge ${scoreBadgeClass(c.score)}">${c.score}</span>
                <span class="detail-source">${escapeHtml(c.sourceFilename)}</span>
                <span class="muted">${c.timeDeltaFormatted}</span>
                <span class="muted" style="font-size:0.75rem">${c.source === 'track' ? 'track' : 'photo'}</span>
                ${c.isInterpolated ? '<span class="chip-format" style="margin-left:4px">interpolated</span>' : ''}
            </div>`;
    });
    return html + '</div>';
}

// buildGPSPreviewWrapper wraps the GPS preview card in its container div so
// the reverse-geocoding handler can target #candidate-gps-preview.
function buildGPSPreviewWrapper(acc) {
    const inner = (acc && acc.lat !== undefined)
        ? buildGPSPreview(acc)
        : '<p class="muted" style="font-size:0.85rem;">Click a candidate above to preview GPS data.</p>';
    return `<div id="candidate-gps-preview" class="detail-section candidate-preview">${inner}</div>`;
}

// buildGPSPreview renders coordinates and location info for the selected candidate.
// Exported so matcher_ui.js can inject the card after a candidate is picked.
// When state.mapEnabled is true, also includes a mini-map container that
// map.js populates after Leaflet lazy-loads.
export function buildGPSPreview(acc) {
    const mapToggleLabel = state.mapEnabled ? 'Hide map' : 'Show map';
    const mapSlot = state.mapEnabled
        ? '<div id="gps-mini-map" class="gps-mini-map"></div>'
        : '';
    return `
        <div class="gps-preview-card">
            <div class="gps-coords">
                <span class="label-small">Coordinates:</span>
                <span class="gps-value">${acc.lat.toFixed(6)}, ${acc.lon.toFixed(6)}</span>
                <button class="btn btn-sm btn-secondary btn-toggle-map"
                        title="Show or hide the mini map preview">${mapToggleLabel}</button>
            </div>
            <div id="gps-location-info" class="gps-location muted">Loading location\u2026</div>
            ${mapSlot}
        </div>`;
}

// buildApplySection renders the Apply GPS / Undo button area below the candidates.
function buildApplySection(photo, acc) {
    if (photo.status === 'applied') {
        return `
        <div class="detail-section" style="margin-top:var(--space-sm)">
            <button class="btn btn-sm btn-secondary btn-undo"
                data-path="${escapeHtml(photo.path)}"
                title="Restore the photo from its backup file">Undo</button>
            <span class="badge badge-applied" style="margin-left:var(--space-xs)">Applied \u2713</span>
        </div>`;
    }
    if (acc) {
        return `
        <div class="detail-section" style="margin-top:var(--space-sm)">
            <button class="btn btn-sm btn-primary btn-apply-single"
                data-path="${escapeHtml(photo.path)}"
                data-lat="${acc.lat}" data-lon="${acc.lon}"
                title="Write these GPS coordinates to the photo file">Apply GPS</button>
        </div>`;
    }
    return '';
}
