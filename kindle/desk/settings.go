package main

// The Settings screen (from the Work / Home / Auto dropdown). Look settings (theme, font)
// live on the Kindle; the rest live on the backend because they affect the computers.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// fontStyle: "sans" (Atkinson Hyperlegible, the default), "serif" (Literata), "inter" or
// "bitter" for task text and headings.
var fontStyle = "sans"

var fontStyles = []string{"sans", "serif", "inter", "bitter"}

func settingsView(v *View, d *Desk, st Status, ui *UI) {
	img := v.Img
	bf := face("uiBold", 17)
	back := image.Rect(pad-8, 16, pad+120, 60)
	chevron(img, float64(back.Min.X+18), 38, 7, "left", ink)
	drawText(img, bf, back.Min.X+34, 38+capHeight(bf)/2, "Back", ink)
	v.add(Target{R: back.Inset(-6), Show: back, Act: "settings"})
	drawCentered(img, face("serifB", 30), LW/2, 48, "Settings", ink)
	// top right, as on the main screens: refresh, then light / dark
	lightDark := image.Rect(LW-pad-44, 14, LW-pad, 58)
	contrastIcon(img, float64(lightDark.Min.X+lightDark.Max.X)/2, 36, 11)
	v.add(Target{R: image.Rect(lightDark.Min.X, 0, LW, barH), Show: lightDark, Act: "dark"})
	ref := image.Rect(lightDark.Min.X-6-44, 14, lightDark.Min.X-6, 58)
	refreshIcon(img, float64(ref.Min.X+ref.Max.X)/2, 36, 10)
	v.add(Target{R: image.Rect(ref.Min.X, 0, ref.Max.X, barH), Show: ref, Act: "refresh"})
	fillRect(img, image.Rect(0, barH, LW, barH+2), ink)

	set := d.Settings
	bval := func(k string, def bool) bool {
		if v, ok := set[k].(bool); ok {
			return v
		}
		return def
	}
	sval := func(k, def string) string {
		if v, ok := set[k].(string); ok && v != "" {
			return v
		}
		return def
	}
	nval := func(k string, def int) int {
		if v, ok := set[k].(float64); ok {
			return int(v)
		}
		return def
	}
	onOff := func(on bool) int {
		if on {
			return 0
		}
		return 1
	}
	pick := func(vals []int, cur int) int {
		for i, x := range vals {
			if x == cur {
				return i
			}
		}
		return 0
	}

	type row struct {
		title, hint string
		labels      []string
		args        []string
		sel         int
		act         string
		off         bool // greyed out and not tappable (what it depends on is off)
	}
	col := func(x0, x1, y int, heading string, rows []row) int {
		label(img, x0, y, heading, grey2)
		y += 18
		step := 74
		if len(rows) >= 6 {
			step = 54
		} else if len(rows) >= 5 {
			step = 62
		}
		for _, r := range rows {
			cw := 72 // each choice at least 72 wide, wider for longer words ("Per computer")
			for _, l := range r.labels {
				cw = max(cw, textW(face("uiBold", 16), l)+28)
			}
			w := min(320, cw*len(r.labels))
			ty, hy, sy, bh := y+30, y+52, y+10, 44
			if step == 54 {
				ty, hy, sy, bh = y+23, y+41, y+5, 40
			} else if step < 74 {
				ty, hy, sy = y+26, y+46, y+8
			}
			drawText(img, face("serifB", 20), x0, ty, r.title, ink)
			if r.hint != "" {
				drawText(img, face("ui", 13), x0, hy, ellipsize(face("ui", 13), r.hint, x1-x0-w-16), grey2)
			}
			n := len(v.Targets)
			segmented(v, image.Rect(x1-w, sy, x1, sy+bh), r.labels, r.args, r.sel, r.act)
			if r.off {
				fade(img, image.Rect(x0, y, x1, y+step-8))
				v.Targets = v.Targets[:n]
			}
			fillRect(img, image.Rect(x0, y+step-6, x1, y+step-5), grey4)
			y += step
		}
		return y
	}
	dark := 0
	if ui.Dark {
		dark = 1
	}
	font := 0
	for i, f := range fontStyles {
		if f == fontStyle {
			font = i
		}
	}
	unit := 0
	if tempF {
		unit = 1
	}
	focus := []int{25, 45, 50}
	brk := []int{5, 10, 15}
	sound := d.Pomodoro != nil && d.Pomodoro.Sound
	what := "the " + listName(d) + " list" // for Clear: "the Mac list", or "the list"
	if listName(d) == "" {
		what = "the list"
	}

	// Left: the sections. Right: the chosen one's options.
	const sideW = 260
	fillRect(img, image.Rect(sideW-2, barH+2, sideW, LH), ink)
	sections := []struct {
		id, name, hint string
		glyph          func(*image.Gray, float64, float64, uint8)
	}{
		{"screen", "Screen", "Theme, size, font", sunGlyph},
		{"focus", "Focus", "Pomodoro and breaks", focusGlyph},
		{"music", "Music", "App, sync and covers", noteGlyph},
		{"tasks", "Tasks", "Lists, clearing", tasksGlyph},
		{"calendar", "Calendar", "Calendars, heads-up", calendarGlyph},
		{"server", "Server", "Cloud or local", cloudGlyph},
		{"updates", "Updates", "Version, battery", refreshGlyph},
		{"themes", "Themes", "Garden, Retro, Sketch…", flowerGlyph},
		{"about", "About", "Who made it, feedback", infoGlyph},
	}
	cur := ui.SettingsTab
	if cur == "" {
		cur = "screen"
	}
	const rowH = 68
	for i, sec := range sections {
		r := image.Rect(0, barH+2+i*rowH, sideW-2, barH+2+(i+1)*rowH)
		fg, soft := ink, grey2
		if sec.id == cur {
			fillRect(img, r, ink) // the open section, inverted (like the chosen day on the Weather tab)
			fg, soft = paper, grey4
		} else if i > 0 && sections[i-1].id != cur {
			fillRect(img, image.Rect(pad, r.Min.Y, sideW-2-pad, r.Min.Y+1), grey4)
		}
		sec.glyph(img, float64(pad+12), float64(r.Min.Y+rowH/2), fg)
		drawText(img, face("serifB", 22), pad+40, r.Min.Y+32, sec.name, fg)
		drawText(img, face("ui", 14), pad+40, r.Min.Y+52, sec.hint, soft)
		if sec.id != cur {
			chevron(img, float64(sideW-2-pad), float64(r.Min.Y+rowH/2), 5, "right", grey2)
		}
		v.add(Target{R: r, Show: r, Act: "settab", Arg: sec.id})
	}

	x0, x1 := sideW+40, LW-pad
	y := barH + 26
	switch cur {
	case "screen":
		theme, themeHint := dark, "Dark: white on black"
		if ui.AutoDark {
			theme, themeHint = 2, "Dark when your Mac is, else sunset to sunrise"
		}
		compSel := 0
		if compactMode {
			compSel = 1
		}
		rotSel := 0
		if ui.Rotation == 270 {
			rotSel = 1
		}
		rows := []row{
			{"Theme", themeHint, []string{"Light", "Dark", "Auto"}, []string{"light", "dark", "auto"}, theme, "settheme", false},
			{"Screen size", "Compact: larger text and tiles for 6\" Basic Kindles", []string{"Standard", "Compact"}, []string{"0", "1"}, compSel, "setcompact", false},
			{"Orientation", "Which side the Kindle's chin is on", []string{"Left", "Right"}, []string{"90", "270"}, rotSel, "setrot", false},
			{"Font", "For tasks and headings", []string{"Sans", "Serif", "Inter", "Bitter"}, fontStyles, font, "setfont", false},
			{"Temperature", "For the weather", []string{"°C", "°F"}, []string{"c", "f"}, unit, "setunit", false},
		}
		// the front light while Auto is dark (greyed out unless the theme is Auto)
		levels := []int{-1, 0, 4, 10}
		sel := 0
		for i, l := range levels {
			if l == ui.NightLight {
				sel = i
			}
		}
		nightHint := "Front light while Auto is dark"
		if !ui.AutoDark {
			nightHint = "Pick Auto above to use this"
		}
		night := row{"Night brightness", nightHint, []string{"Same", "Off", "Low", "Mid"}, []string{"-1", "0", "4", "10"}, sel, "nightlight", !ui.AutoDark}
		rows = append(rows[:1], append([]row{night}, rows[1:]...)...)
		ny := col(x0, x1, y, "SCREEN", rows)
		refreshCards(v, ui, x0, x1, ny)
	case "focus":
		col(x0, x1, y, "FOCUS", []row{
			{"Focus length", "Minutes per Pomodoro", []string{"25", "45", "50"}, []string{"focus_min=25", "focus_min=45", "focus_min=50"}, pick(focus, nval("focus_min", 25)), "setting", false},
			{"Break length", "Minutes between focuses", []string{"5", "10", "15"}, []string{"break_min=5", "break_min=10", "break_min=15"}, pick(brk, nval("break_min", 5)), "setting", false},
			{"Chime on computer", "Plays on the computer you're using when a focus or break ends", []string{"On", "Off"}, []string{"sound:1", "sound:0"}, onOff(sound), "pomo", false},
		})
	case "music":
		col(x0, x1, y, "MUSIC", []row{
			{"Music sync", "Show what's playing on your computer", []string{"On", "Off"}, []string{"music_sync=true", "music_sync=false"}, onOff(bval("music_sync", true)), "setting", false},
			{"Music app", map[string]string{"auto": "Whichever one is playing", "spotify": "Only Spotify", "music": "Only Apple Music"}[sval("music_app", "auto")], []string{"Auto", "Spotify", "Apple Music"}, []string{"music_app=auto", "music_app=spotify", "music_app=music"}, map[string]int{"auto": 0, "spotify": 1, "music": 2}[sval("music_app", "auto")], "setting", !bval("music_sync", true)},
			{"Music app logo", "Shows a small Spotify or Apple Music logo next to the song", []string{"On", "Off"}, []string{"1", "0"}, map[bool]int{true: 0, false: 1}[ui.ShowMusicLogo], "musiclogo", !bval("music_sync", true)},
			{"Album covers", "On the music screen (Spotify, and Apple Music songs with artwork)", []string{"On", "Off"}, []string{"album_covers=true", "album_covers=false"}, onOff(bval("album_covers", true)), "setting", !bval("music_sync", true)},
			{"Cover quality", "Sharp: twice the detail on the big cover screens", []string{"Standard", "Sharp"}, []string{"cover_sharp=false", "cover_sharp=true"}, map[bool]int{false: 0, true: 1}[bval("cover_sharp", true)], "setting", !bval("music_sync", true) || !bval("album_covers", true)},
		})
	case "tasks":
		pc := pcName(d)
		sep, sepHint := 0, "Your Mac and "+pc+" PC each get their own tasks"
		if searching(d) {
			sepHint = "Searching for other devices… connect another computer first"
		}
		if oneList(d) {
			sep, sepHint = 1, "Off: one list for everything"
		}
		col(x0, x1, y, "TASKS", []row{
			{"Separate lists", sepHint, []string{"On", "Off"}, []string{"task_lists=per_computer", "task_lists=one"}, sep, "setting", searching(d)},
			{"Auto-delete done tasks", "Ticked-off tasks go by themselves", []string{"On", "Off"}, []string{"auto_clear=true", "auto_clear=false"}, onOff(bval("auto_clear", false)), "setting", false},
			{"Delete after", "How long after you tick them off", []string{"Instant", "1 min", "5 min", "10 min"}, []string{"auto_clear_min=0", "auto_clear_min=1", "auto_clear_min=5", "auto_clear_min=10"}, pick([]int{0, 1, 5, 10}, nval("auto_clear_min", 5)), "setting", !bval("auto_clear", false)},
		})
		y += 18 + 3*74
		clearHint := "Empties " + what + " (the one showing). Asks first."
		if oneList(d) {
			clearHint = "Empties your list. Asks first."
		}
		drawText(img, face("serifB", 20), x0, y+30, "Clear tasks", ink)
		drawText(img, face("ui", 13), x0, y+52, clearHint, grey2)
		cb := image.Rect(x1-144, y+10, x1, y+54) // same width as On | Off above
		pillAt(img, cb, "Clear…", false)
		v.add(Target{R: cb.Inset(-4), Show: cb, Act: "clearask"})
		fillRect(img, image.Rect(x0, y+68, x1, y+69), grey4)
		nf := face("ui", 15)
		notes := []string{"Which list shows: pick Mac, " + pc + " or Auto in the menu", "at the top left. Auto follows the computer you're using.", "", "Turning Separate lists off adds the " + pc + " tasks", "to the Mac's, as one list."}
		if oneList(d) {
			notes = []string{"Turn Separate lists on to give each computer its own", "list again. Your tasks stay in the Mac's list."}
		} else if searching(d) {
			notes = []string{"To connect another computer, install Desk Buddy", "on it. Then pick which computer the Kindle follows in the", "menu at the top left, or Auto for the one you're using."}
		}
		for i, l := range notes {
			drawText(img, nf, x0, y+108+i*22, l, grey2)
		}
	case "calendar":
		on := 0
		if ui.NoHeadsUp {
			on = 1
		}
		early := ui.HeadsUpMin
		if early == 0 {
			early = 5
		}
		col(x0, x1, y, "MEETING HEADS-UP", []row{
			{"Before meetings", "A card before each calendar event, with Join for Zoom / Meet links", []string{"On", "Off"}, []string{"1", "0"}, on, "headsup", false},
			{"How early", "Minutes before it starts", []string{"2", "5", "10"}, []string{"2", "5", "10"}, pick([]int{2, 5, 10}, early), "headsupmin", ui.NoHeadsUp},
		})
		calendarList(v, d, ui, x0, x1, y+18+2*74+44)
	case "server":
		serverRow(v, ui, x0, x1, y)
	case "updates":
		updatesPane(v, ui, st, x0, x1, y)
	case "themes":
		themesPane(v, x0, x1, y)
	case "about":
		aboutPane(v, x0, x1, y)
	}
	if ui.ClearAsk {
		clearModal(v, d, what)
	}
	if ui.NotesModal && cur == "updates" {
		notesModal(v, ui)
	}
}

