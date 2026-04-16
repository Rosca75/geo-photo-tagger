// table.js — Target photos table rendering for Zone B
// Builds and updates the data table showing scanned target photos.
// Column headers are clickable and toggle sort direction.
// Per-row rendering lives in table_row.js (150-line split).

import { buildRow } from './table_row.js';

// Module-level sort state for column header toggling.
// Read by filters.js via getSortState() so that applyFilters() can sort
// in a single place. Changed by handleHeaderSort().
let sortColumn = 'filename';
let sortDirection = 'asc'; // 'asc' or 'desc'

// getSortState returns the current sort column and direction for external use.
export function getSortState() {
    return { column: sortColumn, direction: sortDirection };
}

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

// buildHeader creates the <thead> with clickable sortable column headers.
// The first column is a select-all checkbox (phase 6); remaining sortable
// headers show an up/down arrow when active.
function buildHeader() {
    const thead = document.createElement('thead');
    const tr = document.createElement('tr');
    const cols = [
        ['select',   '',            false, 'col-select'],
        ['num',      '#',           false, 'col-num'],
        ['filename', 'Filename',    true,  'col-filename'],
        ['date',     'Date / Time', true,  'col-date'],
        ['camera',   'Camera',      true,  'col-camera'],
        ['score',    'Score',       true,  'col-score'],
        ['status',   'Status',      true,  'col-status'],
    ];
    cols.forEach(([key, label, sortable, cls]) => {
        const th = document.createElement('th');
        th.className = cls;
        if (key === 'select') {
            th.innerHTML = '<input type="checkbox" id="cb-select-all" title="Select/deselect all matched photos">';
            tr.appendChild(th);
            // Wire the handler on next tick so the element is in the DOM.
            setTimeout(() => {
                const selAll = document.getElementById('cb-select-all');
                if (selAll) {
                    selAll.addEventListener('change', () => toggleAllSelection(selAll.checked));
                }
            }, 0);
            return;
        }
        if (!sortable) { th.textContent = label; tr.appendChild(th); return; }
        th.dataset.sort = key;
        th.title = `Sort by ${label}`;
        th.classList.add('sortable');
        const arrow = key === sortColumn ? (sortDirection === 'asc' ? ' \u25B2' : ' \u25BC') : '';
        th.innerHTML = `${label}<span class="sort-arrow">${arrow}</span>`;
        if (key === sortColumn) th.classList.add(sortDirection === 'asc' ? 'sort-asc' : 'sort-desc');
        th.addEventListener('click', () => handleHeaderSort(key));
        tr.appendChild(th);
    });
    thead.appendChild(tr);
    return thead;
}

// handleHeaderSort toggles direction on same-column click, or resets to
// ascending on a new column. Dispatches 'sort-changed' for filters.js.
function handleHeaderSort(column) {
    if (sortColumn === column) sortDirection = sortDirection === 'asc' ? 'desc' : 'asc';
    else { sortColumn = column; sortDirection = 'asc'; }
    document.dispatchEvent(new CustomEvent('sort-changed', {
        detail: { column: sortColumn, direction: sortDirection }
    }));
}

// toggleAllSelection flips every visible row checkbox to `checked` and
// fires the change handler on each so state.acceptedMatches updates via
// the per-row toggleRowSelection handler wired in table_row.js.
function toggleAllSelection(checked) {
    document.querySelectorAll('.row-select-cb').forEach(cb => {
        if (cb.checked !== checked) {
            cb.checked = checked;
            cb.dispatchEvent(new Event('change'));
        }
    });
}
