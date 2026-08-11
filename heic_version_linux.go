//go:build linux

package main

// heicDynamicVersionAtLeast returns true on Linux.
// The geo-photo-tagger CI build does not install libheif, so we cannot probe
// the system library at compile time. We return true unconditionally so that
// initHEIC() proceeds to the WASM-based path (github.com/Rosca75/heic), which
// has no native library dependency and works on all platforms.
func heicDynamicVersionAtLeast(major, minor int) bool {
	return true
}