// deleteModal: "Delete this task?" (after a swipe and the bin).
func deleteModal(v *View, d *Desk, id int) {
	img := v.Img
	var t *Task
	kids := 0
	for i := range d.Tasks {
		if d.Tasks[i].ID == id {
			t = &d.Tasks[i]
		}
		if d.Tasks[i].Parent != nil && *d.Tasks[i].Parent == id {
			kids++
		}
	}
	if t == nil {
		return
	}
	for i, p := range img.Pix {
		img.Pix[i] = uint8((int(p)*35 + 172*65) / 100)
	}
	v.Targets = nil
	box := image.Rect(LW/2-300, LH/2-110, LW/2+300, LH/2+110)
	fillRoundRect(img, box.Inset(-3), 16, paper)
	strokeRoundRect(img, box, 14, 2, ink)
	x0, x1 := box.Min.X+32, box.Max.X-32
	drawText(img, face("serifB", 28), x0, box.Min.Y+56, "Delete this task?", ink)
	f := face("serif", 20)
	drawText(img, f, x0, box.Min.Y+94, ellipsize(f, "“"+t.Text+"”", x1-x0), grey2)
	if kids > 0 {
		sub := "Its subtask goes too."
		if kids > 1 {
			sub = fmt.Sprintf("Its %d subtasks go too.", kids)
		}
		drawText(img, face("ui", 16), x0, box.Min.Y+122, sub, grey2)
	}
	by := box.Max.Y - 28 - 48
	cancel := pillAt(img, image.Rect(x0, by, x0+120, by+48), "Cancel", false)
	v.add(Target{R: img.Rect, Show: cancel, Act: "binask", ID: 0}) // Cancel, or tap outside
	del := pillAt(img, image.Rect(x1-130, by, x1, by+48), "Delete", true)
	v.add(Target{R: del.Inset(-4), Show: del, Act: "deltask", ID: id})
}

