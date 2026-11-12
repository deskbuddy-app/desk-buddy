package main

// The Tasks tab's left column: the pinned task, the list with subtasks, pages, progress.

import (
	"fmt"
	"image"
	"strings"
)

const selBg uint8 = 232 // background of the opened task

type entry struct {
	t              Task
	sub            bool
	kids, kidsDone int
}

func tasksColumn(v *View, d *Desk, ui *UI, now int64) {
	right := sideX - 2 - pad
	if ui.SideHidden {
		right = LW - pad // the whole width; « sits on the TODAY line
	}
	img := v.Img
	if !ui.SideHidden {
		fillRect(img, image.Rect(sideX-2, barH+2, sideX, LH), ink)
	}
	x0, x1 := pad, right
	y := barH + 2 + 20

	kids := map[int][]Task{}
	var top []Task
	for _, t := range d.Tasks {
		if t.Parent != nil {
			kids[*t.Parent] = append(kids[*t.Parent], t)
		} else {
			top = append(top, t)
		}
	}
	var pinned *Task
	for i := range top {
		if top[i].Pinned && !top[i].Done {
			pinned = &top[i]
			break
		}
	}
	if pinned != nil {
		y = nowCard(v, d, ui, *pinned, kids[pinned.ID], x0, x1, y, now) + 16
	}

	today := "TODAY"
	if n := listName(d); n != "" {
		today += " · " + strings.ToUpper(n)
	}
	label(img, x0, y+12, today, grey2)
	plusX := x1 - 22 // the ghost + (add a task) at the end of the line, left of « if that's there
	if ui.SideHidden {
		collapseButton(v, true, image.Rect(x1-44, y-10, x1, y+24), "sidehide")
		plusX = x1 - 44 - 30
	}
	if ui.Full {
		pr := image.Rect(plusX-22, y-10, plusX+22, y+24)
		plusGlyph(img, float64(plusX), float64(y+7), 9, ink)
		v.add(Target{R: pr.Inset(-6), Show: pr, Act: "addnew", Arg: "add"})
	}
	y += 22

	var lines []entry
	done := 0
	for _, t := range top {
		if t.Done {
			done++
		}
		if pinned != nil && t.ID == pinned.ID {
			continue
		}
		l := entry{t: t, kids: len(kids[t.ID])}
		for _, k := range kids[t.ID] {
			if k.Done {
				l.kidsDone++
			}
		}
		lines = append(lines, l)
		if !ui.Collapsed[t.ID] {
			for _, k := range kids[t.ID] {
				lines = append(lines, entry{t: k, sub: true})
			}
		}
	}

	// Split into pages that fit above the footer.
	footY := LH - 26
	avail := footY - 40 - y
	// Pages are worked out with every task closed, so opening one never moves it to
	// another page; if the opened task needs more room, the tasks after it on this page
	// are tucked away until it's closed.
	pages := [][]entry{nil}
	h := 0
	for _, l := range lines {
		lh, _ := rowSize(l.sub)
		if h+lh > avail && len(pages[len(pages)-1]) > 0 {
			pages = append(pages, nil)
			h = 0
		}
		pages[len(pages)-1] = append(pages[len(pages)-1], l)
		h += lh
	}
	ui.Page = max(0, min(ui.Page, len(pages)-1))
	// If the opened task needs more room, drop tasks from the bottom of the page
	// (never the opened one, and never anything above it) until it all fits.
	page := pages[ui.Page]
	show := make([]bool, len(page))
	need, selAt := 0, -1
	for i, l := range page {
		show[i] = true
		if l.t.ID == ui.Selected {
			selAt = i
			need += lineHeight(l, ui, x1-x0)
		} else {
			lh, _ := rowSize(l.sub)
			need += lh
		}
	}
	hidden := 0
	room := avail
	if selAt >= 0 && need > avail {
		room = avail - 26 // keep a line for "N more · close this task to see them"
	}
	for i := len(page) - 1; i > selAt && need > room && selAt >= 0; i-- { // below it first
		lh, _ := rowSize(page[i].sub)
		show[i], need = false, need-lh
		hidden++
	}
	for i := 0; i < selAt && need > room; i++ { // then from the top, if it's still too tall
		lh, _ := rowSize(page[i].sub)
		show[i], need = false, need-lh
		hidden++
	}
	for i, l := range page {
		if show[i] {
			y = taskLine(v, l, ui, x0, x1, y)
		}
	}
	if hidden > 0 {
		drawText(img, face("ui", 14), x0+4, y+22, fmt.Sprintf("%d more · close this task to see them", hidden), grey3)
	}
	if len(top) == 0 {
		drawText(img, face("serif", 23), x0+4, y+36, "Nothing on the list", grey2)
		drawText(img, face("ui", 15), x0+4, y+64, "Add tasks from your phone, Mac or Claude.", grey2)
	}

	// Footer: page buttons, progress bar, count.
	cf := face("serifB", 18)
	cx := drawRight(img, cf, x1, footY, fmt.Sprintf("%d / %d done", done, len(top)), ink)
	mid := footY - capHeight(cf)/2
	left := x0
	if len(pages) > 1 {
		prev := image.Rect(x0, mid-22, x0+48, mid+22)
		next := image.Rect(prev.Max.X+8, mid-22, prev.Max.X+56, mid+22)
		iconButton(img, prev, "left", ui.Page > 0)
		iconButton(img, next, "right", ui.Page < len(pages)-1)
		if ui.Page > 0 {
			v.add(Target{R: prev.Inset(-6), Show: prev, Act: "page", Arg: "-1"})
		}
		if ui.Page < len(pages)-1 {
			v.add(Target{R: next.Inset(-6), Show: next, Act: "page", Arg: "1"})
		}
		pf := face("ui", 15)
		left = drawText(img, pf, next.Max.X+12, mid+capHeight(pf)/2, fmt.Sprintf("Page %d of %d", ui.Page+1, len(pages)), grey2) + 16
	}
	bar := image.Rect(left, mid-6, cx-18, mid+6)
	if len(top) > 0 && done > 0 {
		fill := bar
		fill.Max.X = bar.Min.X + bar.Dx()*done/len(top)
		fillRoundRect(img, bar, 6, ink, fill)
	}
	strokeRoundRect(img, bar, 6, 2, ink)
}

