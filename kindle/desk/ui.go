package main

// Lays out one landscape screen (1024 × 758) from the backend's /api/desk data and the
// Kindle-side state (tab, dark mode, selection...). Returns the picture plus its tap targets.

import (
	"fmt"
	"golang.org/x/image/font"
	"image"
	"math"
	"strconv"
	"strings"
	"time"
)

const (
	LW      = 1024 // the layout's width; its height LH is in gfx.go (it follows the panel)
	barH    = 72
	sideX   = 664 // left column | side column divider
	pad     = 28
	holdFor = 3 * time.Second // hold the clock this long to go back to the Kindle
)

type Target struct {
	R    image.Rectangle // tap area
	Show image.Rectangle // area inverted as instant feedback
	Act  string          // see main.go: local actions change the view, the rest go to the backend
	ID   int
	Arg  string
	Temp int `json:",omitempty"` // a task added while offline: its temporary id (see outbox.go)
}

type View struct {
	Img     *image.Gray
	Kind    string // tasks, deck, calendar, night, message, connecting
	Targets []Target
	Splits  []int // x positions changed areas aren't merged across (a sidebar's edge)
	SplitY  []int // y positions likewise (the header's bottom edge)
}

func (v *View) hit(x, y int) *Target {
	p := image.Pt(x, y)
	for i := len(v.Targets) - 1; i >= 0; i-- { // later (smaller, on top) targets win
		if p.In(v.Targets[i].R) {
			return &v.Targets[i]
		}
	}
	return nil
}

func (v *View) add(t Target) {
	if t.Show.Empty() {
		t.Show = t.R
	}
	v.Targets = append(v.Targets, t)
}

// Status is what the app knows besides the backend's data.
type Status struct {
	Now      time.Time // local time, already corrected by the server's clock and offset
	Offline  bool
	Why      string    // why the last fetch failed, in plain words
	OnSince  time.Time // connected without a break since then
	LastOK   time.Time
	Server   string
	Battery  int // Kindle charge 0–100, or -1 when unknown
	Charging bool
	Light    int // front light 0–24, or -1 when unknown
	Waiting  int // changes made offline, not sent yet (outbox.go)
}

// UI is the Kindle-side state: what's being looked at, not what's stored on the backend.
type UI struct {
	Tab           string // tasks, deck, calendar
	Dark          bool
	Selected      int // task whose description and move buttons are open
	Collapsed     map[int]bool
	Page          int
	Weather       *Weather
	WeatherAt     time.Time
	WeatherErr    string
	WeatherDay    int                               // day shown on the Weather tab: 0 = now, 1… = later this week
	Standby       bool                              // full-screen clock (tap the clock; tap anywhere to leave)
	ClockStyle    int                               // standby clock style (index into clockStyles)
	LightMenu     bool                              // the brightness panel under the battery
	StandbyCtl    bool                              // the control bar on the standby clock
	Clock12       bool                              // standby and night clocks: 12-hour time
	NoQuote       bool                              // standby clock: hide the quote line
	NoEvent       bool                              // standby clock: hide the next calendar event
	StandbyAt     time.Time                         // when that bar was opened (it hides after 10 s)
	UpdateReady   bool                              // a newer build is downloaded; the header offers to install it
	Icons         map[string]map[string]*image.Gray // app icons for deck tiles: machine → label → icon
	IconsKey      map[string]string                 // machine → icons version held
	DeckProfile   string                            // deck picked with the MacBook | PC switch ("" = current profile's)
	DeckPage      int
	DeckAll       bool            // the Deck shows every app on the computer instead of the favourites
	DeckEdit      bool            // the Deck is being edited (remove, move, add favourites)
	Focus         bool            // the full focus (Pomodoro) screen is open
	Widget        string          // the Tasks page panel: day, weather, music, info
	SideHidden    bool            // the panel is collapsed
	CalHidden     bool            // the Calendar tab's day panel is collapsed
	WeatherHidden bool            // the Weather tab's week list is collapsed
	ServerLocal   bool            // talking to a computer on the Wi-Fi rather than the cloud
	ServerManual  bool            // Settings › Server › Find automatically: Off (stays on the one picked)
	MacSeen       bool            // the Mac has the helper (desk `computers`): it can be asked to run Desk Buddy
	ServerName    string          // that computer's name
	ServerAddr    string          // and address
	ServerPick    bool            // Settings: "Where's Desk Buddy running?" is open
	Finding       bool            // …looking on the Wi-Fi
	Found         []FoundServer   // …computers found
	ServerNote    string          // …"Couldn't connect: …"
	PairCode      string          // …waiting for Allow in the helper: the code it shows
	NoCloud       bool            // …this Kindle has no cloud address (a local-only setup): no Cloud row
	UpdateNotes   []NoteEntry     // what the waiting update brings (patchnotes.go)
	NotesPage     int             // Settings › Updates: the page of What's new showing
	NotesModal    bool            // …"What's new in the update" is open
	NotesScroll   int             // …the first line showing in it
	CalListPage   int             // Settings › Calendar: the page of calendars showing
	SettingsTab   string          // the Settings section open on the right (screen, focus, music, tasks, server, kindle)
	OfflineInfo   bool            // the offline icon was tapped: say what it means
	AutoDark      bool            // Theme: Auto (dark when the Mac is, else sunset to sunrise)
	NightLight    int             // front light while Auto is dark (-1: leave it alone)
	DayLight      int             // the light to go back to in the morning (-1: none saved)
	NoHeadsUp     bool            // meeting heads-up turned off
	HeadsUpMin    int             // minutes before a meeting (0 = 5)
	NoUpdShow     bool            // Settings › Updates › Show on main screen: Off
	UpdEvery      int             // …Check every N hours (0 = 12)
	SetAt         float64         // ksync.go: when the synced settings last changed (server time)
	SetDirty      bool            // …changed here, not sent yet
	SetKey        string          // …as they were last saved (to notice changes)
	ShowMusicLogo bool            // Settings › Music › Music app logo: On (a Spotify / Apple mark; off by default)
	VolumeOpen    bool            // the volume panel is showing
	VolumeAt      time.Time       // …since (it hides again after a few seconds)
	VolumeAnchor  image.Rectangle // …the speaker it drops down from
	Dismissed     map[string]bool // heads-ups dismissed (by eventKey)
	HeadsUpKey    string          // the heads-up showing now
	Refresh       int             // Screen refresh: 0 gentle, 1 normal (default), 2 clean
	Rotation      int             // 90 (chin on the left) or 270 (chin on the right)
	Swiped        int             // the task swiped right to left: its row shows a bin (0: none)
	DeleteAsk     int             // "Delete this task?" for this task (0: not asking)
	Tour          int             // the first-run tour's page (1–3), 0 when not showing
	ClearAsk      bool            // Settings: "Clear the Work list?" is open
	ConfirmEvent  int             // a Kindle-made event being asked about ("Delete this event?"), or 0
	CalTimeline   bool            // the day panel shows a timeline instead of a list
	CalHour       int             // first hour on that timeline (-1: pick one)
	CalHourShown  int             // the hour it actually starts at
	Settings      bool            // the Settings screen is open
	UpdateNote    string          // "You're up to date" etc. on the Settings screen
	Art           *image.Gray
	ArtKey        string
	ArtBig        *image.Gray // the cover enlarged and dithered for the cover screen
	ArtBigKey     string
	Edit          *Editor     // the keyboard is open (adding or editing a task)
	Cover         bool        // the full-screen album cover is open
	Outputs       bool        // the cover screen's "Play on" list is open
	CoverFull     bool        // …filling the whole screen (cropped), controls on the right
	CoverBare     bool        // …with the controls hidden
	ArtFull       *image.Gray // the cover enlarged to the screen width (not dithered)
	ArtFullKey    string
	ArtSmall      *image.Gray // the cover at the Music widget's size (a Sharp cover is bigger)
	ArtSmallKey   string
	ConfirmRemove int // favourite tile being asked about ("Remove …?"), or -1
	NoteAt        time.Time
	Note          string // a short message shown in the deck after tapping a placeholder
	Menu          bool   // the Work / Home / Auto menu is open
	Full          bool   // the backend has the newer features (move, calendar, deck); the old Mac one doesn't

	Month    time.Time // first day of the month shown in the calendar
	Day      time.Time // selected day
	Event    int       // open event on that day, or -1
	EventKey string    // event to open once the month loads (tapped in the Day widget)
	Cal      []Event   // events around Month
	CalKey   string    // which month and profile Cal holds
	CalAt    time.Time
	CalErr   string
}

