// preview.js — Photo preview card for Zone C (Phase 5).
// Renders a thumbnail + metadata card at the top of the match detail panel.
// Thumbnail loading is async — shows "Loading…" then swaps when the image arrives.

import { getThumbnail } from './api.js';
import { formatDate } from './helpers.js';

// renderPreview builds a .preview-card and prepends it into containerEl.
// Shows a thumbnail (or HEIC placeholder), filename, date, and camera model.
export function renderPreview(photo, containerEl) {
    if (!containerEl) return;

    // Placeholder shown while the async thumbnail request is in flight
    const placeholder = document.createElement('div');
    placeholder.className = 'preview-placeholder';
    placeholder.textContent = 'Loading\u2026';

    // The <img> element that replaces the placeholder on success
    const img = document.createElement('img');
    img.className = 'preview-img';
    img.alt = photo.filename || '';

    const card = document.createElement('div');
    card.className = 'preview-card';
    card.appendChild(placeholder);

    // Kick off async load; swap placeholder → img when done
    renderThumbnail(photo.path, img).then(() => {
        if (img.src) {
            placeholder.replaceWith(img);
        } else {
            // Empty result means HEIC or unsupported format
            placeholder.textContent = '(HEIC \u2014 no preview)';
        }
    });

    // Metadata line: filename · date · camera model (if present)
    const meta = document.createElement('div');
    meta.className = 'preview-meta muted';
    const parts = [photo.filename, formatDate(photo.dateTimeOriginal)];
    if (photo.cameraModel) parts.push(photo.cameraModel);
    meta.textContent = parts.join(' \u00b7 ');
    card.appendChild(meta);

    // Prepend so the thumbnail appears above the match-detail sections
    containerEl.prepend(card);
}

// renderThumbnail calls getThumbnail(path) from api.js.
// On success sets imgEl.src to the base64 data URI.
// On empty result (HEIC) or error leaves imgEl.src unset.
export async function renderThumbnail(path, imgEl) {
    try {
        const b64 = await getThumbnail(path);
        if (b64 && b64.length > 0) {
            imgEl.src = 'data:image/jpeg;base64,' + b64;
        }
    } catch (err) {
        console.warn('Thumbnail load failed for', path, err);
    }
}
