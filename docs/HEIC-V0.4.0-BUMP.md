# Bump `Rosca75/heic` v0.1.0 → v0.4.0, and fix the HEIC backend selection

> Status: ready to execute. Every claim below was verified on Linux against the real library
> and `samples/IMG_2656.HEIC` before this plan was written — see §9 for what was measured and
> how to reproduce it. Where a claim could **not** be verified here, it says so explicitly.
>
> This plan follows the same bump in `dedup-photos` (its `docs/03-HEIC-V0.4.0-BUMP.md`).
> That one shipped, and two of its conclusions turned out to be wrong in ways that matter
> here — §5 and §7 exist because of it.

## 0. Scope

The dependency bump itself is trivial (§2, one command, no source edits). The reason this
document is long is that verifying it surfaced **three real bugs in the HEIC path**, all
pre-existing and none caused by the bump. They are in scope here because they live in the
same file, they are each a few lines, and every one of them is invisible on this developer
box while being live on the platform this app actually ships to.

| § | Work item | Severity |
|---|---|---|
| §2 | Bump `Rosca75/heic` to v0.4.0 | routine |
| §4 | **Bug A** — `heicDynamicVersionAtLeast` never probes anything on Linux | high |
| §5 | **Bug B** — `prewarmHEIC()` is skipped on exactly the platforms that need it | high |
| §6 | **Bug C** — the macOS build does not compile | medium |
| §7 | Correct CLAUDE.md, which contradicts the shipped code | medium |
| §8 | Explicitly NOT adopting `heic.DecodeExif` | — |

Bugs A, B and C are all in `heic_thumbnail.go` / `heic_version_*.go` and are best fixed in one
commit, separate from the dependency bump.

## 1. Why bump

`Rosca75/heic` v0.4.0 syncs the fork onto upstream `gen2brain/heic`, which replaced the
libheif/libde265 C-to-WASM decoder with the **pure-Rust `heic` crate** run under wazero.

- **~13% faster HEIC thumbnail decode** on the WASM path this app uses (§9).
- **~2.8× cheaper WASM startup** — the one-time module compile that `prewarmHEIC()` exists to
  hide drops from 633 ms to 225 ms.
- `DecodeExif` and `DecodeAll`, neither of which this app should adopt (§8).

This is a three-version jump (v0.1.0 → v0.4.0), not the single step `dedup-photos` made.

## 2. The bump is one command, no source edits

Verified on a full copy of this repo at `92fc51b`: builds, vets, tests, cross-compiles to
Windows, and passes `gofmt -l`.

```bash
go get github.com/Rosca75/heic@v0.4.0
go mod tidy
```

| Module | Before | After |
|---|---|---|
| `github.com/Rosca75/heic` | v0.1.0 **(indirect)** | **v0.4.0 (direct)** |
| `github.com/ebitengine/purego` | v0.9.1 | v0.10.1 |
| `github.com/tetratelabs/wazero` (indirect) | v1.9.0 | **v1.12.0** |
| `golang.org/x/sys` (indirect) | v0.30.0 | v0.44.0 |

The `go` directive does **not** change — this repo already declares `go 1.25.0`, which is what
heic v0.4.0 requires.

Note the first row. `heic_thumbnail.go` imports `github.com/Rosca75/heic` directly, but
`go.mod` marks it `// indirect` — a stale annotation. `go mod tidy` promotes it. That is a
correction, not a side effect; do not revert it. Fixing Bug A (§4) also promotes
`ebitengine/purego` to direct for the same reason.

## 3. Bump risks checked and ruled out

### 3.1 The `*image.YCbCr` → `*image.NRGBA` return-type change — DOES NOT APPLY

The WASM backend now returns `*image.NRGBA` (the Rust crate emits RGBA8) where the old one
returned YCbCr planes. That breaks any caller doing a concrete type assertion.

**This repo does none.** A search for `.(*image.`, `image.YCbCr`, `image.NRGBA`, `image.RGBA`
and `image.Gray` across all `.go` files returns **zero** hits. `generateHEICThumbnail` passes
the decoded image straight to `scaleToFit` and `encodeJPEGBase64`, both interface-level.