func render(d *Desk, st Status, ui *UI) *View {
	v := &View{Img: newCanvas(LW, LH)}
	switch {
	case d == nil:
		v.Kind = "connecting"
		connecting(v, st)
		if ui.ServerPick { // first setup: finding the computer, before there's anything to show
			serverModal(v, ui)
		}
	case d.Screen == "message":
		v.Kind = "message"
		message(v, d)
	case d.Screen == "night":
		v.Kind = "night"
		night(v, d, st, ui)
	case ui.Settings:
		v.Kind = "settings"
		v.Splits = []int{260} // the sidebar: its rows redraw apart from the options
		v.SplitY = []int{barH + 1}
		settingsView(v, d, st, ui)
	case ui.Focus && d.Pomodoro.active() && !d.Pomodoro.finished(st.Now.Unix()):
		v.Kind = "focus"
		focusView(v, d, st)
	case ui.Cover:
		v.Kind = "cover"
		coverView(v, d, st, ui)
	case ui.Standby:
		v.Kind = "standby"
		standby(v, d, st, ui)
		if ui.StandbyCtl {
			standbyControls(v, st, ui)
		}
	default:
		ui.Full = d.Deck != nil
		v.SplitY = []int{barH + 1} // the header redraws apart from the page under it
		header(v, d, st, ui)
		switch ui.Tab {
		case "deck":
			v.Kind = "deck"
			deckView(v, d, ui)
			if ui.ConfirmRemove >= 0 {
				confirmRemove(v, d, ui)
			}
		case "calendar":
			v.Kind = "calendar"
			if !ui.CalHidden {
				v.Splits = []int{sideX} // month | day panel
			}
			calendarView(v, d, st, ui)
		case "weather":
			v.Kind = "weather"
			if !ui.WeatherHidden {
				v.Splits = []int{sideX} // the day | the week
			}
			weatherView(v, st, ui)
		default:
			v.Kind = "tasks"
			if !ui.SideHidden {
				v.Splits = []int{sideX} // tasks | panel
			}
			tasksColumn(v, d, ui, st.Now.Unix())
			if !ui.SideHidden {
				sidePanel(v, d, st, ui)
			}
		}

		if ui.LightMenu {
			lightPanel(v, st, ui)
		}
		if ui.VolumeOpen {
			volumePanel(v, d.Music, ui.VolumeAnchor)
		}
		if d.Pomodoro.finished(st.Now.Unix()) {
			timesUp(v, d.Pomodoro) // a focus or break just ended: this takes over the screen
			v.Kind = "timesup"
		}
		if ui.Menu {
			// Refresh and dark mode stay usable while the menu is open: put them back on top.
			var keep []Target
			for _, t := range v.Targets {
				if t.Act == "refresh" || t.Act == "dark" {
					keep = append(keep, t)
				}
			}
			profileMenu(v, d)
			v.Targets = append(v.Targets, keep...)
		}
		if ui.Edit != nil {
			v.Kind = "edit"
			editorView(v, d, ui)
		}
	}
	if ui.Tour > 0 && v.Kind == "tasks" {
		tourOverlay(v, d, ui)
		if ui.Dark {
			invertRect(v.Img, v.Img.Rect)
		}
		return v
	}
	if ui.DeleteAsk != 0 && d != nil && v.Kind == "tasks" {
		deleteModal(v, d, ui.DeleteAsk)
	}
	ui.HeadsUpKey = ""
	if d != nil && ui.Edit == nil && ui.DeleteAsk == 0 && !ui.Settings && v.Kind != "connecting" {
		if e := headsUpEvent(d, ui, st.Now); e != nil {
			headsUp(v, e, ui, st)
		}
	}
	if v.Kind == "tasks" || v.Kind == "deck" || v.Kind == "calendar" || v.Kind == "weather" || v.Kind == "settings" {
		if !ui.LightMenu && !ui.VolumeOpen && !ui.Menu && !ui.NotesModal && !ui.ServerPick && !ui.ClearAsk && ui.HeadsUpKey == "" {
			decorate(v) // the theme's touches (themes.go), not over an open panel
		}
	}
	if ui.OfflineInfo && st.Offline && v.Kind != "standby" && v.Kind != "focus" {
		offlineInfo(v, ui, st)
	}
	if ui.Dark {
		invertRect(v.Img, v.Img.Rect)
	}
	return v
}

// ---------- header ----------

