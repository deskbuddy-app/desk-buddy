package main

// The background work, like the Mac's helper.py: tell the server when this PC is being used,
// wait for Deck taps from the Kindle (open the app, or play / pause / skip the music), keep the
// app list current, and chime when a focus or break ends. Music is in music.go.

import (
	"errors"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func osLabel() string {
	if runtime.GOOS == "linux" {
		return "Linux"
	}
	return "Windows"
}

const inUse = 240 // seconds without input before the PC counts as not in use (the server allows 300)

// heartbeats: says when the PC starts or stops being used, or light / dark changes, and every
// 2 minutes otherwise (the server forgets a computer after 3). Looking is local, every 10 s.
// keyRefused: the last heartbeat was turned away for the key (the window offers "Join again").
var keyRefused atomic.Bool

func heartbeats() {
	var last time.Time
	var wasUsed, wasDark, sent bool
	for {
		c := current()
		if connected() && !c.Paused {
			idle := idleSeconds()
			dark, haveDark := darkMode()
			used := idle < inUse
			if !sent || time.Since(last) >= 2*time.Minute || used != wasUsed || dark != wasDark {
				body := map[string]any{"machine": "pc", "os": runtime.GOOS, "idle": idle}
				if haveDark {
					body["dark"] = dark
				}
				var r struct{ Profile string }
				if err := call(quick, "POST", "/api/heartbeat", body, &r); errors.Is(err, errKeyRefused) {
					setStatus("Desk Buddy on your other computer doesn't know this PC any more. Join it again")
					keyRefused.Store(true)
					last = time.Now().Add(-90 * time.Second)
				} else if err != nil {
					keyRefused.Store(false)
					setStatus("Can't reach the server, retrying")
					last = time.Now().Add(-90 * time.Second) // try again in 30 s
				} else {
					keyRefused.Store(false)
					setStatus(map[string]string{"home": "Running · the Kindle shows the " + osLabel() + " list", "work": "Running · the Kindle shows the Mac list"}[r.Profile])
					last, sent, wasUsed, wasDark = time.Now(), true, used, dark
				}
			}
		} else {
			sent = false
		}
		time.Sleep(10 * time.Second)
	}
}

// ---- the Deck ----

type deckItem struct {
	ID       string    `json:"id"`
	Label    string    `json:"label"`
	Win      string    `json:"win"`
	URL      *string   `json:"url"`
	Media    string    `json:"media"`
	Switch   string    `json:"switch"`
	Pomodoro *pomoInfo `json:"pomodoro"`
	Wake     bool      `json:"wake"`
	// music, in every answer: the Kindle shows it now, and Settings › Music
	MusicWanted bool   `json:"music_wanted"`
	AlbumCovers *bool  `json:"album_covers"`
	MusicApp    string `json:"music_app"`
}

type pomoInfo struct {
	State string  `json:"state"`
	Ends  float64 `json:"ends"`
	Sound bool    `json:"sound"`
	Now   float64 `json:"now"`
}

// deck: one request at a time waits (up to 20 s) at the server for a tap meant for this PC.
func deck() {
	for {
		c := current()
		if !connected() || c.Paused {
			musicOff()
			time.Sleep(3 * time.Second)
			continue
		}
		var it deckItem
		if err := call(long, "GET", "/api/deck/next?machine=pc&wait=20", nil, &it); err != nil {
			musicOff()
			time.Sleep(5 * time.Second) // offline or an old server: don't spin
			continue
		}
		scheduleChime(it.Pomodoro)
		musicFromDeck(it)
		if it.ID == "" || it.Wake || it.Switch != "" {
			continue
		}
		if it.Media != "" { // from the Kindle's music screen (like the Mac, no "done" for these)
			if err := playMusic(it.Media); err != "" {
				logf("music %s: %s", it.Media, err)
			}
			continue
		}
		err := openItem(it)
		setLast(it.Label, err)
		call(quick, "POST", "/api/deck/done", map[string]any{"id": it.ID, "ok": err == "", "error": err}, nil)
	}
}

// Only meeting links from these services open (the Kindle's Join), never anything else.
var meeting = regexp.MustCompile(`(?i)^https://(?:[a-z0-9-]+\.)*(?:zoom\.us|meet\.google\.com|teams\.microsoft\.com|teams\.live\.com|webex\.com|whereby\.com|meet\.jit\.si)(?:/[^\s"'<>()]*)?$`)

// openItem opens a Deck tile: an app from the PC's list (by the tile's win, then its label),
// a bare link like spotify: or a program like chrome.exe (apps.go says which), or a meeting
// link. Never runs commands.
func openItem(it deckItem) string {
	if it.URL != nil {
		if !meeting.MatchString(*it.URL) {
			return "not a meeting link"
		}
		if e := shellOpen(*it.URL); e != nil {
			return "couldn't open it"
		}
		return ""
	}
	appsMu.Lock()
	target := tileTarget(it, apps)
	appsMu.Unlock()
	name := it.Label
	if name == "" {
		name = it.Win
	}
	if target == "" {
		return "no app called " + name + " on this PC"
	}
	if e := launch(target); e != nil {
		logf("open %s: %v", name, e)
		return "couldn't open " + name
	}
	return ""
}

// ---- the apps (Windows' "All apps"), for the Kindle's "All apps" and the tiles' icons ----

var (
	appsMu sync.Mutex
	apps   = map[string]string{} // name → target (apps.go)
)

// loadApps: Windows' "All apps", or the Start menu's shortcuts if Windows won't list them.
func loadApps() map[string]string {
	found, err := listApps()
	if err != nil {
		report("apps: " + err.Error() + ", using the Start menu's shortcuts")
		found = scanShortcuts()
	}
	return found
}

// appList sends the app names and icons when the apps or the PC's favourite tiles change
// (looked at every 10 minutes), and every 6 hours. Like the Mac's IconSync: off the window's
// thread, and an app's icon is made once (kept while the app is there).
func appList() {
	var sent string
	var at time.Time
	made := map[string][]byte{} // target → icon PNG (nil: Windows had none)
	for {
		again := false
		if connected() && !current().Paused {
			found := loadApps()
			appsMu.Lock()
			apps = found
			appsMu.Unlock()
			names := sortedNames(found)
			var deck struct {
				Home []deckItem `json:"home"` // the PC's tiles (the Mac's are "work")
			}
			call(quick, "GET", "/api/deck", nil, &deck)
			tiles := make([]string, len(deck.Home))
			for i, t := range deck.Home {
				tiles[i] = t.Label + "=" + t.Win
			}
			sig := current().Server + "|" + strings.Join(tiles, ",") + "|" + strings.Join(names, "|")
			if sig != sent || time.Since(at) > 6*time.Hour {
				kept := map[string][]byte{}
				icons := collectIcons(deck.Home, found, func(target string) []byte {
					p, ok := kept[target]
					if !ok {
						p, ok = made[target]
					}
					if !ok {
						p = appIcon(target)
						time.Sleep(20 * time.Millisecond) // gently: a few hundred apps the first time
					}
					kept[target] = p
					return p
				})
				made = kept
				var r struct{ Seeded bool } // the server just picked this PC's first favourites
				if err := call(quick, "POST", "/api/deck/icons", map[string]any{"machine": "pc", "os": runtime.GOOS, "icons": icons, "apps": names, "all_apps": true}, &r); err == nil {
					sent, at, again = sig, time.Now(), r.Seeded
				} else {
					report("apps: " + err.Error())
				}
			}
		}
		if !again { // the new tiles' icons go up straight away
			time.Sleep(10 * time.Minute)
		}
	}
}

// ---- the chime at the end of a focus or break (on the server's clock) ----

var (
	chimeMu    sync.Mutex
	chimeEnds  float64
	chimeTimer *time.Timer
)

func scheduleChime(p *pomoInfo) {
	ends := 0.0
	if p != nil && p.Sound && (p.State == "focus" || p.State == "break") {
		ends = p.Ends
	}
	chimeMu.Lock()
	defer chimeMu.Unlock()
	if ends == chimeEnds {
		return
	}
	if chimeTimer != nil {
		chimeTimer.Stop()
		chimeTimer = nil
	}
	chimeEnds = ends
	if ends > 0 {
		delay := ends - p.Now
		if delay > -30 { // just ended and we only now heard: still chime
			chimeTimer = time.AfterFunc(time.Duration(max(0, delay)*float64(time.Second)), chimeSound)
		}
	}
}
