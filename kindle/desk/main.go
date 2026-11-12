// desk: the Desk Buddy full-screen app for a jailbroken Kindle Paperwhite 2.
//
// Draws the backend's /api/desk data in landscape, straight to the e-ink panel, and
// turns taps into backend calls. Press the power button (or hold the clock for 3 s)
// to quit; run.sh then hands the screen back to the normal Kindle.
//
// On the Mac, `go run . -preview out.png` draws one screen to a PNG instead.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/image/font"
	"image"
	"image/png"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	Server           string `json:"server"`
	Key              string `json:"key"`
	Rotation         int    `json:"rotation"`           // 90 or 270
	FullRefreshEvery int    `json:"full_refresh_every"` // partial redraws between flashing ones
	Cloud            string `json:"cloud"`              // the cloud address, kept while using a computer on the Wi-Fi
	ServerName       string `json:"server_name"`        // that computer's name
}

func loadConfig(path string) Config {
	c := Config{Rotation: 90, FullRefreshEvery: 40} // no server yet: the Kindle finds one (server.go)
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, &c); err != nil {
			log.Printf("%s: %v (using defaults)", path, err)
		}
	}
	if c.Rotation != 270 {
		c.Rotation = 90
	}
	if c.FullRefreshEvery < 1 {
		c.FullRefreshEvery = 40
	}
	if c.Cloud == "" && !isLocalServer(c.Server) {
		c.Cloud = c.Server
	}
	return c
}

// Saved is the part of the UI remembered between runs (state.json next to config.json).
type Saved struct {
	Dark          bool    `json:"dark"`
	Collapsed     []int   `json:"collapsed"`
	Tab           string  `json:"tab"`
	Clock         int     `json:"clock"` // standby clock style
	Widget        string  `json:"widget"`
	Font          string  `json:"font"`
	TempF         bool    `json:"temp_f"`
	SideHidden    bool    `json:"side_hidden"`
	Clock12       bool    `json:"clock12"`
	NoQuote       bool    `json:"no_quote"`
	NoEvent       bool    `json:"no_event"`
	CalHidden     bool    `json:"cal_hidden"`
	CalTL         bool    `json:"cal_timeline"`
	WeatherOff    bool    `json:"weather_hidden"`
	AutoDark      bool    `json:"auto_dark"`
	NightLight    *int    `json:"night_light"`
	NoHeadsUp     bool    `json:"no_heads_up"`
	HeadsUpMin    int     `json:"heads_up_min"`
	NoUpdShow     bool    `json:"no_update_show"` // Settings › Updates: no Update button on the main screen, no automatic checks
	ServerManual  bool    `json:"server_manual"`  // Settings › Server › Find automatically: Off (picked by hand)
	UpdEvery      int     `json:"update_every"`   // …hours between automatic checks (0 = 12)
	TourDone      bool    `json:"tour_done"`
	ShowMusicLogo bool    `json:"show_music_logo"`
	Style         string  `json:"style"`          // Settings › Themes (themes.go)
	SetAt         float64 `json:"settings_at"`    // ksync.go: when the synced settings last changed (server time)
	SetDirty      bool    `json:"settings_dirty"` // …changed here, not sent yet
	Refresh       *int    `json:"refresh"`
	Compact       *bool   `json:"compact_set,omitempty"`
	Rotation      int     `json:"rotation,omitempty"`
}

func loadUI(path string) *UI {
	ui := &UI{Tab: "tasks", Widget: "day", Collapsed: map[int]bool{}, Event: -1, CalHour: -1, ConfirmRemove: -1, NightLight: -1, DayLight: -1, Refresh: 1, Rotation: 90, Dismissed: map[string]bool{}, Icons: map[string]map[string]*image.Gray{}, IconsKey: map[string]string{}}
	setCompact(isBasicKindle)
	var s Saved
	ui.Tour = 1 // first start (no state.json yet): the tour
	if b, err := os.ReadFile(path); err == nil && json.Unmarshal(b, &s) == nil {
		ui.Tour = 0
		ui.Dark, ui.ClockStyle, ui.Widget, ui.SideHidden = s.Dark, s.Clock, s.Widget, s.SideHidden
		ui.Clock12, ui.NoQuote, ui.NoEvent, ui.CalHidden, ui.CalTimeline, ui.WeatherHidden = s.Clock12, s.NoQuote, s.NoEvent, s.CalHidden, s.CalTL, s.WeatherOff
		ui.AutoDark, ui.NoHeadsUp, ui.HeadsUpMin = s.AutoDark, s.NoHeadsUp, s.HeadsUpMin
		ui.NoUpdShow, ui.UpdEvery = s.NoUpdShow, s.UpdEvery
		ui.SetAt, ui.SetDirty = s.SetAt, s.SetDirty
		ui.ShowMusicLogo = s.ShowMusicLogo
		for _, t := range themes {
			if s.Style == t.id && t.id != "plain" {
				themeName = t.id
			}
		}
		if !s.TourDone {
			ui.Tour = 1 // never seen the tour: show it on this start
		}
		if s.Refresh != nil {
			ui.Refresh = max(0, min(2, *s.Refresh))
		}
		if s.Compact != nil {
			setCompact(*s.Compact)
		} else if isBasicKindle {
			ui.SetDirty = true
		}
		if s.Rotation == 90 || s.Rotation == 270 {
			ui.Rotation = s.Rotation
		}
		if s.NightLight != nil {
			ui.NightLight = *s.NightLight
		}
		if ui.Widget == "" {
			ui.Widget = "day"
		}
		tempF = s.TempF
		for _, f := range fontStyles {
			if s.Font == f {
				fontStyle = f
			}
		}
		for _, id := range s.Collapsed {
			ui.Collapsed[id] = true
		}
		if ui.SetAt == 0 {
			ui.SetDirty = true // never synced: send this Kindle's settings up first (ksync.go)
		}
		if s.Tab == "deck" || s.Tab == "calendar" || s.Tab == "weather" {
			ui.Tab = s.Tab
		}
	}
	ui.SetKey = settingsKey(ui)
	return ui
}

func saveUI(path string, ui *UI) {
	if k := settingsKey(ui); ui.SetKey != "" && k != ui.SetKey { // a synced setting changed here: send it
		ui.SetDirty = true
		select {
		case settingsChanged <- struct{}{}:
		default:
		}
	}
	ui.SetKey = settingsKey(ui)
	comp := compactMode
	s := Saved{Dark: ui.Dark, Tab: ui.Tab, Clock: ui.ClockStyle, Widget: ui.Widget, SideHidden: ui.SideHidden, Font: fontStyle, TempF: tempF, Clock12: ui.Clock12, NoQuote: ui.NoQuote, NoEvent: ui.NoEvent, CalHidden: ui.CalHidden, CalTL: ui.CalTimeline, WeatherOff: ui.WeatherHidden, AutoDark: ui.AutoDark, NightLight: &ui.NightLight, NoHeadsUp: ui.NoHeadsUp, HeadsUpMin: ui.HeadsUpMin, NoUpdShow: ui.NoUpdShow, UpdEvery: ui.UpdEvery, ServerManual: ui.ServerManual, SetAt: ui.SetAt, SetDirty: ui.SetDirty, ShowMusicLogo: ui.ShowMusicLogo, Style: themeName, TourDone: ui.Tour == 0, Refresh: &ui.Refresh, Compact: &comp, Rotation: ui.Rotation}
	for id, c := range ui.Collapsed {
		if c {
			s.Collapsed = append(s.Collapsed, id)
		}
	}
	b, _ := json.Marshal(s)
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Println("save state:", err)
	}
	if configPath != "" && (ui.Rotation == 90 || ui.Rotation == 270) {
		m := map[string]any{}
		if cb, err := os.ReadFile(configPath); err == nil {
			json.Unmarshal(cb, &m)
		}
		if r, _ := m["rotation"].(float64); int(r) != ui.Rotation && len(m) > 0 {
			m["rotation"] = ui.Rotation
			if out, err := json.MarshalIndent(m, "", "  "); err == nil {
				os.WriteFile(configPath, out, 0o644)
			}
		}
	}
}

