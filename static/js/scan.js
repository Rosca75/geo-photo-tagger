// scan.js — Folder browsing and target photo scanning
// Handles the "Source" button click, triggers the Go scan,
// and updates the status bar and table when results arrive.
// Also owns the reset-source handler and the topbar match-stats summary.

import { state } from './state.js';
import { openFolderDialog, scanTargetFolder } from './api.js';
import { renderTable } from './table.js';

// initScan wires up the source and reset buttons.
// Call once from app.js after DOMContentLoaded.
export function initScan() {
    const browseBtn = document.getElementById('btn-browse');
    if (browseBtn) {
        browseBtn.addEventListener('click', handleBrowseClick);
    }
    const resetBtn = document.getElementById('btn-reset-source');
    if (resetBtn) {
        resetBtn.addEventListener('click', handleResetSource);
    }
    // Bind the Include-subfolders checkbox so source scans reflect the
    // user's choice. Default preserves pre-phase-4 recursive behavior.
    const chk = document.getElementById('chk-source-recursive');
    if (chk) {
        chk.checked = state.sourceRecursive;
        chk.addEventListener('change', () => { state.sourceRecursive = chk.checked; });
    }
    updateMatchStats();
}

// handleBrowseClick opens the native folder picker and kicks off a scan
// if the user selects a folder (dialog not cancelled).
async function handleBrowseClick() {
    // openFolderDialog calls window.go.main.App.OpenFolderDialog() via api.js
    const path = await openFolderDialog();
    if (!path) return; // User cancelled — do nothing

    // Store the chosen path in shared state and show it in the input
    state.targetFolder = path;
    const pathInput = document.getElementById('target-folder-path');
    if (pathInput) pathInput.value = path;

    // Reveal the reset button now that a source has been chosen
    const resetBtn = document.getElementById('btn-reset-source');
    if (resetBtn) resetBtn.style.display = '';

    await runScan(path);
}

// handleResetSource clears the source folder, all target photos, and match results.
// Resets the UI to its initial empty state.
async function handleResetSource() {
    state.targetFolder = null;
    state.targetPhotos = [];
    state.matchResults = null;
    state.selectedPhoto = null;
    state.acceptedMatches.clear();

    const pathInput = document.getElementById('target-folder-path');
    if (pathInput) pathInput.value = '';

    const resetBtn = document.getElementById('btn-reset-source');
    if (resetBtn) resetBtn.style.display = 'none';

    renderTable([]);
    updateMatchStats();
    setStatusBar('Ready \u2014 select a source folder to begin');

    // Clear Zone C back to its initial placeholder
    const panel = document.querySelector('.match-panel');
    if (panel) {
        panel.innerHTML = '<p class="muted panel-placeholder">Select a photo from the list to see match details.</p>';
    }
}

// runScan calls the Go ScanTargetFolder method, shows a scanning indicator,
// then populates the table and stats when results arrive.
async function runScan(path) {
    if (state.scanInProgress) return;

    state.scanInProgress = true;
    setScanIndicator(true);
    setStatusBar('Scanning...');

    try {
        // scanTargetFolder returns Promise<TargetPhoto[]> via Wails bridge
        const photos = await scanTargetFolder(path, state.sourceRecursive);
        state.targetPhotos = Array.isArray(photos) ? photos : [];
        renderTable(state.targetPhotos);
        updateMatchStats();
        setStatusBar(`Scan complete \u2014 ${state.targetPhotos.length} photo${state.targetPhotos.length !== 1 ? 's' : ''} without GPS`);
    } catch (err) {
        // Keep a single error log — no per-file spam
        console.error('Scan failed:', err);
        setStatusBar('Scan failed: ' + String(err));
    } finally {
        state.scanInProgress = false;
        setScanIndicator(false);
    }
}

// updateMatchStats refreshes the #match-stats summary in the topbar.
// Exported so matcher_ui.js can call it after running match operations.
export function updateMatchStats() {
    const el = document.getElementById('match-stats');
    if (!el) return;
    const total = state.targetPhotos.length;
    if (total === 0) {
        el.textContent = 'No scan results yet';
        return;
    }
    const matched = state.matchResults
        ? state.matchResults.filter(r => r.bestCandidate).length
        : 0;
    el.textContent = `${total} photo${total !== 1 ? 's' : ''} \u2502 ${matched} matched \u2502 ${total - matched} unmatched`;
}

// setScanIndicator shows or hides the animated "Scanning..." indicator.
function setScanIndicator(visible) {
    const el = document.getElementById('scan-indicator');
    if (el) el.classList.toggle('hidden', !visible);
}

// setStatusBar updates the footer status bar text.
function setStatusBar(msg) {
    const el = document.getElementById('status-bar-text');
    if (el) el.textContent = msg;
}
