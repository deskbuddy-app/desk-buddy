package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestAppName(t *testing.T) {
	for id, want := range map[string]string{
		"Spotify.exe": "Spotify",
		"SpotifyAB.SpotifyMusic_zpdnekdrzrea0!Spotify": "Spotify",
		"AppleInc.AppleMusicWin_nzyj5cx40ttey!App":     "Music",
		"Chrome":           "Chrome",
		"MSEdge":           "Edge",
		"308046B0AF4A39CB": "Firefox",
		"Microsoft.ZuneMusic_8wekyb3d8bbwe!Microsoft.ZuneMusic": "Media Player",
		`C:\Program Files\Foobar\foobar2000.exe`:                "foobar2000",
		"Contoso.Player_abc123!App":                             "Player",
		"Tidal.exe":                                             "Tidal",
		"":                                                      "Media",
	} {
		if got := appName(id); got != want {
			t.Errorf("appName(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestPickSession(t *testing.T) {
	sp, am, chrome := sessionInfo{App: "Spotify"}, sessionInfo{App: "Music"}, sessionInfo{App: "Chrome"}
	playing := func(s sessionInfo) sessionInfo { s.Playing = true; return s }
	cases := []struct {
		name       string
		list       []sessionInfo
		cur        int
		pref, last string
		want       int
	}{
		{"nothing open", nil, -1, "auto", "", -1},
		{"the one playing", []sessionInfo{sp, playing(chrome)}, 0, "auto", "", 1},
		{"nothing playing: Windows' pick", []sessionInfo{sp, chrome}, 1, "auto", "", 1},
		{"nothing playing: the one shown last", []sessionInfo{sp, chrome}, 1, "auto", "Spotify", 0},
		{"both playing: the one shown last", []sessionInfo{playing(sp), playing(chrome)}, 1, "auto", "Spotify", 0},
		{"Spotify only", []sessionInfo{playing(chrome), sp}, 0, "spotify", "", 1},
		{"Spotify only, not open", []sessionInfo{playing(chrome)}, 0, "spotify", "", -1},
		{"Apple Music only", []sessionInfo{playing(sp), am}, 0, "music", "Spotify", 1},
	}
	for _, c := range cases {
		if got := pickSession(c.list, c.cur, c.pref, c.last); got != c.want {
			t.Errorf("%s: got %d, want %d", c.name, got, c.want)
		}
	}
}

func TestMediaAction(t *testing.T) {
	for a, want := range map[string][2]string{
		"playpause":   {"control", "playpause"},
		"next":        {"control", "next"},
		"previous":    {"control", "previous"},
		"volume:up":   {"volume", "up"},
		"volume:down": {"volume", "down"},
		"output:{0.0.0.00000000}.{6b8c5e7e-1b2a-4f5e-9d3c-1234567890ab}": {"output", "{0.0.0.00000000}.{6b8c5e7e-1b2a-4f5e-9d3c-1234567890ab}"},
		"connect:bt:00-11-22": {"output", "bt:00-11-22"},
		"output:":             {"", ""},
		"volume:loud":         {"", ""},
		"shutdown":            {"", ""},
	} {
		if k, arg := mediaAction(a); k != want[0] || arg != want[1] {
			t.Errorf("mediaAction(%q) = %q %q, want %q %q", a, k, arg, want[0], want[1])
		}
	}
	if stepVolume(95, true) != 100 || stepVolume(5, false) != 0 || stepVolume(40, true) != 50 || stepVolume(40, false) != 30 {
		t.Error("volume steps of 10, between 0 and 100")
	}
	if outputKind(3, "Headphones (WH-1000XM4)") != "headphones" || outputKind(9, "LG TV (NVIDIA High Definition Audio)") != "display" ||
		outputKind(1, "Speakers (Realtek(R) Audio)") != "speaker" || outputKind(1, "AirPods Pro") != "headphones" {
		t.Error("output kinds")
	}
}

func TestCoverKey(t *testing.T) {
	s := musicState{Title: "Song", Artist: "Artist", Album: "Album"}
	if coverKey(s) != "Song|Artist|Album" {
		t.Fatal(coverKey(s))
	}
	long := strings.Repeat("a", 150)
	if k := coverKey(musicState{Title: long, Artist: long}); len(k) != 200 || !strings.HasPrefix(k, strings.Repeat("a", 120)+"|") {
		t.Fatalf("long key: %d %q", len(k), k[:130])
	}
	// the server counts an emoji as 2 (JavaScript strings)
	if k := jsSlice(strings.Repeat("🎵", 70), 120); len([]rune(k)) != 60 {
		t.Fatalf("emoji: %d", len([]rune(k)))
	}
}

func encodePNG(img image.Image) []byte {
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func TestShrinkCover(t *testing.T) {
	// a big, see-through cover: 320 on the longest side, white where it was see-through
	big := image.NewNRGBA(image.Rect(0, 0, 640, 480))
	for y := 0; y < 480; y++ {
		for x := 0; x < 640; x++ {
			if x < 320 {
				big.Set(x, y, color.NRGBA{200, 30, 30, 255})
			}
		}
	}
	out, err := shrinkCover(encodePNG(big))
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal("not a JPEG:", err)
	}
	if b := img.Bounds(); b.Dx() != 320 || b.Dy() != 240 {
		t.Fatalf("size %v", b)
	}
	if r, g, b, _ := img.At(300, 120).RGBA(); r>>8 < 240 || g>>8 < 240 || b>>8 < 240 {
		t.Fatalf("see-through part should be white, got %d %d %d", r>>8, g>>8, b>>8)
	}
	if r, _, _, _ := img.At(20, 120).RGBA(); r>>8 < 170 {
		t.Fatalf("red part lost: %d", r>>8)
	}
	// a noisy full-size photo still fits what the server takes (60,000 base64 characters)
	noisy := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	for i := range noisy.Pix {
		noisy.Pix[i] = uint8(i*7919 + i/13)
	}
	if out, err := shrinkCover(encodePNG(noisy)); err != nil || len(out)*4/3 > 60000 {
		t.Fatalf("noisy cover: %v, %d bytes", err, len(out))
	}
	if out, err := sharpCover(encodePNG(noisy)); err != nil || len(out)*4/3 > 240000 {
		t.Fatalf("noisy sharp cover: %v, %d bytes", err, len(out))
	} else if img, _ := jpeg.Decode(bytes.NewReader(out)); img.Bounds().Dx() > 640 {
		t.Fatal("sharp cover bigger than 640")
	}
	// a small one isn't made bigger
	small := image.NewRGBA(image.Rect(0, 0, 100, 100))
	if out, err := shrinkCover(encodePNG(small)); err != nil {
		t.Fatal(err)
	} else if img, _ := jpeg.Decode(bytes.NewReader(out)); img.Bounds().Dx() != 100 {
		t.Fatal("small cover resized")
	}
	if _, err := shrinkCover([]byte("not a picture")); err == nil {
		t.Fatal("garbage should fail")
	}
}

func TestMusicFromDeck(t *testing.T) {
	os.Setenv("APPDATA", t.TempDir())
	saveConfig(Config{Mode: "remote", Server: "http://127.0.0.1:1", Key: "test"})
	defer saveConfig(Config{})
	off := false
	musicFromDeck(deckItem{MusicWanted: true, AlbumCovers: &off, MusicApp: "spotify"})
	if !musicWanted || coversOn || musicPref != "spotify" {
		t.Fatal("settings not taken", musicWanted, coversOn, musicPref)
	}
	select {
	case <-musicWake:
	default:
		t.Fatal("turning on should wake the watcher")
	}
	coverSent = coverInfo{key: "a|b|c"}
	musicFromDeck(deckItem{MusicWanted: false}) // covers back on (missing means on), app back to Auto
	if musicWanted || !coversOn || musicPref != "auto" || coverSent.key != "" {
		t.Fatal("covers back on should resend the cover", musicWanted, coversOn, musicPref, coverSent.key)
	}
	saveConfig(Config{Mode: "remote", Server: "http://127.0.0.1:1", Key: "test", Paused: true})
	musicFromDeck(deckItem{MusicWanted: true})
	if musicWanted {
		t.Fatal("paused: no music")
	}
}

// What's playing goes to /api/music/state as the PC, only when it changed.
func TestMusicTick(t *testing.T) {
	var mu sync.Mutex
	var posts []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		mu.Lock()
		if r.URL.Path == "/api/music/state" {
			posts = append(posts, b)
		}
		mu.Unlock()
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	os.Setenv("APPDATA", t.TempDir())
	saveConfig(Config{Mode: "remote", Server: srv.URL, Key: "test"})
	defer saveConfig(Config{})
	lastSig = ""
	musicTick()
	musicTick()
	mu.Lock()
	defer mu.Unlock()
	if len(posts) != 1 {
		t.Fatalf("want 1 post, got %d", len(posts))
	}
	if posts[0]["machine"] != "pc" || posts[0]["running"] != false {
		t.Fatalf("body %v", posts[0])
	}
	if playMusic("rm -rf") != "unknown music action" || playMusic("next") != "no music app open" {
		t.Fatal("music taps")
	}
}