func nowCard(v *View, d *Desk, ui *UI, t Task, kids []Task, x0, x1, y int, now int64) int {
	img := v.Img
	p := d.Pomodoro
	timer := p.active() && p.Task == t.ID
	// buttons on the right: idle → Focus, Done, Unpin; timer on → Pause/Resume, Done, Stop
	// label "" with an icon: a square icon button; otherwise a word pill.
	type btn struct {
		label, act, arg, icon string
		solid                 bool
	}
	btns := []btn{{"", "unpin", d.Profile, "unpin", false}} // done: the round box before the title, as in the list
	if d.Pomodoro != nil {                                  // the backend has the timer
		btns = append(btns, btn{"Focus", "pomo", "start", "", false})
	}
	if timer {
		pr := btn{"", "pomo", "pause", "pause", false}
		if p.State == "paused" {
			pr = btn{"", "pomo", "resume", "play", false}
		}
		btns = []btn{{"", "pomo", "stop", "stop", false}, pr}
	}
	bf := face("uiBold", 16)
	bw := 0
	for _, b := range btns {
		if b.icon != "" {
			bw += 50 + 10
		} else {
			bw += textW(bf, b.label) + 32 + 10
		}
	}
	f, lineH := face("serifB", 30), 0
	const tx = 50 // the title starts after the round tick box
	tw := x1 - 16 - bw - 14 - (x0 + tx)
	open := ui.Selected == t.ID // tapped: the whole title, and the description
	lines := []string{t.Text}
	more := false // there's more of the title than shows (closed)
	if textW(f, t.Text) > tw {
		f, lineH = face("serifB", 24), 30
		// lines leave room for the ⌄ / ⌃ after them, so nothing gets cut in the middle
		all := wrap(f, t.Text, tw-24, 99)
		lines = all
		if !open && len(all) > 2 {
			lines = append([]string{}, all[:2]...)
			lines[1] = ellipsize(f, strings.Join(all[1:], " "), tw-24)
			more = true
		}
	}
	var notes []string
	if open && t.Notes != "" {
		notes = wrap(face("ui", 16), t.Notes, x1-x0-32, 4)
	}
	const kidH = 44
	shownKids := 0 // open: its subtasks, each with a box to tick
	if open {
		shownKids = len(kids)
	}
	// the title's first line sits level with the buttons (their middle is 38 below the top)
	base := 38 + capHeight(f)/2
	h := max(76, base+30) + lineH*(len(lines)-1) + len(notes)*22 + shownKids*kidH
	if shownKids > 0 {
		h += 12
	}
	if timer {
		h += 26 // room for the progress bar
	}
	lbl := "NOW WORKING ON"
	if timer {
		lbl = pomoCardLine(p, now)
	}
	if len(kids) > 0 {
		n := 0
		for _, k := range kids {
			if k.Done {
				n++
			}
		}
		lbl += fmt.Sprintf(" · %d/%d SUBTASKS", n, len(kids))
	}
	label(img, x0, y+12, lbl, grey2) // above the card, like "TODAY · MAC"
	y += 24                          // the card starts under it
	card := image.Rect(x0, y, x1, y+h)
	strokeRoundRect(img, card, 9, 3, ink)
	right := x1 - 16
	cy := y + 38
	for _, b := range btns {
		var r image.Rectangle
		if b.icon != "" {
			r = iconPill(img, image.Rect(right-50, cy-23, right, cy+23), b.icon, b.solid)
		} else {
			r = pill(img, right, cy, b.label, b.solid)
		}
		id := 0
		if b.act == "toggle" {
			id = t.ID
		}
		if b.act == "pomo" && b.arg == "start" {
			id = t.ID
		}
		v.add(Target{R: r.Inset(-5), Show: r, Act: b.act, ID: id, Arg: b.arg})
		right = r.Min.X - 10
	}
	lastEnd := 0
	for i, l := range lines {
		lastEnd = drawText(img, f, x0+tx, y+base+i*lineH, l, ink)
	}
	cy0 := float64(y + base + (len(lines)-1)*lineH - capHeight(f)/2)
	if !open && (more || t.Notes != "" || len(kids) > 0) {
		chevron(img, float64(lastEnd+14), cy0, 6, "down", grey2)
	} else if open && (len(lines) > 1 || len(notes) > 0 || len(kids) > 0) {
		chevron(img, float64(lastEnd+14), cy0, 6, "up", grey2)
	}
	ny := y + base + (len(lines)-1)*lineH
	for i, n := range notes {
		drawText(img, face("ui", 16), x0+tx, ny+28+i*22, n, grey2)
	}
	ny += len(notes) * 22
	v.add(Target{R: image.Rect(x0+tx-8, y+12, x0+tx+tw, ny+20), Act: "select", ID: t.ID})
	// the round tick box, level with the title's first line (like every task in the list)
	{
		const r = 14.0
		cx, cy := float64(x0+16)+r-2, float64(y+base-capHeight(f)/2)
		ringCircle(img, cx, cy, r, 2.5, ink)
		v.add(Target{R: image.Rect(x0+4, y+12, x0+tx-6, y+base+lineH*(len(lines)-1)+14), Show: image.Rect(int(cx-r)-4, int(cy-r)-4, int(cx+r)+4, int(cy+r)+4), Act: "toggle", ID: t.ID})
	}
	// its subtasks, when open: tick them off here (they're not in the list below)
	ky := ny + 22
	for i, k := range kids[:shownKids] {
		r := image.Rect(x0+tx, ky+i*kidH, x1-16, ky+(i+1)*kidH)
		fillRect(img, image.Rect(r.Min.X+36, r.Min.Y, r.Max.X, r.Min.Y+1), grey4)
		cx, cyk := float64(r.Min.X+12), float64(r.Min.Y+kidH/2)
		ringCircle(img, cx, cyk, 10, 2.2, ink)
		c := ink
		if k.Done {
			fillCircle(img, cx, cyk, 10, ink)
			line(img, cx-5, cyk, cx-1.5, cyk+4, 2.2, paper)
			line(img, cx-1.5, cyk+4, cx+5, cyk-4, 2.2, paper)
			c = grey3
		}
		kf := face("serif", 20)
		end := drawText(img, kf, r.Min.X+36, int(cyk)+capHeight(kf)/2, ellipsize(kf, k.Text, r.Dx()-40), c)
		if k.Done {
			fillRect(img, image.Rect(r.Min.X+36, int(cyk)-1, end, int(cyk)+1), grey3)
		}
		v.add(Target{R: r, Show: image.Rect(r.Min.X-2, r.Min.Y+4, r.Min.X+26, r.Max.Y-4), Act: "toggle", ID: k.ID})
	}
	if timer {
		bar := image.Rect(x0+16, y+h-26, x1-70, y+h-16)
		pomoBar(img, bar, p, now)
		sp := image.Rect(x1-54, y+h-40, x1-12, y+h-4)
		speakerGlyph(img, float64(sp.Min.X+sp.Max.X)/2, float64(sp.Min.Y+sp.Max.Y)/2, p.Sound, grey2)
		arg := "sound:1"
		if p.Sound {
			arg = "sound:0"
		}
		v.add(Target{R: sp.Inset(-6), Show: sp, Act: "pomo", Arg: arg})
	}
	return y + h
}

