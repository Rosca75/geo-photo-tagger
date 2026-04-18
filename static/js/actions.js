// actions.js — GPS write/undo actions wired to Zone A and Zone C buttons.
// Handles single Apply GPS, batch Apply All Accepted, per-photo Undo,
// and the Clear backups sweep (CR2). Shared UI helpers live in
// actions_shared.js so this file stays under the 150-line ceiling.

import { applyGPS, applyBatchGPS, undoGPS, clearAllBackups } from './api.js';
import { state } from './state.js';
import { buildApplyWarning, showToast, showConfirm } from './actions_shared.js';

// initActions wires up Apply All Accepted + Clear backups (Zone A) and
// delegates .btn-apply-single / .btn-undo clicks within Zone C.
export function initActions() {
    const applyAllBtn = document.getElementById('btn-apply-all');
    if (applyAllBtn) applyAllBtn.addEventListener('click', handleApplyAll);

    const clearBtn = document.getElementById('btn-clear-backups');
    if (clearBtn) clearBtn.addEventListener('click', handleClearBackups);

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

// handleClearBackups deletes every .bak and .bak.json under the scanned
// source folder. Irreversible — once cleared, Undo GPS can no longer
// restore photos that were already tagged in this or a prior session.
async function handleClearBackups() {
    if (!state.targetFolder) {
        showToast('No source folder scanned.', 'error');
        return;
    }
    const warning =
        `This deletes every .bak and .bak.json file under ${state.targetFolder}.

After this, Undo is no longer possible for photos already tagged. Continue?`;
    const ok = await showConfirm(warning);
    if (!ok) return;

    const btn = document.getElementById('btn-clear-backups');
    const originalText = btn ? btn.textContent : '';
    if (btn) { btn.disabled = true; btn.textContent = 'Clearing\u2026'; }
    try {
        const n = await clearAllBackups();
        showToast(`Cleared ${n} backup file${n === 1 ? '' : 's'}.`, 'success');
    } catch (err) {
        showToast('Clear backups failed: ' + String(err), 'error');
    } finally {
        if (btn) { btn.disabled = false; btn.textContent = originalText || 'Clear backups'; }
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