If you later add code that type-switches on a decoded HEIC, remember it can be `*image.NRGBA`
(WASM) **or** `*image.YCbCr` (dynamic libheif).

### 3.2 The 192 KB truncated-header fast path — STILL WORKS

`decodeHEICImage` hands `DecodeThumbnail` a **truncated 192 KB buffer** and relies on the
decoder walking the ISOBMFF `iloc` box to find the thumbnail tile inside that window.

Measured on `samples/IMG_2656.HEIC`:

| | thumbnail from 192 KB header | primary from 192 KB header |
|---|---|---|
| v0.1.0, forced WASM | ok | fails (expected) |
| v0.1.0, dynamic libheif 1.19.8 | ok | fails (expected) |
| **v0.4.0, forced WASM** | **ok** | fails (expected) |
| **v0.4.0, dynamic libheif 1.19.8** | **ok** | fails (expected) |

Rung 1 keeps hitting. Rung 2 never fires from the header on this file, which is normal — the
primary image tile lives well past 192 KB. **The dynamic rows depend on the libheif version
and do not generalise — see §4.**

### 3.3 `prewarmHEIC()`'s compile trick — STILL WORKS

`prewarmHEIC()` decodes a 4-byte throwaway buffer purely for its side effect: forcing the
one-time WASM module compile off the first user-visible thumbnail. That depends on an
*implementation detail* — that the compile happens at the top of the call, before any pixel
work — and v0.4.0 rewrote exactly that code path.

It survives. Timing the prewarm call in a **fresh process**, nothing else having touched the
decoder:

| | prewarm call (4 bytes) | real decode afterwards |
|---|---|---|
| v0.1.0 | 633 ms | 48 ms |
| **v0.4.0** | **225 ms** | 48 ms |

> Methodology warning, learned the hard way: run this probe **alone in its own process**. Any
> earlier decode in the same test binary compiles the module first, and the prewarm call then
> returns in ~60 µs, which reads as "the trick is broken" when it is only "already warm". That
> false negative happened while writing this plan.

### 3.4 `heic.Decoder` — NOT USED HERE, AND MUST NOT BE INTRODUCED

v0.4.0 **deprecates** `heic.Decoder` (`// Deprecated: use Decode and DecodeThumbnail
directly`) because the package now pools WASM modules internally. It still exists, so nothing
breaks.

This repo never used it — `heic_thumbnail.go` calls the package-level functions only. Keep it
that way. `dedup-photos` had hand-rolled per-worker `Decoder` reuse; removing it was worth
**−123 lines** for no measurable throughput change (944 ms vs 938 ms over 96 concurrent
decodes). Do not add here what that repo just deleted.

---

## 4. Bug A — `heicDynamicVersionAtLeast` never probes anything on Linux

### The bug

```go
//go:build linux

// heicDynamicVersionAtLeast returns true on Linux.
// The geo-photo-tagger CI build does not install libheif, so we cannot probe
// the system library at compile time. We return true unconditionally [...]
func heicDynamicVersionAtLeast(major, minor int) bool { return true }
```

The comment's premise is wrong. `dedup-photos` probes the system libheif at **runtime** via
purego `dlopen`, in a 28-line file, with no build-time dependency on libheif at all. CI never
installing libheif is not a reason to skip the check — the probe simply returns `true` when
the library cannot be opened.

Because this always answers "≥ 1.18", `initHEIC()` hands decoding to **whatever libheif is
installed**, however old.

### Why that matters — the version trap

`dedup-photos` §4 asserted that when dynamic libheif is in use, the 192 KB fast path *always*
fails. Acting on that, its `initHEIC()` was changed to force WASM unconditionally. **That was
wrong**, and cost ~4× on the hot path until it was reverted. The truth is version-specific:

| libheif | truncated 192 KB buffer | thumbnail decode |
|---|---|---|
| 1.17.6 (Ubuntu 24.04 stock) | **rejected** — 0/8 on dedup-photos' corpus | — |
| 1.19.8 (strukturag PPA) | accepted — 8/8 | **~3× faster than WASM** |

Both numbers are real; neither generalises. So on a stock Ubuntu 24.04 box (libheif 1.17.6),
this app currently:

1. sends every HEIC thumbnail through a full file read — the 192 KB fast path this file is
   built around never fires; and