func rowSize(sub bool) (h int, r float64) {
	if compactMode {
		if sub {
			return 52, 12.5
		}
		return 62, 15.5
	}
	if sub {
		return 46, 11
	}
	return 56, 14
}

// descLines is the opened task's description, wrapped.
func descLines(t Task, w int) []string {
	if t.Notes == "" {
		return []string{"No description. Add one on your phone."}
	}
	return wrap(face("ui", 16), t.Notes, w, 4)
}

func textLeft(x0 int, sub bool) int {
	_, r := rowSize(sub)
	indent := 0
	if sub {
		indent = 40
	}
	return x0 + indent + 4 + int(2*r) + 14
}

// titleLines: an opened task shows its whole title, over up to 5 lines.
func titleLines(l entry, w int) (lines []string, gap int) {
	f, gap := face("serif", 23), 30
	if l.sub {
		f, gap = face("serif", 20), 26
	}
	return wrap(f, l.t.Text, w-textLeft(0, l.sub)-120, 5), gap
}

func lineHeight(l entry, ui *UI, w int) int {
	h, _ := rowSize(l.sub)
	if ui.Selected == l.t.ID {
		tl, gap := titleLines(l, w)
		h += (len(tl)-1)*gap + len(descLines(l.t, w-(textLeft(0, l.sub))))*22 + 8 + 60
	}
	return h
}