func header(v *View, d *Desk, st Status, ui *UI) {
	img := v.Img
	fillRect(img, image.Rect(0, barH, LW, barH+2), ink)
	// The profile name is the switch: tap it for the Work / Home / Auto menu.
	pf := face("serifB", 28)
	// Under the name: "Connected to cloud" (or "to local") in grey ("Auto · " first when on Auto),
	// or "Offline since 18:40" in bold.
	rf, rc, reason := face("ui", 14), grey2, reasonText(d)
	if !st.OnSince.IsZero() { // where it's connected: the cloud, or a computer at home
		on := "Connected to cloud"
		if isLocalServer(st.Server) {
			on = "Connected to local"
		}
		if reason != "" {
			on = reason + " · " + on
		}
		reason = on
	}
	if st.Offline {
		rf, rc, reason = face("uiBold", 14), ink, "Offline · retrying"
		if !st.LastOK.IsZero() {
			reason = "Offline since " + st.LastOK.In(st.Now.Location()).Format("15:04")
		}
		if st.Waiting == 1 {
			reason += " · 1 change waiting"
		} else if st.Waiting > 1 {
			reason += fmt.Sprintf(" · %d changes waiting", st.Waiting)
		}
	}
	// The name, with a small line under it ("Auto", "Offline since 18:20"); with nothing
	// to say there (Work or Home chosen by hand), the name sits in the middle of the bar.
	ny := 38
	if reason == "" {
		ny = barH/2 + capHeight(pf)/2
	}
	name := listName(d)
	if name == "" {
		name = "Tasks"
	}
	nx := drawText(img, pf, pad, ny, name, ink)
	chevron(img, float64(nx+14), float64(ny-capHeight(pf)/2), 5, "down", ink)
	if reason != "" {
		tx := pad
		cy := float64(60 - capHeight(rf)/2)
		switch {
		case st.Offline: // a small ✕
			line(img, float64(pad), cy-3.5, float64(pad+7), cy+3.5, 1.8, ink)
			line(img, float64(pad+7), cy-3.5, float64(pad), cy+3.5, 1.8, ink)
			tx += 13
		}
		drawText(img, rf, tx, 60, ellipsize(rf, reason, 190-(tx-pad)), rc)
	}
	v.add(Target{R: image.Rect(0, 0, 222, barH), Show: image.Rect(pad-8, ny-30, nx+28, ny+8), Act: "menu"})

	tabBar(v, ui)

	clk := face("serifB", 30)
	clockX := drawRight(img, clk, LW-pad, barH/2+capHeight(clk)/2, st.Now.Format("15:04"), ink)
	v.add(Target{R: image.Rect(clockX-8, 0, LW, barH), Show: image.Rect(clockX-6, 14, LW-pad+6, 58), Act: "standby"})

	dark := image.Rect(clockX-16-44, 14, clockX-16, 58)
	contrastIcon(img, float64(dark.Min.X+dark.Max.X)/2, 36, 11)
	v.add(Target{R: image.Rect(dark.Min.X, 0, dark.Max.X, barH), Show: dark, Act: "dark"})

	ref := image.Rect(dark.Min.X-6-44, 14, dark.Min.X-6, 58)
	refreshIcon(img, float64(ref.Min.X+ref.Max.X)/2, 36, 10)
	v.add(Target{R: image.Rect(ref.Min.X, 0, ref.Max.X, barH), Show: ref, Act: "refresh"})

	// While music plays: a speaker right after the battery (tap: the volume panel)
	bx := ref.Min.X - 14
	if m := d.Music; m != nil && m.Running && m.Title != "" && m.Volume != nil {
		sp := image.Rect(ref.Min.X-6-44, 14, ref.Min.X-6, 58)
		speakerGlyph(img, float64(sp.Min.X+sp.Max.X)/2+2, 36, *m.Volume > 0, ink)
		v.add(Target{R: image.Rect(sp.Min.X, 0, sp.Max.X, barH), Show: sp, Act: "volopen"})
		ui.VolumeAnchor = sp
		bx = sp.Min.X - 14
	}
	// Battery (tap it for brightness and light/dark).
	if st.Battery >= 0 {
		batteryIcon(img, bx, 36, st.Battery, st.Charging)
	} else {
		sunGlyph(img, float64(bx-14), 36, ink)
	}
	v.add(Target{R: image.Rect(bx-92, 0, bx+4, barH), Show: image.Rect(bx-88, 16, bx+2, 56), Act: "lightmenu"})
	if st.Offline { // only while the server can't be reached; tap: what that means
		r := image.Rect(bx-100-48, 14, bx-100, 58)
		offlineGlyph(img, float64(r.Min.X+r.Max.X)/2, 36, ink)
		v.add(Target{R: image.Rect(r.Min.X, 0, r.Max.X, barH), Show: r, Act: "offlineinfo"})
		bx -= 56
	}
	bx = pomoHeader(v, d.Pomodoro, st.Now.Unix(), bx-100) + 100
	if ui.UpdateReady && !ui.NoUpdShow {
		f := face("uiBold", 15)
		r := image.Rect(bx-100-textW(f, "Update")-28, 18, bx-100, 54)
		fillRoundRect(img, r, 18, ink)
		drawCentered(img, f, (r.Min.X+r.Max.X)/2, 36+capHeight(f)/2, "Update", paper)
		v.add(Target{R: r.Inset(-6), Show: r, Act: "showupdates"})
	}

}

// tabStyle picks how Tasks | Deck | Calendar look (the -tabs preview flag compares them).
var tabStyle = "icons"

func tabBar(v *View, ui *UI) {
	img := v.Img
	tabs := []struct {
		id, name string
		glyph    func(img *image.Gray, cx, cy float64, c uint8)
	}{{"tasks", "Tasks", tasksGlyph}, {"deck", "Deck", deckGlyph}, {"calendar", "Calendar", calendarGlyph}, {"weather", "Weather", weatherGlyph}}
	tf := face("uiBold", 16)
	switch tabStyle {
	case "icons": // three square icon buttons, current one filled
		for i, t := range tabs {
			r := image.Rect(232+i*60, 13, 232+i*60+52, 59)
			c := buttonFace(img, r, 8, ui.Tab == t.id)
			t.glyph(img, float64(r.Min.X+r.Max.X)/2, 36, c)
			v.add(Target{R: image.Rect(r.Min.X-4, 0, r.Max.X+4, barH), Show: r, Act: "tab", Arg: t.id})
		}
	case "underline": // icon + word, a thick bar under the current one
		x := 232
		for _, t := range tabs {
			w := 22 + 8 + textW(tf, t.name) + 28
			r := image.Rect(x, 0, x+w, barH)
			t.glyph(img, float64(x+14+11), 36, ink)
			drawText(img, tf, x+14+30, 36+capHeight(tf)/2, t.name, ink)
			if ui.Tab == t.id {
				fillRect(img, image.Rect(x+6, barH-6, x+w-6, barH), ink)
			}
			v.add(Target{R: r, Show: image.Rect(x+4, 12, x+w-4, 60), Act: "tab", Arg: t.id})
			x += w
		}
	default: // segmented: one box, three equal cells with icon + word, current one filled
		cw := 448 / len(tabs)
		seg := image.Rect(232, 13, 232+len(tabs)*cw, 59)
		for i, t := range tabs {
			cell := image.Rect(seg.Min.X+i*cw, seg.Min.Y, seg.Min.X+(i+1)*cw, seg.Max.Y)
			c := ink
			if ui.Tab == t.id {
				fillRoundRect(img, seg, 8, ink, cell)
				c = paper
			}
			if i > 0 {
				fillRect(img, image.Rect(cell.Min.X-1, cell.Min.Y, cell.Min.X+1, cell.Max.Y), ink)
			}
			w := 22 + 8 + textW(tf, t.name)
			x0 := (cell.Min.X+cell.Max.X)/2 - w/2
			t.glyph(img, float64(x0+11), 36, c)
			drawText(img, tf, x0+30, 36+capHeight(tf)/2, t.name, c)
			v.add(Target{R: image.Rect(cell.Min.X, 0, cell.Max.X, barH), Show: cell, Act: "tab", Arg: t.id})
		}
		strokeRoundRect(img, seg, 8, 2, ink)
	}
}

// Tab icons, about 22 px square, centred on (cx, cy).
func tasksGlyph(img *image.Gray, cx, cy float64, c uint8) {
	for i := -1; i <= 1; i++ {
		y := cy + float64(i)*6.5
		fillCircle(img, cx-7, y, 2.4, c)
		line(img, cx-2, y, cx+9, y, 2.2, c)
	}
}

func deckGlyph(img *image.Gray, cx, cy float64, c uint8) {
	for _, o := range [][2]float64{{-5, -5}, {5, -5}, {-5, 5}, {5, 5}} {
		x, y := int(cx+o[0]), int(cy+o[1])
		fillRoundRect(img, image.Rect(x-4, y-4, x+4, y+4), 2, c)
	}
}

