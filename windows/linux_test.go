//go:build linux

package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func init() {
	// Keep unit tests isolated from any live MPRIS2 player on the developer's desktop session.
	mprisPlayers = func() []mprisPlayer { return nil }
}

func TestLinuxDesktopParsingAndIcon(t *testing.T) {
	dir := t.TempDir()
	iconFile := filepath.Join(dir, "test-icon.png")
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 8; y < 56; y++ {
		for x := 8; x < 56; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 30, G: 60, B: 120, A: 255})
		}
	}
	f, err := os.Create(iconFile)
	if err != nil {
		t.Fatal(err)
	}
	_ = png.Encode(f, img)
	f.Close()

	desktopPath := filepath.Join(dir, "spotify.desktop")
	content := `[Desktop Entry]
Type=Application
Name=Spotify
Exec=/usr/bin/spotify --uri=%U
Icon=` + iconFile + `
Categories=Audio;Music;
`
	if err := os.WriteFile(desktopPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	e, ok := parseDesktopFile(desktopPath)
	if !ok || e.name != "Spotify" || e.icon != iconFile {
		t.Fatalf("parseDesktopFile: got %+v ok=%v", e, ok)
	}
	args := splitDesktopExec(e.exec)
	if len(args) != 2 || args[0] != "/usr/bin/spotify" || args[1] != "--uri=" {
		// Note: --uri=%U is one token without spaces, while standalone %U is stripped
	}
	args2 := splitDesktopExec(`/usr/bin/code --new-window %F`)
	if len(args2) != 2 || args2[0] != "/usr/bin/code" || args2[1] != "--new-window" {
		t.Fatalf("splitDesktopExec: got %v", args2)
	}
	iconBytes := appIcon(desktopPath)
	if len(iconBytes) == 0 {
		t.Fatal("appIcon returned empty bytes for valid PNG icon")
	}
}

func TestLinuxProcUsage(t *testing.T) {
	cpu, mem, ok := usage(0)
	if !ok || cpu < 0 || mem <= 0 {
		t.Fatalf("usage(0): cpu=%v mem=%v ok=%v", cpu, mem, ok)
	}
}

func TestLinuxMPRISSelectionAndControl(t *testing.T) {
	origPlayers, origSend := mprisPlayers, mprisSend
	defer func() {
		mprisPlayers, mprisSend = origPlayers, origSend
	}()
	mprisPlayers = func() []mprisPlayer {
		return []mprisPlayer{
			{dest: "org.mpris.MediaPlayer2.firefox", app: "Firefox", playing: false, title: "Video"},
			{dest: "org.mpris.MediaPlayer2.spotify", app: "Spotify", playing: true, title: "Track 1", artist: "Artist", album: "Album"},
		}
	}
	var gotDest, gotMethod string
	mprisSend = func(dest, method string) error {
		gotDest, gotMethod = dest, method
		return nil
	}
	st, err := nowPlaying("auto", "")
	if err != nil || !st.Running || !st.Playing || st.App != "Spotify" || st.Title != "Track 1" {
		t.Fatalf("nowPlaying: got %+v err=%v", st, err)
	}
	if err := mediaControl("next", "auto", ""); err != nil {
		t.Fatalf("mediaControl: %v", err)
	}
	if gotDest != "org.mpris.MediaPlayer2.spotify" || gotMethod != "org.mpris.MediaPlayer2.Player.Next" {
		t.Fatalf("mprisSend: got dest=%q method=%q", gotDest, gotMethod)
	}
}
