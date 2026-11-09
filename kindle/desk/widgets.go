package main

// The Tasks page's right-hand panel: a row of widget tabs (Day, Weather, Music, Info)
// and a separate button that collapses the panel so the task list gets the full width.

import (
	"fmt"
	"image"
	"sort"
	"strings"
	"time"
)

type Music struct {
	App     string   `json:"app"`
	Title   string   `json:"title"`
	Artist  string   `json:"artist"`
	Album   string   `json:"album"`
	Playing bool     `json:"playing"`
	Running bool     `json:"running"`
	Machine string   `json:"machine"`
	ArtKey  string   `json:"art_key"` // which song's cover the backend holds ("" = none)
	Volume  *int     `json:"volume"`  // the computer's output volume 0–100 (nil: unknown, or an older helper)
	Outputs []Output `json:"outputs"` // where the computer's sound can go (empty: no choice)
}

// musicComputer: "PC" when the music comes from the Windows PC, else "Mac" (also when unknown,
// as before Windows had music).
func musicComputer(m *Music) string {
	if m != nil && m.Machine == "pc" {
		return "PC"
	}
	return "Mac"
}

// Output is one of the computer's sound outputs (MacBook speakers, AirPods…).
type Output struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Kind      string `json:"kind"` // headphones, laptop, speaker, display
	Current   bool   `json:"current"`
	Connected bool   `json:"connected"`
}

var widgets = []struct {
	id    string
	glyph func(*image.Gray, float64, float64, uint8)
}{{"day", calendarGlyph}, {"weather", weatherGlyph}, {"music", musicTabGlyph}, {"info", infoGlyph}}

// collapseButton: the separate button that hides (») or shows («) the panel. A ghost
// button: just the icon, no outline.
func collapseButton(v *View, hidden bool, r image.Rectangle, act string) {
	img := v.Img
	cx, cy := float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2
	dir := "right" // » hides the panel
	if hidden {
		dir = "left" // « brings it back
	}
	chevron(img, cx-4, cy, 6, dir, ink)
	chevron(img, cx+5, cy, 6, dir, ink)
	v.add(Target{R: r.Inset(-6), Show: r, Act: act})
}

func sidePanel(v *View, d *Desk, st Status, ui *UI) {
	img := v.Img
	x0, x1 := sideX+22, LW-22
	y := barH + 14

	// widget tabs: icon buttons, the current one filled
	for i, w := range widgets {
		r := image.Rect(x0+i*56, y, x0+i*56+48, y+46)
		c := ink
		if ui.Widget == w.id {
			fillRoundRect(img, r, 8, ink)
			c = paper
		} else {
			strokeRoundRect(img, r, 8, 2, ink)
		}
		w.glyph(img, float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2, c)
		v.add(Target{R: r.Inset(-4), Show: r, Act: "widget", Arg: w.id})
	}
	collapseButton(v, false, image.Rect(LW-22-46, barH+14, LW-22, barH+60), "sidehide")
	y += 66

	switch ui.Widget {
	case "weather":
		weatherWidget(v, st, ui, x0, x1, y)
	case "music":
		musicWidget(v, d, ui, x0, x1, y)
	case "info":
		infoWidget(v, d, st, x0, x1, y)
	default:
		dayWidget(v, d, st, x0, x1, y)
	}
}

// dayWidget: today as a vertical timeline, a block per event and a line at now.
func dayWidget(v *View, d *Desk, st Status, x0, x1, y int) {
	img := v.Img
	loc := st.Now.Location()
	now := st.Now
	today := now.Format("2006-01-02")
	label(img, x0, y+12, strings.ToUpper(now.Format("Monday 2 January")), grey2)
	y += 26
	if d.Calendars == 0 && len(d.Agenda) == 0 {
		drawText(img, face("serifB", 21), x0, y+30, "No calendar yet", ink)
		drawText(img, face("ui", 14), x0, y+54, "Add one on your phone page", grey2)
		return
	}
	// all-day events on top
	for _, e := range d.Agenda {
		if e.AllDay && time.Unix(e.Start, 0).UTC().Format("2006-01-02") == today && y < barH+200 {
			r := image.Rect(x0, y+4, x1, y+30)
			fillRoundRect(img, r, 6, grey4)
			f := face("uiBold", 14)
			drawText(img, f, x0+10, y+17+capHeight(f)/2, ellipsize(f, e.Title, x1-x0-20), ink)
			v.add(Target{R: r, Act: "calevent", Arg: eventKey(e)}) // tap: open it on the Calendar tab
			y += 32
		}
	}
	// the timeline: from the hour before now, as many hours as fit
	perHour := 62
	if compactMode {
		perHour = 72
	}
	top := y + 12
	start := time.Date(now.Year(), now.Month(), now.Day(), max(0, now.Hour()-1), 0, 0, 0, loc)
	n := timeline(v, d.Agenda, st, x0, x1, top, LH-20, start, perHour, func(e Event, _ int) Target {
		return Target{Act: "calevent", Arg: eventKey(e)} // tap: open it on the Calendar tab
	})
	if n == 0 {
		drawText(img, face("ui", 15), x0+56, top+perHour/2+6, "Nothing on for the next few hours", grey2)
	}
}

