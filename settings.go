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

// userSettings is the on-disk schema for %APPDATA%/geo-photo-tagger/settings.json.
// Keep field additions backwards-compatible — old files omitting a field should
// still unmarshal cleanly and fall back to the zero value / default handling.
type userSettings struct {
	// DefaultTimezone is an IANA timezone name (e.g. "Europe/Luxembourg",
	// "America/New_York") or the literal "Local" to mean OS local time.
	// Empty string is treated as "Local".
	DefaultTimezone string `json:"defaultTimezone"`
}

// Package-level state guarded by settingsMu. settingsPath is the resolved
// on-disk location after initSettings() runs; it stays empty if init fails
// (in which case saveSettings becomes a no-op for WriteFile).
var (
	settingsMu      sync.RWMutex
	currentSettings userSettings
	settingsPath    string
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

// saveSettings serialises currentSettings to the resolved settingsPath.
// Caller is responsible for holding settingsMu at least in read mode (the
// write lock is held by SetDefaultTimezone during mutation; init runs
// before any goroutines are spawned so a bare call is safe there).
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