// clearModal: "Clear the Work list?" over the faded Settings screen.
func clearModal(v *View, d *Desk, what string) { // what: "the Mac list", "the list"
	img := v.Img
	for i, p := range img.Pix {
		img.Pix[i] = 255 - uint8(int(255-p)*28/100)
	}
	v.Targets = nil
	all, done := 0, 0
	for _, t := range d.Tasks {
		if t.Parent == nil {
			all++
			if t.Done {
				done++
			}
		}
	}
	box := image.Rect(LW/2-300, LH/2-120, LW/2+300, LH/2+120)
	fillRoundRect(img, box.Inset(-3), 16, paper)
	strokeRoundRect(img, box, 14, 2, ink)
	x0, x1 := box.Min.X+32, box.Max.X-32
	drawText(img, face("serifB", 30), x0, box.Min.Y+60, "Clear "+what+"?", ink)
	f := face("ui", 17)
	drawText(img, f, x0, box.Min.Y+98, fmt.Sprintf("%d tasks, %d of them done. Subtasks go with them.", all, done), grey2)
	drawText(img, f, x0, box.Min.Y+124, "This can't be undone.", grey2)
	by := box.Max.Y - 32 - 50
	cancel := pillAt(img, image.Rect(x0, by, x0+120, by+50), "Cancel", false)
	v.add(Target{R: cancel.Inset(-4), Show: cancel, Act: "clearask"})
	bf := face("uiBold", 16)
	everything := fmt.Sprintf("Everything (%d)", all)
	er := image.Rect(x1-textW(bf, everything)-36, by, x1, by+50)
	pillAt(img, er, everything, true)
	if all > 0 {
		v.add(Target{R: er.Inset(-4), Show: er, Act: "cleartasks", Arg: d.Profile + "|all"})
	}
	doneS := fmt.Sprintf("Done ones (%d)", done)
	dr := image.Rect(er.Min.X-12-textW(bf, doneS)-36, by, er.Min.X-12, by+50)
	if done > 0 {
		pillAt(img, dr, doneS, false)
		v.add(Target{R: dr.Inset(-4), Show: dr, Act: "cleartasks", Arg: d.Profile + "|done"})
	} else {
		strokeRoundRect(img, dr, 7, 2, grey4)
		drawCentered(img, bf, (dr.Min.X+dr.Max.X)/2, (dr.Min.Y+dr.Max.Y)/2+capHeight(bf)/2, doneS, grey3)
	}
}