2. decodes HDR / `tmap`-brand HEICs (iPhone 12+, i.e. most of the target corpus) with a
   version documented to get them wrong.

Both are silent. This developer box has 1.19.8 and shows neither.

### The fix

Port `dedup-photos`'s runtime probe verbatim. Replace `heic_version_linux.go` with:

```go
//go:build linux

package main

import "github.com/ebitengine/purego"

// heicDynamicVersionAtLeast reports whether the dynamically loaded libheif is at
// least major.minor. If the library cannot be opened, returns true — the safe
// default, since initHEIC() only consults this when heic.Dynamic() already
// reported a usable library.
func heicDynamicVersionAtLeast(major, minor int) bool {
	handle, err := purego.Dlopen("libheif.so", purego.RTLD_NOW|purego.RTLD_GLOBAL)
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
```

`purego` is already in the module graph (indirect); this promotes it to direct.

**Acceptance:** on this box (libheif 1.19.8) startup logs no `< 1.18` line and the dynamic
backend is used. Verify the other side by temporarily returning `false` from the probe and
confirming the `[heic] Dynamic libheif < 1.18` line appears and thumbnails still render.

## 5. Bug B — `prewarmHEIC()` is skipped on exactly the platforms that need it

### The bug

```go
func initHEIC() {
	if heic.Dynamic() != nil {
		// A dynamic libheif is available; no WASM compile to pre-warm.
		return
	}
	...
	prewarmHEIC()
}
```

`heic.Dynamic()` returns *the error from opening the shared library* — verified in the library
source (`heic.go:131`, `func Dynamic() error { return dynamicErr }`). So `!= nil` means the
dynamic library **failed to load**, i.e. it is **not** available. The comment states the exact
opposite, and the control flow follows the comment rather than the contract.

The consequence: when no dynamic libheif exists, every decode goes through WASM — and that is
precisely the branch that returns early **without pre-warming**. The first user-visible HEIC
thumbnail then pays the full module compile: **225 ms on v0.4.0, 633 ms on v0.1.0.**

That branch is the normal case on **Windows**, which CLAUDE.md names as the primary target
platform. The prewarm work from `0f0a514` ("real pre-warm") is dead there.

> Not runtime-verified on Windows — this box has a dynamic libheif, so it takes the other
> branch (confirmed: `heic.Dynamic() == nil` here). The finding is from the library's
> documented contract plus control flow, both quoted above. Confirm on Windows when convenient.

### The fix

```go
func initHEIC() {
	if heic.Dynamic() != nil {
		// No dynamic libheif — the normal case on Windows. Every decode goes
		// through WASM, so the one-time module compile must be pre-warmed.
		prewarmHEIC()
		return
	}
	// A dynamic libheif is present. Below 1.18 it both rejects the truncated
	// 192 KB buffer the fast path depends on and mis-decodes HDR/tmap files, so
	// force WASM — and pre-warm, because WASM is now the decode path.
	if !heicDynamicVersionAtLeast(1, 18) {
		heic.ForceWasmMode = true
		log.Println("[heic] Dynamic libheif < 1.18; switching to WASM decoder")
		prewarmHEIC()
		return
	}
	// libheif >= 1.18: use it. Measured ~3x faster than WASM on the fast path,
	// and it handles the truncated buffer. No WASM module will be compiled, so
	// there is nothing to pre-warm.
	log.Println("[heic] Using dynamic libheif >= 1.18")
}
```

Every branch now ends in a state where the decode path that will actually run has been warmed
if, and only if, it is WASM.

**Acceptance:** on Windows, the first HEIC thumbnail after launch appears without a visible
stall. Instrument by logging the duration of the first `generateHEICThumbnail` call if the
difference is not obvious by eye.

## 6. Bug C — the macOS build does not compile

```
CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...
# github.com/Rosca75/geo-photo-tagger
./heic_thumbnail.go:46:6: undefined: heicDynamicVersionAtLeast
```

`heic_version_linux.go` is `//go:build linux`; `heic_version_other.go` is
`//go:build !linux && !darwin`. Nothing provides the symbol on darwin. CI builds only
`windows-latest` and `ubuntu-latest`, so nothing catches it.

