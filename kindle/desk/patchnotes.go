package main

// Patch notes: what changed in each release, in Settings › Updates. patchnotes.json ships
// inside the app (this version and older); release.sh also puts the newest entries in
// release.json, so a waiting update can say what it brings before it's installed.
// Long lists are split into pages (Newer / Older).

import (
	_ "embed"
	"encoding/json"
	"image"
	"strings"
	"time"
)

type NoteEntry struct {
	N     int      `json:"n"` // counts up with each release
	Date  string   `json:"date"`
	Items []string `json:"items"`
}

//go:embed patchnotes.json
var patchNotesJSON []byte

var patchNotes = func() []NoteEntry {
	var n []NoteEntry
	json.Unmarshal(patchNotesJSON, &n)
	return n
}()

// updateNotes: the entries the waiting update brings (newer than this version's).
func updateNotes(update []NoteEntry) []NoteEntry {
	mine := 0
	if len(patchNotes) > 0 {
		mine = patchNotes[0].N
	}
	var out []NoteEntry
	for _, e := range update {
		if e.N > mine {
			out = append(out, e)
		}
	}
	return out
}

const helperReminder = "请同时更新电脑上的 Desk Buddy，以免部分功能无法使用。"

type noteLine struct {
	text         string
	head, bullet bool
	h            int
}

// noteLines lays entries out as lines (a date heading, then wrapped bullet items).
func noteLines(entries []NoteEntry, w int, headSuffix func(i int) string) []noteLine {
	tf := face("ui", 15)
	var lines []noteLine
	for i, e := range entries {
		head := e.Date
		if t, err := time.Parse("2006-01-02", e.Date); err == nil {
			head = displayDate(t, "2 January")
		}
		lines = append(lines, noteLine{text: head + headSuffix(i), head: true, h: 26})
		for _, it := range e.Items {
			for k, l := range wrap(tf, it, w-20, 4) {
				lines = append(lines, noteLine{text: strings.TrimSpace(l), bullet: k == 0, h: 21})
			}
		}
		lines[len(lines)-1].h += 10
	}
	return lines
}

func drawNoteLines(img *image.Gray, lines []noteLine, x0, y int) {
	hf, tf := face("uiBold", 16), face("ui", 15)
	for _, l := range lines {
		switch {
		case l.head:
			drawText(img, hf, x0, y+18, l.text, ink)
		case l.bullet:
			fillCircle(img, float64(x0+5), float64(y+10), 2.5, grey2)
			drawText(img, tf, x0+20, y+15, l.text, grey2)
		default:
			drawText(img, tf, x0+20, y+15, l.text, grey2)
		}
		y += l.h
	}
}

// fitLines: how many lines from start fit in height (at least one; never ending
// halfway through a wrapped item unless one item is taller than the space).
func fitLines(lines []noteLine, start, height int) int {
	h, i := 0, start
	for i < len(lines) && h+lines[i].h <= height {
		h += lines[i].h
		i++
	}
	for j := i; j < len(lines) && j > start+1 && !lines[j].head && !lines[j].bullet; j-- {
		i = j - 1 // back up to the start of the item that didn't fit
	}
	return max(1, i-start)
}

// arrowButton: a small outlined square with a chevron (‹ › ▲ ▼ style), greyed when off.
func arrowButton(v *View, r image.Rectangle, dir string, on bool, act, arg string) {
	img := v.Img
	c := ink
	if !on {
		c = grey4
	}
	strokeRoundRect(img, r, 8, 2, c)
	chevron(img, float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2, 6, dir, c)
	if on {
		v.add(Target{R: r.Inset(-4), Show: r, Act: act, Arg: arg})
	}
}