func main() {
	confPath := flag.String("config", "/mnt/us/desk/config.json", "config file")
	server := flag.String("server", "", "backend URL (overrides the config file)")
	preview := flag.String("preview", "", "draw one screen to this PNG and exit (for testing on a computer)")
	sample := flag.String("json", "", "with -preview: draw this saved /api/desk JSON instead of asking the server")
	tab := flag.String("tab", "tasks", "with -preview: tasks, deck or calendar")
	dark := flag.Bool("dark", false, "with -preview: dark mode")
	sel := flag.Int("select", 0, "with -preview: open this task")
	wday := flag.Int("wday", 0, "with -preview: weather day to show")
	libCoverFlag := flag.Bool("libcover", false, "with -preview: the Kindle library cover")
	welcomeFlag := flag.Bool("welcome", false, "with -preview: the welcome screen shown at start")
	standbyFlag := flag.Bool("standby", false, "with -preview: the standby clock")
	clockFlag := flag.Int("clock", 0, "with -preview: standby clock style")
	lightFlag := flag.Bool("light", false, "with -preview: open the brightness panel (or the standby bar)")
	updateFlag := flag.Bool("update", false, "with -preview: show the Update button")
	allFlag := flag.Bool("all", false, "with -preview: the Deck's All apps view")
	confirmFlag := flag.Int("confirm", -1, "with -preview: ask to remove this favourite")
	editFlag := flag.Bool("edit", false, "with -preview: the Deck in edit mode")
	focusFlag := flag.Bool("focus", false, "with -preview: the focus screen")
	widgetFlag := flag.String("widget", "day", "with -preview: the Tasks panel widget")
	hideFlag := flag.Bool("hide", false, "with -preview: the Tasks panel collapsed")
	settingsFlag := flag.Bool("settings", false, "with -preview: the Settings screen")
	clearFlag := flag.Bool("clearask", false, "with -preview: Settings with \"Clear the list?\" open")
	setTabFlag := flag.String("settab", "", "with -settings: the section open (screen, focus, music, tasks, notify, server, updates, about)")
	aboutFlag := flag.Bool("about", false, "with -preview: the Send feedback box")
	swipedFlag := flag.Int("swiped", 0, "with -preview: this task swiped (its bin showing)")
	delFlag := flag.Int("delask", 0, "with -preview: asking to delete this task")
	tourFlag := flag.Int("tour", 0, "with -preview: the first-run tour, page 1–3")
	autoFlag := flag.Bool("autodark", false, "with -preview: Theme on Auto")
	noHUFlag := flag.Bool("noheadsup", false, "with -preview: meeting heads-up turned off")
	themeFlag := flag.String("theme", "", "with -preview: a theme style (garden)")
	volFlag := flag.Bool("volume", false, "with -preview: the volume panel open (music playing)")
	whatsNewFlag := flag.Bool("whatsnew", false, "with -preview -update: the What's new box open (made-up notes)")
	pairFlag := flag.String("paircode", "", "with -preview -serverpick: waiting for Allow, showing this code")
	pickFlag := flag.Bool("serverpick", false, "with -preview: Settings with the server list (a made-up Mac found)")
	askMacFlag := flag.Bool("askmac", false, "with -serverpick: on the cloud, no computer found, the Mac can be asked")
	offlineFlag := flag.Bool("offline", false, "with -preview: offline, with its explanation open")
	outFlag := flag.Bool("outputs", false, "with -preview -cover: the Play on list open")
	coverFlag := flag.Bool("cover", false, "with -preview: the full-screen album cover")
	previewEvent = flag.String("calevent", "", "with -preview: open this event (start|title) on the Calendar tab")
	tlFlag := flag.Bool("timeline", false, "with -preview: the Calendar day panel as a timeline")
	kbFlag := flag.String("kb", "", "with -preview: the keyboard: add, sub (of -select), edit (-select), event, event2 (its time step), place")
	kbText := flag.String("kbtext", "", "with -preview: text typed on the keyboard")
	bareFlag := flag.Bool("bare", false, "with -preview: standby clock without quote and next event")
	h12Flag := flag.Bool("h12", false, "with -preview: 12-hour clocks")
	coverFullFlag := flag.String("coverfull", "", "with -preview: the cover filling the screen, controls frost|circles|bare")
	fontFlag := flag.String("font", "sans", "with -preview: sans, serif, inter or bitter")
	menu := flag.Bool("menu", false, "with -preview: open the Work / Home / Auto menu")
	previewBattery = flag.Int("battery", -1, "with -preview: show this battery level")
	tabs := flag.String("tabs", "", "with -preview: tab style (segmented, icons, underline)")
	panelFlag := flag.String("panel", "", "with -preview: draw for another Kindle's screen, landscape pixels (1448x1072, 1648x1236)")
	compactFlag := flag.Bool("compact", false, "with -preview: compact layout for 6-inch Basic Kindles")
	flag.Parse()
	log.SetFlags(log.Ltime)

	cfg := loadConfig(*confPath)
	if *server != "" {
		cfg.Server = *server
	}
	if err := loadFonts(); err != nil {
		log.Fatal("fonts: ", err)
	}
	// the screen's real size, before anything is drawn (fonts are made for it)
	if *panelFlag != "" {
		var w, h int
		fmt.Sscanf(*panelFlag, "%dx%d", &w, &h)
		setPanel(w, h)
	} else if *preview == "" {
		// anything within a few pixels of the Paperwhite 2's 758 × 1024 is one: left exactly as it is
		if w, h := panelSize(); abs(w-PW) > 16 || abs(h-PH) > 16 {
			setPanel(h, w) // portrait → landscape
		}
	}
	if *preview != "" {
		setCompact(*compactFlag || isBasicKindle)
	}
	client := NewClient(cfg.Server, cfg.Key)
	if *tabs != "" {
		tabStyle = *tabs
	}
	fontStyle = *fontFlag
	if *coverFullFlag == "frost" {
		coverStyle = "frost"
	}
	if *preview != "" && (*welcomeFlag || *libCoverFlag) {
		f, err := os.Create(*preview)
		if err != nil {
			log.Fatal(err)
		}
		if *libCoverFlag {
			png.Encode(f, coverImage())
		} else {
			png.Encode(f, welcomeImage(*dark))
		}
		f.Close()
		return
	}
	if *preview != "" {
		ui := &UI{NoCloud: cfg.Server == "", Tab: *tab, Dark: *dark, Selected: *sel, Menu: *menu, WeatherDay: *wday, ConfirmRemove: *confirmFlag, Standby: *standbyFlag, ClockStyle: *clockFlag, LightMenu: *lightFlag && !*standbyFlag, StandbyCtl: *lightFlag && *standbyFlag, UpdateReady: *updateFlag, DeckAll: *allFlag, DeckEdit: *editFlag, Focus: *focusFlag, Widget: *widgetFlag, SideHidden: *hideFlag, Settings: *settingsFlag || *clearFlag || *pickFlag || *aboutFlag, ServerPick: *pickFlag, SettingsTab: *setTabFlag, AutoDark: *autoFlag, NoHeadsUp: *noHUFlag, PairCode: *pairFlag, Refresh: 1, Tour: *tourFlag, Swiped: *swipedFlag, DeleteAsk: *delFlag, NightLight: -1, Dismissed: map[string]bool{}, OfflineInfo: *offlineFlag, ClearAsk: *clearFlag, Clock12: *h12Flag, CalHidden: *hideFlag && *tab == "calendar", WeatherHidden: *hideFlag && *tab == "weather", CalTimeline: *tlFlag, CalHour: -1, NoQuote: *bareFlag, NoEvent: *bareFlag, Outputs: *outFlag, Cover: *coverFlag || *coverFullFlag != "", CoverFull: *coverFullFlag != "", CoverBare: *coverFullFlag == "bare", Collapsed: map[int]bool{}, Event: -1,
			Icons: map[string]map[string]*image.Gray{}, IconsKey: map[string]string{}}
		if *kbFlag != "" {
			previewKB = &Editor{Mode: *kbFlag, ID: *sel, Text: *kbText, Shift: *kbText == ""}
		}
		if *previewEvent != "" {
			ui.Tab = "calendar"
		}
		if *aboutFlag {
			ui.SettingsTab = "about"
		}
		if *pickFlag && *askMacFlag {
			ui.Settings, ui.ServerPick, ui.SettingsTab, ui.MacSeen = true, false, "server", true
		} else if *pickFlag {
			ui.ServerPick, ui.SettingsTab = false, "server"
			ui.Found = []FoundServer{{Name: "My MacBook", Addr: "http://192.168.1.23:8787"}}
		}
		if *updateFlag { // made-up notes for the waiting update
			ui.UpdateNotes = []NoteEntry{{N: 999, Date: "2026-10-08", Items: []string{
				"Added: Windows helper: the Deck, music and Auto now work with your PC",
				"Added: covers from Apple Music radio",
				"Changed: the Calendar tab remembers the day you were looking at",
				"Changed: Settings opens where you left it",
				"Fixed: the clock could skip a minute after the Kindle woke up",
				"Fixed: a long task name could overlap the pin",
				"Fixed: swiping between tabs sometimes opened a task",
				"Fixed: the heads-up card stayed after the meeting was deleted",
				"Fixed: the weather showed yesterday's high after midnight",
				"Fixed: the Update button flashed twice",
				"Fixed: the battery icon showed 100% while charging",
				"Fixed: night brightness didn't come back after a restart",
				"Fixed: subtasks added offline appeared under the wrong task",
				"Fixed: the server list showed the same computer twice",
			}}}
			ui.UpdateNotes = append(ui.UpdateNotes, NoteEntry{N: 998, Date: "2026-10-07", Items: ui.UpdateNotes[0].Items[:8]}) // long enough to scroll
			ui.NotesModal = *whatsNewFlag
		}
		ui.VolumeOpen = *volFlag
		themeName = *themeFlag
		previewOffline = *offlineFlag
		writePreview(*preview, *sample, client, cfg, ui)
		return
	}

	debug.SetGCPercent(50) // the Kindle has ~100 MB to spare
	go func() {            // once: how many fingers the touchscreen follows and panel size (seen in GET /api/kindle/log)
		time.Sleep(20 * time.Second)
		client.report(fmt.Sprintf("info: panel %dx%d (compact=%v, basic=%v), touchscreen follows up to %d fingers", cw, ch, compactMode, isBasicKindle, touchFingers))
	}()
	dir := filepath.Dir(*confPath)
	configPath = *confPath
	os.Exit(run(cfg, client, filepath.Join(dir, "state.json"), dir))
}

var previewEvent *string

// configPath is config.json (Settings › Server rewrites its server).
var configPath string
var previewKB *Editor
var previewOffline bool

