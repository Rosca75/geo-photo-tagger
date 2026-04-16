// geocode.js — Reverse-geocoding with in-memory caching.
// Wraps api.reverseGeocode() with a Map cache keyed on rounded coordinates.
// Nominatim's usage policy is 1 request/second; the cache is what makes
// that survivable when the user clicks between candidates quickly.

import { state } from './state.js';
import { reverseGeocode as apiReverseGeocode } from './api.js';

// cacheKey rounds coordinates to 5 decimals (~1.1 m precision) so that near-
// duplicate photos share a cache entry. The key format is deterministic and
// canonical — do not change it without clearing existing caches.
function cacheKey(lat, lon) {
    return `${lat.toFixed(5)},${lon.toFixed(5)}`;
}

// getLocationForCoords returns a location string for (lat, lon), using the
// cache if possible. Returns "" on any failure — callers should fall back
// to showing raw coordinates only.
export async function getLocationForCoords(lat, lon) {
    const key = cacheKey(lat, lon);
    if (state.geocodeCache.has(key)) {
        return state.geocodeCache.get(key);
    }
    try {
        const loc = await apiReverseGeocode(lat, lon);
        // Cache empty strings too — no point retrying a Nominatim miss.
        state.geocodeCache.set(key, loc || '');
        return loc || '';
    } catch {
        return '';
    }
}

// refreshLocationFor updates the #gps-location-info element inside
// containerEl for the given accepted match. Safe to call repeatedly — the
// cache absorbs duplicate calls. Writes "Loading location…" first so the
// user sees something change before the (possibly cached) resolution.
//
// Re-queries the container after the await so that if the panel re-rendered
// while we were waiting, we write into whichever element is currently in
// the DOM — this is what kills the race bug in matcher_ui.js.
export async function refreshLocationFor(acc, containerEl) {
    if (!acc || !containerEl) return;
    const el = containerEl.querySelector('#gps-location-info');
    if (!el) return;
    el.textContent = 'Loading location\u2026';
    const loc = await getLocationForCoords(acc.lat, acc.lon);
    const stillThere = containerEl.querySelector('#gps-location-info');
    if (stillThere) {
        stillThere.textContent = loc || 'Location not available';
    }
}
