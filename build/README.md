# build/ — Wails packaging assets

This directory holds platform-specific icons and manifests used by
`wails build`. The source glyph for the app icon is tracked here as
`appicon.svg`; the binary forms (`appicon.png`, `windows/icon.ico`) are
committed once the user has rasterized them on a machine with image tools.

## Replacing the default Wails "W" icon

Wails looks up icon files by well-known paths in `build/`. You need two
binary assets:

| File | Used for | Required size |
|------|----------|---------------|
| `build/appicon.png` | Linux build + default Wails icon source | 1024 × 1024, square, transparent background |
| `build/windows/icon.ico` | Windows `.exe` icon, taskbar, window title | multi-resolution: 16, 24, 32, 48, 64, 256 |

### Step 1 — Rasterize `appicon.svg` to PNG

The committed glyph (`build/appicon.svg`) is a map-pin with an inner
camera-lens motif in the CLAUDE.md §9 palette. Convert it to PNG:

- Web: <https://cloudconvert.com/svg-to-png> (set width/height to 1024)
- Local (ImageMagick): `convert -background none -resize 1024x1024 build/appicon.svg build/appicon.png`

Save the result as **`build/appicon.png`**.

### Step 2 — Build a multi-resolution `.ico`

- Web: <https://icoconvert.com> — upload `appicon.png`, select sizes
  16 / 24 / 32 / 48 / 64 / 256, download as `icon.ico`.
- Local (ImageMagick): `convert appicon.png -define icon:auto-resize=256,64,48,32,24,16 build/windows/icon.ico`

Save the result as **`build/windows/icon.ico`**.

### Step 3 — Build

```bash
wails build -platform windows/amd64              # Windows
wails build -tags webkit2_41 -platform linux/amd64  # Linux (CLAUDE.md §14)
```

Wails bundles `appicon.png` and `windows/icon.ico` automatically — no
`wails.json` changes needed.

## Files in this directory

- `appicon.svg` — source glyph (tracked). Edit this when the design
  changes; rasterize again afterwards.
- `appicon.png` — **not tracked yet**. Generate from `appicon.svg` per
  Step 1 and commit.
- `windows/icon.ico` — **not tracked yet**. Generate per Step 2 and commit.
- `windows/info.json` — Wails Windows metadata (tracked).
- `windows/wails.exe.manifest` — Windows application manifest with
  DPI-awareness + visual styles (tracked). Regenerate via
  `wails generate` on a Windows machine if upstream defaults change; see
  <https://wails.io/docs/reference/options#windows>.