func writePreview(out, sample string, client *Client, cfg Config, ui *UI) {
	var d *Desk
	var err error
	if sample != "" {
		var b []byte
		if b, err = os.ReadFile(sample); err == nil {
			d = &Desk{}
			err = json.Unmarshal(b, d)
		}
	} else {
		d, err = client.Desk()
	}
	st := Status{Server: cfg.Server, Now: time.Now(), LastOK: time.Now(), Battery: *previewBattery, Charging: *previewBattery >= 0, Light: 12}
	if err != nil {
		log.Println("no data:", err)
		d, st.Offline = nil, true
	} else if d.Now > 0 {
		st.Now = time.Unix(d.Now, 0).In(time.FixedZone("", d.UTCOffset))
	}
	st.OnSince = st.Now.Add(-95 * time.Minute)
	if previewOffline {
		st.Offline, st.LastOK = true, time.Now().Add(-20*time.Minute)
	}
	if d != nil && d.Deck != nil && ui.Tab == "deck" {
		for _, dk := range d.Deck.Decks {
			ui.Icons[dk.Machine], _ = client.DeckIcons(dk.Machine)
		}
	}
	if d != nil && d.Music != nil && d.Music.ArtKey != "" && (ui.Widget == "music" || ui.Cover) {
		if k, b64, err := client.Art(); err == nil && k == d.Music.ArtKey {
			ui.Art, ui.ArtKey = decodeArt(b64, 640), k
		}
	}
	if d != nil && (ui.Tab == "weather" || ui.Widget == "weather") {
		ui.Weather, err = client.Weather()
	}
	if d != nil && previewKB != nil {
		switch previewKB.Mode {
		case "event", "event2":
			day := time.Date(st.Now.Year(), st.Now.Month(), st.Now.Day(), 0, 0, 0, 0, st.Now.Location())
			openEventEditor(ui, day, st.Now)
			if previewKB.Mode == "event2" {
				ui.Edit.Step = 1
			}
		case "place":
			ui.Edit = &Editor{Mode: "place"}
			if previewKB.Text != "" {
				ui.Edit.Places, _ = client.Places(previewKB.Text)
			}
		default:
			openEditor(ui, d, previewKB.Mode, previewKB.ID)
		}
		if previewKB.Text != "" {
			ui.Edit.Text, ui.Edit.Shift = previewKB.Text, false
		}
		if previewKB.Mode != "sub" {
			ui.Selected = 0
		}
	}
	if d != nil && ui.Tab == "calendar" {
		calReset(ui, st.Now)
		ui.EventKey = *previewEvent
		from, to := calRange(ui)
		if ui.Cal, err = client.Calendar(from, to, d.Profile); err != nil {
			ui.CalErr = err.Error()
		}
		ui.CalKey = "preview"
	}
	f, err := os.Create(out)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, render(d, st, ui).Img)
}

var previewBattery *int

// How often to ask the backend for changes. ~8,600 requests a day: well within Cloudflare's free plan.
const pollEvery = 10 * time.Second

type fetched struct {
	d   *Desk
	err error
	seq int
	at  time.Time
}

type calFetched struct {
	key string
	evs []Event
	err error
}

// calReset points the calendar at today's month and day.
func calReset(ui *UI, now time.Time) {
	ui.Month = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	ui.Day = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	ui.Event, ui.EventKey, ui.CalHour = -1, "", -1
}

// calRange is the span the month grid shows, with a day to spare each side.
func calRange(ui *UI) (int64, int64) {
	s := gridStart(ui.Month)
	return s.AddDate(0, 0, -1).Unix(), s.AddDate(0, 0, 43).Unix()
}

// isLocal reports whether an action only changes what the Kindle shows.
func isLocal(act string) bool {
	switch act {
	case "tab", "dark", "select", "collapse", "page", "month", "day", "event", "menu", "note", "refresh", "deckfor", "deckpage", "wday", "standby", "clockstyle", "lightmenu", "light", "setdark", "standbyctl", "deckall", "favcancel", "favask", "deckedit", "focusview", "widget", "sidehide", "settings", "setfont", "checkupdate", "cover", "coverfull", "coverbare", "calevent", "clock12", "clockquote", "clockevent", "key", "kbcancel", "addnew", "calhide", "calmode", "calhour", "weatherhide", "eventnew", "placenew", "evtime", "evlen", "evallday", "evtitle", "delask", "setunit", "clearask", "outputs", "settheme", "nightlight", "headsup", "headsupmin", "dismiss", "tour", "binask", "setrefresh", "serverpick", "serverfind", "offlineinfo", "retry", "about", "settab", "showupdates", "notespage", "calpage", "updshow", "updevery", "notesmodal", "notesscroll", "volopen", "musiclogo", "settheme2", "serverauto", "setcompact", "setrot", "fliprot":
		return true
	}
	return false
}

