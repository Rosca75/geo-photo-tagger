// app.js — Entry point for the GeoPhotoTagger frontend
// Imports all feature modules and calls their init functions.
// Loaded by index.html via <script type="module" src="/js/app.js">

import { state } from './state.js';
import { initScan } from './scan.js';
import { initTrack } from './track.js';
import { initReference } from './reference.js';
import { initMatcher } from './matcher_ui.js';

// init wires up all UI modules after the DOM is ready.
function init() {
    console.log('GeoPhotoTagger initialising...');

    if (typeof feather !== 'undefined') {
        feather.replace();
    }

    initScan();
    initTrack();
    initReference();
    initMatcher();
    console.log('GeoPhotoTagger ready. State:', state);
}

// Guard against the script running before DOMContentLoaded
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
