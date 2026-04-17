// settings.js — User-settings UI wiring. Currently drives the "Default TZ"
// dropdown, which controls how EXIF DateTime is interpreted for photos that
// lack an OffsetTimeOriginal tag (Pentax, older DSLRs).

import { state } from './state.js';
import { getSettings, setDefaultTimezone } from './api.js';

// initSettings loads persisted settings from Go, populates the dropdown,
// and wires the change handler. Errors fall back to the default "Local".
export async function initSettings() {
    try {
        const s = await getSettings();
        state.defaultTimezone = s.defaultTimezone || 'Local';
    } catch { /* use default */ }

    const sel = document.getElementById('select-default-tz');
    if (!sel) return;
    sel.value = state.defaultTimezone;
    sel.addEventListener('change', async () => {
        try {
            await setDefaultTimezone(sel.value);
            state.defaultTimezone = sel.value;
            // User must re-scan for the new TZ to take effect — already-loaded
            // photos were anchored at scan time and are not re-parsed here.
            notifyTZChanged();
        } catch (err) {
            console.error('setDefaultTimezone failed:', err);
        }
    });
}

// notifyTZChanged shows a transient toast. Duplicates the minimal toast
// builder from actions.js rather than exporting it (keeps this module
// self-contained and the change surgical).
function notifyTZChanged() {
    const msg = 'Default timezone changed — re-scan sources to apply.';
    let container = document.getElementById('toast-container');
    if (!container) {
        container = document.createElement('div');
        container.id = 'toast-container';
        document.body.appendChild(container);
    }
    const toast = document.createElement('div');
    toast.className = 'toast toast-success';
    toast.textContent = msg;
    container.appendChild(toast);
    setTimeout(() => {
        toast.classList.add('removing');
        toast.addEventListener('animationend', () => toast.remove(), { once: true });
    }, 3000);
}
