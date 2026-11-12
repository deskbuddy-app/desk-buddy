package main

// Music on the Kindle, like the Mac's helper.py: while the Kindle shows the music widget or
// screen (the server says so in each Deck answer), look at what Windows' media controls say is
// playing (Spotify, Apple Music, a browser…) and send it with its cover. Play / pause / skip,
// the volume and where the sound goes come in as Deck taps. Nothing at all otherwise.
// The Windows calls are in music_windows.go and audio_windows.go.

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // covers come as JPEG or PNG, now and then GIF
	"image/jpeg"
	_ "image/png"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// output: somewhere the sound can go (speakers, headphones, a monitor), as the Kindle shows it.
type output struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // headphones, display or speaker (the Kindle's icons)
	Current   bool   `json:"current"`
	Connected bool   `json:"connected"`
}

// musicState: what's playing, in the same shape the Mac sends to /api/music/state.
type musicState struct {
	App     string   `json:"app,omitempty"` // "Spotify", "Music" (Apple Music), or the app's name
	Running bool     `json:"running"`
	Playing bool     `json:"playing"`
	Title   string   `json:"title,omitempty"`
	Artist  string   `json:"artist,omitempty"`
	Album   string   `json:"album,omitempty"`
	Outputs []output `json:"outputs,omitempty"` // only when there's a choice
	Volume  *int     `json:"volume,omitempty"`  // 0–100
}

var errNoMusic = errors.New("no music app open")

// appName turns the name Windows gives a media app (an app id like "Spotify.exe" or
// "AppleInc.AppleMusicWin_nzyj5cx40ttey!App") into what the Kindle shows. Spotify and Apple Music
// get the names the Mac uses, so the Kindle shows their logos.
func appName(id string) string {
	low := strings.ToLower(id)
	for _, k := range []struct{ has, name string }{
		{"spotify", "Spotify"}, {"applemusic", "Music"}, {"zunemusic", "Media Player"},
		{"msedge", "Edge"}, {"chrome", "Chrome"}, {"firefox", "Firefox"}, {"308046b0af4a39cb", "Firefox"},
		{"brave", "Brave"}, {"opera", "Opera"}, {"vlc", "VLC"},
	} {
		if strings.Contains(low, k.has) {
			return k.name
		}
	}
	name := id
	if i := strings.LastIndexAny(name, `\/`); i >= 0 { // a full path to a program
		name = name[i+1:]
	}
	if i := strings.Index(name, "!"); i >= 0 { // a Store app: "Publisher.App_id!Entry"
		name = name[:i]
	}
	if i := strings.Index(name, "_"); i > 0 {
		name = name[:i]
	}
	if strings.HasSuffix(strings.ToLower(name), ".exe") {
		name = name[:len(name)-4]
	}
	if i := strings.LastIndex(name, "."); i >= 0 && i < len(name)-1 {
		name = name[i+1:]
	}
	if r := []rune(name); len(r) > 30 {
		name = string(r[:30])
	}
	if name == "" {
		return "Media"
	}
	return name
}

// sessionInfo: one app Windows lists as playing media.
type sessionInfo struct {
	App     string
	Playing bool
}

// pickSession picks the app to show, per Settings › Music › Music app: Spotify or Apple Music
// only, or on Auto the one playing (the one shown last first, then the one Windows itself
// shows). When nothing plays: the one shown last, else Windows' own pick. -1: none.
func pickSession(list []sessionInfo, current int, pref, last string) int {
	ok := func(i int) bool {
		switch pref {
		case "spotify":
			return list[i].App == "Spotify"
		case "music":
			return list[i].App == "Music"
		}
		return true
	}
	best, score := -1, -1
	for i := range list {
		if !ok(i) {
			continue
		}
		s := 0
		if list[i].Playing {
			s += 4
		}
		if last != "" && list[i].App == last {
			s += 2
		}
		if i == current {
			s++
		}
		if s > score {
			best, score = i, s
		}
	}
	return best
}

// mediaAction splits a Kindle music tap: "playpause", "next", "previous", "volume:up",
// "volume:down", "output:<id>" or "connect:<id>" (paired headphones, from the Mac's list).
func mediaAction(a string) (kind, arg string) {
	switch {
	case a == "playpause" || a == "next" || a == "previous":
		return "control", a
	case a == "volume:up" || a == "volume:down":
		return "volume", strings.TrimPrefix(a, "volume:")
	case strings.HasPrefix(a, "output:") || strings.HasPrefix(a, "connect:"):
		if _, id, _ := strings.Cut(a, ":"); id != "" {
			return "output", id
		}
	}
	return "", ""
}

// stepVolume: the volume after a tap on the Kindle's + or −, in steps of 10 like the Mac.
func stepVolume(cur int, up bool) int {
	if up {
		cur += 10
	} else {
		cur -= 10
	}
	return max(0, min(100, cur))
}

