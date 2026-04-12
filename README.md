# GeoPhotoTagger

[![CI](https://github.com/Rosca75/geo-photo-tagger/actions/workflows/ci.yml/badge.svg)](https://github.com/Rosca75/geo-photo-tagger/actions/workflows/ci.yml)
[![Latest Release](https://img.shields.io/github/v/release/Rosca75/geo-photo-tagger)](https://github.com/Rosca75/geo-photo-tagger/releases/latest)

A desktop application to add GPS coordinates to photos that lack geolocation data, by cross-referencing timestamps with geolocated photos from phones/tablets or GPS track files.

## The Problem

Digital cameras (DSLRs, mirrorless) often lack built-in GPS. Photos taken with these cameras have no location data. Meanwhile, phones carried by the photographer or family members are recording geolocated photos at the same time and place.

## The Solution

GeoPhotoTagger matches camera photos to phone photos by timestamp. If a phone photo was taken within minutes of a camera photo, they were likely at the same location. The app copies GPS coordinates from the phone photo to the camera photo.

## Features

* **Option 1 — Reference photos:** Add folders of geolocated photos from phones/tablets. The app matches by timestamp and copies GPS data.
* **Option 2 — GPS tracks:** Import GPX, KML, or CSV track files. The app interpolates coordinates for each camera photo's timestamp.
* Time-based confidence scoring (closer timestamps = higher confidence)
* Configurable time thresholds (10 / 30 / 60 minutes)
* Thumbnail previews for JPG, PNG, DNG, ARW
* HEIC reference photos supported for GPS data (no thumbnail preview)
* Backup creation before any file modification
* Undo support (restore from backup)
* Single self-contained binary — no external dependencies
* Cross-platform (Windows, Linux)

## Supported Formats

**Target photos** (to be geotagged): JPG, DNG

**Reference photos** (GPS source): JPG, PNG, DNG, ARW, HEIC

**GPS tracks**: GPX, KML, CSV

## Quick Start

1. Download the latest binary from the [Releases](https://github.com/Rosca75/geo-photo-tagger/releases/latest) page.
2. Run GeoPhotoTagger.
3. Browse to your camera photos folder (photos without GPS).
4. Add one or more reference folders (phone/tablet photos with GPS) or import a GPS track file.
5. Click **Match All** — the app finds the best GPS match for each photo.
6. Review matches, adjust if needed, then **Apply** to write GPS data.

## Build from Source

Requires Go 1.21+, Wails CLI v2, Node.js 16+.

```bash
# Install Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# Clone and build
git clone https://github.com/Rosca75/geo-photo-tagger.git
cd geo-photo-tagger
wails build -platform windows/amd64
```

For Linux:
```bash
sudo apt-get install -y libwebkit2gtk-4.1-dev libgtk-3-dev
wails build -tags webkit2_41 -platform linux/amd64
```

## How Matching Works

1. **Scan** — target folder is scanned for photos without GPS data.
2. **Reference collection** — reference folders and GPS tracks are indexed by timestamp.
3. **Matching** — for each target photo, the app finds the closest reference by `DateTimeOriginal`.
4. **Scoring** — matches are scored by time proximity (≤1 min = 100, ≤5 min = 90, ≤30 min = 50, etc.).
5. **Review** — the user reviews matches in a side-by-side UI with thumbnails and confidence scores.
6. **Apply** — GPS coordinates are written to accepted matches. Originals are backed up as `.bak` files.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