func taskLine(v *View, l entry, ui *UI, x0, x1, y int) int {
	img := v.Img
	t := l.t
	rh, r := rowSize(l.sub)
	tf := face("serif", 23)
	if l.sub {
		tf = face("serif", 20)
	}
	sel := ui.Selected == t.ID
	total := lineHeight(l, ui, x1-x0)
	if sel {
		fillRoundRect(img, image.Rect(x0-10, y+2, x1+6, y+total-2), 8, selBg)
	} else {
		fillRect(img, image.Rect(x0+textLeft(0, l.sub)-14, y+total-1, x1, y+total), grey4)
	}

	indent := 0
	if l.sub {
		indent = 40
	}
	cx, cy := float64(x0+indent+4)+r, float64(y)+float64(rh)/2
	tickMark(img, cx, cy, r, t.Done) // in the theme's style (themes.go)
	box := image.Rect(x0-8+indent, y, int(cx+r)+10, y+rh)
	v.add(Target{R: box, Show: image.Rect(int(cx-r)-4, int(cy-r)-4, int(cx+r)+4, int(cy+r)+4), Act: "toggle", ID: t.ID})

	right := x1
	if ui.Swiped == t.ID { // swiped right to left: a bin at the end of the row (it covers the rest)
		defer func() {
			b := image.Rect(x1-64, y+5, x1, y+rh-5)
			fillRect(img, image.Rect(b.Min.X-16, y+1, x1+pad, y+rh-1), paper)
			fillRoundRect(img, b, 10, ink)
			binGlyph(img, float64(b.Min.X+b.Max.X)/2, float64(b.Min.Y+b.Max.Y)/2, paper)
			v.add(Target{R: b.Inset(-6), Show: b, Act: "binask", ID: t.ID})
		}()
	}
	if !l.sub && !t.Done {
		pr := image.Rect(right-46, y, right, y+rh)
		pinIcon(img, float64(pr.Min.X+pr.Max.X)/2, cy, false, grey2)
		v.add(Target{R: pr, Show: pr.Inset(4), Act: "pin", ID: t.ID})
		right = pr.Min.X - 4
	}
	if l.kids > 0 {
		f := face("uiBold", 14)
		s := fmt.Sprintf("%d/%d", l.kidsDone, l.kids)
		w := textW(f, s) + 40
		kr := image.Rect(right-w, int(cy)-16, right, int(cy)+16)
		strokeRoundRect(img, kr, 16, 1.5, grey3)
		drawText(img, f, kr.Min.X+12, int(cy)+capHeight(f)/2, s, grey2)
		dir := "down"
		if ui.Collapsed[t.ID] {
			dir = "right"
		}
		chevron(img, float64(kr.Max.X-16), cy, 4.5, dir, grey2)
		v.add(Target{R: image.Rect(kr.Min.X-6, y, kr.Max.X+4, y+rh), Show: kr, Act: "collapse", ID: t.ID})
		right = kr.Min.X - 12
	}
	if t.Tag != "" {
		f := face("uiBold", 13)
		w := textW(f, t.Tag) + 14
		tag := image.Rect(right-w, int(cy)-13, right, int(cy)+13)
		strokeRoundRect(img, tag, 4, 1.5, grey3)
		drawCentered(img, f, (tag.Min.X+tag.Max.X)/2, int(cy)+capHeight(f)/2, t.Tag, grey2)
		right = tag.Min.X - 12
	}
	if t.Notes != "" && !sel {
		nx := float64(right - 16)
		for i, w := range []float64{12, 12, 7} {
			yy := cy - 5 + float64(i)*5
			line(img, nx-6, yy, nx-6+w, yy, 1.8, grey3)
		}
		right -= 26
	}

	tx := textLeft(x0, l.sub)
	c := ink
	if t.Done {
		c = grey3
	}
	base := int(cy) + capHeight(tf)/2
	shown := []string{ellipsize(tf, t.Text, right-tx)}
	if shown[0] != t.Text { // cut off: leave room for the ⌄ after it
		shown[0] = ellipsize(tf, t.Text, right-tx-22)
	}
	extra := 0
	if sel {
		var gap int
		shown, gap = titleLines(l, x1-x0)
		extra = (len(shown) - 1) * gap
		for i, s := range shown[1:] {
			drawText(img, tf, tx, base+(i+1)*gap, s, c)
		}
	}
	end := drawText(img, tf, tx, base, shown[0], c)
	if t.Done {
		mid := base - capHeight(tf)*2/5
		fillRect(img, image.Rect(tx, mid-1, end, mid+1), grey3)
	}
	// ⌄ when there's more to see (a cut-off title), ⌃ when it's open
	if sel {
		chevron(img, float64(min(end+16, right-8)), float64(base-capHeight(tf)/2), 5, "up", grey2)
	} else if shown[0] != t.Text {
		chevron(img, float64(end+12), float64(base-capHeight(tf)/2), 5, "down", grey2)
	}
	v.add(Target{R: image.Rect(tx-8, y, right, y+rh+extra), Act: "select", ID: t.ID})

	if sel {
		dy := y + rh + extra
		df := face("ui", 16)
		for _, s := range descLines(t, x1-tx) {
			drawText(img, df, tx, dy+14, s, grey2)
			dy += 22
		}
		dy += 8
		// Focus, then move up / move down as icon buttons. No Close: tap the task again,
		// or another one, to close it.
		bx := tx
		if ui.Full && !t.Done && !l.sub {
			br := pillAt(img, image.Rect(bx, dy+4, bx+textW(face("uiBold", 16), "Focus")+32, dy+50), "Focus", false)
			v.add(Target{R: br.Inset(-4), Show: br, Act: "pomo", ID: t.ID, Arg: "start"})
			bx = br.Max.X + 10
		}
		if ui.Full {
			for _, dir := range []string{"up", "down"} {
				br := pillAt(img, image.Rect(bx, dy+4, bx+46, dy+50), "", false)
				arrowGlyph(img, float64(br.Min.X+br.Max.X)/2, float64(br.Min.Y+br.Max.Y)/2, dir, ink)
				v.add(Target{R: br.Inset(-4), Show: br, Act: "move", ID: t.ID, Arg: dir})
				bx = br.Max.X + 10
			}
			if !l.sub { // add a subtask
				f := face("uiBold", 16)
				br := pillAt(img, image.Rect(bx, dy+4, bx+textW(f, "Subtask")+58, dy+50), "", false)
				plusGlyph(img, float64(br.Min.X+22), float64(br.Min.Y+br.Max.Y)/2, 7, ink)
				drawText(img, f, br.Min.X+38, (br.Min.Y+br.Max.Y)/2+capHeight(f)/2, "Subtask", ink)
				v.add(Target{R: br.Inset(-4), Show: br, Act: "addnew", ID: t.ID, Arg: "sub"})
			}
		}
	}
	return y + total
}

