#!/bin/sh
# Makes the downloads for a GitHub release (nothing is installed on this Mac):
#   dist/Desk-Buddy-for-Mac.zip     Desk Buddy.app for Apple Silicon and Intel, with the server
#                                   for both chips, the current Kindle build (so it can put
#                                   Desk Buddy on a Kindle) and no settings or key inside
#   dist/Desk-Buddy-for-Kindle.zip  desk/ + documents/Desk Buddy.sh, to copy by hand
# Needs Xcode's command line tools, Go, and `npm install` in ../worker (for Bun).
# The app is signed ad hoc (no Apple Developer ID): macOS asks people to allow it once.
set -e
cd "$(dirname "$0")"
OUT=dist
APP="$OUT/Desk Buddy.app"
rm -rf "$OUT" && mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
VERSION=$(python3 -c 'import json;print(json.load(open("patchnotes.json"))[0]["version"])')

# the Kindle app (its build goes inside the Mac app, and into the Kindle zip)
(cd ../kindle/desk && ./build.sh > /dev/null)
KD=../kindle/desk/dist
SHA=$(shasum -a 256 "$KD/desk/desk" | cut -d' ' -f1)
RSHA=$(shasum -a 256 "$KD/desk/run.sh" | cut -d' ' -f1)
rm -f ../worker/public/kindle/desk-*.bin ../worker/public/kindle/run-*.sh
cp "$KD/desk/desk" "../worker/public/kindle/desk-$SHA.bin"
cp "$KD/desk/run.sh" "../worker/public/kindle/run-$RSHA.sh"
python3 - "$SHA" "$(wc -c < "$KD/desk/desk" | tr -d ' ')" "$RSHA" > ../worker/src/release.json <<'PY'
import json, sys
sha, size, rsha = sys.argv[1], int(sys.argv[2]), sys.argv[3]
notes = json.load(open("../kindle/desk/patchnotes.json"))[:5]
print(json.dumps({"sha256": sha, "size": size, "file": f"/kindle/desk-{sha}.bin",
                  "run_sha256": rsha, "run_file": f"/kindle/run-{rsha}.sh", "notes": notes}))
PY

# the server, for both chips (it carries release.json, so build it after the Kindle app)
(cd ../worker && npm run build:local > /dev/null)
cp ../worker/local/dist/desk-buddy-server-arm64 ../worker/local/dist/desk-buddy-server-x64 "$APP/Contents/MacOS/"

# the app and audio-out, for both chips, joined into one
SRC="main.swift Backend.swift CalendarSync.swift IconSync.swift Permissions.swift HelperUI.swift Localization.swift Updater.swift KindleSetup.swift MoveToApplications.swift"
for a in arm64 x86_64; do
  swiftc -O -target $a-apple-macos13 -o "$OUT/DeskBuddy-$a" $SRC
  swiftc -O -target $a-apple-macos13 -o "$OUT/audio-out-$a" AudioOut.swift
done
lipo -create "$OUT/DeskBuddy-arm64" "$OUT/DeskBuddy-x86_64" -output "$APP/Contents/MacOS/DeskBuddy"
lipo -create "$OUT/audio-out-arm64" "$OUT/audio-out-x86_64" -output "$APP/Contents/MacOS/audio-out"
rm -f "$OUT"/DeskBuddy-* "$OUT"/audio-out-*

# what install.sh also puts in, minus anyone's settings: config.example.json (starts on "This Mac")
cp ../backend/helper.py AppIcon.icns patchnotes.json "$APP/Contents/Resources/"
cp ../backend/config.example.json "$APP/Contents/Resources/config.json"
cp ../brand/menubar.png ../brand/menubar@2x.png "$APP/Contents/Resources/"
cp ../brand/spotify-mark-alpha.png "$APP/Contents/Resources/spotify-mark.png"
cp ../brand/buddy-kindle.png "$APP/Contents/Resources/buddy.png"
cp ../kindle/desk/fonts/AtkinsonHyperlegible-Regular.ttf ../kindle/desk/fonts/AtkinsonHyperlegible-Bold.ttf "$APP/Contents/Resources/"
cp ../kindle/desk/fonts/SourceHanSansSC-Regular.otf ../kindle/desk/fonts/OFL-*.txt ../LICENSE "$APP/Contents/Resources/"
cp -R ../worker/public "$APP/Contents/Resources/public"
rm -rf "$APP/Contents/Resources/public/mac" "$APP/Contents/Resources/public/windows"
mkdir -p "$APP/Contents/Resources/kindle-setup" && cp ../kindle/desk/kindle-files/launcher.sh "$APP/Contents/Resources/kindle-setup/"
mkdir -p "$APP/Contents/Resources/cloud-kit"
cp -R ../worker/src ../worker/schema.sql ../worker/setup.sh "$APP/Contents/Resources/cloud-kit/"
node -e 'const p=require("../worker/package.json");delete p.devDependencies.bun;delete p.scripts;console.log(JSON.stringify(p,null,2))' > "$APP/Contents/Resources/cloud-kit/package.json"
cat > "$APP/Contents/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>CFBundleIdentifier</key><string>app.deskbuddy.mac</string>
  <key>CFBundleName</key><string>Desk Buddy</string>
  <key>CFBundleExecutable</key><string>DeskBuddy</string>
  <key>CFBundleIconFile</key><string>AppIcon</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
PLIST
sed -n '/NSCalendarsFullAccessUsageDescription/,/LSMinimumSystemVersion/p' install.sh >> "$APP/Contents/Info.plist"
echo '</dict></plist>' >> "$APP/Contents/Info.plist"
plutil -lint "$APP/Contents/Info.plist" > /dev/null
codesign --force --deep --sign - "$APP"

(cd "$OUT" && ditto -c -k --norsrc --noextattr --keepParent "Desk Buddy.app" Desk-Buddy-for-Mac.zip)
K="$OUT/kindle/Desk Buddy for Kindle"
mkdir -p "$K/documents" && cp -R "$KD/desk" "$K/desk"
cp "$KD/documents/"*.sh "$K/documents/Desk Buddy.sh"
(cd "$OUT/kindle" && ditto -c -k --norsrc --noextattr --keepParent "Desk Buddy for Kindle" ../Desk-Buddy-for-Kindle.zip)
rm -rf "$OUT/kindle"
ls -la "$OUT"