// timeline draws hour lines from start down to bottom and the (timed) events on them;
// tap(e, i) gives each event's tap action (i is its index in evs). Returns how many showed.
func timeline(v *View, evs []Event, st Status, x0, x1, top, bottom int, start time.Time, perHour int, tap func(Event, int) Target) int {
	img := v.Img
	now := st.Now
	loc := now.Location()
	hours := (bottom - top) / perHour
	end := start.Add(time.Duration(hours) * time.Hour)
	ypos := func(t time.Time) int { return top + int(t.Sub(start).Minutes()*float64(perHour)/60) }
	hf := face("ui", 13)
	for h := 0; h <= hours; h++ {
		t := start.Add(time.Duration(h) * time.Hour)
		yy := ypos(t)
		drawText(img, hf, x0, yy+5, t.Format("15:04"), grey2)
		fillRect(img, image.Rect(x0+46, yy, x1, yy+1), grey4)
	}
	type block struct {
		r image.Rectangle
		e Event
		i int
	}
	var blocks []block
	for i, e := range evs {
		if e.AllDay {
			continue
		}
		s, f := time.Unix(e.Start, 0).In(loc), time.Unix(e.End, 0).In(loc)
		if !f.After(start) || !s.Before(end) {
			continue
		}
		if s.Before(start) {
			s = start
		}
		if f.After(end) {
			f = end
		}
		blocks = append(blocks, block{image.Rect(x0+52, ypos(s)+1, x1, max(ypos(f)-1, ypos(s)+22)), e, i})
	}
	sort.SliceStable(blocks, func(i, j int) bool { return blocks[i].r.Min.Y < blocks[j].r.Min.Y })
	for i, b := range blocks {
		r := b.r
		if i > 0 && r.Min.Y < blocks[i-1].r.Max.Y { // overlapping: sit beside the previous one
			r.Min.X = (blocks[i-1].r.Min.X + x1) / 2
		}
		past := time.Unix(b.e.End, 0).Before(now)
		fill, txt := grey4, ink
		if past {
			fill, txt = 240, grey3
		}
		fillRoundRect(img, r, 6, fill)
		fillRect(img, image.Rect(r.Min.X, r.Min.Y+2, r.Min.X+4, r.Max.Y-2), txt)
		tf := face("uiBold", 14)
		drawText(img, tf, r.Min.X+10, r.Min.Y+17, ellipsize(tf, b.e.Title, r.Dx()-16), txt)
		if r.Dy() > 40 {
			sf := face("ui", 12)
			when := hm(b.e.Start, st) + "–" + hm(b.e.End, st)
			if b.e.Location != "" {
				when += " · " + b.e.Location
			}
			drawText(img, sf, r.Min.X+10, r.Min.Y+34, ellipsize(sf, when, r.Dx()-16), grey2)
		}
		t := tap(b.e, b.i)
		t.R = r
		v.add(t)
	}
	// now
	if now.After(start) && now.Before(end) {
		yy := ypos(now)
		fillCircle(img, float64(x0+48), float64(yy), 5, ink)
		fillRect(img, image.Rect(x0+48, yy-1, x1, yy+1), ink)
	}
	return len(blocks)
}