func calendarGlyph(img *image.Gray, cx, cy float64, c uint8) {
	r := image.Rect(int(cx)-10, int(cy)-8, int(cx)+10, int(cy)+10)
	strokeRoundRect(img, r, 3, 2, c)
	fillRect(img, image.Rect(r.Min.X+1, r.Min.Y+1, r.Max.X-1, r.Min.Y+5), c)
	line(img, cx-5, cy-11, cx-5, cy-6, 2, c)
	line(img, cx+5, cy-11, cx+5, cy-6, 2, c)
	for _, o := range [][2]float64{{-5, 1}, {0, 1}, {5, 1}, {-5, 6}, {0, 6}} {
		fillCircle(img, cx+o[0], cy+o[1], 1.4, c)
	}
}

// reasonText is the small line under the profile name: "Auto" when it switches by itself.
func reasonText(d *Desk) string {
	if d.Mode == "auto" && !searching(d) { // one computer: nothing switches, so no "Auto"
		return "Auto"
	}
	return ""
}

// modeDetail explains the current choice, shown under it in the menu.
func modeDetail(d *Desk) string {
	switch {
	case d.Mode != "auto":
		return "Chosen by you · back to Auto at 4am"
	case strings.HasPrefix(d.Reason, "auto · "):
		return "Now: " + strings.TrimPrefix(d.Reason, "auto · ")
	}
	return "The computer you're using, else the last one"
}

// profileMenu: Mac / Windows / Auto (with one list: just Send feedback and Settings), dropped down from the profile name. Tapping anywhere
// else closes it.
func profileMenu(v *View, d *Desk) {
	img := v.Img
	v.add(Target{R: img.Rect, Act: "menu"})
	pc := pcName(d)
	opts := []struct{ mode, name, desc string }{ // one list per computer: which one to show
		{"work", "Mac", "Tasks for your Mac"},
		{"home", pc, "Tasks for your " + pc + " PC"},
		{"auto", "Auto", "The computer you're using, else the last one"},
	}
	if oneList(d) || searching(d) {
		opts = nil // one list, or one computer: nothing to pick
	}
	extra := 0
	if searching(d) && !oneList(d) {
		extra = 96 // "Searching for other devices…"
	}
	box := image.Rect(pad-8, barH-4, pad-8+400, barH-4+len(opts)*76+16+70+62+62+extra)
	fillRoundRect(img, box.Inset(-3), 12, paper)
	strokeRoundRect(img, box, 10, 3, ink)
	tf, sf := face("serifB", 22), face("ui", 15)
	for i, o := range opts {
		r := image.Rect(box.Min.X+8, box.Min.Y+8+i*76, box.Max.X-8, box.Min.Y+8+(i+1)*76)
		on := d.Mode == o.mode
		if on && o.mode == "auto" { // Auto says what it's following now; Mac / Windows keep their own line
			o.desc = modeDetail(d)
		}
		cx, cy := float64(r.Min.X+26), float64(r.Min.Y+38)
		ringCircle(img, cx, cy, 12, 2.5, ink)
		if on {
			fillCircle(img, cx, cy, 6.5, ink)
		}
		drawText(img, tf, r.Min.X+52, r.Min.Y+34, o.name, ink)
		drawText(img, sf, r.Min.X+52, r.Min.Y+58, o.desc, grey2)
		if i > 0 {
			fillRect(img, image.Rect(r.Min.X+52, r.Min.Y, r.Max.X-12, r.Min.Y+1), grey4)
		}
		v.add(Target{R: r, Show: r.Inset(2), Act: "mode", Arg: o.mode})
	}
	if extra > 0 { // only this computer so far: say we're looking, and how to add another
		r := image.Rect(box.Min.X+8, box.Min.Y+8, box.Max.X-8, box.Min.Y+8+extra)
		searchGlyph(img, float64(r.Min.X+26), float64(r.Min.Y+34), ink)
		drawText(img, tf, r.Min.X+52, r.Min.Y+40, "Searching for other devices…", ink)
		for i, l := range wrap(sf, "Install Desk Buddy on another computer to connect it", r.Max.X-r.Min.X-64, 2) {
			drawText(img, sf, r.Min.X+52, r.Min.Y+64+i*20, l, grey2)
		}
	}
	// a separator, then Send feedback, Settings and Exit to Kindle
	sy := box.Min.Y + 8 + len(opts)*76 + 8 + extra
	if len(opts) > 0 || extra > 0 {
		fillRect(img, image.Rect(box.Min.X+12, sy, box.Max.X-12, sy+2), ink)
	} else {
		sy -= 14
	}
	fb := image.Rect(box.Min.X+8, sy+6, box.Max.X-8, sy+66)
	feedbackGlyph(img, float64(fb.Min.X+26), float64(fb.Min.Y+28), ink)
	drawText(img, tf, fb.Min.X+52, fb.Min.Y+30, "Send feedback", ink)
	drawText(img, sf, fb.Min.X+52, fb.Min.Y+52, "Ideas, bugs, feature requests", grey2)
	v.add(Target{R: fb, Show: fb.Inset(2), Act: "about"}) // opens Settings › About
	sy += 62
	fillRect(img, image.Rect(fb.Min.X+52, sy+4, box.Max.X-20, sy+5), grey4)
	r := image.Rect(box.Min.X+8, sy+6, box.Max.X-8, sy+62)
	gearGlyph(img, float64(r.Min.X+26), float64(r.Min.Y+28), ink)
	drawText(img, tf, r.Min.X+52, r.Min.Y+36, "Settings", ink)
	v.add(Target{R: r, Show: r.Inset(2), Act: "settings"})
	sy += 62
	fillRect(img, image.Rect(fb.Min.X+52, sy+4, box.Max.X-20, sy+5), grey4)
	ex := image.Rect(box.Min.X+8, sy+6, box.Max.X-8, sy+66)
	exitGlyph(img, float64(ex.Min.X+26), float64(ex.Min.Y+28), ink)
	drawText(img, tf, ex.Min.X+52, ex.Min.Y+30, "Exit to Kindle", ink)
	drawText(img, sf, ex.Min.X+52, ex.Min.Y+52, "Back to your library and books", grey2)
	v.add(Target{R: ex, Show: ex.Inset(2), Act: "quitapp"})
}

func exitGlyph(img *image.Gray, cx, cy float64, c uint8) {
	// a door frame on the left and an arrow pointing right out of it
	line(img, cx-2, cy-10, cx-10, cy-10, 2.5, c)
	line(img, cx-10, cy-10, cx-10, cy+10, 2.5, c)
	line(img, cx-10, cy+10, cx-2, cy+10, 2.5, c)
	line(img, cx-4, cy, cx+10, cy, 2.5, c)
	chevron(img, cx+7, cy, 5, "right", c)
}

// searchGlyph: a magnifying glass.
func searchGlyph(img *image.Gray, cx, cy float64, c uint8) {
	ringCircle(img, cx-3, cy-3, 8, 2.6, c)
	line(img, cx+3, cy+3, cx+10, cy+10, 3, c)
}

func gearGlyph(img *image.Gray, cx, cy float64, c uint8) {
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		line(img, cx+math.Cos(a)*8, cy+math.Sin(a)*8, cx+math.Cos(a)*12.5, cy+math.Sin(a)*12.5, 4.5, c)
	}
	fillCircle(img, cx, cy, 9, c)
	fillCircle(img, cx, cy, 4, paper)
}

