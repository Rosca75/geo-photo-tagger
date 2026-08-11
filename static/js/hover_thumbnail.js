// hover_thumbnail.js — Floating 32x32 thumbnail preview next to the cursor
// when the user hovers the filename / source thumbnail inside a candidate
// card in Zone C.
//
// Supported: JPEG, PNG, DNG, ARW, HEIC/HEIF (all decoded by the Go thumbnail
// pipeline). GPS track points (empty path) are handled by never attaching
// hover handlers — no error state, no placeholder.
//
// Thumbnails are cached in-memory by source path. One shared floating <div>
// is appended to <body> on first use; per-card DOM allocation is avoided.

import { getCandidateThumbnail } from './api.js';

const thumbnailCache = new Map();  // path -> dataURL or '' (negative cache)
let hoverEl = null;
let currentPath = null;

// Extensions the existing Go thumbnail pipeline can decode.
const SUPPORTED_EXTS = ['.jpg', '.jpeg', '.png', '.dng', '.arw', '.tif', '.tiff', '.heic', '.heif'];

// isSupported returns true if path has a format the hover preview can render.
// Empty path (GPS track point candidates) returns false so attachHover no-ops.
export function isSupported(path) {
    if (!path) return false;
    const lower = path.toLowerCase();
    return SUPPORTED_EXTS.some(ext => lower.endsWith(ext));
}

// ensureHoverEl lazily creates the shared floating <div> on first use.
function ensureHoverEl() {
    if (hoverEl) return hoverEl;
    hoverEl = document.createElement('div');
    hoverEl.id = 'hover-thumbnail';
    hoverEl.style.display = 'none';
    document.body.appendChild(hoverEl);
    return hoverEl;
}

// showAt positions the hover element at (clientX+16, clientY+16), clamped
// to the viewport so it never clips off the right/bottom edges.
function showAt(clientX, clientY) {
    const el = ensureHoverEl();
    const OFFSET = 16;
    const SIZE = 48;  // approx bounding box incl. padding/border
    const x = Math.min(clientX + OFFSET, window.innerWidth - SIZE);
    const y = Math.min(clientY + OFFSET, window.innerHeight - SIZE);
    el.style.left = `${x}px`;
    el.style.top = `${y}px`;
    el.style.display = 'block';
}

function hide() {
    if (hoverEl) hoverEl.style.display = 'none';
    currentPath = null;
}

// loadThumbnail fetches the thumbnail for `path` via the bound Go method,
// caches the result (including empty-string misses so we don't retry), and
// returns it. Empty string means "no preview available".
async function loadThumbnail(path) {
    if (thumbnailCache.has(path)) return thumbnailCache.get(path);
    try {
        const dataURL = await getCandidateThumbnail(path);
        thumbnailCache.set(path, dataURL || '');
        return dataURL || '';
    } catch {
        thumbnailCache.set(path, '');
        return '';
    }
}

// attachHover wires mouseenter/mousemove/mouseleave on `target` to show a
// floating thumbnail for `path`. No-op when the path is unsupported or empty
// (GPS track point candidates — the feature simply isn't there).
export function attachHover(target, path) {
    if (!target || !isSupported(path)) return;

    target.addEventListener('mouseenter', async (e) => {
        currentPath = path;
        const el = ensureHoverEl();
        const dataURL = await loadThumbnail(path);
        // Stale-result guard: if the user moved to a different card while we
        // were fetching, drop this result to avoid flashing the wrong preview.
        if (currentPath !== path) return;
        if (!dataURL) return;
        el.innerHTML = `<img src="${dataURL}" alt="">`;
        showAt(e.clientX, e.clientY);
    });

    target.addEventListener('mousemove', (e) => {
        if (hoverEl && hoverEl.style.display === 'block') {
            showAt(e.clientX, e.clientY);
        }
    });

    target.addEventListener('mouseleave', hide);
}

// wireCandidateHovers attaches hover handlers to every candidate card in the
// given panel. Idempotent per render: showPhotoDetail fully replaces
// panel.innerHTML each call, so listeners on stale elements are discarded
// with the elements themselves — no leak from re-attaching on re-render.
export function wireCandidateHovers(panel) {
    panel.querySelectorAll('.candidate-hover-target').forEach(el =>
        attachHover(el, el.dataset.candidateSourcePath));
}
