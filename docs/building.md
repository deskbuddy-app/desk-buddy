# Building Desk Buddy yourself

## Layout

| Folder | What | Language |
|---|---|---|
| `kindle/desk/` | The Kindle app: draws the screen with FBInk, reads touch, talks to the server, updates itself | Go (static 32-bit ARM) |
| `worker/` | The server: tasks, calendar, weather, Deck, music, settings, the phone page | JavaScript (Cloudflare Workers, or Bun on your Mac) |
| `menubar/` | Desk Buddy for Mac: the menu bar app and its window | Swift, SwiftUI |
| `windows/` | Desk Buddy for Windows & Linux: tray icon, local UI window, Deck apps, music, volume, USB Kindle setup | Go (`*_windows.go` and `*_linux.go`) |
| `backend/helper.py` | The Mac app's helper: "you're at this Mac", Deck, music, chime | Python (macOS's own) |
| `brand/` | The Desk Buddy character, app and menu bar icons | |

## Needs

- **Go** 1.23+ (Kindle app)
- **Node.js** 20+ with `npm install` in `worker/` (server tools; it also installs Bun, used to build the Mac's server)
- **Xcode command line tools** (`xcode-select --install`) for the Mac app

## Mac app

```sh
cd worker && npm install && cd ..
menubar/install.sh
```

Builds `~/Applications/Desk Buddy.app` (with the server inside), signs it for your Mac and starts it. Re-run it after changes. Without a `backend/config.json` it starts on **This Mac** with a new key.

## Kindle app

```sh
cd kindle/desk
go vet . && GOOS=linux GOARCH=arm GOARM=7 go vet . && go test .
./build.sh            # dist/desk/ and dist/documents/Desk Buddy.sh, to copy onto the Kindle
```

The Kindle is 32-bit: use `int64` for anything with timestamps (`GOARCH=arm go vet` helps catch it).

**Previews** on the Mac (render a screen to a PNG, no Kindle needed):

```sh
go run . -config kindle-files/config.example.json -preview out.png                 # needs a running server in the config
go run . -preview out.png -json sample.json                                        # from saved /api/desk JSON
go run . ... -tab calendar | -dark | -settings -settab server | -standby -clock 1 -theme garden
go run . -preview out-basic-2024.png -panel 1448x1072 -compact                     # 6" Kindle Basic (2022 / 2024) in Compact mode
go run . -preview out-basic-800.png -panel 800x600 -compact                        # older 6" Kindle Basic (800×600) in Compact mode
```

`main.go` lists every preview flag.

## Windows & Linux app

```sh
cd worker && npm install && cd ../windows
./build.sh            # Windows x64: dist/Desk Buddy for Windows/ and dist/Desk-Buddy-for-Windows.zip
./build-linux.sh      # Linux x64/arm64: dist/Desk Buddy for Linux/ and dist/Desk-Buddy-for-Linux.zip
```

On Debian/Ubuntu, building the Linux systray requires `libgtk-3-dev` and `libayatana-appindicator3-dev` (or `libappindicator3-dev`).

## Server

```sh
cd worker
bun local/server.js --data /tmp/db --key dev --port 8787      # the Mac version
npx wrangler dev                                              # the Cloudflare version (needs .dev.vars with KEY=…)
```

## Releasing a Kindle build to your own server

`kindle/desk/release.sh` builds, puts the build in `worker/public/kindle/`, records it in `worker/src/release.json` (with the newest patch notes from `patchnotes.json`) and deploys your Worker. On This Mac, re-run `menubar/install.sh` afterwards: the Mac's server serves the build bundled in the app.
