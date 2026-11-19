#!/bin/sh
# Builds the Kindle app and lays out what to copy onto the Kindle's USB drive:
#   dist/desk/                     -> copy to the Kindle's root  (/mnt/us/desk)
#   dist/documents/Desk Buddy.sh  -> copy into the Kindle's documents folder
set -e
cd "$(dirname "$0")"
rm -rf dist && mkdir -p dist/desk dist/documents
CGO_ENABLED=0 GOOS=linux GOARCH=arm GOARM=7 go build -trimpath -ldflags "-s -w -X main.buildVersion=$(date +%Y-%m-%d.%H%M)" -o dist/desk/desk .
cp kindle-files/run.sh dist/desk/
cp fonts/OFL-*.txt dist/desk/
cp ../../LICENSE dist/desk/LICENSE-DeskBuddy.txt
# a new Kindle starts with no server: it finds the computer on the Wi-Fi (Find it → Allow)
if [ -f kindle-files/config.json ]; then cp kindle-files/config.json dist/desk/; else cp kindle-files/config.example.json dist/desk/config.json; fi
cp kindle-files/launcher.sh "dist/documents/Desk Buddy.sh"
ls -la dist/desk dist/documents
