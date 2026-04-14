// filters.js — Photo filter controls (Phase 5).
// initFilters() wires the filter buttons in Zone A.
// applyFilters() rebuilds the table from state using the current filter
// and the sort state exported by table.js.
// Listens for 'match-complete' (from matcher_ui.js) and 'sort-changed'
// (from table.js) so sorting/filter updates automatically.

import { state } from './state.js';
import { renderTable, getSortState } from './table.js';

// Module-local filter state (not shared cross-module → not in state.js).
let currentFilter = 'all';

// initFilters wires filter buttons and the match-complete / sort-changed listeners.
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

    // Re-apply filters automatically after matching finishes
    document.addEventListener('match-complete', () => applyFilters());

    // Re-apply filters when the user clicks a sortable column header in table.js
    document.addEventListener('sort-changed', () => applyFilters());
}

// applyFilters filters state.targetPhotos by match status, sorts the result
// using table.js sort state, and calls renderTable() with the result.
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

    // ── Sort step — comparators drive ascending order; dir inverts ────
    const { column, direction } = getSortState();
    const dir = direction === 'desc' ? -1 : 1;

    filtered = [...filtered].sort((a, b) => {
        let cmp = 0;
        if (column === 'date') {
            const ta = a.dateTimeOriginal || '';
            const tb = b.dateTimeOriginal || '';
            cmp = ta.localeCompare(tb);
        } else if (column === 'score') {
            const sa = bestMap.has(a.path) ? bestMap.get(a.path).score : -1;
            const sb = bestMap.has(b.path) ? bestMap.get(b.path).score : -1;
            cmp = sa - sb;
        } else if (column === 'status') {
            // Matched first (0), unmatched second (1); tiebreak by filename
            const ha = bestMap.has(a.path) ? 0 : 1;
            const hb = bestMap.has(b.path) ? 0 : 1;
            cmp = ha !== hb ? ha - hb : (a.filename || '').localeCompare(b.filename || '');
        } else if (column === 'camera') {
            cmp = (a.cameraModel || '').localeCompare(b.cameraModel || '');
        } else {
            // Default: filename
            cmp = (a.filename || '').localeCompare(b.filename || '');
        }
        return cmp * dir;
    });

    renderTable(filtered);
}
