package main

// app_settings.go — Wails-bound settings methods. Split from app.go to keep
// that file from growing further; both methods read/write the package-level
// state defined in settings.go.

import (
	"fmt"
	"time"
)

// GetSettings returns the current user settings to the frontend.
// A read lock on settingsMu prevents a torn read if SetDefaultTimezone
// is racing with a GetSettings call.
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