### The fix

Add `heic_version_darwin.go`, ported from `dedup-photos` (it also probes Homebrew's path):

```go
//go:build darwin

package main

import "github.com/ebitengine/purego"

// heicDynamicVersionAtLeast reports whether the dynamically loaded libheif is at
// least major.minor. Returns true if the library cannot be opened.
func heicDynamicVersionAtLeast(major, minor int) bool {
	var handle uintptr
	var err error
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
```

Leave `heic_version_other.go`'s `!linux && !darwin` tag as is — it is correct once a darwin
file exists.

**Acceptance:** `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./...` succeeds. Add a
darwin cross-compile step to `ci.yml` so it cannot regress — a cross-compile needs no macOS
runner and costs seconds.

## 7. Correct CLAUDE.md, which contradicts the shipped code

CLAUDE.md still says:

> | HEIC | ❌ | ✅ | **GPS data only — no thumbnail/preview** |
>
> **HEIC limitation:** Go has no pure-Go HEVC decoder […] The UI will show a placeholder icon
> instead of a thumbnail for HEIC reference images.

and rule 14: *"Never attempt to decode HEIC pixels or generate HEIC thumbnails."*

All false since `9e662b3`. `heic_thumbnail.go` decodes HEIC pixels; `thumbnail.go:48` routes
`.heic`/`.heif` to `generateHEICThumbnail`; there is a thumbnail cache and a prewarm path
built around it. A session that obeys rule 14 will refuse to touch working, shipped code — and
CLAUDE.md bills itself as the single source of truth.

Replace with an accurate description: HEIC thumbnails are supported via `Rosca75/heic`
(no CGo), decoded from a 192 KB byte-range fast path with a full-read fallback, backend chosen
by `initHEIC()` per §4/§5, cached by `thumbnail_cache.go`. Update the format table row, the
"HEIC limitation" paragraph, rule 14, and the `thumbnail.go` line in the structure map that
says "(not HEIC)".

`dedup-photos` had the identical bug — its CLAUDE.md claimed HEIC was unsupported and skipped
while HEIC was a first-class scanned format.

## 8. Explicitly NOT doing: switching EXIF to `heic.DecodeExif`

`dedup-photos`'s plan §6 recommended the opposite:

> For contrast: `geo-photo-tagger` *should* adopt `heic.DecodeExif`, because it currently
> pulls a whole extra dependency, `jdeng/goheif`, just for HEIC EXIF.

**Do not do this.** That recommendation was written from outside this repo and does not
survive contact with it. `heic.Exif` exposes no timezone offset:

```go
type Exif struct {
    Orientation int; Width, Height int
    Make, Model, Software string
    DateTime, DateTimeOriginal string   // local wall-clock strings, no offset
    ExposureTime, FNumber float64; ISOSpeed int; FocalLength float64; Flash int
    GPSLatitude, GPSLongitude, GPSAltitude float64
    Copyright, Artist string
}
```

There is no `OffsetTimeOriginal` (EXIF tag 0x9011). This app has an entire file,
`exif_reader_offset.go`, that walks the raw TIFF bytes to recover that tag precisely because
goexif does not expose it, plus `docs/CHANGE-REQUEST-timezone-fix.md` covering the work.

Measured on `samples/IMG_2656.HEIC`, both readers on the same file:

| | GPS | timestamp |
|---|---|---|
| `heic.DecodeExif` | 48.841725, 2.356467 | `"2025:12:30 14:02:05"` — local, no offset |
| `ReadHEICExif` (current) | 48.841725, 2.356467 | `2025-12-30T13:02:05Z` — **UTC** |

The GPS matches exactly. The timestamps differ by the +01:00 offset that `ReadHEICExif`
recovers and applies. Swapping in `DecodeExif` would silently reintroduce the exact timezone
bug this project already fixed, on the HEIC path only, in exchange for dropping one
dependency.

Two secondary reasons pointing the same way:

- `DecodeExif` returns `GPSLatitude`/`GPSLongitude` as bare floats with no "present" flag.
  `EXIFData.HasGPS` would have to be inferred from `0,0`, which is a real coordinate.
- `ReadHEICExif` feeds raw TIFF bytes to the same goexif decoder the JPEG path uses, so both
  formats share one parsing path. `DecodeExif` would fork that.

Revisit only if upstream adds offset support. Until then `jdeng/goheif` earns its place.

## 9. Execution

This repo has no `preview` branch; `main` is the base. Two commits.

### Commit 1 — the dependency bump

```bash
cd /home/oscar/Documents/geo-photo-tagger
git switch -c chore/heic-v0.4.0

go get github.com/Rosca75/heic@v0.4.0
go mod tidy

go build ./... && go vet ./... && go test ./... && gofmt -l .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...

git add go.mod go.sum
git commit -m "Bump Rosca75/heic to v0.4.0 (upstream Rust/WASM backend)"
```

All confirmed green on a copy of this repo at `92fc51b`.

### Commit 2 — the HEIC backend fixes

Apply §4 (rewrite `heic_version_linux.go`), §5 (rewrite `initHEIC`), §6 (add
`heic_version_darwin.go`), §7 (CLAUDE.md), then:

```bash
go build ./... && go vet ./... && go test ./... && gofmt -l .
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...
CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build ./...   # must now succeed — §6

wails dev -tags webkit2_41    # Ubuntu 24.04+ ships webkit2gtk-4.1, not 4.0
```

Point it at a folder of HEIC reference photos and confirm thumbnails render rather than
placeholder icons, and that the first one appears without a visible stall.

**Acceptance for the whole change:** HEIC thumbnails render on Linux and Windows; `go test
./...` passes; the darwin cross-compile succeeds; startup logs the correct one of the two
`[heic]` lines for the installed libheif; no new warnings.

Rollback for commit 1: `go get github.com/Rosca75/heic@v0.1.0 && go mod tidy`. v0.1.0 is
untouched and still tagged.

## 10. What was measured, and how to reproduce

Environment: Ubuntu 24.04, Go 1.26.5, **libheif 1.19.8** (strukturag PPA — *not* the 1.17.6
Ubuntu stock version, which behaves differently; see §4), `samples/IMG_2656.HEIC`.

**`decodeHEICImage` on the 192 KB header path**, best of 5 after warm-up:

| | WASM | dynamic libheif 1.19.8 |
|---|---|---|
| v0.1.0 | 44.6 ms | 13.4 ms |
| **v0.4.0** | **38.8 ms** | **12.3 ms** |

≈13% faster on WASM, ≈8% on dynamic. All decodes succeeded.

**WASM module compile**, timed as the `prewarmHEIC()` 4-byte call, fresh process, isolated:
633 ms (v0.1.0) → 225 ms (v0.4.0).

Indicative, not benchmarks: one machine, one file, wall-clock. To reproduce, copy the repo,
run the two `go get` variants, and time `decodeHEICImage` with `heic.ForceWasmMode` toggled.

`samples/` holds a single HEIC. Enough to prove the fast path decodes and to compare versions,
but **not** enough to characterise a corpus — one file cannot show how often rung 1 wins in
practice. `dedup-photos` earns that visibility with a `[perf] HEIC ladder:` counter printed at
the end of each scan. If HEIC thumbnailing becomes hot here, port that counter rather than
guessing.

## 11. Lessons carried over from the dedup-photos bump

1. **Do not generalise backend behaviour from one library version.** "libheif rejects
   truncated buffers" was measured on 1.17.6 and asserted for all versions. On 1.19.8 it is
   false, and acting on the generalisation cost ~4×. §4.
2. **Do not inherit a recommendation across repos without testing it there.** "Adopt
   `DecodeExif`" was sound reasoning about a dependency and wrong about this app's
   requirements. §8.
3. **Isolate probes that measure one-time costs.** A warm module in the same process turns a
   working prewarm into an apparent regression. §3.3.
4. **Read the contract, not the comment.** Bug B is a comment that states the inverse of what
   `heic.Dynamic()` returns, with control flow written to match the comment. §5.
5. **Check the docs against the code before trusting either.** Both repos' CLAUDE.md files
   claimed HEIC was unsupported while shipping HEIC features. §7.
6. **Verify what a plan says is already done.** dedup-photos' plan listed doc fixes as
   outstanding that had in fact been committed. Re-check state at execution time.
