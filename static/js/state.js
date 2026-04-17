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
    acceptedMatches: new Map(),

    // Cache: "lat5,lon5" → "City, Region, Country" (rounded to 5 decimals ≈ 1.1 m).
    // Prevents repeat Nominatim calls when the user clicks back and forth
    // between candidates with the same coordinates. Nominatim's usage policy
    // is 1 request/second; the cache is what makes that survivable.
    geocodeCache: new Map(),

    // Whether the mini world map is visible in Zone C (populated in phase 5).
    // Default false: no Leaflet CDN download until the user opts in.
    mapEnabled: false,

    // Whether Source scans recurse into subfolders. Default true preserves
    // pre-phase-4 behavior.
    sourceRecursive: true,

    // Whether Ref scans recurse into subfolders. Default true.
    refRecursive: true,

    // Which source feeds the next match run:
    //   'refs'   — external reference folders (module 1)
    //   'track'  — imported GPX/KML/CSV (module 2)
    //   'same'   — photos inside the source folder with GPS (module 3)
    // Default 'refs' preserves pre-phase-7 behavior.
    matchMode: 'refs',

    // Default timezone used to interpret EXIF DateTime for photos that lack
    // an OffsetTimeOriginal tag (Pentax, older DSLRs). IANA name or "Local".
    defaultTimezone: 'Local'
};
