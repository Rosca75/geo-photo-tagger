// app.js — Entry point for the GeoPhotoTagger frontend
// Imports all feature modules and calls their init functions.
// Loaded by index.html via <script type="module" src="/js/app.js">

import { state } from './state.js';
import { initScan } from './scan.js';

// init wires up all UI modules after the DOM is ready.
function init() {
    console.log('GeoPhotoTagger initialising...');
    initScan();
    console.log('GeoPhotoTagger ready. State:', state);
}

// Guard against the script running before DOMContentLoaded
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