// refreshIcon: a circular arrow (redraw the screen and fetch everything now).
func refreshIcon(img *image.Gray, cx, cy, r float64) {
	const from, to = -1.0, 3.9 // radians, clockwise on screen; the gap sits at the top right
	px, py := cx+r*math.Cos(from), cy+r*math.Sin(from)
	for i := 1; i <= 24; i++ {
		a := from + (to-from)*float64(i)/24
		x, y := cx+r*math.Cos(a), cy+r*math.Sin(a)
		line(img, px, py, x, y, 2.4, ink)
		px, py = x, y
	}
	// Solid arrow head at the start of the arc, pointing back along it (anticlockwise).
	ex, ey := cx+r*math.Cos(from), cy+r*math.Sin(from)
	tx, ty := math.Sin(from), -math.Cos(from) // anticlockwise tangent
	nx, ny := math.Cos(from), math.Sin(from)  // outward
	fillTriangle(img, ex+tx*6, ey+ty*6, ex+nx*5-tx*1.5, ey+ny*5-ty*1.5, ex-nx*5-tx*1.5, ey-ny*5-ty*1.5, ink)
}

// batteryIcon draws "86%" and a battery outline filled to the level, ending at right.
func batteryIcon(img *image.Gray, right, cy, level int, charging bool) {
	f := face("uiBold", 18) // big enough to read at a glance
	// an upright battery (saves room in the header): the nub on top, filling from the bottom
	body := image.Rect(right-12, cy-9, right, cy+11)                                     // about the size of the header's other icons
	fillRect(img, image.Rect(body.Min.X+3, body.Min.Y-3, body.Max.X-3, body.Min.Y), ink) // the nub
	strokeRoundRect(img, body, 3, 2, ink)
	inner := body.Inset(3)
	inner.Min.Y = inner.Max.Y - inner.Dy()*max(0, min(100, level))/100
	fillRect(img, inner, ink)
	if charging { // a bolt with a white outline over the battery: clear whatever the level
		cx, fy := float64(body.Min.X+body.Max.X)/2, float64(body.Min.Y+body.Max.Y)/2
		var bolt [][2]float64
		for _, p := range [][2]float64{{2, -7.5}, {-4, 1}, {-0.5, 1}, {-2, 7.5}, {4, -1}, {0.5, -1}} {
			bolt = append(bolt, [2]float64{cx + p[0], fy + p[1]})
		}
		for i := range bolt {
			a, b := bolt[i], bolt[(i+1)%len(bolt)]
			line(img, a[0], a[1], b[0], b[1], 4, paper)
		}
		fillPolygon(img, bolt, paper)
		for i := range bolt { // the bolt itself in black, outlined in white so it shows on a full battery too
			a, b := bolt[i], bolt[(i+1)%len(bolt)]
			line(img, a[0], a[1], b[0], b[1], 1.2, ink)
		}
		fillPolygon(img, bolt, ink)
	}
	drawRight(img, f, body.Min.X-8, cy+capHeight(f)/2, itoa(level)+"%", ink)
}

// segmented draws a row of equal buttons with the chosen one filled; each sends act+arg.
func segmented(v *View, r image.Rectangle, labels, args []string, sel int, act string) {
	img := v.Img
	f := face("uiBold", 16)
	cw := r.Dx() / len(labels)
	if separateButtons() { // retro, sketch, game boy: a row of buttons, the chosen one pressed / filled
		for i, l := range labels {
			cell := image.Rect(r.Min.X+i*cw+2, r.Min.Y, r.Min.X+(i+1)*cw-2, r.Max.Y)
			if i == len(labels)-1 {
				cell.Max.X = r.Max.X
			}
			c := buttonFace(img, cell, 6, i == sel)
			drawCentered(img, f, (cell.Min.X+cell.Max.X)/2, (cell.Min.Y+cell.Max.Y)/2+capHeight(f)/2, l, c)
			v.add(Target{R: cell, Show: cell.Inset(2), Act: act, Arg: args[i]})
		}
		return
	}
	for i, l := range labels {
		cell := image.Rect(r.Min.X+i*cw, r.Min.Y, r.Min.X+(i+1)*cw, r.Max.Y)
		if i == len(labels)-1 {
			cell.Max.X = r.Max.X
		}
		c := ink
		if i == sel {
			fillRoundRect(img, r, 8, ink, cell)
			c = paper
		}
		if i > 0 {
			fillRect(img, image.Rect(cell.Min.X-1, cell.Min.Y, cell.Min.X+1, cell.Max.Y), ink)
		}
		drawCentered(img, f, (cell.Min.X+cell.Max.X)/2, (cell.Min.Y+cell.Max.Y)/2+capHeight(f)/2, l, c)
		v.add(Target{R: cell, Show: cell.Inset(2), Act: act, Arg: args[i]})
	}
	strokeRoundRect(img, r, 8, 2, ink)
}

// lightChoice is the button nearest the Kindle's current front light level.
func lightChoice(level int) int {
	best := 0
	for i, l := range lightLevels {
		if abs(l-level) < abs(lightLevels[best]-level) {
			best = i
		}
	}
	return best
}

func lightButtons() (labels, args []string) {
	labels = []string{"Off", "1", "2", "3", "4", "5"}
	for _, l := range lightLevels {
		args = append(args, itoa(l))
	}
	return
}

// lightPanel: brightness and light/dark, dropped down from the battery.
func lightPanel(v *View, st Status, ui *UI) {
	img := v.Img
	v.add(Target{R: img.Rect, Act: "lightmenu"}) // tapping elsewhere closes it
	box := image.Rect(LW-pad-430, barH-4, LW-pad, barH-4+214)
	fillRoundRect(img, box.Inset(-3), 12, paper)
	strokeRoundRect(img, box, 10, 3, ink)
	x0, x1 := box.Min.X+20, box.Max.X-20
	label(img, x0, box.Min.Y+34, "BRIGHTNESS", grey2)
	if st.Battery >= 0 {
		s := itoa(st.Battery) + "% battery"
		if st.Charging {
			s += " · charging"
		}
		drawRight(img, face("ui", 15), x1, box.Min.Y+34, s, grey2)
	}
	labels, args := lightButtons()
	segmented(v, image.Rect(x0, box.Min.Y+48, x1, box.Min.Y+96), labels, args, lightChoice(st.Light), "light")
	label(img, x0, box.Min.Y+138, "SCREEN", grey2)
	sel := 0
	if ui.Dark {
		sel = 1
	}
	segmented(v, image.Rect(x0, box.Min.Y+152, x0+240, box.Min.Y+198), []string{"Light", "Dark"}, []string{"0", "1"}, sel, "setdark")
	flip := pillAt(img, image.Rect(x0+254, box.Min.Y+152, x1, box.Min.Y+198), "Flip screen", false)
	v.add(Target{R: flip.Inset(-4), Show: flip, Act: "fliprot"})
}

