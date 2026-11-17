#!/usr/bin/env bash
set -euo pipefail

version="${1:?usage: build-macos-installer.sh <version> <binary> <output-dir>}"
binary="${2:?usage: build-macos-installer.sh <version> <binary> <output-dir>}"
output_dir="${3:?usage: build-macos-installer.sh <version> <binary> <output-dir>}"
repo="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

version="${version#v}"
bundle_version="${version%%-*}"
asset="evergate_${version}_macos_arm64_setup.dmg"
work="$(mktemp -d)"
cleanup() {
  rm -rf "$work"
  rm -f "$work/EverGate.zip" "$work/EverGateInstaller.zip"
}
trap cleanup EXIT

app="$work/EverGate.app"
installer_app="$work/EverGate 安装器.app"
dmg_root="$work/dmg"

"$repo/scripts/build-macos-app.sh" "$version" "$binary" "$app"

if [[ "${REQUIRE_MACOS_SIGNING:-false}" == "true" ]]; then
  : "${MACOS_SIGN_IDENTITY:?MACOS_SIGN_IDENTITY is required for signed releases}"
  : "${APPLE_ID:?APPLE_ID is required for notarization}"
  : "${APPLE_TEAM_ID:?APPLE_TEAM_ID is required for notarization}"
  : "${APPLE_APP_PASSWORD:?APPLE_APP_PASSWORD is required for notarization}"
  ditto -c -k --keepParent "$app" "$work/EverGate.zip"
  xcrun notarytool submit "$work/EverGate.zip" \
    --apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" --password "$APPLE_APP_PASSWORD" --wait
  xcrun stapler staple "$app"
  /usr/bin/codesign --verify --deep --strict "$app"
  spctl -a -vv "$app"
fi

mkdir -p "$installer_app/Contents/MacOS" "$installer_app/Contents/Resources"
swift_args=(-O "$repo/installer/macos/Installer.swift"
  -o "$installer_app/Contents/MacOS/EverGateInstaller" -framework Cocoa)
if [[ -n "${MACOS_SWIFT_TARGET:-}" ]]; then
  swift_args+=(-target "$MACOS_SWIFT_TARGET")
fi
swiftc "${swift_args[@]}"
cp -R "$app" "$installer_app/Contents/Resources/EverGate.app"
cp "$repo/LICENSE" "$installer_app/Contents/Resources/LICENSE"
cat > "$installer_app/Contents/Info.plist" <<'EOF'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleDevelopmentRegion</key><string>zh_CN</string>
  <key>CFBundleDisplayName</key><string>EverGate 安装器</string>
  <key>CFBundleExecutable</key><string>EverGateInstaller</string>
  <key>CFBundleIdentifier</key><string>com.evergate.installer</string>
  <key>CFBundleInfoDictionaryVersion</key><string>6.0</string>
  <key>CFBundleName</key><string>EverGate 安装器</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>__VERSION__</string>
  <key>CFBundleVersion</key><string>__BUNDLE_VERSION__</string>
  <key>LSMinimumSystemVersion</key><string>11.0</string>
  <key>NSHighResolutionCapable</key><true/>
</dict>
</plist>
EOF
/usr/bin/sed -i '' "s/__VERSION__/${version}/g; s/__BUNDLE_VERSION__/${bundle_version}/g" "$installer_app/Contents/Info.plist"

if [[ -n "${MACOS_SIGN_IDENTITY:-}" ]]; then
  /usr/bin/codesign --force --options runtime --timestamp \
    --sign "$MACOS_SIGN_IDENTITY" "$installer_app"
fi

if [[ "${REQUIRE_MACOS_SIGNING:-false}" == "true" ]]; then
  ditto -c -k --keepParent "$installer_app" "$work/EverGateInstaller.zip"
  xcrun notarytool submit "$work/EverGateInstaller.zip" \
    --apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" --password "$APPLE_APP_PASSWORD" --wait
  xcrun stapler staple "$installer_app"
  /usr/bin/codesign --verify --strict "$installer_app"
  spctl -a -vv "$installer_app"
fi

mkdir -p "$dmg_root"
cp -R "$installer_app" "$dmg_root/"
mkdir -p "$output_dir"
rm -f "$output_dir/$asset"
hdiutil create -quiet -volname "EverGate $version" \
  -srcfolder "$dmg_root" -ov -format UDZO "$output_dir/$asset"

if [[ -n "${MACOS_SIGN_IDENTITY:-}" ]]; then
  /usr/bin/codesign --force --timestamp --sign "$MACOS_SIGN_IDENTITY" "$output_dir/$asset"
fi

if [[ "${REQUIRE_MACOS_SIGNING:-false}" == "true" ]]; then
  xcrun notarytool submit "$output_dir/$asset" \
    --apple-id "$APPLE_ID" --team-id "$APPLE_TEAM_ID" --password "$APPLE_APP_PASSWORD" --wait
  xcrun stapler staple "$output_dir/$asset"
  spctl -a -vv -t open --context context:primary-signature "$output_dir/$asset"
fi

echo "$output_dir/$asset"
