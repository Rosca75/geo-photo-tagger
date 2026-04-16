// actions.js — GPS write/undo actions wired to Zone A and Zone C buttons.
// Handles single Apply GPS, batch Apply All Accepted, and per-photo Undo.
// Uses event delegation on .match-panel so matcher_ui.js stays independent.

import { applyGPS, applyBatchGPS, undoGPS } from './api.js';
import { state } from './state.js';

// initActions wires up the Apply All Accepted button (Zone A) and delegates
// .btn-apply-single / .btn-undo clicks within Zone C.
export function initActions() {
    const applyAllBtn = document.getElementById('btn-apply-all');
    if (applyAllBtn) applyAllBtn.addEventListener('click', handleApplyAll);

    // Use event delegation on Zone C so dynamically-rendered buttons work.
    const panel = document.querySelector('.match-panel');
    if (panel) panel.addEventListener('click', handlePanelActionClick);
}

// handlePanelActionClick routes Zone C button clicks to the correct handler.
function handlePanelActionClick(e) {
    const applyBtn = e.target.closest('.btn-apply-single');
    const undoBtn  = e.target.closest('.btn-undo');
    if (applyBtn)     handleApplySingle(applyBtn);
    else if (undoBtn) handleUndo(undoBtn);
}

// buildApplyWarning returns the standard warning text shown in apply
// confirmation dialogs. Reminds the user to close tools like Lightroom or
// Bridge that may hold the DNG file open — on Windows an open file handle
// will cause the GPS write to fail mid-flight, and the lazy-backup undo
// depends on a clean pre-apply state.
function buildApplyWarning(photoCount) {
    const plural = photoCount !== 1 ? 's' : '';
    return `Apply GPS data to ${photoCount} photo${plural}?

Please close any app (Lightroom, Photoshop, Bridge, Explorer preview)
that may have these photos open — they must not be locked by another
process, or the write will fail.`;
}

// handleApplySingle writes GPS to one photo when its Apply GPS button is clicked.
async function handleApplySingle(btn) {
    const { path, lat, lon } = btn.dataset;
    const ok = await showConfirm(buildApplyWarning(1));
    if (!ok) return;
    btn.disabled = true;
    btn.textContent = 'Applying\u2026';
    try {
        await applyGPS(path, parseFloat(lat), parseFloat(lon));
        updatePhotoBadge(path, 'applied', 'applied');
        const p = state.targetPhotos.find(t => t.path === path);
        if (p) p.status = 'applied';
        showToast('GPS applied: ' + path.split(/[\\/]/).pop(), 'success');
        // Re-render Zone C so the Undo button appears in place of Apply GPS.
        reRenderDetail(path);
    } catch (err) {
        showToast('Apply failed: ' + String(err), 'error');
        btn.disabled = false;
        btn.textContent = 'Apply GPS';
    }
}

// handleUndo restores one photo from its .bak backup.
async function handleUndo(btn) {
    const { path } = btn.dataset;
    btn.disabled = true;
    try {
        await undoGPS(path);
        updatePhotoBadge(path, 'matched', 'matched');
        const p = state.targetPhotos.find(t => t.path === path);
        if (p) p.status = 'matched';
        showToast('Undo successful: ' + path.split(/[\\/]/).pop(), 'success');
        reRenderDetail(path);
    } catch (err) {
        showToast('Undo failed: ' + String(err), 'error');
        btn.disabled = false;
    }
}

// handleApplyAll batch-writes GPS to all accepted matches after a confirm dialog.
async function handleApplyAll() {
    const count = state.acceptedMatches.size;
    if (count === 0) { showToast('No accepted matches to apply.', 'error'); return; }

    const ok = await showConfirm(buildApplyWarning(count));
    if (!ok) return;

    const matches = [];
    state.acceptedMatches.forEach((v, path) => {
        matches.push({ targetPath: path, lat: v.lat, lon: v.lon });
    });

    try {
        const result = await applyBatchGPS(matches);
        matches.forEach(m => {
            updatePhotoBadge(m.targetPath, 'applied', 'applied');
            const p = state.targetPhotos.find(t => t.path === m.targetPath);
            if (p) p.status = 'applied';
        });
        const msg = `Applied ${result.applied} photo${result.applied !== 1 ? 's' : ''}` +
            (result.errors > 0 ? `, ${result.errors} failed` : '');
        showToast(msg, result.errors > 0 ? 'error' : 'success');
    } catch (err) {
        showToast('Batch apply failed: ' + String(err), 'error');
    }
}

// reRenderDetail re-fires photo-selected so matcher_ui.js rebuilds Zone C.
function reRenderDetail(path) {
    const photo = state.targetPhotos.find(p => p.path === path);
    if (photo) {
        document.dispatchEvent(new CustomEvent('photo-selected', { detail: { photo } }));
    }
}

// updatePhotoBadge sets the status badge text and class in Zone B for targetPath.
function updatePhotoBadge(targetPath, badgeClass, badgeText) {
    const row = document.querySelector(`.photo-row[data-path="${CSS.escape(targetPath)}"]`);
    if (!row) return;
    const badge = row.querySelector('.col-status .badge');
    if (badge) {
        badge.className = `badge badge-${badgeClass}`;
        badge.textContent = badgeText;
    }
}

// showToast displays a transient notification at the bottom-right of the screen.
function showToast(msg, type = 'success') {
    let container = document.getElementById('toast-container');
    if (!container) {
        container = document.createElement('div');
        container.id = 'toast-container';
        document.body.appendChild(container);
    }
    const toast = document.createElement('div');
    toast.className = `toast toast-${type}`;
    toast.textContent = msg;
    container.appendChild(toast);
    setTimeout(() => {
        toast.classList.add('removing');
        toast.addEventListener('animationend', () => toast.remove(), { once: true });
    }, 3000);
}

// showConfirm shows a modal confirmation dialog and resolves to true (OK) or false (Cancel).
// The message is split on blank lines and each block is set via textContent on a <p>
// element — safe for multi-line warnings (phase 3c) and XSS-safe by construction.
function showConfirm(message) {
    return new Promise(resolve => {
        const overlay = document.createElement('div');
        overlay.className = 'confirm-overlay';

        const box = document.createElement('div');
        box.className = 'confirm-box';

        // Split on one or more blank lines — each paragraph becomes its own <p>.
        const paragraphs = String(message).split(/\n\s*\n/);
        paragraphs.forEach(block => {
            const p = document.createElement('p');
            p.style.whiteSpace = 'pre-line';
            p.textContent = block.trim();
            box.appendChild(p);
        });

        const btnRow = document.createElement('div');
        btnRow.className = 'btn-row';
        const okBtn = document.createElement('button');
        okBtn.className = 'btn btn-primary';
        okBtn.id = 'confirm-ok';
        okBtn.textContent = 'Confirm';
        const cancelBtn = document.createElement('button');
        cancelBtn.className = 'btn btn-secondary';
        cancelBtn.id = 'confirm-cancel';
        cancelBtn.textContent = 'Cancel';
        btnRow.appendChild(okBtn);
        btnRow.appendChild(cancelBtn);
        box.appendChild(btnRow);
        overlay.appendChild(box);

        document.body.appendChild(overlay);
        okBtn.addEventListener('click', () => { overlay.remove(); resolve(true); });
        cancelBtn.addEventListener('click', () => { overlay.remove(); resolve(false); });
    });
}