// run shows the desk until the user leaves (returns 0) or an update is ready (exitUpdate).
func run(cfg Config, client *Client, statePath, dir string) int {
	pauseKindleUI()
	clearScreen()
	go ensureLibraryItem(dir) // the library's "Desk Buddy" item and its cover (buddy.go)
	go wifiDiary(dir, client) // why does the Wi-Fi drop? (wifidiary.go)
	ui := loadUI(statePath)
	if ui.Rotation != 90 && ui.Rotation != 270 {
		ui.Rotation = cfg.Rotation
	}
	scr := NewScreen(ui.Rotation, cfg.FullRefreshEvery)
	taps := make(chan Tap, 8)
	power := make(chan struct{}, 1)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	go readTouch(taps)
	go readPower(power)

	var (
		desk       *Desk
		view       *View
		cloudURL   = cfg.Cloud
		wifiNudged time.Time     // last time we asked the Kindle to turn Wi-Fi on
		clockTried bool          // set the clock from the server once per run
		skew       time.Duration // server clock minus Kindle clock
		zone       = time.UTC
		st         = Status{Server: cfg.Server}
		seq        int // bumped on every tap, so replies from before it are ignored
		pending    int // taps still being sent
		fetching   bool
		calBusy    bool
		iconBusy   bool
		iconWait   time.Time // after a failed download, wait before trying again
		nextPoll   time.Time
		lastRev    string             // the server's change stamp from the last reply ("" = old backend: poll every 10 s)
		pollStart  time.Time          // when the request in flight was sent
		cancelPoll context.CancelFunc // ends a waiting request early
		moveTried  time.Time          // last time we tried to follow the computer to another server
		askedAt    time.Time          // when we asked the Mac to move Desk Buddy (Settings › Server)
		minute     = -1
		refindAt   time.Time // last time we looked for the local computer on the Wi-Fi
		outbox     = loadOutbox(dir)
		last       = &lastDesk{path: filepath.Join(dir, "last-desk.json")}
	)
	// start on the last-known screen (with changes still waiting on top), not "Connecting…"
	if d, at := last.load(); d != nil {
		outbox.applyAll(d)
		desk, st.LastOK, zone = d, at, time.FixedZone("", d.UTCOffset)
	}
	st.Waiting = len(outbox.Items)
	// welcome screen for a moment, then the desk (drawn fresh over it)
	scr.Show(welcomeImage(ui.Dark), Flash)
	time.Sleep(2 * time.Second)
	scr.Forget()
	results := make(chan fetched, 4)
	calResults := make(chan calFetched, 2)
	type iconFetched struct {
		machine, key string
		icons        map[string]*image.Gray
		err          error
	}
	iconResults := make(chan iconFetched, 1)
	type artFetched struct {
		key   string
		image *image.Gray
	}
	artResults := make(chan artFetched, 1)
	artBusy := false
	// fetchArt downloads the album cover when the Music widget shows a new song (kept in memory only).
	fetchArt := func() {
		if desk == nil || desk.Music == nil || artBusy {
			return
		}
		key := desk.Music.ArtKey
		if key == ui.ArtKey || (!ui.Cover && (ui.Widget != "music" || ui.SideHidden)) {
			return
		}
		if key == "" {
			ui.Art, ui.ArtKey = nil, ""
			return
		}
		artBusy = true
		go func() {
			k, b64, err := client.Art()
			var img *image.Gray
			if err == nil && k == key {
				img = decodeArt(b64, 640)
			}
			artResults <- artFetched{key, img}
		}()
	}
	type weatherFetched struct {
		w   *Weather
		err error
	}
	weatherResults := make(chan weatherFetched, 2)
	type placesFetched struct {
		q      string
		places []Place
		err    error
	}
	placeResults := make(chan placesFetched, 1)
	calForce := make(chan bool, 2) // an event was added or deleted: reload the month
	foundCh := make(chan []FoundServer, 1)
	type serverSwitched struct {
		name, addr string
		err        error
	}
	serverCh := make(chan serverSwitched, 1)
	pairCh := make(chan string, 2) // the pairing code to show ("" when done)
	ui.ServerLocal, ui.ServerName, ui.ServerAddr = isLocalServer(cfg.Server), cfg.ServerName, cfg.Server
	ui.NoCloud = cloudURL == "" && isLocalServer(cfg.Server)
	findNow := func() { // look for computers running the local server on this Wi-Fi
		if ui.Finding {
			return
		}
		ui.Finding, ui.Found = true, nil
		go func() { foundCh <- findServers() }()
	}
	if cfg.Server == "" { // a new Kindle: look for the computer straight away
		ui.ServerPick = true
		findNow()
	}
	kick := make(chan bool, 4) // fetch the desk now (soon after a music tap)
	weatherBusy := false
	// fetchWeather loads the forecast while the Weather tab is open (every 15 minutes).
	fetchWeather := func() {
		wanted := ui.Tab == "weather" || (ui.Tab == "tasks" && ui.Widget == "weather" && !ui.SideHidden)
		if !wanted || weatherBusy || time.Since(ui.WeatherAt) < 15*time.Minute {
			return
		}
		weatherBusy = true
		go func() {
			w, err := client.Weather()
			weatherResults <- weatherFetched{w, err}
		}()
	}
	// fetchIcons downloads the deck's app icons when the computer has uploaded new ones.
	fetchIcons := func() {
		if desk == nil || desk.Deck == nil || iconBusy || time.Now().Before(iconWait) {
			return
		}
		for _, dk := range desk.Deck.Decks { // one computer at a time
			if dk.IconsVersion == "" || ui.IconsKey[dk.Machine] == dk.IconsVersion {
				continue
			}
			iconBusy = true
			machine, key := dk.Machine, dk.IconsVersion
			go func() {
				icons, err := client.DeckIcons(machine)
				iconResults <- iconFetched{machine, key, icons, err}
			}()
			return
		}
	}
	type sentResult struct {
		t   Target
		err error
	}
	sent := make(chan sentResult, 8)
	type flushResult struct {
		done int
		ids  map[int]int
		err  error
	}
	flushed := make(chan flushResult, 1)
	flushing := false
	// tryFlush sends the changes made while offline (outbox.go), in order.
	tryFlush := func() {
		if flushing || len(outbox.Items) == 0 {
			return
		}
		flushing = true
		seq++
		pending++ // replies fetched meanwhile are ignored, so nothing shows twice
		items := append([]Target(nil), outbox.Items...)
		go func() {
			n, ids, err := outbox.flush(client, items)
			flushed <- flushResult{n, ids, err}
		}()
	}

	// Updates over Wi-Fi: checked once connected, then every 30 minutes.
	type updFetched struct {
		ready bool
		err   error
	}
	updResults := make(chan updFetched, 1)
	var updBusy bool
	var updNext time.Time
	// checkUpdate looks for a newer build: by itself every Settings › Updates › Check every
	// hours (unless that's off), or straight away from Check for updates (manual).
	checkUpdate := func(manual bool) {
		if updBusy || ui.UpdateReady || (!manual && (ui.NoUpdShow || time.Now().Before(updNext))) {
			return
		}
		every := ui.UpdEvery
		if every == 0 {
			every = 12
		}
		updBusy, updNext = true, time.Now().Add(time.Duration(every)*time.Hour)
		go func() {
			ready, err := client.checkUpdate(dir)
			updResults <- updFetched{ready, err}
		}()
	}
	// installUpdate shows a message and hands over to run.sh, which swaps in the new build.
	installUpdate := func() bool {
		img := newCanvas(LW, LH)
		drawBuddy(img, LW/2, 140, 160, ink)
		drawCentered(img, face("serifB", 40), LW/2, 360, "Updating Desk Buddy…", ink)
		drawCentered(img, face("ui", 18), LW/2, 410, "Back in a few seconds", grey2)
		if ui.Dark {
			invertRect(img, img.Rect)
		}
		scr.Show(img, Flash)
		if err := installDownloaded(dir); err != nil {
			log.Println("update:", err)
			client.report("update install failed: " + err.Error())
			ui.UpdateReady = false
			scr.Forget()
			return false
		}
		log.Println("installing update")
		return true
	}

	fetch := func() {
		if fetching || client.Base() == "" {
			return
		}
		fetching = true
		s := seq
		music := ui.Cover || (ui.Tab == "tasks" && ui.Widget == "music" && !ui.SideHidden && !ui.Standby && !ui.Focus)
		ctx, cancel := context.WithCancel(context.Background())
		cancelPoll, pollStart = cancel, time.Now()
		rev := lastRev
		go func() {
			d, err := client.DeskWait(ctx, music, rev)
			cancel()
			results <- fetched{d, err, s, time.Now()}
		}()
	}
	// fetchCal loads the calendar month if the Calendar tab needs it.
	fetchCal := func() {
		if ui.Tab != "calendar" || desk == nil || calBusy || ui.Month.IsZero() {
			return
		}
		key := ui.Month.Format("2006-01") + "/" + desk.Profile
		if key == ui.CalKey && time.Since(ui.CalAt) < 10*time.Minute {
			return
		}
		calBusy = true
		from, to := calRange(ui)
		profile := desk.Profile
		go func() {
			evs, err := client.Calendar(from, to, profile)
			calResults <- calFetched{key, evs, err}
		}()
	}
	// safeRender draws a screen; if that screen has a bug and panics, it reports it,
	// goes back to the Tasks screen and draws that instead of crashing the app.
	safeRender := func() (v *View) {
		defer func() {
			if r := recover(); r != nil {
				msg := fmt.Sprintf("render panic on %s (standby=%v, clock=%d): %v\n%s", ui.Tab, ui.Standby, ui.ClockStyle, r, debug.Stack())
				log.Println(msg)
				client.report(msg)
				ui.Tab, ui.Standby, ui.StandbyCtl, ui.Menu, ui.LightMenu, ui.Cover = "tasks", false, false, false, false, false
				v = render(desk, st, ui)
			}
		}()
		return render(desk, st, ui)
	}
	// nightLight sets the front light for Auto's dark (the chosen night level, keeping the
	// daytime one) or light (the daytime level back). Nothing when no night level is chosen.
	nightLight := func(dark bool) {
		switch {
		case dark && ui.NightLight >= 0:
			if ui.DayLight < 0 {
				ui.DayLight = st.Light
			}
			setFrontLight(ui.NightLight)
			st.Light = ui.NightLight
		case ui.DayLight >= 0: // light again, or "Same" chosen: the daytime level back
			setFrontLight(ui.DayLight)
			st.Light, ui.DayLight = ui.DayLight, -1
		}
	}
	draw := func(w Waveform) {
		st.Now = time.Now().Add(skew).In(zone)
		// Theme: Auto. Switching flashes the whole screen; the night light level, if one is
		// chosen, comes on with dark and the daytime level comes back with light.
		if ui.AutoDark && desk != nil {
			if want := autoDark(desk, st.Now); want != ui.Dark {
				ui.Dark = want
				scr.Forget()
				nightLight(want)
			}
		}
		minute = st.Now.Minute()
		v := safeRender()
		tabs := map[string]bool{"tasks": true, "deck": true, "calendar": true, "weather": true}
		if view != nil && view.Kind != v.Kind && !(tabs[view.Kind] && tabs[v.Kind]) {
			w = Flash // a different screen altogether: clear it properly
		}
		// (between the four tabs the header stays: only the page below it flashes, as the
		// big changed area it is, and the two tab buttons redraw quietly)
		view = v
		if ui.Rotation == 90 || ui.Rotation == 270 {
			scr.rot = ui.Rotation
		}
		scr.Dark = ui.Dark
		scr.Quiet = (v.Kind == "standby" && !ui.StandbyCtl) || v.Kind == "focus"
		scr.Splits, scr.SplitY, scr.Level = v.Splits, v.SplitY, ui.Refresh
		scr.Show(v.Img, w)
	}
	// sendOrQueue tells the backend about a tap, or keeps it for later (outbox.go) while
	// earlier changes are still waiting, so they arrive in order.
	sendOrQueue := func(t Target) {
		if canWait(t.Act) && len(outbox.Items) > 0 {
			t = outbox.add(t)
			if desk != nil && t.Act == "addtask" {
				apply(desk, t)
			}
			st.Waiting = len(outbox.Items)
			draw(Clean)
			tryFlush()
			return
		}
		seq++
		pending++
		go func() { sent <- sentResult{t, client.send(t)} }()
	}
	// local handles taps that only change the view.
	local := func(t Target) {
		tidy := ui.Dark // white on black ghosts: flash the changed area clean
		if t.Act != "menu" {
			ui.Menu = false // refresh, dark mode etc. also close the menu, in the same tap
		}
		if t.Act != "lightmenu" && t.Act != "light" && t.Act != "setdark" {
			ui.LightMenu = false
		}
		switch t.Act {
		case "tab":
			ui.Tab, ui.Note = t.Arg, ""
			ui.DeckProfile, ui.DeckPage, ui.DeckAll, ui.ConfirmRemove, ui.DeckEdit = "", 0, false, -1, false // the Deck opens on the favourites of the computer in use
			ui.WeatherDay = 0                                                                                // and the weather on now
			if t.Arg == "calendar" {
				calReset(ui, st.Now)
			}
			saveUI(statePath, ui)
		case "refresh":
			scr.ForceFlash() // Refresh: a proper clean-up, whatever the Screen refresh level
			ui.CalAt = time.Time{}
			ui.WeatherAt = time.Time{}
			nextPoll = time.Now()
			fetch()
		case "standby":
			ui.Standby, ui.StandbyCtl = !ui.Standby, false
		case "standbyctl":
			ui.StandbyCtl, ui.StandbyAt = !ui.StandbyCtl, time.Now()
			tidy = true
			if !ui.StandbyCtl {
				scr.DeepClean() // closing the bar: wipe and redraw so nothing of it is left behind
			}
		case "lightmenu":
			ui.LightMenu = !ui.LightMenu
			tidy = true
			if !ui.LightMenu && ui.Dark {
				scr.DeepClean()
			}
		case "light":
			n, _ := strconv.Atoi(t.Arg)
			setFrontLight(n)
			st.Light = n
			ui.StandbyAt = time.Now()
		case "setdark": // Light or Dark by hand: Auto ends
			ui.Dark, ui.AutoDark = t.Arg == "1", false
			scr.Forget()
			saveUI(statePath, ui)
			ui.StandbyAt = time.Now()
		case "eventnew":
			openEventEditor(ui, ui.Day, st.Now)
		case "placenew":
			ui.Edit = &Editor{Mode: "place", Shift: true}
		case "evtime":
			if e := ui.Edit; e != nil {
				n, _ := strconv.Atoi(t.Arg)
				m := max(0, min(23*60+45, e.Hour*60+e.Min+n))
				e.Hour, e.Min = m/60, m%60
			}
		case "evlen":
			if ui.Edit != nil {
				ui.Edit.Length, _ = strconv.Atoi(t.Arg)
			}
		case "evallday":
			if ui.Edit != nil {
				ui.Edit.AllDay = t.Arg == "1"
			}
		case "evtitle":
			if ui.Edit != nil {
				ui.Edit.Step = 0
			}
		case "delask":
			ui.ConfirmEvent = t.ID
		case "weatherhide":
			ui.WeatherHidden = !ui.WeatherHidden
			tidy = true
			saveUI(statePath, ui)
		case "calmode":
			ui.CalTimeline, ui.CalHour = t.Arg == "1", -1
			saveUI(statePath, ui)
		case "calhour":
			n, _ := strconv.Atoi(t.Arg)
			ui.CalHour = max(0, min(23, ui.CalHourShown+n))
		case "calhide":
			ui.CalHidden = !ui.CalHidden
			tidy = true
			saveUI(statePath, ui)
		case "addnew": // the keyboard: Arg add (new task) or sub (subtask of ID)
			if desk != nil {
				openEditor(ui, desk, t.Arg, t.ID)
			}
		case "key":
			if ui.Edit != nil {
				editorKey(ui.Edit, t.Arg)
			}
		case "kbcancel":
			ui.Edit = nil
		case "clock12", "clockquote", "clockevent":
			on := t.Arg == "1"
			switch t.Act {
			case "clock12":
				ui.Clock12 = on
			case "clockquote":
				ui.NoQuote = !on
			default:
				ui.NoEvent = !on
			}
			ui.StandbyAt = time.Now() // keep the bar open while choosing
			saveUI(statePath, ui)
		case "clockstyle":
			n, _ := strconv.Atoi(t.Arg)
			ui.ClockStyle = (ui.ClockStyle + n + len(clockStyles)) % len(clockStyles)
			scr.Forget() // a whole new picture: redraw it cleanly
			saveUI(statePath, ui)
		case "wday":
			ui.WeatherDay = t.ID
		case "settings":
			ui.Settings, ui.UpdateNote = !ui.Settings, ""
			ui.Menu = false
		case "serverpick":
			ui.ServerNote = ""
			if desk != nil { // switching is done on the computer: Settings › Server just says where it is
				ui.Settings, ui.SettingsTab, ui.ServerPick = true, "server", false
				break
			}
			ui.ServerPick = !ui.ServerPick // first setup: the pop-up over the connecting screen
			if ui.ServerPick {
				findNow()
			}
		case "serverfind":
			findNow()
		case "offlineinfo":
			ui.OfflineInfo = !ui.OfflineInfo
		case "settab":
			ui.SettingsTab = t.Arg
		case "showupdates": // the header's Update: what it brings, and Install
			ui.Settings, ui.SettingsTab, ui.NotesPage, ui.UpdateNote = true, "updates", 0, ""
			ui.NotesModal, ui.NotesScroll = true, 0 // straight to what the update brings
		case "calpage":
			n, _ := strconv.Atoi(t.Arg)
			ui.CalListPage += n
		case "settheme2": // Settings › Themes
			themeName = t.Arg
			if themeName == "plain" {
				themeName = ""
			}
			faces = map[string]font.Face{} // a theme may bring its own fonts
			scr.Forget()
			saveUI(statePath, ui)
		case "musiclogo": // Settings › Music › Music app logo
			ui.ShowMusicLogo = t.Arg == "1"
			saveUI(statePath, ui)
		case "volopen": // the volume panel (the header's speaker); again, or a tap elsewhere, closes it
			ui.VolumeOpen, ui.VolumeAt, ui.Outputs, ui.LightMenu = !ui.VolumeOpen, time.Now(), false, false
		case "notesmodal":
			ui.NotesModal, ui.NotesScroll = !ui.NotesModal, 0
		case "notesscroll": // a few lines at a time
			n, _ := strconv.Atoi(t.Arg)
			ui.NotesScroll = max(0, ui.NotesScroll+4*n)
		case "notespage":
			n, _ := strconv.Atoi(t.Arg)
			ui.NotesPage += n
		case "about": // Send feedback: Settings, on About
			ui.Settings, ui.SettingsTab, ui.UpdateNote = true, "about", ""
		case "retry": // check the connection now rather than in up to 15 s
			ui.OfflineInfo = false
			nextPoll = time.Now()
			fetch()
		case "settheme": // Light, Dark or Auto (Settings › Screen)
			ui.AutoDark = t.Arg == "auto"
			if ui.AutoDark && desk != nil {
				ui.Dark = autoDark(desk, st.Now)
				nightLight(ui.Dark) // already night: the night level now
			} else {
				ui.Dark = t.Arg == "dark"
				nightLight(false) // leaving Auto: the daytime level back, if one was saved
			}
			scr.Forget()
			saveUI(statePath, ui)
		case "nightlight": // takes effect now if it's already dark
			ui.NightLight, _ = strconv.Atoi(t.Arg)
			if ui.AutoDark && ui.Dark {
				nightLight(true)
			}
			saveUI(statePath, ui)
		case "headsup":
			ui.NoHeadsUp = t.Arg == "0"
			saveUI(statePath, ui)
		case "updshow":
			ui.NoUpdShow = t.Arg == "0"
			saveUI(statePath, ui)
		case "serverauto":
			ui.ServerManual = t.Arg == "0"
			saveUI(statePath, ui)
		case "updevery":
			ui.UpdEvery, _ = strconv.Atoi(t.Arg)
			updNext = time.Time{} // the new interval counts from the next check
			saveUI(statePath, ui)
		case "headsupmin":
			ui.HeadsUpMin, _ = strconv.Atoi(t.Arg)
			saveUI(statePath, ui)
		case "tour": // start, next page, skip
			switch t.Arg {
			case "start": // from Settings › About
				ui.Tour, ui.Settings, ui.Tab, ui.SideHidden, ui.Selected = 1, false, "tasks", false, 0
			case "next":
				ui.Tour++
			default:
				ui.Tour = tourPages + 1
			}
			if ui.Tour > tourPages {
				ui.Tour = 0
			}
			scr.Forget()
			saveUI(statePath, ui)
		case "setrefresh": // Settings › Screen refresh
			n, _ := strconv.Atoi(t.Arg)
			ui.Refresh = max(0, min(2, n))
			saveUI(statePath, ui)
		case "binask": // the bin on a swiped row: ask; ID 0 (Cancel): never mind
			ui.DeleteAsk, ui.Swiped = t.ID, 0
		case "dismiss":
			ui.Dismissed[ui.HeadsUpKey] = true
		case "outputs":
			ui.Outputs = !ui.Outputs
			nextPoll = time.Now() // fresh list
		case "setunit":
			tempF = t.Arg == "f"
			saveUI(statePath, ui)
		case "clearask":
			ui.ClearAsk = !ui.ClearAsk
		case "setfont":
			fontStyle = t.Arg
			scr.Forget()
			saveUI(statePath, ui)
		case "setcompact":
			setCompact(t.Arg == "1")
			scr.Forget()
			saveUI(statePath, ui)
		case "setrot":
			if t.Arg == "270" {
				ui.Rotation = 270
			} else {
				ui.Rotation = 90
			}
			scr.rot = ui.Rotation
			clearScreen()
			scr.ForceFlash()
			saveUI(statePath, ui)
		case "fliprot":
			if ui.Rotation == 270 {
				ui.Rotation = 90
			} else {
				ui.Rotation = 270
			}
			scr.rot = ui.Rotation
			clearScreen()
			scr.ForceFlash()
			saveUI(statePath, ui)
		case "checkupdate":
			ui.UpdateNote = "Checking…"
			checkUpdate(true)
		case "widget":
			ui.Widget = t.Arg
			saveUI(statePath, ui)
			if t.Arg == "music" {
				nextPoll = time.Now() // tell the computers straight away
			}
		case "sidehide":
			ui.SideHidden = !ui.SideHidden
			tidy = true
			saveUI(statePath, ui)
		case "cover":
			ui.Cover, ui.CoverFull, ui.CoverBare, ui.Outputs = !ui.Cover, false, false, false
			nextPoll = time.Now() // the music state, straight away
			fetchArt()
		case "coverfull":
			ui.CoverFull, ui.CoverBare = !ui.CoverFull, false
		case "coverbare":
			ui.CoverBare = !ui.CoverBare
		case "focusview":
			ui.Focus = !ui.Focus
		case "deckedit":
			ui.DeckEdit = !ui.DeckEdit
			ui.ConfirmRemove = -1
			tidy = true
		case "favask":
			ui.ConfirmRemove = t.ID
		case "favcancel":
			ui.ConfirmRemove = -1
			tidy = true
		case "deckall":
			ui.DeckAll, ui.DeckPage = t.Arg == "1", 0
		case "deckfor":
			ui.DeckProfile, ui.DeckPage, ui.Note = t.Arg, 0, ""
		case "deckpage":
			n, _ := strconv.Atoi(t.Arg)
			ui.DeckPage += n
		case "menu":
			ui.Menu = !ui.Menu
			tidy = true
		case "note":
			ui.Note = t.Arg
		case "dark":
			ui.Dark = !ui.Dark
			scr.Forget() // redraw everything, with a flash
			saveUI(statePath, ui)
		case "select":
			if ui.Selected == t.ID || t.Arg == "" && t.ID == ui.Selected {
				ui.Selected = 0
			} else {
				ui.Selected = t.ID
			}
		case "collapse":
			ui.Collapsed[t.ID] = !ui.Collapsed[t.ID]
			saveUI(statePath, ui)
		case "page":
			n, _ := strconv.Atoi(t.Arg)
			ui.Page += n
			ui.Selected = 0
		case "month":
			switch t.Arg {
			case "0":
				calReset(ui, st.Now)
			default:
				n, _ := strconv.Atoi(t.Arg)
				ui.Month = ui.Month.AddDate(0, n, 0)
				ui.Day, ui.Event, ui.CalHour = ui.Month, -1, -1
			}
		case "day":
			if d, err := time.ParseInLocation("2006-01-02", t.Arg, zone); err == nil {
				ui.Day, ui.Event, ui.EventKey, ui.CalHour = d, -1, "", -1
				if ui.CalHidden { // tapping a day opens the panel with it (and it stays open)
					ui.CalHidden = false
					saveUI(statePath, ui)
				}
				if d.Month() != ui.Month.Month() {
					ui.Month = time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, zone)
				}
			}
		case "event":
			ui.Event, ui.EventKey, ui.ConfirmEvent = t.ID, "", 0
		case "calevent": // an event tapped in the Day widget: open today on the Calendar tab with it showing
			ui.Tab, ui.Note = "calendar", ""
			calReset(ui, st.Now)
			ui.EventKey, ui.CalHidden = t.Arg, false
			saveUI(statePath, ui)
		}
		fetchCal()
		fetchWeather()
		if tidy {
			draw(Tidy)
		} else {
			draw(Clean)
		}
	}

	st.Battery, st.Charging = battery()
	deskSHA := fileSHA(filepath.Join(dir, "desk")) // which build this is, for the helper's Home screen
	setStatusQuery(st.Battery, st.Charging, deskSHA)
	type setPushed struct {
		at  float64
		key string
		err error
	}
	setCh := make(chan setPushed, 1)
	setBusy := false
	// pushSettings sends this Kindle's synced settings when they changed here (ksync.go).
	pushSettings := func() {
		if setBusy || !ui.SetDirty || client.Base() == "" {
			return
		}
		setBusy = true
		v, key := kindleSettings(ui), settingsKey(ui)
		go func() {
			at, err := client.pushSettings(v)
			setCh <- setPushed{at, key, err}
		}()
	}
	st.Light = frontLight()
	draw(Flash)
	fetch()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()

	for {
		select {
		case <-tick.C:
			if !fetching && time.Now().After(nextPoll) {
				fetch()
			} else if fetching && cancelPoll != nil && nextPoll.After(pollStart) && time.Now().After(nextPoll) {
				cancelPoll() // a waiting request, but something here wants an answer now: ask again
				cancelPoll = nil
			}
			if !askedAt.IsZero() && time.Since(askedAt) > 30*time.Second { // the Mac didn't move it
				askedAt = time.Time{}
				if ui.ServerPick || (ui.Settings && ui.SettingsTab == "server") {
					ui.ServerNote = "Your Mac didn't answer. Is Desk Buddy open on it?"
					draw(Clean)
				}
			}
			if ui.Note != "" && !ui.NoteAt.IsZero() && time.Since(ui.NoteAt) > 6*time.Second {
				ui.Note, ui.NoteAt = "", time.Time{}
				draw(Clean)
			}
			if ui.VolumeOpen && time.Since(ui.VolumeAt) > 8*time.Second {
				ui.VolumeOpen = false // closes by itself after a while
				draw(Clean)
			}
			if ui.StandbyCtl && time.Since(ui.StandbyAt) > 10*time.Second {
				local(Target{Act: "standbyctl"}) // hide the standby control bar again
			}
			// A focus or break running out: show Time's up straight away (not at the next minute).
			if desk != nil && desk.Pomodoro.running() && view != nil && view.Kind != "timesup" &&
				desk.Pomodoro.finished(time.Now().Add(skew).Unix()) {
				draw(Flash)
			}
			if time.Now().Add(skew).In(zone).Minute() != minute {
				st.Battery, st.Charging = battery()
				setStatusQuery(st.Battery, st.Charging, deskSHA)
				if l := frontLight(); l >= 0 {
					st.Light = l
				}
				fetchCal()
				fetchWeather()
				draw(Clean)
			}

		case r := <-results:
			fetching, cancelPoll = false, nil
			if errors.Is(r.err, context.Canceled) {
				continue // cancelled on purpose (above): the next tick asks again
			}
			if r.err != nil {
				log.Println("fetch:", r.err)
				noteFetchErr(r.err) // wifidiary.go
				nextPoll = time.Now().Add(15 * time.Second)
				wifi := wifiState()
				why := explainNetErr(r.err, wifi)
				if !st.Offline || why != st.Why {
					st.Offline, st.Why = true, why
					draw(Clean)
				}
				// Find automatically (the default): after 90 s without the server, look for the live
				// one (same computer at a new address, the cloud, another computer), once a minute.
				// Off: only the same computer at a new address (local mode).
				if time.Since(refindAt) > time.Minute && ((!ui.ServerManual && time.Since(st.LastOK) > 90*time.Second) || (ui.ServerManual && ui.ServerLocal)) {
					refindAt = time.Now()
					go func(name, cur, cloud string, auto bool) {
						var n, a string
						var ok bool
						if auto {
							n, a, ok = autoFind(client, name, cur, cloud)
						} else {
							n, a, ok = refind(client, name, cur)
						}
						c := ""
						if ok && !isLocalServer(a) {
							c = a
						}
						if ok && saveServer(configPath, a, n, c) == nil {
							log.Println("found Desk Buddy at", a)
							serverCh <- serverSwitched{n, a, nil}
						}
					}(ui.ServerName, client.Base(), cloudURL, !ui.ServerManual)
				}
				// fix what can be fixed here, then try again straight away
				if wifi != "" && wifi != "CONNECTED" && time.Since(wifiNudged) > 5*time.Minute && wifiOff() {
					wifiNudged = time.Now()
					go func() { wifiOn(); time.Sleep(8 * time.Second); kick <- true }()
				} else if clockError(r.err) && !clockTried {
					clockTried = true
					base := client.Base()
					go func() {
						if fixClock(base) {
							log.Println("clock set from the server")
							kick <- true
						}
					}()
				}
				continue
			}
			st.Why = ""
			nextPoll = time.Now().Add(pollEvery)
			if r.seq != seq || pending > 0 {
				// A tap happened meanwhile, so this reply may predate it. Once the tap
				// has been sent, its handler fetches again.
				if pending == 0 {
					fetch()
				}
				continue
			}
			// The server's key was changed (PUBLISHING.md: rotating the key): keep the new one.
			if k := r.d.NewKey; k != "" && k != client.Key() {
				if err := saveServer(configPath, client.Base(), ui.ServerName, "", k); err != nil {
					log.Println("new key:", err)
				} else {
					client.SetKey(k)
					log.Println("switched to the server's new key")
				}
			}
			if r.d.Rev != "" { // the server waits for changes itself: ask again straight away
				lastRev, nextPoll = r.d.Rev, time.Now()
			}
			// The computer moved Desk Buddy (Cloud ↔ a computer): follow it, if it answers with our key.
			// The cloud's address: remember it, so Cloud can always be picked (and Auto can go back).
			ui.MacSeen = slices.Contains(r.d.Computers, "mac")
			if c := strings.TrimRight(r.d.Cloud, "/"); c != "" && c != cloudURL {
				if err := saveServer(configPath, client.Base(), ui.ServerName, c); err == nil {
					cloudURL, ui.NoCloud = c, false
				}
			}
			if mv := r.d.MoveTo; !ui.ServerManual && mv != nil && mv.Server != "" && strings.TrimRight(mv.Server, "/") != strings.TrimRight(client.Base(), "/") && time.Since(moveTried) > 30*time.Second {
				moveTried = time.Now()
				go func(name, addr string) {
					if err := tryServer(client, addr); err != nil {
						log.Println("move to", addr, ":", err) // try again in 30 s
						return
					}
					cloud := ""
					if !isLocalServer(addr) {
						cloud, name = addr, ""
					}
					if err := saveServer(configPath, addr, name, cloud); err != nil {
						log.Println("move:", err)
						return
					}
					log.Println("followed the computer to", addr)
					serverCh <- serverSwitched{name, addr, nil}
				}(mv.Name, mv.Server)
			}
			if st.Offline || st.OnSince.IsZero() {
				st.OnSince = time.Unix(r.d.Now, 0) // back online (or first contact): from now
			}
			last.keep(r.d) // for the next start (before the waiting changes go on top)
			outbox.applyAll(r.d)
			desk, st.Offline, st.LastOK = r.d, false, r.at
			tryFlush()
			sendDiary(client)
			lastFetchErr = ""
			checkUpdate(false) // we're online: time to look for a newer build?
			skew = time.Unix(r.d.Now, 0).Sub(r.at)
			fetchedAt = r.at.Unix()
			zone = time.FixedZone("", r.d.UTCOffset)
			if ui.Month.IsZero() {
				calReset(ui, time.Now().Add(skew).In(zone))
			}
			fetchCal()
			fetchIcons()
			fetchArt()
			// the Kindle's own settings: ours go up if changed here, newer ones from the Mac come down
			if ui.SetDirty {
				pushSettings()
			} else if ks := r.d.KindleSet; ks != nil && ks.At > ui.SetAt+0.001 {
				look := applyKindleSettings(ui, ks.Values)
				ui.SetAt = ks.At
				if ui.AutoDark {
					ui.Dark = autoDark(desk, time.Now().Add(skew).In(zone))
				}
				ui.SetKey = settingsKey(ui) // came from the backend: nothing to send back
				saveUI(statePath, ui)
				if look {
					scr.Forget()
				}
			}
			draw(Clean)

		case a := <-artResults:
			artBusy = false
			ui.Art, ui.ArtKey = a.image, a.key // the previous cover is simply dropped
			if view != nil && (view.Kind == "cover" || view.Kind == "coverfull" || (view.Kind == "tasks" && ui.Widget == "music")) {
				draw(Clean)
			}

		case u := <-updResults:
			updBusy = false
			if u.err != nil {
				log.Println("update check:", u.err)
				updNext = time.Now().Add(5 * time.Minute)
				if ui.UpdateNote != "" {
					ui.UpdateNote = "Couldn't check just now"
					draw(Clean)
				}
			} else if !u.ready {
				if ui.UpdateNote != "" {
					ui.UpdateNote = "You're up to date"
					draw(Clean)
				}
			} else if u.ready {
				ui.UpdateReady = true // never installs by itself: the header offers it
				ui.UpdateNotes = lastNotes
				draw(Clean)
			}

		case pr := <-placeResults:
			if e := ui.Edit; e != nil && e.Mode == "place" {
				e.Searching, e.Places = false, pr.places
				if e.Places == nil {
					e.Places = []Place{}
				}
				e.SearchErr = ""
				if pr.err != nil {
					e.SearchErr = pr.err.Error()
				}
				draw(Clean)
			}
		case f := <-flushed:
			flushing = false
			pending--
			rest := outbox.Items[f.done:]
			for i := range rest { // tasks added offline now have real ids
				if n, ok := f.ids[rest[i].ID]; ok {
					rest[i].ID = n
				}
			}
			outbox.Items = rest
			outbox.save()
			st.Waiting = len(outbox.Items)
			if f.err != nil {
				log.Println("outbox:", f.err)
				st.Offline = true
			}
			if pending == 0 {
				nextPoll = time.Now()
				fetch()
			}
		case <-kick:
			if pending == 0 {
				nextPoll = time.Now()
				fetch()
			}
		case <-wifiRedraw: // the Kindle's software was awake for Wi-Fi (wifidiary.go): draw over it
			scr.ForceFlash()
			draw(Clean)
			if pending == 0 {
				nextPoll = time.Now()
				fetch()
			}
		case f := <-foundCh:
			ui.Finding, ui.Found = false, f
			if ui.ServerPick || (ui.Settings && ui.SettingsTab == "server") {
				draw(Clean)
			}
		case code := <-pairCh:
			ui.PairCode = code
			if code != "" {
				ui.ServerNote = ""
			}
			draw(Clean)
		case sw := <-serverCh:
			if sw.err != nil {
				ui.ServerNote = "Couldn't switch: " + sw.err.Error()
				draw(Clean)
				continue
			}
			client.SetBase(sw.addr)
			st.Server, askedAt = sw.addr, time.Time{}
			outbox.Items = nil // changes kept for the old server mustn't land on another one's tasks
			outbox.save()
			st.Waiting = 0
			ui.ServerLocal, ui.ServerName, ui.ServerAddr = isLocalServer(sw.addr), sw.name, sw.addr
			ui.ServerPick, ui.ServerNote = false, ""
			ui.Art, ui.ArtKey, ui.CalAt, ui.WeatherAt = nil, "", time.Time{}, time.Time{}
			ui.IconsKey = map[string]string{}
			scr.Forget()
			nextPoll = time.Now()
			fetch()
			draw(Flash)
		case <-settingsChanged:
			pushSettings()
		case p := <-setCh:
			setBusy = false
			if p.err != nil {
				log.Println("settings:", p.err) // still dirty: goes with the next good fetch
			} else {
				ui.SetAt = p.at
				if settingsKey(ui) == p.key {
					ui.SetDirty = false
				}
				saveUI(statePath, ui)
				pushSettings() // changed again meanwhile
			}
		case <-calForce:
			ui.CalAt = time.Time{}
			fetchCal()
		case wr := <-weatherResults:
			weatherBusy = false
			ui.WeatherAt = time.Now() // also after a failure: try again in 15 minutes
			if wr.err != nil {
				log.Println("weather:", wr.err)
				ui.WeatherErr = wr.err.Error()
			} else {
				ui.Weather, ui.WeatherErr = wr.w, ""
			}
			if view != nil && view.Kind == "weather" {
				draw(Clean)
			}

		case ic := <-iconResults:
			iconBusy = false
			if ic.err != nil {
				log.Println("icons:", ic.err)
				iconWait = time.Now().Add(5 * time.Minute)
			} else {
				ui.Icons[ic.machine], ui.IconsKey[ic.machine] = ic.icons, ic.key
				fetchIcons() // the other computer's, if they changed too
				if view != nil && view.Kind == "deck" {
					draw(Clean)
				}
			}

		case c := <-calResults:
			calBusy = false
			// Mark the month as loaded even on failure, so it waits 10 minutes before
			// trying again instead of retrying non-stop.
			ui.CalAt, ui.CalKey = time.Now(), c.key
			if c.err != nil {
				if ui.CalErr != c.err.Error() {
					log.Println("calendar:", c.err)
				}
				ui.Cal, ui.CalErr = nil, c.err.Error()
			} else {
				ui.Cal, ui.CalErr = c.evs, ""
			}
			fetchCal() // the month may have changed while this was loading
			draw(Clean)

		case t := <-taps:
			if view == nil {
				continue
			}
			x, y := scr.toLandscape(t.X, t.Y)
			ux, uy := scr.toLandscape(t.UpX, t.UpY)
			dx, dy := ux-x, uy-y
			// Press and hold on the Deck: add an app to favourites, or ask to remove a tile.
			if t.Hold >= 700*time.Millisecond && t.Hold < holdFor && view.Kind == "deck" && abs(dx) <= 40 && abs(dy) <= 40 && ui.ConfirmRemove < 0 {
				if tg := view.hit(x, y); tg != nil {
					switch tg.Act {
					case "deckapp":
						_, app, _ := strings.Cut(tg.Arg, "|")
						add := Target{Act: "favadd", Arg: tg.Arg, Show: tg.Show}
						ui.Note, ui.NoteAt = "Added "+app+" to favourites", time.Now()
						if desk != nil {
							apply(desk, add)
						}
						draw(Clean)
						seq++
						pending++
						go func() { sent <- sentResult{add, client.send(add)} }()
						continue
					case "deck":
						local(Target{Act: "favask", ID: tg.ID})
						continue
					}
				}
			}
			// Two or more fingers swiped sideways: the next or previous tab (Tasks, Deck,
			// Calendar, Weather). One-finger swipes keep their own jobs (pages, months, clocks).
			if t.Fingers >= 2 && abs(dx) > 100 && abs(dx) > abs(dy) && ui.Edit == nil && !ui.Menu &&
				(view.Kind == "tasks" || view.Kind == "deck" || view.Kind == "calendar" || view.Kind == "weather") {
				tabs := []string{"tasks", "deck", "calendar", "weather"}
				i := 0
				for j, tb := range tabs {
					if tb == ui.Tab {
						i = j
					}
				}
				if dx < 0 {
					i++ // swipe left: the next tab, like turning a page
				} else {
					i--
				}
				if i >= 0 && i < len(tabs) {
					local(Target{Act: "tab", Arg: tabs[i]})
				}
				continue
			}
			// Press and hold a task: edit its text.
			if t.Hold >= 700*time.Millisecond && t.Hold < holdFor && view.Kind == "tasks" && abs(dx) <= 40 && abs(dy) <= 40 && desk != nil {
				if tg := view.hit(x, y); tg != nil && tg.Act == "select" {
					openEditor(ui, desk, "edit", tg.ID)
					draw(Clean)
					continue
				}
			}
			if t.Hold >= holdFor {
				if y < barH && x > LW-120 {
					log.Println("clock held: leaving")
					leave()
					return 0
				}
				continue
			}
			// One finger right to left on a task row: a bin on that row (left to right: away).
			if t.Fingers < 2 && abs(dx) > 80 && abs(dx) > 2*abs(dy) && view.Kind == "tasks" && ui.DeleteAsk == 0 {
				ui.Swiped = 0
				if dx < 0 {
					if id := taskRowAt(view, x, y); id != 0 {
						ui.Swiped = id
					}
				}
				draw(Clean)
				continue
			}
			// Swipes: up/down turns task pages, left/right changes calendar month.
			if abs(dy) > 90 && abs(dy) > 2*abs(dx) && view.Kind == "tasks" {
				local(Target{Act: "page", Arg: map[bool]string{true: "1", false: "-1"}[dy < 0]})
				continue
			}
			if abs(dx) > 120 && abs(dx) > 2*abs(dy) && view.Kind == "standby" {
				local(Target{Act: "clockstyle", Arg: map[bool]string{true: "1", false: "-1"}[dx < 0]})
				continue
			}
			if abs(dx) > 120 && abs(dx) > 2*abs(dy) && view.Kind == "deck" {
				local(Target{Act: "deckpage", Arg: map[bool]string{true: "1", false: "-1"}[dx < 0]})
				continue
			}
			if abs(dy) > 90 && abs(dy) > 2*abs(dx) && view.Kind == "calendar" && ui.CalTimeline && !ui.CalHidden && x > sideX {
				local(Target{Act: "calhour", Arg: map[bool]string{true: "3", false: "-3"}[dy < 0]})
				continue
			}
			if abs(dx) > 120 && abs(dx) > 2*abs(dy) && view.Kind == "calendar" {
				local(Target{Act: "month", Arg: map[bool]string{true: "1", false: "-1"}[dx < 0]})
				continue
			}
			// Anything with arrows turns with a swipe too: left / right for pages (Settings ›
			// Calendar's calendars, Updates' notes), up / down in the What's new box; with no
			// page to turn, a swipe right goes Back (Settings, the cover screen).
			if side, up := abs(dx) > 100 && abs(dx) > 2*abs(dy), abs(dy) > 90 && abs(dy) > 2*abs(dx); side || up {
				if tg := swipeTarget(view, dx, dy, side); tg != nil {
					local(*tg)
					continue
				}
				if side && dx > 0 && (view.Kind == "settings" && !ui.NotesModal && !ui.ServerPick && !ui.ClearAsk) {
					local(Target{Act: "settings"})
					continue
				}
				if side && dx > 0 && view.Kind == "cover" {
					local(Target{Act: "cover"})
					continue
				}
			}
			// A finger that moved isn't a tap: never let a stray swipe press a button
			// (on the Deck that would open an app on the computer).
			if abs(dx) > 40 || abs(dy) > 40 {
				continue
			}
			tg := view.hit(x, y)
			if ui.Swiped != 0 && (tg == nil || tg.Act != "binask") { // a bin is showing: any other tap just hides it
				ui.Swiped = 0
				draw(Clean)
				continue
			}
			if tg == nil {
				continue
			}
			target := *tg
			if target.Act == "quitapp" {
				log.Println("exit to kindle: leaving")
				leave()
				return 0
			}
			if target.Act == "restartapp" { // run.sh starts the app again, as after an update
				log.Println("restart asked for")
				return exitUpdate
			}
			if target.Act == "update" {
				if installUpdate() {
					return exitUpdate
				}
				draw(Flash) // couldn't install: carry on with this version
				continue
			}
			if (target.Act == "evadd" || target.Act == "delevent" || target.Act == "setplace") && desk != nil {
				fb := image.NewGray(view.Img.Rect)
				copy(fb.Pix, view.Img.Pix)
				invertRect(fb, target.Show)
				scr.Show(fb, Fast)
				switch target.Act {
				case "evadd": // show it straight away; the calendar reloads once it's saved
					if ui.Edit == nil {
						continue
					}
					var ev Event
					target, ev = eventTarget(ui.Edit)
					ui.Cal = append(ui.Cal, ev)
					ui.Edit = nil
				case "delevent":
					var keep []Event
					for _, e := range ui.Cal {
						if e.ID != target.ID {
							keep = append(keep, e)
						}
					}
					ui.Cal, ui.Event, ui.ConfirmEvent = keep, -1, 0
				case "setplace":
					ui.Edit = nil
					if ui.Weather != nil {
						if p := strings.SplitN(target.Arg, "|", 3); len(p) == 3 {
							ui.Weather.Place = p[2]
						}
					}
				}
				draw(Clean)
				seq++
				pending++
				go func(t Target) {
					err := client.send(t)
					sent <- sentResult{t, err}
					switch t.Act {
					case "setplace": // the new place's forecast, now
						w, werr := client.Weather()
						weatherResults <- weatherFetched{w, werr}
					default:
						calForce <- true
					}
				}(target)
				continue
			}
			// Moving between the cloud and the Mac is the Mac's job: ask it (it switches, copies the
			// data and leaves a note the Kindle follows on Auto). Picking Cloud while on the Mac too.
			if target.Act == "serverask" || (target.Act == "serverset" && target.Arg == "cloud" && ui.ServerLocal) {
				to := "local"
				if target.Act == "serverset" {
					to = "cloud"
				}
				ui.ServerNote, askedAt = "Asking your Mac to switch…", time.Now()
				ui.ServerManual = false // follow its note
				saveUI(statePath, ui)
				draw(Clean)
				go func() {
					if err := client.Post("/api/kindle/switch", map[string]string{"to": to}); err != nil {
						log.Println("ask to switch:", err)
					}
				}()
				continue
			}
			if target.Act == "serverset" {
				name, addr := "", cloudURL
				if target.Arg != "cloud" {
					name, addr, _ = strings.Cut(target.Arg, "|")
				}
				if addr == "" {
					ui.ServerNote = "The cloud address isn't known on this Kindle."
					draw(Clean)
					continue
				}
				ui.ServerNote = "Connecting…"
				draw(Clean)
				go func(name, addr string) {
					err := tryServer(client, addr)
					key := ""
					if err == errWrongKey && isLocalServer(addr) { // a computer we haven't met: ask it to let us in
						key, err = pairWith(client, addr, func(code string) { pairCh <- code })
						pairCh <- ""
						if err == nil {
							client.SetKey(key)
						}
					}
					if err == nil {
						cloud := ""
						if !isLocalServer(addr) {
							cloud = addr
						}
						err = saveServer(configPath, addr, name, cloud, key)
					}
					serverCh <- serverSwitched{name, addr, err}
				}(name, addr)
				continue
			}
			if target.Act == "kbdone" && ui.Edit != nil && ui.Edit.Mode == "event" {
				if strings.TrimSpace(ui.Edit.Text) != "" {
					ui.Edit.Step = 1
					draw(Clean)
				}
				continue
			}
			if target.Act == "kbdone" && ui.Edit != nil && ui.Edit.Mode == "place" {
				if q := strings.TrimSpace(ui.Edit.Text); q != "" && !ui.Edit.Searching {
					ui.Edit.Searching, ui.Edit.SearchErr, ui.Edit.Places = true, "", nil
					draw(Clean)
					go func() {
						ps, err := client.Places(q)
						placeResults <- placesFetched{q, ps, err}
					}()
				}
				continue
			}
			if target.Act == "kbdone" && ui.Edit != nil && desk != nil {
				fb := image.NewGray(view.Img.Rect)
				copy(fb.Pix, view.Img.Pix)
				invertRect(fb, target.Show)
				scr.Show(fb, Fast)
				target = editorDone(ui.Edit, desk)
				ui.Edit = nil
				if target.Act == "" {
					draw(Clean)
					continue
				}
				if target.Act == "addtask" && target.ID > 0 {
					ui.Collapsed[target.ID] = false // show the new subtask
				}
				apply(desk, target)
				draw(Clean)
				sendOrQueue(target)
				continue
			}
			if isLocal(target.Act) {
				local(target)
				continue
			}
			ui.Menu = false // picking Work / Home / Auto closes the menu
			if target.Act == "deltask" {
				ui.DeleteAsk = 0
				if ui.Selected == target.ID {
					ui.Selected = 0
				}
			}
			if target.Act == "join" { // the heads-up goes once you've joined
				ui.Dismissed[ui.HeadsUpKey] = true
			}
			if target.Act == "music" && strings.HasPrefix(target.Arg, "volume:") {
				ui.VolumeAt = time.Now() // still adjusting: keep the volume control open
			} else if target.Act == "music" && strings.Contains(target.Arg, ":") {
				ui.Outputs = false // picked where the sound goes: back to the song
			}
			if target.Act == "cleartasks" {
				ui.ClearAsk, ui.Selected = false, 0
			}
			if target.Act == "pomo" && (target.Arg == "start" || target.Arg == "break") {
				ui.Focus = true // starting a focus or a break opens the focus screen
			}
			if target.Act == "pomo" && target.Arg == "stop" {
				ui.Focus = false
			}
			if target.Act == "favremove" {
				if ui.ConfirmRemove >= 0 {
					scr.Forget() // the box covered the tiles: redraw cleanly
				}
				ui.ConfirmRemove = -1
				ui.Note, ui.NoteAt = "Removed from favourites", time.Now()
			}
			if target.Act == "favadd" {
				_, app, _ := strings.Cut(target.Arg, "|")
				ui.Note, ui.NoteAt = "Added "+app+" to favourites", time.Now()
			}
			// 1. Instant feedback: invert what was tapped with the fastest waveform.
			fb := image.NewGray(view.Img.Rect)
			copy(fb.Pix, view.Img.Pix)
			invertRect(fb, target.Show)
			scr.Show(fb, Fast)
			// 2. Show the result straight away where we can predict it.
			if desk != nil && apply(desk, target) {
				draw(Clean)
			}
			// 3. Tell the backend (or keep it for later), then fetch the real state.
			sendOrQueue(target)
			if target.Act == "music" { // a skip or play/pause: look again a few times for the new song and cover
				go func() {
					for _, d := range []time.Duration{1500, 2000, 2500} {
						time.Sleep(d * time.Millisecond)
						kick <- true
					}
				}()
			}

		case s := <-sent:
			pending--
			if s.err != nil {
				log.Println("send:", s.err)
				st.Offline = true
				if canWait(s.t.Act) && netError(s.err) { // keep it; it goes when the server is back
					t := outbox.add(s.t)
					if desk != nil && t.Act == "addtask" {
						apply(desk, t)
					}
					st.Waiting = len(outbox.Items)
					draw(Clean)
				}
			}
			if pending == 0 {
				nextPoll = time.Now() // refetch on the next tick, or right away if idle
				fetch()
			}

		case <-power:
			log.Println("power button: leaving")
			leave()
			return 0

		case s := <-sigs:
			log.Println("signal:", s)
			leave()
			return 0
		}
	}
}

