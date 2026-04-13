// state.js — Shared state object (single source of truth)
// All shared application state lives here. No other module stores state
// in module-level variables. See CLAUDE.md rule #8.

export const state = {
    // Path to folder containing target photos (no GPS)
    targetFolder: null,

    // Array of { path, photoCount } objects for added reference folders
    referenceFolders: [],

    // Array of imported GPX/KML/CSV file paths
    gpsTrackFiles: [],

    // Scanned target photos (array of TargetPhoto objects from Go)
    targetPhotos: [],

    // Reference photos collected from all reference folders
    referencePhotos: [],

    // GPS track points from imported track files
    gpsTrackPoints: [],

    // Matching results from RunMatching()
    matchResults: null,

    // Whether a scan or match operation is currently running
    scanInProgress: false,

    // Maximum time distance in minutes for matching (default: 30)
    matchThreshold: 30,

    // Currently selected target photo path (for Zone C details)
    selectedPhoto: null,

    // Map of targetPath → { lat, lon, score, source } for accepted matches
    acceptedMatches: new Map()
};
