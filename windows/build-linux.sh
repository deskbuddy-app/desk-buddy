#!/bin/sh
# Builds Desk Buddy for Linux (x64 or arm64), as a folder and a zip:
#   dist/Desk Buddy for Linux/  desk-buddy, desk-buddy-server (the server, for "On this PC"),
#                               public/ (the Kindle's update files, fonts), cloud-kit/
#   dist/Desk-Buddy-for-Linux.zip
# Needs Go, GTK3 + Ayatana AppIndicator3 development headers (for systray), and `npm install`
# in ../worker (for Bun, which builds the local server).
set -e
cd "$(dirname "$0")"

ARCH="${GOARCH:-$(go env GOARCH 2>/dev/null || uname -m)}"
case "$ARCH" in
  x86_64|amd64) GO_ARCH="amd64"; BUN_TARGET="bun-linux-x64" ;;
  aarch64|arm64) GO_ARCH="arm64"; BUN_TARGET="bun-linux-arm64" ;;
  *) GO_ARCH="amd64"; BUN_TARGET="bun-linux-x64" ;;
esac

go vet .
OUT="dist/Desk Buddy for Linux"
rm -rf dist && mkdir -p "$OUT"

# Build the Kindle app (32-bit ARM) and bundle it into ../worker/public/kindle so USB install and Wi-Fi updates have it
(
  cd ../kindle/desk
  ./build.sh >/dev/null
  SHA=$(sha256sum dist/desk/desk | cut -d' ' -f1)
  SIZE=$(wc -c < dist/desk/desk | tr -d ' ')
  RSHA=$(sha256sum dist/desk/run.sh | cut -d' ' -f1)
  PUB=../../worker/public/kindle
  rm -f "$PUB"/desk-*.bin "$PUB"/run-*.sh
  mkdir -p "$PUB"
  cp dist/desk/desk "$PUB/desk-$SHA.bin"
  cp dist/desk/run.sh "$PUB/run-$RSHA.sh"
  python3 - "$SHA" "$SIZE" "$RSHA" > ../../worker/src/release.json <<'PY'
import json, sys
sha, size, rsha = sys.argv[1], int(sys.argv[2]), sys.argv[3]
notes = json.load(open("patchnotes.json"))[:5]
print(json.dumps({"sha256": sha, "size": size, "file": f"/kindle/desk-{sha}.bin",
                  "run_sha256": rsha, "run_file": f"/kindle/run-{rsha}.sh", "notes": notes}))
PY
)

cp ../kindle/desk/kindle-files/launcher.sh kindle-launcher.sh # the Kindle's library item (kindlesetup.go)
GOOS=linux GOARCH="$GO_ARCH" go build -trimpath -o "$OUT/desk-buddy" .

if [ -x "../worker/node_modules/.bin/bun" ]; then
  BUN="../worker/node_modules/.bin/bun"
elif command -v bun >/dev/null 2>&1; then
  BUN="$(command -v bun)"
elif [ -x "$HOME/.bun/bin/bun" ]; then
  BUN="$HOME/.bun/bin/bun"
else
  echo "Installing worker dependencies (for bun)..."
  (cd ../worker && npm install)
  BUN="../worker/node_modules/.bin/bun"
fi

if [ ! -d "../worker/node_modules/ical.js" ]; then
  echo "Installing worker dependencies..."
  (cd ../worker && "$BUN" install --production)
fi

(cd ../worker && "$BUN" build local/server.js --compile --minify --target="$BUN_TARGET" --outfile "../windows/$OUT/desk-buddy-server" >/dev/null)
cp -R ../worker/public "$OUT/public"
mkdir -p "$OUT/cloud-kit"
cp -R ../worker/src ../worker/schema.sql ../worker/setup.js "$OUT/cloud-kit/"
"$BUN" -e 'const p=require("../worker/package.json");delete p.devDependencies.bun;delete p.scripts;console.log(JSON.stringify(p,null,2))' > "$OUT/cloud-kit/package.json"
(cd dist && zip -q -r -X Desk-Buddy-for-Linux.zip "Desk Buddy for Linux")
ls -la dist "$OUT"
