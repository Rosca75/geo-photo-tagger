// app.js — Entry point for the GeoPhotoTagger frontend
// Imports all modules and wires up the init() function.
// Loaded by index.html via <script type="module" src="/js/app.js">

import { state } from './state.js';

// init() is called when the DOM is ready.
// It sets up event listeners and initializes the UI.
function init() {
    console.log('GeoPhotoTagger initialized');
    console.log('State:', state);
}

// Wait for the DOM to be fully loaded before initializing.
if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', init);
} else {
    init();
}
