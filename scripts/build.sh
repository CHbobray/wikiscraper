#!/usr/bin/env bash
# Builds release executables for Windows, macOS and Linux into dist/.
#
#   dist/windows/wikiscraper.exe      double-click or run from a terminal
#   dist/macos/WikiScraper.app        double-click; opens Terminal and runs the crawl
#                                     (Contents/MacOS/wikiscraper-<arch> are the
#                                     command-line binaries for Apple silicon / Intel)
#   dist/linux/wikiscraper
#
# Go cross-compiles without extra tools, so this runs on any OS with Go and bash.
set -euo pipefail

cd "$(dirname "$0")/.."
DIST=dist
LDFLAGS="-s -w" # strip debug symbols for smaller binaries

rm -rf "$DIST"
mkdir -p "$DIST/windows" "$DIST/linux" "$DIST/macos"

build() { # build <goos> <goarch> <output>
  echo "building $3"
  CGO_ENABLED=0 GOOS="$1" GOARCH="$2" go build -trimpath -ldflags "$LDFLAGS" -o "$3" ./cmd/wikiscraper
}

build windows amd64 "$DIST/windows/wikiscraper.exe"
build linux   amd64 "$DIST/linux/wikiscraper"

# --- macOS application bundle -------------------------------------------------
# The scraper is a command-line program, so the .app is a thin launcher that
# opens Terminal, runs the binary for this Mac's CPU, and leaves the results in
# ~/Documents/WikiScraper.
APP="$DIST/macos/WikiScraper.app"
mkdir -p "$APP/Contents/MacOS"
build darwin arm64 "$APP/Contents/MacOS/wikiscraper-arm64"
build darwin amd64 "$APP/Contents/MacOS/wikiscraper-x86_64"

cat > "$APP/Contents/MacOS/WikiScraper" <<'LAUNCHER'
#!/bin/bash
APP_DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY="$APP_DIR/wikiscraper-$(uname -m)"
OUTPUT_DIR="$HOME/Documents/WikiScraper"
mkdir -p "$OUTPUT_DIR"
COMMAND="cd '$OUTPUT_DIR' && '$BINARY' -log wikiscraper.log; echo; echo 'Results are in $OUTPUT_DIR'; open '$OUTPUT_DIR'"
osascript -e "tell application \"Terminal\" to do script \"$COMMAND\"" -e 'tell application "Terminal" to activate'
LAUNCHER
chmod +x "$APP/Contents/MacOS/"*

cat > "$APP/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>WikiScraper</string>
  <key>CFBundleDisplayName</key><string>WikiScraper</string>
  <key>CFBundleIdentifier</key><string>edu.example.wikiscraper</string>
  <key>CFBundleVersion</key><string>1.0</string>
  <key>CFBundleShortVersionString</key><string>1.0</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleExecutable</key><string>WikiScraper</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
</dict>
</plist>
PLIST

echo "done:"
find "$DIST" -type f | sort
