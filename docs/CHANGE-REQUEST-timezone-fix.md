# Change Request — Timezone-aware DateTime matching

> **Audience:** Claude Sonnet 4.6 working on `Rosca75/geo-photo-tagger` on Windows 11.
> **Before touching any file, read `CLAUDE.md` in full.** All its rules apply.
> **Scope:** one bug, one fix. No opportunistic refactoring. Keep it surgical.

---

## 1. The bug

Matching is done by comparing `DateTimeOriginal` timestamps. The current reader parses EXIF DateTime with `time.Parse("2006:01:02 15:04:05", s)` — a format with no timezone token, so Go silently assigns **UTC**. The matcher's subsequent `.UTC()` call is then a no-op, and the EXIF `OffsetTimeOriginal` tag (0x9011) is never read.

Consequence: photos from cameras that write an offset (iPhone, modern Android, most mirrorless since ~2016) are mis-anchored in time, and photos from cameras that don't write an offset (Pentax, older DSLRs) are treated as UTC when they really represent local wall-clock time.

**Concrete case:** IMGP3984.DNG (Pentax, `16:05:59`, no offset) and IMG_8375.HEIC (iPhone, `10:21:19 -05:00`) were shot 15 minutes apart in reality. The matcher sees a 5h 44min delta and rejects the match at any reasonable threshold.

---

## 2. The fix (Level 2)

1. **Read `OffsetTimeOriginal`** (and, for robustness, `OffsetTime`) when present in EXIF. Use it to anchor `DateTimeOriginal` to UTC.
2. **For photos without an offset tag**, interpret the naive DateTime in a **user-configurable default timezone**, defaulting to the operating system's local timezone. Then convert to UTC.
3. **Expose the setting** in the UI as a single global dropdown. Persist it across sessions.

Result: every `DateTimeOriginal` in the system is a correctly anchored UTC `time.Time` by the time it reaches the matcher. The matcher's existing `.UTC()` calls become genuinely idempotent.

---

## 3. Files to change

### 3.1 `dng_scan_reader.go` + `dng_scan_reader_datetime.go`

The DNG scan reader currently calls `parseEXIFDateTime`. It needs to also read the offset tag (if present) and the default-timezone setting, and produce a UTC-anchored `time.Time`.

**In `dng_scan_reader.go`**, extend `scanIFD0ForGPSAndDateTime` to also collect the `OffsetTimeOriginal` tag:

- Add constant `tagOffsetTimeOriginal = 0x9011` alongside the existing tag constants.
- Add a fourth return value `offsetStr string`.
- In the tag-walk loop, handle `case tagOffsetTimeOriginal:` the same way `tagDateTime` is handled (ASCII, external-offset via `readASCIITag`, save/restore position).

Note: `OffsetTimeOriginal` lives in the **Exif sub-IFD**, not IFD0. If IFD0 doesn't contain it, also walk the Exif sub-IFD (tag `0x8769` `ExifIFDPointer`) once — add a small helper `scanExifIFDForOffset` that seeks to the Exif IFD offset and walks looking only for `0x9011`. Return `""` if either the Exif IFD pointer is absent or the tag isn't found.

**In `readDNGScanFields`**, after the walk:

```go
if dateTimeStr != "" {
    if t, tErr := parseEXIFDateTimeToUTC(dateTimeStr, offsetStr); tErr == nil {
        result.HasDateTime = true
        result.DateTimeOriginal = t
    }
}
```

**In `dng_scan_reader_datetime.go`**, replace `parseEXIFDateTime` with:

```go
// parseEXIFDateTimeToUTC parses an EXIF "YYYY:MM:DD HH:MM:SS" timestamp and
// returns a UTC-anchored time.Time.
//
// If offsetStr is non-empty and valid (e.g. "-05:00", "+01:00"), the offset
// is applied directly. Otherwise the timestamp is interpreted in the
// user-configured default timezone (see GetDefaultTimezone), which defaults
// to the OS local timezone.
//
// This is the single canonical EXIF DateTime parser for the scan path.
// Every scanned photo flows through here so matcher comparisons operate on
// correctly-anchored UTC values.
func parseEXIFDateTimeToUTC(dateTimeStr, offsetStr string) (time.Time, error) {
    const layout = "2006:01:02 15:04:05"

    if offsetStr != "" {
        // Offset format is "+HH:MM" or "-HH:MM". time.Parse with a layout
        // that includes "-07:00" handles both signs.
        combined := dateTimeStr + " " + offsetStr
        if t, err := time.Parse(layout+" -07:00", combined); err == nil {
            return t.UTC(), nil
        }
        // Fall through to default-tz path if offset string is malformed.
    }

    loc := GetDefaultTimezone()
    t, err := time.ParseInLocation(layout, dateTimeStr, loc)
    if err != nil {
        return time.Time{}, err
    }
    return t.UTC(), nil
}
```

Delete the old `parseEXIFDateTime` function.

### 3.2 `exif_reader.go` (JPEG + HEIC path)