// decodeArt turns the album cover from the backend into a small, punchy greyscale picture.
func decodeArt(b64 string, size int) *image.Gray {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	size = min(size, max(src.Bounds().Dx(), 1)) // keep a small cover small (Standard is 320 px)
	g := image.NewGray(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(g, g.Rect, src, src.Bounds(), xdraw.Src, nil)
	photoTone(g) // so it doesn't look flat and dark on e-ink
	return g
}

var _ = strings.Cut

// fade greys out an area (a setting that's switched off): black becomes mid grey, white stays.
func fade(img *image.Gray, r image.Rectangle) {
	r = pxRect(img, r).Intersect(img.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := img.Pix[img.PixOffset(r.Min.X, y):img.PixOffset(r.Max.X, y)]
		for i, p := range row {
			row[i] = uint8(255 - (255-int(p))*40/100)
		}
	}
}

// refreshCards: Settings › Screen › Screen refresh as three cards, each with a small picture
// of what flashes: nothing (Gentle), big changes where they are (Normal), every change (Clean).
func refreshCards(v *View, ui *UI, x0, x1, y int) {
	img := v.Img
	drawText(img, face("serifB", 20), x0, y+30, "Screen refresh", ink)
	drawText(img, face("ui", 13), x0, y+52, "What flashes when the screen changes", grey2)
	cards := []struct{ name, text string }{
		{"Gentle", "Never flashes. Faint ghosts of old text may stay."},
		{"Normal", "Big changes flash, only where they changed."},
		{"Clean", "Every change flashes, even small ones. Sharpest."},
	}
	const gap = 16
	cw := (x1 - x0 - 2*gap) / 3
	top, bottom := y+62, LH-18
	for i, c := range cards {
		r := image.Rect(x0+i*(cw+gap), top, x0+i*(cw+gap)+cw, bottom)
		if i == ui.Refresh {
			strokeRoundRect(img, r, 12, 3, ink)
		} else {
			strokeRoundRect(img, r, 12, 2, grey4)
		}
		// the picture: a little screen with a list on it
		pw, ph := 150, min(96, max(64, (bottom-top)-136))
		p := image.Rect((r.Min.X+r.Max.X-pw)/2, r.Min.Y+16, (r.Min.X+r.Max.X+pw)/2, r.Min.Y+16+ph)
		fillRect(img, p, paper)
		rowGap := max(13, (ph-20)/4)
		for k := 0; k < 4; k++ { // the list's lines
			ly := p.Min.Y + 12 + k*rowGap
			if i == 0 && k == 1 { // gentle: a faint ghost of what was there before
				fillRect(img, image.Rect(p.Min.X+18, ly+5, p.Min.X+18+70, ly+9), grey4)
			}
			fillCircle(img, float64(p.Min.X+16), float64(ly+2), 4, ink)
			fillRect(img, image.Rect(p.Min.X+26, ly, p.Min.X+26+[]int{92, 70, 100, 56}[k], ly+5), ink)
		}
		switch i {
		case 1: // normal: only the changed row flashes
			fl := image.Rect(p.Min.X+6, p.Min.Y+12+rowGap-4, p.Max.X-6, p.Min.Y+12+rowGap+12)
			invertRect(img, fl)
		case 2: // clean: every change flashes, the small ones too
			invertRect(img, image.Rect(p.Min.X+6, p.Min.Y+12+rowGap-4, p.Max.X-6, p.Min.Y+12+rowGap+12))
			invertRect(img, image.Rect(p.Min.X+8, p.Max.Y-24, p.Min.X+24, p.Max.Y-10))
			invertRect(img, image.Rect(p.Max.X-30, p.Min.Y+6, p.Max.X-8, p.Min.Y+18))
		}
		strokeRoundRect(img, p.Inset(-2), 6, 2, ink)
		// name, with a filled dot when chosen, and what it does
		nf := face("uiBold", 18)
		ny := p.Max.Y + 30
		if i == ui.Refresh {
			fillCircle(img, float64(r.Min.X+24), float64(ny-6), 6, ink)
		} else {
			ringCircle(img, float64(r.Min.X+24), float64(ny-6), 6, 2, grey3)
		}
		drawText(img, nf, r.Min.X+38, ny, c.name, ink)
		tf := face("ui", 14)
		for k, l := range wrap(tf, c.text, cw-32, 3) {
			drawText(img, tf, r.Min.X+16, ny+26+k*19, l, grey2)
		}
		v.add(Target{R: r, Show: r, Act: "setrefresh", Arg: itoa(i)})
	}
}

// calendarList: Settings › Calendar: every calendar, and which list it shows on (or off).
// Long lists get pages (Newer / Older style arrows by the heading).
func calendarList(v *View, d *Desk, ui *UI, x0, x1, y int) {
	img := v.Img
	label(img, x0, y, "CALENDARS", grey2)
	if len(d.CalList) == 0 {
		drawText(img, face("ui", 15), x0, y+34, "None yet. Desk Buddy brings in your computer's calendars.", grey2)
		return
	}
	pc := pcName(d)
	labels, args := []string{"Both", "Mac", pc, "Off"}, []string{"both", "work", "home", "off"}
	hint := map[string]string{"both": "On both lists", "work": "Only on the Mac list", "home": "Only on the " + pc + " list", "off": "Not shown"}
	if oneList(d) || searching(d) {
		labels, args = []string{"On", "Off"}, []string{"both", "off"}
		hint = map[string]string{"both": "Shown", "work": "Shown", "home": "Shown", "off": "Not shown"}
	}
	const rowH = 64
	per := max(1, (LH-16-(y+14))/rowH)
	pages := (len(d.CalList) + per - 1) / per
	ui.CalListPage = max(0, min(ui.CalListPage, pages-1))
	if pages > 1 {
		pf := face("uiBold", 15)
		older := image.Rect(x1-80, y-24, x1, y+10)
		newer := image.Rect(older.Min.X-92, y-24, older.Min.X-12, y+10)
		for _, b := range []struct {
			r   image.Rectangle
			s   string
			on  bool
			arg string
		}{{newer, "‹ Back", ui.CalListPage > 0, "-1"}, {older, "More ›", ui.CalListPage < pages-1, "1"}} {
			c := ink
			if !b.on {
				c = grey4
			}
			drawCentered(img, pf, (b.r.Min.X+b.r.Max.X)/2, (b.r.Min.Y+b.r.Max.Y)/2+5, b.s, c)
			if b.on {
				v.add(Target{R: b.r.Inset(-6), Show: b.r, Act: "calpage", Arg: b.arg})
			}
		}
	}
	yy := y + 14
	end := min(len(d.CalList), (ui.CalListPage+1)*per)
	for _, c := range d.CalList[ui.CalListPage*per : end] {
		sel := 0
		for i, a := range args {
			if a == c.Show || (len(args) == 2 && c.Show != "off" && a == "both") {
				sel = i
			}
		}
		w := min(320, 80*len(labels))
		tf := face("serifB", 18)
		drawText(img, tf, x0, yy+26, ellipsize(tf, c.Name, x1-x0-w-16), ink)
		drawText(img, face("ui", 13), x0, yy+46, hint[c.Show], grey2)
		cargs := make([]string, len(args))
		for i, a := range args {
			cargs[i] = a + "|" + c.Name
		}
		segmented(v, image.Rect(x1-w, yy+8, x1, yy+50), labels, cargs, sel, "calshow")
		fillRect(img, image.Rect(x0, yy+rowH-4, x1, yy+rowH-3), grey4)
		yy += rowH
	}
}