// patchNotesPane: Settings › Updates › what was new in this version, then older ones with
// the arrows (a version too long for the space continues on the next page).
func patchNotesPane(v *View, ui *UI, x0, x1, y int) {
	img := v.Img
	if len(patchNotes) == 0 {
		return
	}
	type page struct{ entry, from, to int }
	top, bottom := y+18, LH-20
	var pages []page
	for ei, e := range patchNotes {
		lines := noteLines([]NoteEntry{e}, x1-x0, func(int) string { return "" })
		for from := 0; from < len(lines); {
			n := fitLines(lines, from, bottom-top)
			pages = append(pages, page{ei, from, from + n})
			from += n
		}
	}
	ui.NotesPage = max(0, min(ui.NotesPage, len(pages)-1))
	p := pages[ui.NotesPage]
	title := "此版本的更新内容"
	if p.entry > 0 {
		title = "此前的更新内容"
	}
	label(img, x0, y, title, grey2)
	if len(pages) > 1 {
		next := image.Rect(x1-40, y-26, x1, y+10)
		prev := image.Rect(next.Min.X-50, y-26, next.Min.X-10, y+10)
		arrowButton(v, prev, "left", ui.NotesPage > 0, "notespage", "-1")
		arrowButton(v, next, "right", ui.NotesPage < len(pages)-1, "notespage", "1")
	}
	lines := noteLines([]NoteEntry{patchNotes[p.entry]}, x1-x0, func(int) string {
		if p.entry == 0 {
			return " · 当前版本"
		}
		return ""
	})
	drawNoteLines(img, lines[p.from:p.to], x0, top)
}

// notesModal: "What's new in the update", over the faded Settings screen: the waiting
// update's changes (▲ ▼ when they don't fit), Install and Close, and the helper reminder.
func notesModal(v *View, ui *UI) {
	img := v.Img
	for i, p := range img.Pix {
		img.Pix[i] = 255 - uint8(int(255-p)*28/100)
	}
	v.Targets = nil
	box := image.Rect(LW/2-340, 60, LW/2+340, LH-60)
	fillRoundRect(img, box.Inset(-3), 16, paper)
	strokeRoundRect(img, box, 14, 2, ink)
	x0, x1 := box.Min.X+32, box.Max.X-32
	drawText(img, face("serifB", 28), x0, box.Min.Y+56, "更新内容", ink)
	entries := updateNotes(ui.UpdateNotes)
	listTop, listBottom := box.Min.Y+80, box.Max.Y-130
	if len(entries) == 0 {
		drawText(img, face("ui", 16), x0, listTop+24, "修复与小幅改进。", grey2)
	} else {
		lines := noteLines(entries, x1-x0-60, func(int) string { return "" })
		ui.NotesScroll = max(0, min(ui.NotesScroll, len(lines)-1))
		n := fitLines(lines, ui.NotesScroll, listBottom-listTop)
		drawNoteLines(img, lines[ui.NotesScroll:ui.NotesScroll+n], x0, listTop)
		if ui.NotesScroll > 0 || ui.NotesScroll+n < len(lines) { // ▲ ▼ down the right side
			up := image.Rect(x1-40, listTop, x1, listTop+40)
			down := image.Rect(x1-40, listBottom-40, x1, listBottom)
			arrowButton(v, up, "up", ui.NotesScroll > 0, "notesscroll", "-1")
			arrowButton(v, down, "down", ui.NotesScroll+n < len(lines), "notesscroll", "1")
		}
	}
	fillRect(img, image.Rect(x0, listBottom+12, x1, listBottom+13), grey4)
	for i, l := range wrap(face("ui", 15), helperReminder, x1-x0, 2) {
		drawText(img, face("ui", 15), x0, listBottom+40+i*20, l, grey2)
	}
	by := box.Max.Y - 70
	inst := pillAt(img, image.Rect(x1-200, by, x1, by+46), "安装更新", true)
	v.add(Target{R: inst.Inset(-4), Show: inst, Act: "update"})
	cl := pillAt(img, image.Rect(inst.Min.X-132, by, inst.Min.X-12, by+46), "关闭", false)
	v.add(Target{R: cl.Inset(-4), Show: cl, Act: "notesmodal"})
}
