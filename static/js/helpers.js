// helpers.js — Pure utility functions shared across modules.
// Extracted from matcher_ui.js and table.js to avoid duplication.
// See CLAUDE.md rule #8: no shared state here — only stateless functions.

// escapeHtml prevents XSS when inserting user-controlled strings into innerHTML.
// Escapes the five characters that are special in HTML contexts.
export function escapeHtml(str) {
    if (!str) return '';
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
}

// formatDate converts an ISO 8601 string (from Go time.Time) to a locale string.
// Returns an em-dash for missing or zero-value timestamps.
export function formatDate(raw) {
    if (!raw || raw === '0001-01-01T00:00:00Z') return '\u2014';
    return new Date(raw).toLocaleString();
}
