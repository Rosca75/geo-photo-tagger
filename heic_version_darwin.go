//go:build darwin

package main

// heic_version_darwin.go — runtime probe for the system libheif version on macOS.
//
// This is the darwin twin of heic_version_linux.go. Without it the darwin build
// does not compile at all: heic_thumbnail.go calls heicDynamicVersionAtLeast,
// heic_version_linux.go is //go:build linux, and heic_version_other.go is
// //go:build !linux && !darwin, so nothing provided the symbol on darwin.

import "github.com/ebitengine/purego"

// heicDynamicVersionAtLeast reports whether the dynamically loaded libheif is
// at least major.minor. Returns true if the library cannot be opened — see the
// linux twin for why that is the safe default.
func heicDynamicVersionAtLeast(major, minor int) bool {
	var handle uintptr
	var err error
	// Try the plain name first (picks up anything already on the loader path),
	// then Homebrew's default location, which is where libheif normally lands
	// on Apple Silicon and is not on the default dlopen search path.
	for _, lib := range []string{"libheif.dylib", "/opt/homebrew/lib/libheif.dylib"} {
		handle, err = purego.Dlopen(lib, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err == nil {
			break
		}
	}
	if err != nil {
		return true
	}
	defer purego.Dlclose(handle)

	var getMajor func() uint32
	var getMinor func() uint32
	purego.RegisterLibFunc(&getMajor, handle, "heif_get_version_number_major")
	purego.RegisterLibFunc(&getMinor, handle, "heif_get_version_number_minor")

	maj := int(getMajor())
	min := int(getMinor())
	if maj != major {
		return maj > major
	}
	return min >= minor
}