// standbyControls: a slim bar on the standby clock (brightness, light/dark, back).
func standbyControls(v *View, st Status, ui *UI) {
	img := v.Img
	v.add(Target{R: img.Rect, Act: "standbyctl"}) // tapping the clock hides the bar again
	box := image.Rect(80, LH-196, LW-80, LH-36)
	fillRoundRect(img, box.Inset(-4), 14, paper)
	strokeRoundRect(img, box, 12, 3, ink)
	// first row: how the clock looks (only what this clock style has), centred
	style := clockStyles[ui.ClockStyle%len(clockStyles)]
	y0, y1 := box.Min.Y+18, box.Min.Y+66
	lf := face("uiBold", 16)
	type opt struct {
		name, act string
		off       bool
	}
	var opts []opt
	if style == "classic" || style == "dressed" || style == "day" {
		opts = append(opts, opt{"Quote", "clockquote", ui.NoQuote})
	}
	if style != "stacked" {
		opts = append(opts, opt{"Next event", "clockevent", ui.NoEvent})
	}
	hours := style != "analog"
	const segW, onW, gap = 180, 130, 40
	w := 0
	if hours {
		w += segW
	}
	for _, o := range opts {
		if w > 0 {
			w += gap
		}
		w += textW(lf, o.name) + 14 + onW
	}
	x := (box.Min.X + box.Max.X - w) / 2
	if hours {
		h := 0
		if ui.Clock12 {
			h = 1
		}
		segmented(v, image.Rect(x, y0, x+segW, y1), []string{"24-hour", "12-hour"}, []string{"0", "1"}, h, "clock12")
		x += segW + gap
	}
	for _, o := range opts {
		x = drawText(img, lf, x, (y0+y1)/2+capHeight(lf)/2, o.name, grey2) + 14
		sel := 0
		if o.off {
			sel = 1
		}
		segmented(v, image.Rect(x, y0, x+onW, y1), []string{"On", "Off"}, []string{"1", "0"}, sel, o.act)
		x += onW + gap
	}
	// second row: light, light/dark, back
	y0, y1 = box.Max.Y-66, box.Max.Y-18
	labels, args := lightButtons()
	segmented(v, image.Rect(box.Min.X+20, y0, box.Min.X+20+390, y1), labels, args, lightChoice(st.Light), "light")
	sel := 0
	if ui.Dark {
		sel = 1
	}
	segmented(v, image.Rect(box.Min.X+430, y0, box.Min.X+430+200, y1), []string{"Light", "Dark"}, []string{"0", "1"}, sel, "setdark")
	back := pillAt(img, image.Rect(box.Max.X-20-150, y0, box.Max.X-20, y1), "Back to desk", true)
	v.add(Target{R: back.Inset(-4), Show: back, Act: "standby"})
}

// centreRows moves whatever is drawn between rows top and bottom up or down so it
// sits in the middle of that band.
func centreRows(img *image.Gray, top, bottom int) {
	s := scaleOf(img) // rows of pixels
	top, bottom = px(top, s), px(bottom, s)
	first, last := -1, -1
	for y := top; y < bottom; y++ {
		row := img.Pix[y*img.Stride : y*img.Stride+img.Rect.Dx()]
		for _, p := range row {
			if p != paper {
				if first < 0 {
					first = y
				}
				last = y
				break
			}
		}
	}
	if first < 0 {
		return
	}
	shift := (top+bottom)/2 - (first+last+1)/2
	if shift == 0 {
		return
	}
	band := image.NewGray(image.Rect(0, 0, img.Rect.Dx(), bottom-top))
	for i := range band.Pix {
		band.Pix[i] = paper
	}
	for y := first; y <= last; y++ {
		copy(band.Pix[(y+shift-top)*band.Stride:], img.Pix[y*img.Stride:y*img.Stride+img.Rect.Dx()])
	}
	copy(img.Pix[top*img.Stride:bottom*img.Stride], band.Pix)
}

// clockText is the time as "15:04", or "3:04 pm" for the 12-hour clock.
func clockText(t time.Time, h12 bool) string {
	if h12 {
		return t.Format("3:04 pm")
	}
	return t.Format("15:04")
}

// bigTime draws a large time centred on cx; the 12-hour clock gets a small "AM"/"PM"
// after it, sitting on the same baseline, without pushing the time off-centre.
func bigTime(img *image.Gray, f font.Face, cx, y int, t time.Time, h12 bool, c uint8) {
	if !h12 {
		drawCentered(img, f, cx, y, t.Format("15:04"), c)
		return
	}
	s := t.Format("3:04")
	drawCentered(img, f, cx, y, s, c)
	sf := face("uiBold", float64(max(14, capHeight(f)/4)))
	drawText(img, sf, cx+textW(f, s)/2+max(8, capHeight(f)/14), y, t.Format("PM"), grey2)
}

// nextEvent: "Now · Standup until 10:30", or the next event within 24 hours
// ("Design review in 25 min", "Tomorrow 09:00 · Standup"); "" when there's none.
func nextEvent(d *Desk, st Status, h12 bool) string {
	unix := st.Now.Unix()
	at := func(t int64) string { return clockText(time.Unix(t, 0).In(st.Now.Location()), h12) }
	var next *Event
	for i, e := range d.Agenda {
		if e.AllDay || e.End <= unix {
			continue
		}
		if e.Start <= unix {
			return "Now · " + e.Title + " until " + at(e.End)
		}
		if e.Start-unix <= 24*3600 && (next == nil || e.Start < next.Start) {
			next = &d.Agenda[i]
		}
	}
	switch {
	case next == nil:
		return ""
	case next.Start-unix <= 3600:
		return next.Title + " " + relative(next.Start, st)
	default:
		return dayPrefix(next.Start, st) + at(next.Start) + " · " + next.Title
	}
}

func sunGlyph(img *image.Gray, cx, cy float64, c uint8) {
	ringCircle(img, cx, cy, 5, 2, c)
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		line(img, cx+math.Cos(a)*8, cy+math.Sin(a)*8, cx+math.Cos(a)*10.5, cy+math.Sin(a)*10.5, 1.8, c)
	}
}

// contrastIcon: a circle, half filled (the dark mode switch).
func contrastIcon(img *image.Gray, cx, cy, r float64) {
	ringCircle(img, cx, cy, r, 2.2, ink)
	for y := int(cy - r); y <= int(cy+r); y++ {
		for x := int(cx); x <= int(cx+r); x++ {
			dx, dy := float64(x)+0.5-cx, float64(y)+0.5-cy
			if dx*dx+dy*dy <= (r-1.5)*(r-1.5) {
				blend(img, x, y, ink, 1)
			}
		}
	}
}

// ---------- night clock ----------

func night(v *View, d *Desk, st Status, ui *UI) {
	img := v.Img
	bigTime(img, face("display", 190), LW/2, 360, st.Now, ui.Clock12, ink)
	drawCentered(img, face("serif", 26), LW/2, 425, st.Now.Format("Monday 2 January"), grey2)
	waiting := "Nothing waiting for tomorrow"
	if d.OpenTotal == 1 {
		waiting = "1 task waiting"
	} else if d.OpenTotal > 1 {
		waiting = itoa(d.OpenTotal) + " tasks waiting"
	}
	drawCentered(img, face("ui", 18), LW/2, 490, waiting, grey2)
	lf := face("uiBold", 14)
	drawSpaced(img, lf, LW/2-spacedW(lf, "TAP TO WAKE", 2)/2, 560, "TAP TO WAKE", 2, grey3)
	v.add(Target{R: img.Rect, Show: image.Rect(LW/2-140, 538, LW/2+140, 572), Act: "wake"})
}

// ---------- standby: just a clock ----------

// Standby clock styles; swipe sideways to change.
// "dressed": the classic clock with the theme's decorations around it (themes.go)
var clockStyles = []string{"classic", "dressed", "analog", "stacked", "words", "day", "quote"}

