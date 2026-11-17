#!/usr/bin/env bash
set -euo pipefail

version="${1:?usage: build-macos-app.sh <version> <binary> <output-app>}"
binary="${2:?usage: build-macos-app.sh <version> <binary> <output-app>}"
output="${3:?usage: build-macos-app.sh <version> <binary> <output-app>}"

version="${version#v}"
bundle_version="${version%%-*}"
rm -rf "$output"
mkdir -p "$output/Contents/MacOS" "$output/Contents/Resources"
cp "$binary" "$output/Contents/MacOS/evergate"
chmod 0755 "$output/Contents/MacOS/evergate"

cat > "$output/Contents/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key><string>zh_CN</string>
  <key>CFBundleDisplayName</key><string>EverGate</string>
  <key>CFBundleExecutable</key><string>evergate</string>
  <key>CFBundleIdentifier</key><string>com.evergate.app</string>
  <key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
  <key>CFBundleName</key><string>EverGate</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>${version}</string>
  <key>CFBundleVersion</key><string>${bundle_version}</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
EOF

plutil -lint "$output/Contents/Info.plist"

if [[ -n "${MACOS_SIGN_IDENTITY:-}" ]]; then
  /usr/bin/codesign --force --deep --options runtime --timestamp \
    --sign "$MACOS_SIGN_IDENTITY" "$output"
fi
