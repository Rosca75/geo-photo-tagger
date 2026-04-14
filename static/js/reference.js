// reference.js — Reference folder import UI (Phase 4)
// Handles [+ Reference] button: opens folder picker, calls Go AddReferenceFolder,
// renders the imported folder list with per-folder remove buttons.

import { state } from './state.js';
import { openFolderDialog, addReferenceFolder, removeReferenceFolder } from './api.js';

// initReference wires up the [+ Reference] button.
// Call once from app.js after DOMContentLoaded.
export function initReference() {
    const btn = document.getElementById('btn-add-reference');
    if (btn) {
        btn.addEventListener('click', handleAddReferenceClick);
    }
    renderReferenceList();
}

// handleAddReferenceClick opens the native folder picker and imports the
// chosen folder as a GPS reference source.
async function handleAddReferenceClick() {
    const path = await openFolderDialog();
    if (!path) return; // user cancelled

    try {
        // Returns { path, photoCount } from Go
        const info = await addReferenceFolder(path);
        if (!info || !info.path) return;

        // Replace existing entry (if re-added) or push new one
        const idx = state.referenceFolders.findIndex(f => f.path === info.path);
        if (idx >= 0) {
            state.referenceFolders[idx] = info;
        } else {
            state.referenceFolders.push(info);
        }
        renderReferenceList();
        updateStatusBar();
    } catch (err) {
        console.error('Add reference folder failed:', err);
    }
}

// renderReferenceList rebuilds the #reference-list element from state.referenceFolders.
// Also toggles the visibility of the whole chip row based on whether any
// reference folders are present.
export function renderReferenceList() {
    const container = document.getElementById('reference-list');
    if (!container) return;

    container.innerHTML = '';
    state.referenceFolders.forEach(info => {
        container.appendChild(buildReferenceChip(info));
    });

    // Hide the row entirely when empty so the topbar stays compact.
    const row = document.getElementById('reference-list-row');
    if (row) row.style.display = state.referenceFolders.length > 0 ? '' : 'none';
}

// buildReferenceChip creates one chip element for an imported reference folder.
function buildReferenceChip(info) {
    const div = document.createElement('div');
    div.className = 'folder-chip';

    // Show just the last path segment as the folder name
    const name = info.path.replace(/\\/g, '/').split('/').filter(Boolean).pop() || info.path;
    div.innerHTML = `
        <span class="chip-format">REF</span>
        <span class="chip-name" title="${escapeHtml(info.path)}">${escapeHtml(name)}</span>
        <span class="chip-count">${info.photoCount.toLocaleString()} photos</span>
        <button class="chip-remove" title="Remove this folder" aria-label="Remove">&#x2715;</button>`;

    div.querySelector('.chip-remove').addEventListener('click', () => handleRemoveReference(info.path));
    return div;
}

// handleRemoveReference calls Go to remove the folder's photos, then updates state and UI.
async function handleRemoveReference(path) {
    try {
        await removeReferenceFolder(path);
        state.referenceFolders = state.referenceFolders.filter(f => f.path !== path);
        renderReferenceList();
        updateStatusBar();
    } catch (err) {
        console.error('Remove reference folder failed:', err);
    }
}

// updateStatusBar appends the reference photo count to the footer status line.
function updateStatusBar() {
    const el = document.getElementById('status-bar-text');
    if (!el) return;

    const total = state.referenceFolders.reduce((sum, f) => sum + f.photoCount, 0);
    if (total === 0) return;

    const current = el.textContent || '';
    const suffix = ` \u2502 ${total.toLocaleString()} reference photo${total !== 1 ? 's' : ''}`;
    if (!current.includes('reference photo')) {
        el.textContent = current + suffix;
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