// outputKind: the Kindle's icon for a Windows sound output, from its form factor (what Windows
// knows it is) and its name.
func outputKind(formFactor uint32, name string) string {
	n := strings.ToLower(name)
	switch {
	case formFactor == 3 || formFactor == 5 || formFactor == 6: // headphones, headset, handset
		return "headphones"
	case strings.Contains(n, "headphone") || strings.Contains(n, "airpods") || strings.Contains(n, "buds"):
		return "headphones"
	case formFactor == 9: // a monitor or TV over HDMI / DisplayPort
		return "display"
	}
	return "speaker"
}

// jsSlice cuts s to n UTF-16 units, as the server's String.slice does (so cover keys match).
func jsSlice(s string, n int) string {
	u := utf16.Encode([]rune(s))
	if len(u) <= n {
		return s
	}
	return string(utf16.Decode(u[:n]))
}

// coverKey: which song a cover belongs to; the server builds the same from what's playing.
func coverKey(s musicState) string {
	return jsSlice(jsSlice(s.Title, 120)+"|"+jsSlice(s.Artist, 120)+"|"+jsSlice(s.Album, 120), 200)
}

// shrinkCover makes the small JPEG the Mac sends (longest side 320, quality 70) from the
// cover Windows gives, on white where it's see-through. The server takes up to 60,000
// characters of base64.
func shrinkCover(raw []byte) ([]byte, error) { return shrinkCoverTo(raw, 320, 44000) }

// sharpCover: the same at 640 (Settings › Music › Cover quality: Sharp, from 1.3), under the
// server's new limit (240,000 base64 characters).
func sharpCover(raw []byte) ([]byte, error) { return shrinkCoverTo(raw, 640, 175000) }

func shrinkCoverTo(raw []byte, side, limit int) ([]byte, error) {
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 {
		return nil, errors.New("empty picture")
	}
	flat := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(flat, flat.Rect, &image.Uniform{color.White}, image.Point{}, draw.Src)
	draw.Draw(flat, flat.Rect, src, b.Min, draw.Over)
	tw, th := w, h
	if m := max(w, h); m > side {
		tw, th = max(1, w*side/m), max(1, h*side/m)
	}
	small := flat
	if tw != w || th != h {
		small = boxScale(flat, tw, th)
	}
	for _, q := range []int{70, 55, 40} {
		var out bytes.Buffer
		if err := jpeg.Encode(&out, small, &jpeg.Options{Quality: q}); err != nil {
			return nil, err
		}
		if out.Len() <= limit {
			return out.Bytes(), nil
		}
	}
	return nil, errors.New("cover too big")
}

// boxScale shrinks a picture by averaging the pixels each new one covers.
func boxScale(src *image.RGBA, tw, th int) *image.RGBA {
	w, h := src.Rect.Dx(), src.Rect.Dy()
	dst := image.NewRGBA(image.Rect(0, 0, tw, th))
	for y := 0; y < th; y++ {
		y0, y1 := y*h/th, max((y+1)*h/th, y*h/th+1)
		for x := 0; x < tw; x++ {
			x0, x1 := x*w/tw, max((x+1)*w/tw, x*w/tw+1)
			var r, g, bl, n int
			for sy := y0; sy < y1; sy++ {
				i := sy*src.Stride + x0*4
				for sx := x0; sx < x1; sx++ {
					r, g, bl, n = r+int(src.Pix[i]), g+int(src.Pix[i+1]), bl+int(src.Pix[i+2]), n+1
					i += 4
				}
			}
			j := y*dst.Stride + x*4
			dst.Pix[j], dst.Pix[j+1], dst.Pix[j+2], dst.Pix[j+3] = uint8(r/n), uint8(g/n), uint8(bl/n), 255
		}
	}
	return dst
}

// ---- the watching (only while the Kindle wants it) ----

var (
	musicMu     sync.Mutex
	musicWanted bool     // the Kindle is showing the music widget or screen
	coversOn    = true   // Settings › Music › Album covers
	coversSharp = true   // Settings › Music › Cover quality: Sharp
	musicPref   = "auto" // Settings › Music › Music app: auto, spotify or music
	musicWake   = make(chan struct{}, 1)
)

// musicFromDeck takes what each Deck answer says about music.
func musicFromDeck(it deckItem) {
	musicMu.Lock()
	was := musicWanted
	musicWanted = it.MusicWanted && connected() && !current().Paused
	covers := it.AlbumCovers == nil || *it.AlbumCovers
	if covers && !coversOn {
		coverSent = coverInfo{} // covers back on: the server deleted the last one, so send it again
	}
	coversOn = covers
	sharp := it.CoverSharp == nil || *it.CoverSharp
	if sharp != coversSharp {
		coverSent = coverInfo{} // quality changed: send the current cover again
	}
	coversSharp = sharp
	musicPref = it.MusicApp
	if musicPref == "" {
		musicPref = "auto"
	}
	now := musicWanted && !was
	musicMu.Unlock()
	if now {
		nudgeMusic()
	}
}

// musicOff: offline or paused, so stop looking.
func musicOff() {
	musicMu.Lock()
	musicWanted = false
	musicMu.Unlock()
}

func nudgeMusic() {
	select {
	case musicWake <- struct{}{}:
	default:
	}
}

