//go:build !linux && !darwin

package main

// heicDynamicVersionAtLeast returns true on platforms (Windows, BSD, etc.)
// where we do not probe for a dynamic libheif installation.
// On these platforms, initHEIC() will have already returned early after
// checking heic.Dynamic(), so this function is only reachable if a dynamic
// library somehow loaded — we treat that as "version is fine".
func heicDynamicVersionAtLeast(major, minor int) bool {
	return true
}
