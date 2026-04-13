// filters.js — Photo filter and sort controls (Phase 5).
// initFilters() wires the filter buttons and sort dropdown in Zone A.
// applyFilters() rebuilds the table from state using current filter/sort settings.
// Listens for the 'match-complete' event dispatched by matcher_ui.js.

import { state } from './state.js';
import { renderTable } from './table.js';

// Module-level UI state — not shared with other modules, so stored locally
// rather than in state.js (which holds cross-module state only).
let currentFilter = 'all';
let currentSort   = 'filename';

// initFilters wires filter buttons, the sort dropdown, and the match-complete listener.
// Call once from app.js after DOMContentLoaded.
export function initFilters() {
    // Filter toggle buttons: All / Matched / Unmatched
    document.querySelectorAll('.filter-btn').forEach(btn => {
        btn.addEventListener('click', () => {
            document.querySelectorAll('.filter-btn').forEach(b => b.classList.remove('active'));
            btn.classList.add('active');
            currentFilter = btn.dataset.filter;
            applyFilters();
        });
    });

    // Sort dropdown
    const sortSelect = document.getElementById('sort-select');
    if (sortSelect) {
        sortSelect.addEventListener('change', () => {
            currentSort = sortSelect.value;
            applyFilters();
        });
    }

    // Re-apply filters automatically after matching finishes
    document.addEventListener('match-complete', () => applyFilters());
}

// applyFilters filters state.targetPhotos by match status, sorts the result,
// and calls renderTable() with the filtered+sorted array.
export function applyFilters() {
    if (!state.targetPhotos.length) return;

    // Build a quick lookup: targetPath → bestCandidate
    const bestMap = new Map();
    if (state.matchResults) {
        state.matchResults.forEach(r => {
            if (r.bestCandidate) bestMap.set(r.targetPath, r.bestCandidate);
        });
    }

    // ── Filter step ────────────────────────────────────────────────────
    let filtered = state.targetPhotos.filter(photo => {
        const hasMatch = bestMap.has(photo.path);
        if (currentFilter === 'matched')   return hasMatch;
        if (currentFilter === 'unmatched') return !hasMatch;
        return true; // 'all'
    });

    // ── Sort step ──────────────────────────────────────────────────────
    filtered = [...filtered].sort((a, b) => {
        if (currentSort === 'date') {
            // Chronological by EXIF datetime; missing dates sort to the end
            const ta = a.dateTimeOriginal || '';
            const tb = b.dateTimeOriginal || '';
            return ta.localeCompare(tb);
        }
        if (currentSort === 'score') {
            // Descending score; unmatched photos (-1) sort to the bottom
            const sa = bestMap.has(a.path) ? bestMap.get(a.path).score : -1;
            const sb = bestMap.has(b.path) ? bestMap.get(b.path).score : -1;
            return sb - sa;
        }
        if (currentSort === 'status') {
            // Matched first, then unmatched; within each group, alphabetical
            const ha = bestMap.has(a.path) ? 0 : 1;
            const hb = bestMap.has(b.path) ? 0 : 1;
            if (ha !== hb) return ha - hb;
            return (a.filename || '').localeCompare(b.filename || '');
        }
        // Default: 'filename' — simple alphabetical
        return (a.filename || '').localeCompare(b.filename || '');
    });

    renderTable(filtered);
}
