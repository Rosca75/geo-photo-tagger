// map.js — Leaflet mini-map lazy loader + marker renderer for Zone C.
//
// Leaflet CSS+JS is loaded on first use only — zero network cost when the
// map toggle is off. Subsequent renders reuse the already-loaded library.
//
// The map is deliberately non-interactive: no drag, no zoom controls, no
// scroll-wheel zoom, no double-click-to-zoom. This is a "you are here"
// preview, not a navigator — and it keeps CPU cost predictable when the
// user clicks rapidly through candidates.

import { state } from './state.js';

const LEAFLET_CSS = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.css';
const LEAFLET_JS  = 'https://unpkg.com/leaflet@1.9.4/dist/leaflet.js';

let leafletLoadPromise = null;

// loadLeaflet lazily injects Leaflet's CSS+JS on first call. Returns a
// promise resolving to the global L object. Subsequent callers wait on
// the same cached promise.
function loadLeaflet() {
    if (typeof window.L !== 'undefined') return Promise.resolve(window.L);
    if (leafletLoadPromise) return leafletLoadPromise;

    leafletLoadPromise = new Promise((resolve, reject) => {
        const link = document.createElement('link');
        link.rel = 'stylesheet';
        link.href = LEAFLET_CSS;
        document.head.appendChild(link);

        const script = document.createElement('script');
        script.src = LEAFLET_JS;
        script.async = true;
        script.onload = () => resolve(window.L);
        script.onerror = () => reject(new Error('Leaflet CDN load failed'));
        document.head.appendChild(script);
    });
    return leafletLoadPromise;
}

// renderMap renders a mini-map inside the #gps-mini-map container for the
// given coordinates. Safe to call repeatedly — previous Leaflet instances
// are torn down before creating a new one (Leaflet does not garbage-collect
// on its own, so explicit .remove() is important).
export async function renderMap(lat, lon, containerEl) {
    if (!containerEl) return;
    try {
        const L = await loadLeaflet();
        const target = containerEl.querySelector('#gps-mini-map');
        if (!target) return;
        if (target._leafletInstance) {
            target._leafletInstance.remove();
            target._leafletInstance = null;
        }
        const map = L.map(target, {
            zoomControl: false,
            dragging: false,
            scrollWheelZoom: false,
            doubleClickZoom: false,
            boxZoom: false,
            keyboard: false,
            attributionControl: true,
        }).setView([lat, lon], 10);

        L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', {
            maxZoom: 18,
            // OSM tile usage requires attribution — do not remove.
            attribution: '&copy; OpenStreetMap contributors',
        }).addTo(map);

        L.marker([lat, lon]).addTo(map);
        target._leafletInstance = map;
    } catch (err) {
        console.warn('Map render failed:', err);
    }
}

// toggleMap flips state.mapEnabled and re-renders Zone C via the
// photo-selected event. Session-only — resets on app restart.
export function toggleMap() {
    state.mapEnabled = !state.mapEnabled;
    if (state.selectedPhoto) {
        const photo = state.targetPhotos.find(p => p.path === state.selectedPhoto);
        if (photo) {
            document.dispatchEvent(new CustomEvent('photo-selected', { detail: { photo } }));
        }
    }
}
