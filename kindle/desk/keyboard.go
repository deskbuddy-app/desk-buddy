package main

// Adding and editing tasks on the Kindle: our own on-screen keyboard (the Kindle's own
// one isn't available while the app has the screen). The desk stays visible behind it,
// faded; the text being typed sits in a card just above the keyboard.
//
// Opened by the ghost + on the TODAY line (new task), "+ Subtask" on an open task, or
// holding a task (edit it).

import (
	"image"
	"strings"
	"time"
	"unicode/utf8"
)

// Editor is the keyboard and what's being typed.
type Editor struct {
	Mode   string // add, sub, edit
	ID     int    // the parent (sub) or the task being edited (edit)
	Parent string // the parent's text, for the card's label
	Text   string
	Shift  bool // next letter in capitals
	Sym    bool // numbers and symbols instead of letters

	// a new calendar event (Mode "event"): step 0 its name, step 1 when it is
	Step              int
	Day               time.Time
	AllDay            bool
	Hour, Min, Length int // start and length in minutes

	// the weather place (Mode "place"): the matches for what was typed
	Places    []Place
	Searching bool
	SearchErr string
}

var kbTop = LH - 318 // where the keyboard starts (setPanel moves it with LH)

var kbLetters = [3]string{"qwertyuiop", "asdfghjkl", "zxcvbnm"}
var kbSymbols = [3]string{"1234567890", "-/:;()£&@", "'?!#%+*"}

// openEditor starts typing: a new task (mode add), a subtask of id (sub), or editing id (edit).
func openEditor(ui *UI, d *Desk, mode string, id int) {
	e := &Editor{Mode: mode, ID: id, Shift: true}
	for _, t := range d.Tasks {
		if t.ID == id {
			if mode == "edit" {
				e.Text, e.Shift = t.Text, false
			} else {
				e.Parent = t.Text
			}
		}
	}
	ui.Edit, ui.Menu, ui.LightMenu = e, false, false
}

// editorKey handles a key on the keyboard (Arg of a "key" tap).
func editorKey(e *Editor, k string) {
	switch k {
	case "shift":
		e.Shift = !e.Shift
	case "sym":
		e.Sym = !e.Sym
	case "back":
		if e.Text != "" {
			_, n := utf8.DecodeLastRuneInString(e.Text)
			e.Text = e.Text[:len(e.Text)-n]
		}
		e.Shift = e.Text == "" || strings.HasSuffix(e.Text, ". ")
	default:
		if utf8.RuneCountInString(e.Text) >= 200 {
			return
		}
		if e.Shift && !e.Sym {
			k = strings.ToUpper(k)
		}
		e.Text += k
		// capitals again at the start of a sentence, otherwise one letter only
		e.Shift = strings.HasSuffix(e.Text, ". ") || strings.HasSuffix(e.Text, "! ") || strings.HasSuffix(e.Text, "? ")
	}
}

// editorDone turns the typed text into the backend action ("" when there's nothing to send).
func editorDone(e *Editor, d *Desk) Target {
	text := strings.TrimSpace(e.Text)
	if text == "" {
		return Target{}
	}
	switch e.Mode {
	case "edit":
		return Target{Act: "edittask", ID: e.ID, Arg: text}
	case "sub":
		return Target{Act: "addtask", ID: e.ID, Arg: d.Profile + "|" + text}
	}
	return Target{Act: "addtask", Arg: d.Profile + "|" + text}
}