// binGlyph: a rubbish bin (delete a task).
func binGlyph(img *image.Gray, cx, cy float64, c uint8) {
	fillRect(img, image.Rect(int(cx)-11, int(cy)-9, int(cx)+11, int(cy)-6), c) // lid
	fillRect(img, image.Rect(int(cx)-4, int(cy)-12, int(cx)+4, int(cy)-9), c)  // handle
	fillPolygon(img, [][2]float64{{cx - 9, cy - 5}, {cx + 9, cy - 5}, {cx + 7, cy + 12}, {cx - 7, cy + 12}}, c)
	bg := paper ^ c ^ ink
	for _, dx := range []float64{-3.5, 0, 3.5} { // the slats
		line(img, cx+dx, cy-1, cx+dx*0.85, cy+8, 1.4, bg)
	}
}

// arrowGlyph: an arrow pointing up or down (move a task).
func arrowGlyph(img *image.Gray, cx, cy float64, dir string, c uint8) {
	k := 1.0
	if dir == "down" {
		k = -1
	}
	line(img, cx, cy-9*k, cx, cy+10*k, 2.4, c)
	line(img, cx, cy-10*k, cx-7, cy-3*k, 2.4, c)
	line(img, cx, cy-10*k, cx+7, cy-3*k, 2.4, c)
}