func standby(v *View, d *Desk, st Status, ui *UI) {
	img := v.Img
	now := st.Now
	style := ui.ClockStyle
	notesY := 560
	h12 := ui.Clock12
	quote := func(y int) int { // the quote line, unless turned off; returns where the notes go
		if ui.NoQuote {
			return y
		}
		quoteLine(img, now, y)
		return y + 80
	}
	switch clockStyles[style%len(clockStyles)] {
	case "analog":
		analogClock(img, LW/2, 330, 250, now)
		drawCentered(img, face("serif", 24), LW/2, 640, now.Format("Monday 2 January"), grey2)
		notesY = 680
	case "stacked":
		f := face("display", 330)
		hour := now.Format("15")
		if h12 {
			hour = now.Format("3")
		}
		drawCentered(img, f, LW/2, 330, hour, ink)
		drawCentered(img, f, LW/2, 640, now.Format("04"), ink)
		label(img, LW/2-spacedW(face("uiBold", 13), strings.ToUpper(now.Format("Mon 2 Jan")), 2)/2, 700, strings.ToUpper(now.Format("Mon 2 Jan")), grey2)
		notesY = 9999 // no room: the stacked clock is only the time
	case "words":
		f := face("serifB", 76)
		lines := wrap(f, timeInWords(now), LW-160, 3)
		top := 330 - (len(lines)-1)*45
		for i, l := range lines {
			drawCentered(img, f, LW/2, top+i*92, l, ink)
		}
		y := top + (len(lines)-1)*92 + 70
		drawCentered(img, face("serif", 26), LW/2, y, clockText(now, h12)+" · "+now.Format("Monday 2 January"), grey2)
		notesY = y + 70
	case "quote":
		bigTime(img, face("serifB", 44), LW/2, 120, now, h12, ink)
		drawCentered(img, face("ui", 18), LW/2, 156, now.Format("Monday 2 January"), grey2)
		text, who := quoteFor(now)
		f := face("serif", 50)
		lines := wrap(f, "“"+text+"”", LW-220, 4)
		top := 400 - (len(lines)-1)*34
		for i, l := range lines {
			drawCentered(img, f, LW/2, top+i*68, l, ink)
		}
		y := top + (len(lines)-1)*68
		if who != "" {
			drawCentered(img, face("ui", 20), LW/2, y+56, "— "+who, grey2)
		}
		notesY = y + 120
	case "day":
		bigTime(img, face("display", 200), LW/2, 330, now, h12, ink)
		drawCentered(img, face("serif", 26), LW/2, 395, now.Format("Monday 2 January"), grey2)
		// how much of the day has gone
		bar := image.Rect(140, 450, LW-140, 466)
		strokeRoundRect(img, bar, 8, 2, ink)
		mins := now.Hour()*60 + now.Minute()
		fill := bar
		fill.Max.X = bar.Min.X + bar.Dx()*mins/(24*60)
		fillRoundRect(img, bar, 8, ink, fill)
		tf := face("uiBold", 13)
		for _, h := range []int{0, 6, 12, 18, 24} {
			x := bar.Min.X + bar.Dx()*h/24
			names := map[int]string{0: "00", 6: "06", 12: "12", 18: "18", 24: "24"}
			drawCentered(img, tf, x, 492, names[h], grey2)
		}
		notesY = quote(540)
	default: // classic
		bigTime(img, face("display", 220), LW/2, 380, now, h12, ink)
		drawCentered(img, face("serif", 30), LW/2, 450, now.Format("Monday 2 January"), grey2)
		notesY = quote(510)
	}

	// Only what's worth knowing right now, at most two lines.
	var notes []string
	if p := d.Pomodoro; p.active() {
		n := pomoLine(p, now.Unix())
		if p.Title != "" {
			n += " · " + p.Title
		}
		notes = append(notes, n)
	}
	if c := d.Claude; c != nil {
		ago := now.Sub(time.Unix(int64(c.At), 0))
		switch {
		case c.State == "input":
			notes = append(notes, "Claude needs your input · "+c.Text)
		case c.State == "done" && ago < 30*time.Minute:
			notes = append(notes, "Claude finished · "+c.Text)
		}
	}
	if !ui.NoEvent { // the event on now, or the next one in the coming 24 hours
		if n := nextEvent(d, st, h12); n != "" {
			notes = append(notes, n)
		}
	}
	nf := face("uiBold", 19)
	for i, n := range notes {
		if i == 2 || notesY+i*38 > LH-50 {
			break
		}
		drawCentered(img, nf, LW/2, notesY+i*38, ellipsize(nf, n, LW-200), ink)
	}
	// Whatever the face shows (quote or not, a note or not), centre it on the screen.
	centreRows(img, 0, LH-50)
	if clockStyles[style%len(clockStyles)] == "dressed" {
		clockDecor(img)
	}
	if st.Battery >= 0 && st.Battery <= 20 && !st.Charging {
		drawText(img, face("ui", 15), pad, LH-24, "Battery "+itoa(st.Battery)+"%", grey3)
	}
	// which style: small dots at the bottom
	n := len(clockStyles)
	for i := 0; i < n; i++ {
		x := float64(LW/2 + (i-n/2)*18)
		if i == style%n {
			fillCircle(img, x, float64(LH-26), 4.5, grey2)
		} else {
			ringCircle(img, x, float64(LH-26), 4.5, 1.5, grey3)
		}
	}
	v.add(Target{R: img.Rect, Act: "standbyctl"}) // tap: show the control bar (Back is on it)
}

// quoteLine: this hour's quote, small and quiet, under the date.
func quoteLine(img *image.Gray, now time.Time, y int) {
	text, who := quoteFor(now)
	f := face("serif", 21)
	if who != "" {
		text += " — " + who
	}
	for i, l := range wrap(f, text, LW-260, 2) {
		drawCentered(img, f, LW/2, y+i*30, l, grey2)
	}
}

// analogClock: a face with hour marks and two hands.
func analogClock(img *image.Gray, cx, cy, r int, t time.Time) {
	fx, fy, fr := float64(cx), float64(cy), float64(r)
	ringCircle(img, fx, fy, fr, 4, ink)
	for i := 0; i < 60; i++ {
		a := float64(i) * math.Pi / 30
		in, w := 0.9, 2.0
		if i%5 == 0 {
			in, w = 0.82, 5
		}
		if i%15 == 0 {
			in, w = 0.76, 7
		}
		sx, sy := math.Sin(a), -math.Cos(a)
		line(img, fx+sx*fr*in, fy+sy*fr*in, fx+sx*fr*0.95, fy+sy*fr*0.95, w, ink)
	}
	m := float64(t.Minute())
	h := float64(t.Hour()%12) + m/60
	ha, ma := h*math.Pi/6, m*math.Pi/30
	line(img, fx, fy, fx+math.Sin(ha)*fr*0.5, fy-math.Cos(ha)*fr*0.5, 14, ink)
	line(img, fx, fy, fx+math.Sin(ma)*fr*0.78, fy-math.Cos(ma)*fr*0.78, 8, ink)
	fillCircle(img, fx, fy, 13, ink)
	fillCircle(img, fx, fy, 5, paper)
}