// editorView fades what's on screen and draws the card and keyboard over it.
func editorView(v *View, d *Desk, ui *UI) {
	img := v.Img
	e := ui.Edit
	for i, p := range img.Pix { // fade the desk behind
		img.Pix[i] = 255 - uint8(int(255-p)*28/100)
	}
	v.Targets = nil
	if e.Mode == "event" && e.Step == 1 {
		eventTimeView(v, ui)
		return
	}
	// Tapping the faded desk closes the keyboard, but only before anything's been
	// typed, so a stray tap can't throw text away.
	if strings.TrimSpace(e.Text) == "" || e.Mode == "edit" && e.Text == editOriginal(d, e) {
		v.add(Target{R: image.Rect(0, 0, LW, kbTop), Show: image.Rectangle{}, Act: "kbcancel"})
	}

	// the card
	card := image.Rect(pad, kbTop-178, LW-pad, kbTop-22)
	fillRoundRect(img, card.Inset(-3), 16, paper)
	strokeRoundRect(img, card, 14, 2, ink)
	title := "新建任务"
	if n := listName(d); n != "" {
		title += " · " + strings.ToUpper(n)
	}
	switch e.Mode {
	case "sub":
		title = "新建子任务 · " + strings.ToUpper(e.Parent)
	case "edit":
		title = "编辑任务"
	case "event":
		title = "新建日程 · " + strings.ToUpper(displayDate(e.Day, "Monday 2 January"))
	case "place":
		title = "天气地点"
	}
	lf := face("uiBold", 14)
	label(img, card.Min.X+24, card.Min.Y+30, ellipsize(lf, title, card.Dx()-200), grey2)
	cancel := pillAt(img, image.Rect(card.Max.X-20-110, card.Min.Y+14, card.Max.X-20, card.Min.Y+56), "取消", false)
	v.add(Target{R: cancel.Inset(-6), Show: cancel, Act: "kbcancel"})

	tf := face("serif", 28)
	tw := card.Dx() - 48
	ty := card.Min.Y + 98
	if e.Text == "" {
		ph := map[string]string{"sub": "其中一个步骤…", "event": "有什么安排？", "place": "城镇或城市"}[e.Mode]
		if ph == "" {
			ph = "需要做什么？"
		}
		fillRect(img, image.Rect(card.Min.X+24, ty-capHeight(tf)-4, card.Min.X+26, ty+8), ink)
		drawText(img, tf, card.Min.X+34, ty, ph, grey3)
	} else {
		lines := wrap(tf, e.Text, tw-10, 99)
		if strings.HasSuffix(e.Text, " ") && len(lines) > 0 {
			lines[len(lines)-1] += " " // wrap trims it; the cursor should still move on
		}
		if len(lines) > 2 { // show the end: that's where the typing is
			lines = lines[len(lines)-2:]
			lines[0] = "…" + lines[0]
		}
		var end int
		for i, l := range lines {
			end = drawText(img, tf, card.Min.X+24, ty+i*38, l, ink)
		}
		cy := ty + (len(lines)-1)*38
		fillRect(img, image.Rect(end+3, cy-capHeight(tf)-4, end+5, cy+8), ink) // cursor
	}

	if e.Mode == "place" {
		placeResults(v, e, card)
	}

	// the keyboard
	fillRect(img, image.Rect(0, kbTop, LW, LH), paper)
	fillRect(img, image.Rect(0, kbTop, LW, kbTop+2), ink)
	const kx0, gap, kh, rowGap = 16, 8, 62, 10
	kw := (LW - 2*kx0 - 9*gap) / 10
	rows := kbLetters
	if e.Sym {
		rows = kbSymbols
	}
	key := func(r image.Rectangle, text, arg string, style int) { // style 0 ghost, 1 grey, 2 solid
		c := ink
		switch style {
		case 1:
			fillRoundRect(img, r, 9, grey4)
		case 2:
			fillRoundRect(img, r, 9, ink)
			c = paper
		default:
			strokeRoundRect(img, r, 9, 2, ink)
		}
		f := face("uiBold", 24)
		if utf8.RuneCountInString(text) > 1 {
			f = face("uiBold", 17)
		}
		drawCentered(img, f, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2+capHeight(f)/2, text, c)
		v.add(Target{R: r.Inset(-gap / 2), Show: r, Act: "key", Arg: arg})
	}
	y := kbTop + 16
	for i, row := range rows[:2] {
		x := kx0 + i*(kw+gap)/2 // the second row sits half a key in
		for _, ch := range row {
			s := string(ch)
			shown := s
			if e.Shift && !e.Sym {
				shown = strings.ToUpper(s)
			}
			key(image.Rect(x, y, x+kw, y+kh), shown, s, 0)
			x += kw + gap
		}
		y += kh + rowGap
	}
	// third row: shift, letters, backspace
	wide := kw + (kw+gap)/2
	x := kx0
	if !e.Sym {
		style := 1
		if e.Shift {
			style = 2
		}
		r := image.Rect(x, y, x+wide, y+kh)
		key(r, "", "shift", style)
		shiftGlyph(img, float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2, style == 2)
	}
	x += wide + gap
	for _, ch := range rows[2] {
		s := string(ch)
		shown := s
		if e.Shift && !e.Sym {
			shown = strings.ToUpper(s)
		}
		key(image.Rect(x, y, x+kw, y+kh), shown, s, 0)
		x += kw + gap
	}
	br := image.Rect(LW-kx0-wide, y, LW-kx0, y+kh)
	key(br, "", "back", 1)
	backGlyph(img, float64(br.Min.X+br.Max.X)/2, float64(br.Min.Y+br.Max.Y)/2)
	y += kh + rowGap
	// bottom row: 123/ABC, comma, space, full stop, Add/Save
	x = kx0
	sym := "123"
	if e.Sym {
		sym = "ABC"
	}
	key(image.Rect(x, y, x+wide, y+kh), sym, "sym", 1)
	x += wide + gap
	key(image.Rect(x, y, x+kw, y+kh), ",", ",", 0)
	x += kw + gap
	space := 5*kw + 4*gap
	key(image.Rect(x, y, x+space, y+kh), "space", " ", 0)
	x += space + gap
	key(image.Rect(x, y, x+kw, y+kh), ".", ".", 0)
	x += kw + gap
	done := map[string]string{"edit": "保存", "event": "下一步", "place": "搜索"}[e.Mode]
	if done == "" {
		done = "添加"
	}
	dr := image.Rect(x, y, LW-kx0, y+kh)
	if strings.TrimSpace(e.Text) == "" {
		strokeRoundRect(img, dr, 9, 2, grey3)
		f := face("uiBold", 19)
		drawCentered(img, f, (dr.Min.X+dr.Max.X)/2, (dr.Min.Y+dr.Max.Y)/2+capHeight(f)/2, done, grey3)
	} else {
		fillRoundRect(img, dr, 9, ink)
		f := face("uiBold", 19)
		drawCentered(img, f, (dr.Min.X+dr.Max.X)/2, (dr.Min.Y+dr.Max.Y)/2+capHeight(f)/2, done, paper)
		v.add(Target{R: dr.Inset(-gap / 2), Show: dr, Act: "kbdone"})
	}
}