Locate the JPEG/HEIC DateTime parsing. Currently it extracts `DateTimeOriginal` via goexif and parses it the same naive way. Modify it to:

1. Also extract `OffsetTimeOriginal` (goexif tag name: `exif.OffsetTimeOriginal` — verify by grepping goexif's field names).
2. Call `parseEXIFDateTimeToUTC(dateTimeStr, offsetStr)` instead of the naive parse.

If the JPEG path currently stores the parsed time directly into `ExifResult.DateTimeOriginal` via `time.Parse`, replace with the new function. The field type stays `time.Time`.

**Important:** The matcher's `.UTC()` calls stay as-is. They're harmless (idempotent) and act as a safety net.

### 3.3 New file: `settings.go`

```go
package main

// settings.go — Persisted user settings. Currently holds only the default
// timezone used to interpret EXIF DateTime from cameras that do not write
// an OffsetTimeOriginal tag (Pentax, older DSLRs). Defaults to the OS
// local timezone.

import (
    "encoding/json"
    "os"
    "path/filepath"
    "sync"
    "time"
)

type userSettings struct {
    // DefaultTimezone is an IANA timezone name (e.g. "Europe/Luxembourg",
    // "America/New_York") or the literal "Local" to mean OS local time.
    // Empty string is treated as "Local".
    DefaultTimezone string `json:"defaultTimezone"`
}

var (
    settingsMu    sync.RWMutex
    currentSettings userSettings
    settingsPath  string
)

// initSettings loads settings from disk. Call once at app startup.
// Creates an empty settings file if none exists.
func initSettings() error {
    dir, err := os.UserConfigDir()
    if err != nil {
        return err
    }
    settingsPath = filepath.Join(dir, "geo-photo-tagger", "settings.json")
    if err := os.MkdirAll(filepath.Dir(settingsPath), 0755); err != nil {
        return err
    }
    data, err := os.ReadFile(settingsPath)
    if os.IsNotExist(err) {
        currentSettings = userSettings{DefaultTimezone: "Local"}
        return saveSettings()
    }
    if err != nil {
        return err
    }
    return json.Unmarshal(data, &currentSettings)
}

func saveSettings() error {
    data, err := json.MarshalIndent(currentSettings, "", "  ")
    if err != nil {
        return err
    }
    return os.WriteFile(settingsPath, data, 0644)
}

// GetDefaultTimezone returns the user's configured default timezone, or
// time.Local if the setting is empty / "Local" / invalid. Safe to call
// from any goroutine.
func GetDefaultTimezone() *time.Location {
    settingsMu.RLock()
    name := currentSettings.DefaultTimezone
    settingsMu.RUnlock()

    if name == "" || name == "Local" {
        return time.Local
    }
    loc, err := time.LoadLocation(name)
    if err != nil {
        return time.Local
    }
    return loc
}
```

### 3.4 `app.go`

In `startup`, after the logger setup, add:

```go
if err := initSettings(); err != nil {
    slog.Warn("settings_init_failed", "error", err)
}
```

Add two bound methods (can go in `app.go` or a new `app_settings.go` if `app.go` is near 150 lines):

```go
// GetSettings returns the current user settings to the frontend.
func (a *App) GetSettings() userSettings {
    settingsMu.RLock()
    defer settingsMu.RUnlock()
    return currentSettings
}

// SetDefaultTimezone updates the default timezone used for photos that lack
// an EXIF OffsetTimeOriginal tag. Accepts any IANA timezone name that
// time.LoadLocation recognises, or "Local" for OS-local time. Returns
// error if the name is invalid.
func (a *App) SetDefaultTimezone(tz string) error {
    if tz != "Local" && tz != "" {
        if _, err := time.LoadLocation(tz); err != nil {
            return fmt.Errorf("invalid timezone %q: %w", tz, err)
        }
    }
    settingsMu.Lock()
    currentSettings.DefaultTimezone = tz
    settingsMu.Unlock()
    return saveSettings()
}
```

### 3.5 `static/js/api.js`

```javascript
export async function getSettings() {
    return window.go.main.App.GetSettings();
}
export async function setDefaultTimezone(tz) {
    return window.go.main.App.SetDefaultTimezone(tz);
}
```

### 3.6 `static/js/state.js`

```javascript
// Default timezone used to interpret EXIF DateTime for photos that lack
// an OffsetTimeOriginal tag (Pentax, older DSLRs). IANA name or "Local".
defaultTimezone: 'Local',
```

### 3.7 UI — settings dropdown

Add a dropdown near the Data Sources zone. Keep it minimal: a `<select>` with the most common timezones plus "Local (OS default)".

**In `static/index.html`**, add to a suitable spot (e.g. a small settings row at the top right of the Data Sources zone):

```html
<label class="settings-row" title="Timezone used to interpret photos without an OffsetTime EXIF tag (e.g. Pentax DSLRs)">
    <span class="settings-label">Default TZ:</span>
    <select id="select-default-tz">
        <option value="Local">Local (OS)</option>
        <option value="UTC">UTC</option>
        <option value="Europe/Luxembourg">Europe/Luxembourg</option>
        <option value="Europe/Paris">Europe/Paris</option>
        <option value="Europe/London">Europe/London</option>
        <option value="America/New_York">America/New_York</option>
        <option value="America/Los_Angeles">America/Los_Angeles</option>
        <option value="Asia/Tokyo">Asia/Tokyo</option>
    </select>
</label>
```

**In `static/css/components.css`**, append:

```css
.settings-row {
    display: inline-flex;
    align-items: center;
    gap: var(--space-xs);
    font-size: 0.75rem;
    color: var(--text-light);
}
.settings-label { white-space: nowrap; }
#select-default-tz {
    font-size: 0.8rem;
    padding: 2px 4px;
}
```

**In a suitable init function** (e.g. create a small `static/js/settings.js` if nothing else makes sense; keep it under 50 lines):

```javascript
import { state } from './state.js';
import { getSettings, setDefaultTimezone } from './api.js';

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
            // User must re-scan for the new TZ to take effect.
            showToastOrLog('Default timezone changed — re-scan sources to apply.');
        } catch (err) {
            console.error('setDefaultTimezone failed:', err);
        }
    });
}
```

Wire `initSettings()` into `app.js` alongside the other init calls. `showToastOrLog` is a placeholder — if the app has a notification helper, use it; otherwise a `console.log` is fine.

---

## 4. Important: the timezone setting applies to future scans

The parsed timestamps are **stored in UTC on each photo struct** after a scan. Changing the default timezone does NOT retroactively re-anchor already-scanned photos. The UI should communicate this (the toast above does). If the user changes the setting, they re-scan.

Do NOT attempt to re-parse already-loaded photos in place — that path is error-prone and out of scope.

---

## 5. Regression test

**File:** `dng_scan_reader_datetime_test.go`

```go
package main

import (
    "testing"
    "time"
)

func TestParseEXIFDateTimeToUTC(t *testing.T) {
    tests := []struct {
        name      string
        dt        string
        offset    string
        wantUTC   string // RFC3339
    }{
        {
            name:    "iPhone with negative offset",
            dt:      "2025:02:14 10:21:19",
            offset:  "-05:00",
            wantUTC: "2025-02-14T15:21:19Z",
        },
        {
            name:    "CET photo with positive offset",
            dt:      "2025:02:14 16:05:59",
            offset:  "+01:00",
            wantUTC: "2025-02-14T15:05:59Z",
        },
        // The no-offset case depends on GetDefaultTimezone() which reads
        // settings from disk; cover that via an integration test rather
        // than a unit test.
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got, err := parseEXIFDateTimeToUTC(tc.dt, tc.offset)
            if err != nil {
                t.Fatalf("error: %v", err)
            }
            want, _ := time.Parse(time.RFC3339, tc.wantUTC)
            if !got.Equal(want) {
                t.Errorf("got %v, want %v", got.UTC(), want)
            }
        })
    }
}
```

---

## 6. Verification

1. `go vet ./...` clean
2. `go test ./... -run TestParseEXIFDateTimeToUTC` passes
3. `wails build -platform windows/amd64` succeeds
4. `wails dev`: the "Default TZ" dropdown is visible and changing it persists across app restarts (check `%APPDATA%\geo-photo-tagger\settings.json`)
5. End-to-end: scan a folder with IMG_8375.HEIC as source and IMGP3984.DNG as reference (or vice versa). With the default TZ set to `Europe/Luxembourg` and the delta slider at 30 min, the two files should match (true delta ≈ 15 min).

---

## 7. Commit

Single commit: `feat: timezone-aware DateTime matching`.

Commit body:

```
EXIF DateTime was parsed with a layout lacking any timezone token, which
Go silently treats as UTC. OffsetTimeOriginal (0x9011) was never read.
Result: iPhones' local-time + offset pairs were mis-anchored, and
Pentax-style naive timestamps were treated as UTC when they represent
local wall-clock time.

Fix:
- Read OffsetTimeOriginal from the Exif sub-IFD when present (DNG scan
  path; JPEG/HEIC path via goexif).
- New parseEXIFDateTimeToUTC applies the offset when available, falls
  back to a user-configured default timezone otherwise.
- New persisted setting "defaultTimezone" (IANA name, default "Local")
  with a small UI dropdown and two bound methods.

Changing the TZ does not retroactively re-anchor already-scanned photos;
the user re-scans. A toast informs them.
```

---

## 8. Exit criteria

- [ ] `parseEXIFDateTimeToUTC` is the single DateTime-parsing entry point for the scan path
- [ ] `OffsetTimeOriginal` is read for both DNG and JPEG/HEIC
- [ ] Settings persist to `%APPDATA%\geo-photo-tagger\settings.json`
- [ ] Dropdown visible, functional, persisted
- [ ] Regression test passes
- [ ] `go vet ./...` clean
- [ ] `wails build -platform windows/amd64` succeeds
- [ ] No file exceeds 150 lines