// pinIcon draws a push-pin tilted 45°, centred on (cx, cy).
func pinIcon(img *image.Gray, cx, cy float64, filled bool, c uint8) {
	const s = 0.7071
	seg := func(ax, ay, bx, by, w float64) {
		line(img, cx+(ax-ay)*s, cy+(ax+ay)*s, cx+(bx-by)*s, cy+(bx+by)*s, w, c)
	}
	seg(-5, -9, 5, -9, 3) // head
	if filled {
		seg(0, -8, 0, -2, 8)
	} else {
		seg(-3.5, -8, -3.5, -2, 2.2)
		seg(3.5, -8, 3.5, -2, 2.2)
	}
	seg(-8, -1, 8, -1, 3) // collar
	seg(0, -1, 0, 10, 2)  // needle
}

// iconPill: a square button with a drawn icon (check, unpin, pause, play, stop).
func iconPill(img *image.Gray, r image.Rectangle, icon string, solid bool) image.Rectangle {
	c := ink
	if solid {
		fillRoundRect(img, r, 8, ink)
		c = paper
	} else {
		strokeRoundRect(img, r, 8, 2, ink)
	}
	cx, cy := float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2
	switch icon {
	case "check":
		line(img, cx-8, cy+1, cx-2.5, cy+7, 3.5, c)
		line(img, cx-2.5, cy+7, cx+9, cy-6, 3.5, c)
	case "unpin": // a pin with a line through it
		pinIcon(img, cx, cy, false, c)
		line(img, cx-11, cy-11, cx+11, cy+11, 2.6, paper)
		line(img, cx-11, cy-11, cx+11, cy+11, 2, c)
	case "pause":
		fillRoundRect(img, image.Rect(int(cx)-8, int(cy)-9, int(cx)-3, int(cy)+9), 1.5, c)
		fillRoundRect(img, image.Rect(int(cx)+3, int(cy)-9, int(cx)+8, int(cy)+9), 1.5, c)
	case "play":
		fillPolygon(img, [][2]float64{{cx - 6, cy - 10}, {cx + 9, cy}, {cx - 6, cy + 10}}, c)
	case "stop":
		fillRoundRect(img, image.Rect(int(cx)-8, int(cy)-8, int(cx)+8, int(cy)+8), 2, c)
	}
	return r
}