// taskRowAt: the task whose row in the list is under (x, y), 0 if none (the pinned card
// above the TODAY line doesn't count).
func taskRowAt(v *View, x, y int) int {
	top := 0
	for _, t := range v.Targets {
		if t.Act == "addnew" && t.Arg == "add" {
			top = t.Show.Max.Y
		}
	}
	if y <= top {
		return 0
	}
	p := image.Pt(x, y)
	for _, t := range v.Targets {
		if (t.Act == "select" || t.Act == "toggle" || t.Act == "pin" || t.Act == "collapse") && t.ID != 0 && p.In(t.R) {
			return t.ID
		}
	}
	return 0
}

// swipeTarget: the arrow a swipe stands for on this screen (nil if none): a swipe left is
// the next page (the "1" arrow), right the previous; up scrolls on, down back.
func swipeTarget(v *View, dx, dy int, side bool) *Target {
	acts, arg := []string{"notespage", "calpage"}, map[bool]string{true: "1", false: "-1"}[dx < 0]
	if !side {
		acts, arg = []string{"notesscroll"}, map[bool]string{true: "1", false: "-1"}[dy < 0]
	}
	for i := range v.Targets {
		for _, a := range acts {
			if v.Targets[i].Act == a && v.Targets[i].Arg == arg {
				return &v.Targets[i]
			}
		}
	}
	return nil
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// leave blanks the panel and wakes the Kindle's own UI, which redraws itself.
func leave() {
	clearScreen()
	resumeKindleUI()
}
