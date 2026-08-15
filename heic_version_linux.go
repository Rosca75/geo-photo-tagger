//go:build linux

package main

// heic_version_linux.go — runtime probe for the system libheif version on Linux.
//
// Why a runtime probe and not a build-time check: purego opens the shared
// library with dlopen at run time, so this costs nothing at build time and
// needs no libheif present when compiling. CI never installing libheif is
// therefore not a reason to skip the check — the probe simply answers "true"
// when the library cannot be opened.

import "github.com/ebitengine/purego"

// heicDynamicVersionAtLeast reports whether the dynamically loaded libheif is
// at least major.minor.
//
// If the library cannot be opened we return true, which is the safe default:
// initHEIC() only consults this function when heic.Dynamic() has already
// reported a usable library, so a failure to open here means we could not
// determine the version rather than that the version is old.
func heicDynamicVersionAtLeast(major, minor int) bool {
	// RTLD_NOW resolves all symbols immediately so a missing symbol fails here
	// rather than at first call; RTLD_GLOBAL matches how the heic package itself
	// opens the library.
	handle, err := purego.Dlopen("libheif.so", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		return true
	}
	defer purego.Dlclose(handle)

	// RegisterLibFunc binds a Go func variable to a C symbol by name. Both
	// symbols are part of libheif's stable public API.
	var getMajor func() uint32
	var getMinor func() uint32
	purego.RegisterLibFunc(&getMajor, handle, "heif_get_version_number_major")
	purego.RegisterLibFunc(&getMinor, handle, "heif_get_version_number_minor")

	maj := int(getMajor())
	min := int(getMinor())
	// A different major version decides the comparison on its own; only when the
	// majors are equal does the minor matter.
	if maj != major {
		return maj > major
	}
	return min >= minor
}
