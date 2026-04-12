// scan.js — Folder browsing and target photo scanning
// Handles the "Browse" button click, triggers the Go scan,
// and updates the status bar and table when results arrive.

import { state } from './state.js';
import { openFolderDialog, scanTargetFolder } from './api.js';
import { renderTable } from './table.js';

// initScan wires up the browse button event listener.
// Call once from app.js after DOMContentLoaded.
export function initScan() {
    const browseBtn = document.getElementById('btn-browse');
    if (browseBtn) {
        browseBtn.addEventListener('click', handleBrowseClick);
    }
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

    await runScan(path);
}

// runScan calls the Go ScanTargetFolder method, shows a scanning indicator,
// then populates the table and status bar with results.
async function runScan(path) {
    if (state.scanInProgress) return;

    state.scanInProgress = true;
    setScanIndicator(true);
    setStatusBar('Scanning...');

    try {
        // scanTargetFolder returns Promise<TargetPhoto[]> via Wails bridge
        const photos = await scanTargetFolder(path);
        state.targetPhotos = Array.isArray(photos) ? photos : [];
        renderTable(state.targetPhotos);
        setStatusBar(buildStatusText(state.targetPhotos.length));
    } catch (err) {
        console.error('Scan failed:', err);
        setStatusBar('Scan failed: ' + String(err));
    } finally {
        state.scanInProgress = false;
        setScanIndicator(false);
    }
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

// buildStatusText formats the footer status line from a photo count.
function buildStatusText(total) {
    return `${total} target photo${total !== 1 ? 's' : ''} \u2502 0 matched \u2502 ${total} unmatched`;
}