// weatherWidget: now, the next few hours, and a nudge about rain.
func weatherWidget(v *View, st Status, ui *UI, x0, x1, y int) {
	img := v.Img
	w := ui.Weather
	if w == nil || len(w.Hours) == 0 {
		msg := "Loading the weather…"
		if ui.WeatherErr != "" {
			msg = "Weather: " + ui.WeatherErr
		}
		drawText(img, face("ui", 16), x0, y+30, msg, grey2)
		return
	}
	loc := st.Now.Location()
	label(img, x0, y+12, strings.ToUpper(w.Place), grey2)
	c := w.Current
	drawWeather(img, c.Code, c.Day, float64(x0+60), float64(y+96), 110)
	drawText(img, face("display", 80), x0+128, y+122, deg(c.Temp), ink)
	_, words := weatherKind(c.Code)
	drawText(img, face("serifB", 21), x0, y+180, words, ink)
	if len(w.Days) > 0 {
		dd := w.Days[0]
		drawText(img, face("ui", 15), x0, y+204, fmt.Sprintf("High %s · low %s · feels %s", deg(dd.Max), deg(dd.Min), deg(c.Feels)), grey2)
	}
	// rain nudge
	note := "Dry for the next 12 hours"
	for i, h := range w.Hours {
		if i >= 12 {
			break
		}
		if h.Rain >= 50 {
			at := time.Unix(h.T, 0).In(loc)
			note = "Rain likely around " + at.Format("15:04")
			if at.Day() != st.Now.Day() {
				note = "Rain likely overnight, around " + at.Format("15:04")
			}
			break
		}
	}
	drawText(img, face("uiBold", 15), x0, y+232, note, ink)
	// next hours
	yy := y + 260
	fillRect(img, image.Rect(x0, yy, x1, yy+1), grey4)
	n := min(5, len(w.Hours))
	colW := (x1 - x0) / 5
	for i := 0; i < n; i++ {
		h := w.Hours[i]
		cx := x0 + i*colW + colW/2
		lab := time.Unix(h.T, 0).In(loc).Format("15")
		if i == 0 {
			lab = "Now"
		}
		drawCentered(img, face("uiBold", 13), cx, yy+26, lab, grey2)
		drawWeather(img, h.Code, h.Day, float64(cx), float64(yy+58), 34)
		drawCentered(img, face("serifB", 17), cx, yy+100, deg(h.Temp), ink)
		if h.Rain >= 20 {
			drawCentered(img, face("ui", 12), cx, yy+120, fmt.Sprintf("%d%%", h.Rain), grey2)
		}
	}
	if len(w.Days) > 0 {
		dd := w.Days[0]
		drawText(img, face("ui", 14), x0, yy+160, "Sunrise "+time.Unix(dd.Sunrise, 0).In(loc).Format("15:04")+" · sunset "+time.Unix(dd.Sunset, 0).In(loc).Format("15:04"), grey3)
	}
	v.add(Target{R: image.Rect(x0, y, x1, yy), Act: "tab", Arg: "weather"}) // tap: the full forecast
}

// musicWidget: what's playing on the computer, with previous / play-pause / next.
func musicWidget(v *View, d *Desk, ui *UI, x0, x1, y int) {
	img := v.Img
	m := d.Music
	if m == nil || !m.Running || m.Title == "" {
		hint := "Open Spotify or Apple Music on your " + musicComputer(m)
		if m != nil && m.Running { // open, but nothing loaded here (or playing on another device)
			hint = map[string]string{"Music": "Apple Music"}[m.App]
			if hint == "" {
				hint = m.App
			}
			hint += " is open, but not playing on this " + musicComputer(m)
		}
		label(img, x0, y+12, "MUSIC", grey2)
		drawText(img, face("serifB", 22), x0, y+50, "Nothing playing", ink)
		for i, l := range wrap(face("ui", 15), hint, x1-x0, 2) {
			drawText(img, face("ui", 15), x0, y+76+i*21, l, grey2)
		}
		return
	}
	state := "PAUSED"
	if m.Playing {
		state = "PLAYING"
	}
	top := y + 30                               // gap under the label
	if m.App == "Spotify" || m.App == "Music" { // the app's mark goes small, by the artist (below)
		top = y + 6
	} else {
		label(img, x0, y+12, strings.ToUpper(m.App)+" · "+state, grey2)
	}
	// The cover fills the width (square), sized to what's left after the label above and
	// the title, artist and controls below, with breathing room either side.
	tf := face("serifB", 23)
	title := wrap(tf, m.Title, x1-x0, 2)
	hasAlbum := m.Album != "" && m.Album != m.Title
	textH := 28*len(title) + 26 // title lines + artist
	if hasAlbum {
		textH += 22
	}
	bottom := LH - 96 - 24 - textH - 18
	if len(m.Outputs) > 1 { // room for where the sound goes (AirPods ⌄), above the controls
		bottom -= 48
	} // controls, gap, text, gap
	size := min(x1-x0, bottom-top)
	r := image.Rect(x0+(x1-x0-size)/2, top, x0+(x1-x0-size)/2+size, top+size)
	if art := ui.Art; art != nil && m.ArtKey != "" && m.ArtKey == ui.ArtKey {
		drawImage(img, r, art)
		if ui.Dark { // the whole screen gets inverted for dark mode: pre-invert so the cover comes out right
			invertRect(img, r)
		}
		strokeRoundRect(img, r, 2, 1.5, grey3)
	} else {
		drawRecord(img, r)
	}
	v.add(Target{R: r, Act: "cover"}) // tap: the cover screen
	ty := r.Max.Y + 18 + capHeight(tf)
	for i, l := range title {
		drawText(img, tf, x0, ty+i*28, l, ink)
	}
	ty += 28 * (len(title) - 1)
	ax := x0 // the artist, after a small mark of where the sound comes from (Spotify / Apple)
	if (m.App == "Spotify" || m.App == "Music") && ui.ShowMusicLogo {
		appMark(img, m.App, float64(x0+9), float64(ty+20), 9, grey2)
		ax += 26
	}
	drawText(img, face("ui", 17), ax, ty+26, ellipsize(face("ui", 17), m.Artist, x1-ax), grey2)
	if hasAlbum {
		drawText(img, face("ui", 14), x0, ty+48, ellipsize(face("ui", 14), m.Album, x1-x0), grey3)
	}
	musicControls(v, m, x0, x1, LH-96, 64)
	outputPicker(v, m, ui, x0, x1, LH-96-8)
}