func editOriginal(d *Desk, e *Editor) string {
	for _, t := range d.Tasks {
		if t.ID == e.ID {
			return t.Text
		}
	}
	return ""
}

// shiftGlyph: an outlined arrow pointing up (filled when shift is on).
func shiftGlyph(img *image.Gray, cx, cy float64, on bool) {
	c := ink
	if on {
		c = paper
	}
	pts := [][2]float64{{cx, cy - 13}, {cx + 12, cy}, {cx + 5, cy}, {cx + 5, cy + 11}, {cx - 5, cy + 11}, {cx - 5, cy}, {cx - 12, cy}}
	if on {
		fillPolygon(img, pts, c)
		return
	}
	for i := range pts {
		a, b := pts[i], pts[(i+1)%len(pts)]
		line(img, a[0], a[1], b[0], b[1], 2, c)
	}
}

// backGlyph: a backspace key, an arrow-shaped tag with an × in it.
func backGlyph(img *image.Gray, cx, cy float64) {
	pts := [][2]float64{{cx - 18, cy}, {cx - 8, cy - 11}, {cx + 16, cy - 11}, {cx + 16, cy + 11}, {cx - 8, cy + 11}}
	for i := range pts {
		a, b := pts[i], pts[(i+1)%len(pts)]
		line(img, a[0], a[1], b[0], b[1], 2, ink)
	}
	line(img, cx-1, cy-5, cx+9, cy+5, 2, ink)
	line(img, cx+9, cy-5, cx-1, cy+5, 2, ink)
}

// plusGlyph: a plus sign.
func plusGlyph(img *image.Gray, cx, cy, s float64, c uint8) {
	line(img, cx-s, cy, cx+s, cy, 2.4, c)
	line(img, cx, cy-s, cx, cy+s, 2.4, c)
}