// watchMusic: while the Kindle shows music, reads Windows' media controls every 3 seconds
// (a few quick calls; it only sends when something changed, or every minute). Otherwise it
// sleeps until the Kindle opens music again.
func watchMusic() {
	for {
		musicMu.Lock()
		on := musicWanted
		musicMu.Unlock()
		wait := time.Minute
		if on {
			musicTick()
			wait = 3 * time.Second
		}
		select {
		case <-musicWake:
		case <-time.After(wait):
		}
	}
}

type coverInfo struct {
	key, album string
	hash       [20]byte
}

var (
	tickMu     sync.Mutex // one read at a time
	lastMusic  musicState
	lastSig    string
	sentAt     time.Time
	coverSent  coverInfo // the last cover sent (under musicMu)
	coverTries = map[string]int{}
)

// musicTick reads what's playing; sends the cover (new song), then the song, when it changed.
func musicTick() {
	tickMu.Lock()
	defer tickMu.Unlock()
	musicMu.Lock()
	pref := musicPref
	musicMu.Unlock()
	st, err := nowPlaying(pref, lastMusic.App)
	if err != nil {
		report("music: " + err.Error())
		return
	}
	if st.Title != "" { // like the Mac: the volume and outputs go along with a song
		if outs := soundOutputs(); len(outs) > 1 {
			st.Outputs = outs
		}
		if v, ok := soundVolume(); ok {
			st.Volume = &v
		}
	} else {
		musicMu.Lock()
		coverSent = coverInfo{} // the server drops the cover now; send it again if the song comes back
		musicMu.Unlock()
	}
	coverForSong(st) // before the song, so they arrive together
	b, _ := json.Marshal(st)
	if sig := string(b); sig != lastSig || time.Since(sentAt) > time.Minute {
		body := map[string]any{}
		json.Unmarshal(b, &body)
		body["machine"] = "pc"
		if err := call(quick, "POST", "/api/music/state", body, nil); err == nil {
			lastSig, sentAt = sig, time.Now()
		}
	}
	lastMusic = st
}

// coverForSong sends the playing song's cover once (retried a few times when it isn't there
// yet, or Windows still has the previous song's).
func coverForSong(st musicState) {
	key := coverKey(st)
	musicMu.Lock()
	skip := st.Title == "" || !coversOn || coverSent.key == key
	prev := coverSent
	musicMu.Unlock()
	if skip {
		return
	}
	coverTries[key]++
	for k := range coverTries { // only the current song's count matters
		if k != key {
			delete(coverTries, k)
		}
	}
	if coverTries[key] > 5 {
		return
	}
	again := func() { time.AfterFunc(2500*time.Millisecond, nudgeMusic) }
	raw, err := coverArt()
	if err != nil || len(raw) == 0 {
		again()
		return
	}
	hash := sha1.Sum(raw)
	if hash == prev.hash && st.Album != prev.album { // still the last song's picture
		again()
		return
	}
	musicMu.Lock()
	sharp := coversSharp
	musicMu.Unlock()
	sent := false
	tries := []func([]byte) ([]byte, error){shrinkCover}
	if sharp {
		tries = append([]func([]byte) ([]byte, error){sharpCover}, tries...)
	}
	for _, shrink := range tries {
		small, err := shrink(raw)
		if err != nil {
			report("cover: " + err.Error())
			continue
		}
		img := base64.StdEncoding.EncodeToString(small)
		if call(quick, "POST", "/api/music/art", map[string]string{"key": key, "image": img}, nil) == nil {
			sent = true // an older server refuses the sharp one: the standard one is next
			break
		}
	}
	if !sent {
		again()
		return
	}
	musicMu.Lock()
	coverSent = coverInfo{key: key, album: st.Album, hash: hash}
	musicMu.Unlock()
}

// playMusic does a tap from the Kindle's music screen. Returns an error message, or "".
func playMusic(action string) string {
	kind, arg := mediaAction(action)
	var err error
	msg := ""
	switch kind {
	case "control":
		musicMu.Lock()
		pref := musicPref
		musicMu.Unlock()
		tickMu.Lock()
		last := lastMusic.App
		tickMu.Unlock()
		if err = mediaControl(arg, pref, last); err == errNoMusic {
			msg = err.Error()
		} else if err != nil {
			msg = "couldn't control the music"
		}
	case "volume":
		if err = changeVolume(arg == "up"); err != nil {
			msg = "couldn't change the volume"
		}
	case "output":
		if err = setOutput(arg); err != nil {
			msg = "couldn't switch the sound there"
		}
	default:
		return "unknown music action"
	}
	if err != nil && err != errNoMusic {
		report("music " + action + ": " + err.Error())
	}
	go afterMedia()
	return msg
}

// afterMedia: right after a tap, the new song and cover quickly (apps take a moment to
// change track, so look a few times).
func afterMedia() {
	for _, d := range []time.Duration{600 * time.Millisecond, 900 * time.Millisecond, 1500 * time.Millisecond} {
		time.Sleep(d)
		musicTick()
	}
}