// infoWidget: Claude, and today in numbers.
func infoWidget(v *View, d *Desk, st Status, x0, x1, y int) {
	img := v.Img
	w := x1 - x0
	c := d.Claude
	ago := ""
	if c != nil {
		ago = agoText(st.Now.Sub(time.Unix(int64(c.At), 0)))
	}
	switch {
	case c == nil || c.State == "idle":
		y = card(img, x0, y, w, "CLAUDE", "No sessions running", "", false)
	case c.State == "input":
		y = card(img, x0, y, w, "CLAUDE · NEEDS YOU", "Waiting for your input", c.Text+" · "+ago, true)
	case c.State == "working":
		y = card(img, x0, y, w, "CLAUDE", "Working: "+c.Text, "started "+ago, false)
	default:
		y = card(img, x0, y, w, "CLAUDE", "Finished: "+c.Text, ago, false)
	}
	open := "Nothing open in Work or Home"
	if d.OpenTotal == 1 {
		open = "1 task open across Work and Home"
	} else if d.OpenTotal > 1 {
		open = fmt.Sprintf("%d tasks open across Work and Home", d.OpenTotal)
	}
	card(img, x0, y, w, "TODAY", st.Now.Format("Monday 2 January"), open, false)
	drawText(img, face("ui", 13), x0, LH-22, "Hold the clock 3 s, or press power, to leave", grey3)
}

// musicTabGlyph: the Music tab is a plain note (the app's mark sits above the cover).
func musicTabGlyph(img *image.Gray, cx, cy float64, c uint8) {
	noteGlyph(img, cx, cy, c)
}

// appMark: Spotify's mark, Apple Music's, or a plain note.
func appMark(img *image.Gray, app string, cx, cy, r float64, c uint8) {
	switch app {
	case "Spotify":
		spotifyMark(img, cx, cy, r, c)
	case "Music":
		appleMusicMark(img, cx, cy, r, c)
	default:
		noteGlyph(img, cx, cy, c)
	}
}

// appleMark: the Apple logo, for Apple Music (drawn: two round lobes, a bite on the right,
// a leaf on top).
func appleMusicMark(img *image.Gray, cx, cy, r float64, c uint8) {
	bg := paper // the bite is the background's colour: white, or black under a white logo (chosen tab)
	if c == paper {
		bg = ink
	}
	cy += r * 0.08
	for _, p := range [][3]float64{{-0.3, -0.05, 0.56}, {0.3, -0.05, 0.56}, {-0.24, 0.32, 0.5}, {0.24, 0.32, 0.5}} {
		fillCircle(img, cx+p[0]*r, cy+p[1]*r, p[2]*r, c)
	}
	fillCircle(img, cx, cy-0.62*r, 0.17*r, bg)                       // the dip at the top
	fillCircle(img, cx, cy+0.86*r, 0.13*r, bg)                       // and at the bottom
	fillCircle(img, cx+0.98*r, cy-0.02*r, 0.36*r, bg)                // the bite
	line(img, cx+0.04*r, cy-0.68*r, cx+0.34*r, cy-1.02*r, 0.24*r, c) // the leaf
}

func noteGlyph(img *image.Gray, cx, cy float64, c uint8) {
	fillCircle(img, cx-5, cy+6, 4.5, c)
	line(img, cx-1, cy+6, cx-1, cy-9, 2.4, c)
	line(img, cx-1, cy-9, cx+7, cy-5, 2.4, c)
}

func infoGlyph(img *image.Gray, cx, cy float64, c uint8) {
	ringCircle(img, cx, cy, 11, 2, c)
	fillCircle(img, cx, cy-5, 1.8, c)
	line(img, cx, cy-1, cx, cy+6, 2.4, c)
}
