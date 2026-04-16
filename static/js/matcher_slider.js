// matcher_slider.js — Max-delta slider wiring for Zone A.
// Split out of matcher_ui.js to keep that file under the 150-line limit.

import { state } from './state.js';

// formatDeltaMinutes turns a minute count into the compact label shown next
// to the slider: "5 min", "30 min", "2 h", "1 h 30".
function formatDeltaMinutes(m) {
    if (m < 60) return `${m} min`;
    const h = Math.floor(m / 60);
    const rem = m % 60;
    if (rem === 0) return `${h} h`;
    return `${h} h ${rem}`;
}

// initDeltaSlider binds the range input to state.matchThreshold.
// The 'input' handler updates the label live on drag; the 'change' handler
// commits to state only on release, so downstream stats don't recompute
// 60 times per second while the user is still moving the thumb.
export function initDeltaSlider() {
    const slider = document.getElementById('delta-slider');
    const valueLabel = document.getElementById('delta-slider-value');
    if (!slider || !valueLabel) return;
    slider.value = String(state.matchThreshold);
    valueLabel.textContent = formatDeltaMinutes(state.matchThreshold);
    slider.addEventListener('input', () => {
        valueLabel.textContent = formatDeltaMinutes(parseInt(slider.value, 10));
    });
    slider.addEventListener('change', () => {
        state.matchThreshold = parseInt(slider.value, 10);
    });
}