// timeInWords: "It's twenty past nine", to the nearest five minutes.
func timeInWords(t time.Time) string {
	names := []string{"twelve", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven"}
	m := (t.Minute() + 2) / 5 * 5
	h := t.Hour() % 12
	if m > 30 || m == 60 {
		h = (h + 1) % 12
	}
	hour := names[h]
	switch m {
	case 0, 60:
		if t.Hour() == 0 && m == 0 || t.Hour() == 23 && m == 60 {
			return "It's midnight"
		}
		if t.Hour() == 12 && m == 0 || t.Hour() == 11 && m == 60 {
			return "It's noon"
		}
		return "It's " + hour + " o'clock"
	case 15:
		return "It's quarter past " + hour
	case 30:
		return "It's half past " + hour
	case 45:
		return "It's quarter to " + hour
	}
	words := map[int]string{5: "five", 10: "ten", 20: "twenty", 25: "twenty-five"}
	if m < 30 {
		return "It's " + words[m] + " past " + hour
	}
	return "It's " + words[60-m] + " to " + hour
}

// ---------- heart message ----------

func message(v *View, d *Desk) {
	img := v.Img
	const s = 236.0 // heart scale: (x²+y²−1)³ − x²y³ ≤ 0 spans x ±1.14, y −1…1.25
	cx, cy := float64(LW)/2, float64(LH)/2+0.125*s
	in := func(px, py float64) bool {
		x, y := (px-cx)/s, -(py-cy)/s
		a := x*x + y*y - 1
		return a*a*a-x*x*y*y*y <= 0
	}
	for y := int(cy - 1.3*s); y <= int(cy+1.05*s); y++ {
		for x := int(cx - 1.2*s); x <= int(cx+1.2*s); x++ {
			n := 0
			for _, o := range [4][2]float64{{.25, .25}, {.75, .25}, {.25, .75}, {.75, .75}} {
				if in(float64(x)+o[0], float64(y)+o[1]) {
					n++
				}
			}
			blend(img, x, y, ink, float64(n)/4)
		}
	}
	words := strings.Fields(d.Message)
	lines := words
	if len(words) > 1 {
		lines = []string{strings.Join(words[:len(words)-1], " "), words[len(words)-1]}
	}
	f := face("serifB", 64)
	top := int(cy-0.32*s) - (len(lines)*72)/2 + capHeight(f)
	for i, l := range lines {
		drawCentered(img, f, int(cx), top+i*72, ellipsize(f, l, int(1.5*s)), paper)
	}
}

// ---------- waiting for the server ----------

func connecting(v *View, st Status) {
	img := v.Img
	drawBuddy(img, LW/2, 90, 130, ink)
	drawCentered(img, face("serifB", 40), LW/2, 270, "Desk Buddy", ink)
	if st.Server == "" { // a new Kindle: find the computer running Desk Buddy
		drawCentered(img, face("ui", 20), LW/2, 330, "Find the computer running Desk Buddy on this Wi-Fi.", grey2)
		find := pillAt(img, image.Rect(LW/2-120, 380, LW/2+120, 426), "Find it", true)
		v.add(Target{R: find.Inset(-6), Show: find, Act: "serverpick"})
		quit := pillAt(img, image.Rect(LW/2-120, 540, LW/2+120, 586), "Exit to Kindle", false)
		v.add(Target{R: quit.Inset(-6), Show: quit, Act: "quitapp"})
		return
	}
	msg := "Connecting to " + st.Server + "…"
	if st.Offline {
		msg = "Can't reach " + st.Server + ". Retrying every 15 seconds."
	}
	drawCentered(img, face("ui", 20), LW/2, 330, ellipsize(face("ui", 20), msg, LW-120), grey2)
	if st.Why != "" {
		drawCentered(img, face("uiBold", 18), LW/2, 372, ellipsize(face("uiBold", 18), st.Why, LW-120), ink)
	}
	if st.Offline {
		retry := pillAt(img, image.Rect(LW/2-160-8, 420, LW/2-8, 466), "Try again", false)
		v.add(Target{R: retry.Inset(-6), Show: retry, Act: "retry"})
		restart := pillAt(img, image.Rect(LW/2+8, 420, LW/2+168, 466), "Restart app", false)
		v.add(Target{R: restart.Inset(-6), Show: restart, Act: "restartapp"})
		other := pillAt(img, image.Rect(LW/2-160-8, 482, LW/2+168, 528), "Use another computer…", false)
		v.add(Target{R: other.Inset(-6), Show: other, Act: "serverpick"})
	}
	quit := pillAt(img, image.Rect(LW/2-120, 550, LW/2+120, 596), "Exit to Kindle", false)
	v.add(Target{R: quit.Inset(-6), Show: quit, Act: "quitapp"})
}

// ---------- bits ----------

func label(img *image.Gray, x, y int, s string, c uint8) {
	drawSpaced(img, face("uiBold", 13), x, y, s, 2, c)
}

// pill draws a button whose right edge is at right and vertical centre at cy; returns its rect.
func pill(img *image.Gray, right, cy int, s string, solid bool) image.Rectangle {
	w := textW(face("uiBold", 16), s) + 32
	return pillAt(img, image.Rect(right-w, cy-23, right, cy+23), s, solid)
}

func pillAt(img *image.Gray, r image.Rectangle, s string, solid bool) image.Rectangle {
	f := face("uiBold", 16)
	c := buttonFace(img, r, 7, solid) // in the theme's style (themes.go)
	drawCentered(img, f, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2+capHeight(f)/2, s, c)
	return r
}

// chevron draws a small arrow head pointing left, right, up or down.
func chevron(img *image.Gray, cx, cy, s float64, dir string, c uint8) {
	switch dir {
	case "left":
		line(img, cx+s/2, cy-s, cx-s/2, cy, 2.6, c)
		line(img, cx-s/2, cy, cx+s/2, cy+s, 2.6, c)
	case "right":
		line(img, cx-s/2, cy-s, cx+s/2, cy, 2.6, c)
		line(img, cx+s/2, cy, cx-s/2, cy+s, 2.6, c)
	case "up":
		line(img, cx-s, cy+s/2, cx, cy-s/2, 2.6, c)
		line(img, cx, cy-s/2, cx+s, cy+s/2, 2.6, c)
	default:
		line(img, cx-s, cy-s/2, cx, cy+s/2, 2.6, c)
		line(img, cx, cy+s/2, cx+s, cy-s/2, 2.6, c)
	}
}

// iconButton draws an outlined square button with a chevron; dim when it can't be used.
func iconButton(img *image.Gray, r image.Rectangle, dir string, enabled bool) {
	c := ink
	if !enabled {
		c = grey4
	}
	strokeRoundRect(img, r, 7, 2, c)
	chevron(img, float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2, 7, dir, c)
}

// oneList: Settings › Tasks › Task lists is "One for everything" (no Mac / Windows lists).
func oneList(d *Desk) bool { v, _ := d.Settings["task_lists"].(string); return v == "one" }

// searching: only one computer has the helper, so there's no Mac / Windows to choose
// between yet ("Searching for other devices…").
func searching(d *Desk) bool { return d != nil && d.Computers != nil && len(d.Computers) < 2 }

// pcName: "Linux" when the connected PC runs Linux, else "Windows".
func pcName(d *Desk) string {
	if d != nil && d.PCLabel != "" {
		return d.PCLabel
	}
	if d != nil && d.Deck != nil {
		for _, dk := range d.Deck.Decks {
			if dk.Profile == "home" && dk.MachineLabel != "" {
				return dk.MachineLabel
			}
		}
	}
	return "Windows"
}

// listName: the list on screen: "Mac", "Windows" or "Linux" (one list per computer), or "" (one list).
func listName(d *Desk) string {
	if oneList(d) {
		return ""
	}
	if d.Profile == "home" {
		return pcName(d)
	}
	return "Mac"
}

func title(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func itoa(n int) string { return strconv.Itoa(n) }
