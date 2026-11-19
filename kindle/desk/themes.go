package main

// Themes (Settings › Themes): the same screens and layout, drawn in a different style.
//
//	plain     the usual look
//	garden    vines and flowers (garden.go); ticked tasks become flowers
//	sketch    hand-drawn: wobbly double outlines, scribbled ticks, doodles
//	classic   the 1984 black-and-white desktop: striped title bars, square ticks, dotted patterns
//	retro     the 90s desktop: raised grey 3D buttons, sunken boxes, title bars with _ □ ✕
//	notebook  ruled paper: a margin line, punched holes, hand-drawn ticks, tape
//
// Everything goes through a few hooks: buttonFace (pills, segmented choices, tabs), tickMark
// (task circles), decorate (the main screens, after they're drawn) and clockDecor (the
// standby clock's second style, "dressed"). Decoration stays off text and anything tappable,
// and uses fixed seeds, so it's the same on every redraw (no extra flashing).

import (
	"image"
	"math"
	"math/rand"
)

var themes = []struct{ id, name, about string }{
	{"plain", "简洁", "经典外观"},
	{"garden", "花园", "藤蔓与花朵，完成任务时开花"},
	{"sketch", "手绘", "手绘线条与涂鸦"},
	{"classic", "经典", "1984 年风格的黑白桌面"},
	{"retro", "复古", "90 年代的灰色立体按钮"},
	{"notebook", "笔记本", "横线纸、页边距与装订孔"},
	{"typewriter", "打字机", "打字机文字与 [x] 方框"},
	{"gameboy", "Game Boy", "掌机风格的像素外观"},
}

// separateButtons: themes that draw a choice as a row of separate buttons.
func separateButtons() bool {
	return themeName == "retro" || themeName == "sketch" || themeName == "gameboy"
}

// pixelBox: a box with stepped (pixel) corners, b pixels thick.
func pixelBox(img *image.Gray, r image.Rectangle, b int, fill, edge uint8, filled bool) {
	s := b * 2 // the corner step
	inner := []image.Rectangle{
		image.Rect(r.Min.X+s, r.Min.Y, r.Max.X-s, r.Max.Y),
		image.Rect(r.Min.X, r.Min.Y+s, r.Max.X, r.Max.Y-s),
		image.Rect(r.Min.X+b, r.Min.Y+b, r.Max.X-b, r.Max.Y-b),
	}
	for _, q := range inner {
		fillRect(img, q, edge)
	}
	c := fill
	if !filled {
		c = paper
	}
	fillRect(img, image.Rect(r.Min.X+s, r.Min.Y+b, r.Max.X-s, r.Max.Y-b), c)
	fillRect(img, image.Rect(r.Min.X+b, r.Min.Y+s, r.Max.X-b, r.Max.Y-s), c)
}

// ---------- hand-drawn lines (sketch, notebook) ----------

// wobble draws a line as if by hand: slightly wavy, from a fixed seed.
func wobble(img *image.Gray, x0, y0, x1, y1, w float64, c uint8, seed int64) {
	rng := rand.New(rand.NewSource(seed))
	n := int(math.Max(4, math.Hypot(x1-x0, y1-y0)/14))
	nx, ny := -(y1 - y0), x1-x0
	l := math.Hypot(nx, ny)
	if l > 0 {
		nx, ny = nx/l, ny/l
	}
	px, py := x0, y0
	ph := rng.Float64() * 6
	for i := 1; i <= n; i++ {
		t := float64(i) / float64(n)
		off := math.Sin(t*math.Pi*2+ph) * 1.3 * math.Sin(t*math.Pi)
		x, y := x0+(x1-x0)*t+nx*off, y0+(y1-y0)*t+ny*off
		line(img, px, py, x, y, w, c)
		px, py = x, y
	}
}

// sketchRect: a box drawn twice by hand, the strokes overshooting the corners a little.
func sketchRect(img *image.Gray, r image.Rectangle, c uint8, seed int64) {
	x0, y0, x1, y1 := float64(r.Min.X), float64(r.Min.Y), float64(r.Max.X), float64(r.Max.Y)
	for pass := int64(0); pass < 2; pass++ {
		o := float64(pass)*1.5 - 0.5
		s := seed*7 + pass*3
		wobble(img, x0-3, y0+o, x1+2, y0-o, 1.6, c, s)
		wobble(img, x1+o, y0-3, x1-o, y1+2, 1.6, c, s+1)
		wobble(img, x1+3, y1-o, x0-2, y1+o, 1.6, c, s+2)
		wobble(img, x0-o, y1+3, x0+o, y0-2, 1.6, c, s+3)
	}
}

// sketchCircle: a circle drawn by hand, a little more than once round.
func sketchCircle(img *image.Gray, cx, cy, r float64, c uint8, seed int64) {
	rng := rand.New(rand.NewSource(seed))
	start := rng.Float64() * 6
	px, py := cx+r*math.Cos(start), cy+r*math.Sin(start)
	for i := 1; i <= 46; i++ {
		a := start + float64(i)/40*2*math.Pi
		rr := r * (1 + 0.06*math.Sin(a*3+start))
		x, y := cx+rr*math.Cos(a), cy+rr*math.Sin(a)
		line(img, px, py, x, y, 2, c)
		px, py = x, y
	}
}

// handTick: a bold hand-drawn ✓, a little bigger than its box.
func handTick(img *image.Gray, cx, cy, r float64, c uint8) {
	wobble(img, cx-r*0.9, cy-r*0.05, cx-r*0.2, cy+r*0.75, 3.4, c, 11)
	wobble(img, cx-r*0.2, cy+r*0.75, cx+r*1.2, cy-r*1.1, 3.4, c, 12)
}

// ---------- frames: every outlined or filled box on every screen ----------

// strokeRoundRect draws a box's outline. Black outlines (buttons, cards, tabs, keys, boxes)
// take the theme's style; grey ones (quiet dividers, pictures) stay as they are.
func strokeRoundRect(img *image.Gray, r image.Rectangle, rad, t float64, c uint8) {
	if c != ink || t < 1.4 || r.Dx() < 22 || r.Dy() < 20 { // small pieces of icons stay as drawn
		rawStrokeRoundRect(img, r, rad, t, c)
		return
	}
	switch themeName {
	case "sketch":
		sketchRect(img, r, ink, int64(r.Min.X*31+r.Min.Y*7+r.Dx()))
	case "retro": // raised: a light edge, dark bottom and right
		rawStrokeRoundRect(img, r, 0, 1, grey3)
		fillRect(img, image.Rect(r.Min.X, r.Max.Y-2, r.Max.X, r.Max.Y), ink)
		fillRect(img, image.Rect(r.Max.X-2, r.Min.Y, r.Max.X, r.Max.Y), ink)
	case "gameboy":
		pixelFrame(img, r, 3)
	case "typewriter":
		rawStrokeRoundRect(img, r, 0, math.Max(1.5, t*0.75), ink)
	case "classic":
		rawStrokeRoundRect(img, r, math.Min(rad, 4), t, ink)
	default:
		rawStrokeRoundRect(img, r, rad, t, c)
	}
}

// fillRoundRect fills a box. Black fills (the chosen tab, the main button) take the theme's
// corners and edges; they stay black, so whatever is drawn on them in white still reads.
func fillRoundRect(img *image.Gray, r image.Rectangle, rad float64, c uint8, clip ...image.Rectangle) {
	if c != ink || r.Dx() < 22 || r.Dy() < 20 { // small pieces of icons, thin bars: as drawn
		rawFillRoundRect(img, r, rad, c, clip...)
		return
	}
	if len(clip) > 0 { // part of a box (a chosen choice): just the corners
		switch themeName {
		case "typewriter", "gameboy", "retro":
			rad = 0
		case "classic":
			rad = math.Min(rad, 4)
		}
		rawFillRoundRect(img, r, rad, c, clip...)
		return
	}
	switch themeName {
	case "sketch":
		rawFillRoundRect(img, r, 3, ink)
		sketchRect(img, r, ink, int64(r.Min.X*13+r.Min.Y))
	case "retro": // black, with the 3D highlight on top and left
		rawFillRoundRect(img, r, 0, ink)
		fillRect(img, image.Rect(r.Min.X+2, r.Min.Y+2, r.Max.X-2, r.Min.Y+4), grey2)
		fillRect(img, image.Rect(r.Min.X+2, r.Min.Y+2, r.Min.X+4, r.Max.Y-2), grey2)
	case "gameboy": // square, with stepped corners
		rawFillRoundRect(img, r, 0, ink)
		for _, q := range []image.Point{r.Min, {r.Max.X - 3, r.Min.Y}, {r.Min.X, r.Max.Y - 3}, {r.Max.X - 3, r.Max.Y - 3}} {
			fillRect(img, image.Rect(q.X, q.Y, q.X+3, q.Y+3), paper)
		}
	case "typewriter":
		rawFillRoundRect(img, r, 0, ink)
	case "classic":
		rawFillRoundRect(img, r, math.Min(rad, 4), ink)
	default:
		rawFillRoundRect(img, r, rad, c)
	}
}

// pixelFrame: a box's outline in pixels (b thick), with stepped corners; inside left alone.
func pixelFrame(img *image.Gray, r image.Rectangle, b int) {
	s := b * 2
	fillRect(img, image.Rect(r.Min.X+s, r.Min.Y, r.Max.X-s, r.Min.Y+b), ink)
	fillRect(img, image.Rect(r.Min.X+s, r.Max.Y-b, r.Max.X-s, r.Max.Y), ink)
	fillRect(img, image.Rect(r.Min.X, r.Min.Y+s, r.Min.X+b, r.Max.Y-s), ink)
	fillRect(img, image.Rect(r.Max.X-b, r.Min.Y+s, r.Max.X, r.Max.Y-s), ink)
	for _, q := range []image.Point{{r.Min.X + b, r.Min.Y + b}, {r.Max.X - s, r.Min.Y + b}, {r.Min.X + b, r.Max.Y - s}, {r.Max.X - s, r.Max.Y - s}} {
		fillRect(img, image.Rect(q.X, q.Y, q.X+b, q.Y+b), ink)
	}
}

// ---------- the hooks ----------

// buttonFace draws a button's face in the current theme and returns the colour for its text.
// filled: the main or chosen one (black in Plain).
func buttonFace(img *image.Gray, r image.Rectangle, rad float64, filled bool) uint8 {
	switch themeName {
	case "retro":
		return bevel(img, r, filled)
	case "sketch":
		if filled {
			fillRect(img, r.Inset(2), ink)
		}
		sketchRect(img, r, ink, int64(r.Min.X*31+r.Min.Y))
		if filled {
			return paper
		}
		return ink
	case "classic":
		if filled {
			fillRoundRect(img, r, 4, ink)
			return paper
		}
		strokeRoundRect(img, r, 4, 2, ink)
		return ink
	case "typewriter": // square corners, a thin rule; the chosen one inked in
		if filled {
			fillRect(img, r, ink)
			return paper
		}
		strokeRoundRect(img, r, 0, 1.5, ink)
		return ink
	case "gameboy":
		pixelBox(img, r, 3, ink, ink, filled)
		if filled {
			return paper
		}
		return ink
	}
	if filled {
		rawFillRoundRect(img, r, rad, ink)
		return paper
	}
	rawStrokeRoundRect(img, r, rad, 2, ink)
	return ink
}

// bevel: a 90s 3D button, raised (light grey, white top-left, black bottom-right), or pressed
// in when chosen.
func bevel(img *image.Gray, r image.Rectangle, in bool) uint8 {
	fillRect(img, r, grey4)
	hi, lo := paper, ink
	if in {
		hi, lo = ink, paper
		fillRect(img, r.Inset(2), grey3)
	}
	fillRect(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+2), hi)
	fillRect(img, image.Rect(r.Min.X, r.Min.Y, r.Min.X+2, r.Max.Y), hi)
	fillRect(img, image.Rect(r.Min.X, r.Max.Y-2, r.Max.X, r.Max.Y), lo)
	fillRect(img, image.Rect(r.Max.X-2, r.Min.Y, r.Max.X, r.Max.Y), lo)
	if !in {
		fillRect(img, image.Rect(r.Min.X+2, r.Max.Y-4, r.Max.X-2, r.Max.Y-2), grey2)
		fillRect(img, image.Rect(r.Max.X-4, r.Min.Y+2, r.Max.X-2, r.Max.Y-2), grey2)
	}
	return ink
}

// tickMark draws a task's circle (r its radius), ticked or not, in the current theme.
func tickMark(img *image.Gray, cx, cy, r float64, done bool) {
	switch themeName {
	case "garden":
		if done {
			flower(img, cx, cy, r*1.08, 0.3, true)
			return
		}
		ringCircle(img, cx, cy, r, 2.5, ink)
	case "sketch":
		sketchCircle(img, cx, cy, r, ink, int64(cx*7+cy))
		if done { // scribbled in
			for i := -3; i <= 3; i++ {
				o := float64(i) * r / 3.6
				h := math.Sqrt(math.Max(0, r*r*0.75-o*o))
				wobble(img, cx+o-h*0.5, cy+h*0.85, cx+o+h*0.5, cy-h*0.85, 2.2, ink, int64(i+9))
			}
		}
	case "classic":
		b := image.Rect(int(cx-r*0.85), int(cy-r*0.85), int(cx+r*0.85), int(cy+r*0.85))
		strokeRoundRect(img, b, 1, 2, ink)
		if done { // an ✕ across the box
			line(img, float64(b.Min.X)+3, float64(b.Min.Y)+3, float64(b.Max.X)-3, float64(b.Max.Y)-3, 2.4, ink)
			line(img, float64(b.Max.X)-3, float64(b.Min.Y)+3, float64(b.Min.X)+3, float64(b.Max.Y)-3, 2.4, ink)
		}
	case "retro":
		b := image.Rect(int(cx-r*0.85), int(cy-r*0.85), int(cx+r*0.85), int(cy+r*0.85))
		fillRect(img, b, paper) // sunken: dark top-left, light bottom-right
		fillRect(img, image.Rect(b.Min.X, b.Min.Y, b.Max.X, b.Min.Y+2), grey2)
		fillRect(img, image.Rect(b.Min.X, b.Min.Y, b.Min.X+2, b.Max.Y), grey2)
		fillRect(img, image.Rect(b.Min.X+2, b.Min.Y+2, b.Max.X, b.Min.Y+4), ink)
		fillRect(img, image.Rect(b.Min.X+2, b.Min.Y+2, b.Min.X+4, b.Max.Y), ink)
		fillRect(img, image.Rect(b.Min.X, b.Max.Y-2, b.Max.X, b.Max.Y), grey4)
		fillRect(img, image.Rect(b.Max.X-2, b.Min.Y, b.Max.X, b.Max.Y), grey4)
		if done {
			k := r / 14
			line(img, cx-6*k, cy, cx-2*k, cy+5*k, 3.2*k, ink)
			line(img, cx-2*k, cy+5*k, cx+6.5*k, cy-5*k, 3.2*k, ink)
		}
	case "notebook":
		ringCircle(img, cx, cy, r, 1.6, grey2)
		if done {
			handTick(img, cx, cy, r, ink)
		}
	case "typewriter": // [ ] and [x], typed
		f := face("monoB", r*1.9)
		txt := "[ ]"
		if done {
			txt = "[x]"
		}
		drawCentered(img, f, int(cx), int(cy)+capHeight(f)/2, txt, ink)
	case "gameboy": // a pixel box; ticked: a pixel check inside
		b := image.Rect(int(cx-r*0.85), int(cy-r*0.85), int(cx+r*0.85), int(cy+r*0.85))
		pixelBox(img, b, 3, ink, ink, false)
		if done {
			p := int(math.Max(2, r/7)) // pixel size
			for i, d := range [][2]int{{-3, 0}, {-2, 1}, {-1, 2}, {0, 1}, {1, 0}, {2, -1}, {3, -2}} {
				_ = i
				x, y := int(cx)+d[0]*p, int(cy)+d[1]*p
				fillRect(img, image.Rect(x-p/2-1, y-p/2-1, x+p/2+1, y+p/2+1), ink)
			}
		}
	default:
		if done {
			fillCircle(img, cx, cy, r, ink)
			k := r / 14
			line(img, cx-6.5*k, cy+0.5*k, cx-2*k, cy+5*k, 3*k, paper)
			line(img, cx-2*k, cy+5*k, cx+6.5*k, cy-5*k, 3*k, paper)
			return
		}
		ringCircle(img, cx, cy, r, 2.5, ink)
	}
}

// onPaper: set the layout point (x, y) to c only where it's still blank (patterns go behind
// what's drawn); on a larger canvas, each pixel it covers.
func onPaper(img *image.Gray, x, y int, c uint8) {
	s := scaleOf(img)
	if s == 1 {
		onPaperPx(img, x, y, c)
		return
	}
	for py := px(y, s); py < px(y+1, s); py++ {
		for pxx := px(x, s); pxx < px(x+1, s); pxx++ {
			onPaperPx(img, pxx, py, c)
		}
	}
}

// onPaperPx: onPaper for one pixel.
func onPaperPx(img *image.Gray, x, y int, c uint8) {
	if x >= 0 && y >= 0 && x < img.Rect.Dx() && y < img.Rect.Dy() && img.Pix[y*img.Stride+x] == paper {
		img.Pix[y*img.Stride+x] = c
	}
}

// pixAt: the grey at the layout point (x, y) (the middle of the pixels it covers).
func pixAt(img *image.Gray, x, y int) uint8 {
	s := scaleOf(img)
	if s != 1 {
		x, y = int((float64(x)+0.5)*s), int((float64(y)+0.5)*s)
	}
	return img.Pix[y*img.Stride+x]
}

// columnLine: is there a full-height black line at x (the column divider)?
func columnLine(img *image.Gray, x int) bool {
	for _, y := range []int{barH + 40, LH / 2, LH - 30} {
		dark := false
		for dx := -1; dx <= 1; dx++ {
			if pixAt(img, x+dx, y) < 100 {
				dark = true
			}
		}
		if !dark {
			return false
		}
	}
	return true
}

// decorate dresses the main screens (Tasks, Deck, Calendar, Weather) and Settings after
// they're drawn. divX: the screen's column line (none on the Deck and the full Calendar).
func decorate(v *View) {
	img := v.Img
	divX := 0 // the column line, only if it's on screen (a hidden side panel takes it away)
	for _, x := range []int{sideX - 1, 258} {
		if columnLine(img, x) {
			divX = x
		}
	}
	switch themeName {
	case "garden":
		gardenTasks(v, divX)          // the top bar's vine always; the column's only when there is one
		for _, t := range v.Targets { // a little sprout on the chosen tab
			if t.Act == "tab" && t.Show.Dy() > 30 && pixAt(img, t.Show.Min.X+10, t.Show.Min.Y+4) == ink {
				x, y := float64(t.Show.Max.X-6), float64(t.Show.Min.Y+3)
				leaf(img, x, y, -0.9, 13, 7)
				leaf(img, x, y, -2.2, 10, 6)
			}
		}
	case "classic": // a striped title bar: fine lines in the top bar's empty gaps only
		clear := make([]bool, LW)
		for x := 0; x < LW; x++ {
			clear[x] = true
			for y := 4; y < barH-2 && clear[x]; y++ {
				clear[x] = pixAt(img, x, y) == paper
			}
		}
		for x := 0; x < LW; {
			if !clear[x] {
				x++
				continue
			}
			e := x
			for e < LW && clear[e] {
				e++
			}
			if a, b := x+10, e-10; b-a > 24 { // a gap wide enough: stripes, clear of its neighbours
				for y := 14; y <= barH-14; y += 4 {
					fillRect(img, image.Rect(a, y, b, y+1), ink)
				}
			}
			x = e
		}
	case "retro": // a 3D edge under the top bar and down the column line
		fillRect(img, image.Rect(0, barH-2, LW, barH), paper)
		fillRect(img, image.Rect(0, barH+2, LW, barH+3), grey2)
	case "notebook": // punched holes down the left edge, and the margin line
		for y := barH + 60; y < LH-30; y += 110 {
			fillCircle(img, 10, float64(y), 7, paper)
			ringCircle(img, 10, float64(y), 7, 1.6, grey3)
		}
		for y := barH + 2; y < LH; y++ {
			onPaper(img, 21, y, grey3)
			onPaper(img, 24, y, grey3)
		}
	case "sketch": // a hand-drawn line along the top bar and the column
		wobble(img, 6, float64(barH)+4, LW-6, float64(barH)+3, 1.4, ink, 5)
		if divX > 0 {
			wobble(img, float64(divX)+4, float64(barH)+8, float64(divX)+5, float64(LH-6), 1.4, ink, 6)
		}
	case "typewriter": // a typed rule under the top bar: – – – –
		fillRect(img, image.Rect(0, barH, LW, barH+2), paper)
		for x := 4; x < LW-4; x += 14 {
			fillRect(img, image.Rect(x, barH, x+8, barH+2), ink)
		}
	case "gameboy": // a chunky dotted line under the top bar and down the column
		fillRect(img, image.Rect(0, barH, LW, barH+2), paper)
		for x := 0; x < LW; x += 8 {
			fillRect(img, image.Rect(x, barH-1, x+4, barH+3), ink)
		}
		if divX > 0 {
			fillRect(img, image.Rect(divX-1, barH+2, divX+2, LH), paper)
			for y := barH + 6; y < LH; y += 8 {
				fillRect(img, image.Rect(divX-2, y, divX+2, y+4), ink)
			}
		}
	}
}

// clockDecor dresses the standby clock's second style in the current theme (Plain: the garden).
func clockDecor(img *image.Gray) {
	switch themeName {
	case "classic":
		classicClock(img)
	case "retro":
		retroClock(img)
	case "notebook":
		notebookClock(img)
	case "sketch":
		sketchClock(img)
	case "typewriter":
		typewriterClock(img)
	case "gameboy":
		gameboyClock(img)
	default:
		gardenStandby(img)
	}
}

// typewriterClock: a typed page: a heading, a rule, and a page number at the foot.
func typewriterClock(img *image.Gray) {
	sheet := image.Rect(130, 40, LW-130, LH-40)
	strokeRoundRect(img, sheet, 0, 1.2, grey3)
	for i := 1; i <= 4; i++ { // the page's shadow, stepped
		fillRect(img, image.Rect(sheet.Max.X+i, sheet.Min.Y+4+i, sheet.Max.X+i+1, sheet.Max.Y+i), grey4)
		fillRect(img, image.Rect(sheet.Min.X+4+i, sheet.Max.Y+i, sheet.Max.X+i, sheet.Max.Y+i+1), grey4)
	}
	f := face("monoB", 17)
	drawText(img, f, sheet.Min.X+40, sheet.Min.Y+52, "DESK BUDDY — 每日记录", ink)
	for x := sheet.Min.X + 40; x < sheet.Max.X-40; x += 12 {
		fillRect(img, image.Rect(x, sheet.Min.Y+66, x+7, sheet.Min.Y+68), ink)
	}
	drawCentered(img, face("mono", 15), LW/2, sheet.Max.Y-24, "- 1 -", grey2)
}

// gameboyClock: a chunky pixel frame, three hearts and a little pixel critter.
func gameboyClock(img *image.Gray) {
	frame := image.Rect(60, 40, LW-60, LH-70)
	for _, k := range []int{0, 10} { // a double pixel border
		r := frame.Inset(k)
		fillRect(img, image.Rect(r.Min.X+8, r.Min.Y, r.Max.X-8, r.Min.Y+4), ink)
		fillRect(img, image.Rect(r.Min.X+8, r.Max.Y-4, r.Max.X-8, r.Max.Y), ink)
		fillRect(img, image.Rect(r.Min.X, r.Min.Y+8, r.Min.X+4, r.Max.Y-8), ink)
		fillRect(img, image.Rect(r.Max.X-4, r.Min.Y+8, r.Max.X, r.Max.Y-8), ink)
		for _, c := range [][2]int{{r.Min.X + 4, r.Min.Y + 4}, {r.Max.X - 8, r.Min.Y + 4}, {r.Min.X + 4, r.Max.Y - 8}, {r.Max.X - 8, r.Max.Y - 8}} {
			fillRect(img, image.Rect(c[0], c[1], c[0]+4, c[1]+4), ink)
		}
	}
	sprite := func(x, y, p int, rows []string) {
		for j, row := range rows {
			for i, ch := range row {
				if ch == '#' {
					fillRect(img, image.Rect(x+i*p, y+j*p, x+(i+1)*p, y+(j+1)*p), ink)
				} else if ch == '+' {
					fillRect(img, image.Rect(x+i*p, y+j*p, x+(i+1)*p, y+(j+1)*p), grey3)
				}
			}
		}
	}
	heart := []string{".##.##.", "#######", "#######", ".#####.", "..###..", "...#..."}
	for i := 0; i < 3; i++ {
		sprite(frame.Min.X+30+i*40, frame.Min.Y+30, 4, heart)
	}
	critter := []string{ // a little round creature, smiling
		"..######..",
		".#++++++#.",
		"#++#++#++#",
		"#++#++#++#",
		"#++++++++#",
		"#+#++++#+#",
		"#++####++#",
		".#++++++#.",
		"..##..##..",
	}
	sprite(frame.Max.X-110, frame.Max.Y-90, 7, critter)
	sprite(frame.Min.X+30, frame.Max.Y-60, 5, []string{"..#..", ".###.", "#####", "..#..", "..#.."}) // a little tree
	sprite(frame.Min.X+70, frame.Max.Y-50, 5, []string{".#.", "###", ".#."})
}

// classicClock: the clock in a 1984 window on a dotted desktop.
func classicClock(img *image.Gray) {
	win := image.Rect(110, 70, LW-110, LH-80)
	around := pxRect(img, win.Inset(-3))
	for y := 0; y < img.Rect.Dy(); y++ { // the desktop: a fine dotted grey (only outside the window), per pixel
		for x := 0; x < img.Rect.Dx(); x++ {
			if !image.Pt(x, y).In(around) && (x+y)%2 == 0 {
				onPaperPx(img, x, y, ink)
			}
		}
	}
	fillRect(img, image.Rect(win.Min.X+6, win.Max.Y, win.Max.X+6, win.Max.Y+6), ink) // its shadow
	fillRect(img, image.Rect(win.Max.X, win.Min.Y+6, win.Max.X+6, win.Max.Y), ink)
	strokeRoundRect(img, win, 1, 2, ink)
	title := image.Rect(win.Min.X, win.Min.Y, win.Max.X, win.Min.Y+30)
	fillRect(img, image.Rect(title.Min.X, title.Max.Y-2, title.Max.X, title.Max.Y), ink)
	for y := title.Min.Y + 7; y < title.Max.Y-5; y += 3 {
		fillRect(img, image.Rect(title.Min.X+6, y, title.Max.X-6, y+1), ink)
	}
	f := face("uiBold", 16)
	tw := textW(f, "时钟")
	fillRect(img, image.Rect(LW/2-tw/2-12, title.Min.Y+3, LW/2+tw/2+12, title.Max.Y-3), paper)
	drawCentered(img, f, LW/2, title.Min.Y+21, "时钟", ink)
	box := image.Rect(title.Min.X+18, title.Min.Y+8, title.Min.X+34, title.Min.Y+22) // the close box
	fillRect(img, box.Inset(-3), paper)
	strokeRoundRect(img, box, 0, 1.6, ink)
	dots := image.Rect(LW/2-80, LH-42, LW/2+80, LH-10) // clear under the page dots
	fillRoundRect(img, dots, 8, paper)
	strokeRoundRect(img, dots, 8, 1.5, ink)
}

// retroClock: the clock in a 90s window on a grey desktop, with a taskbar.
func retroClock(img *image.Gray) {
	win := image.Rect(100, 60, LW-100, LH-100)
	inside := pxRect(img, win)
	for y := 0; y < img.Rect.Dy(); y++ {
		for x := 0; x < img.Rect.Dx(); x++ {
			if !image.Pt(x, y).In(inside) {
				onPaperPx(img, x, y, grey3)
			}
		}
	}
	// the window's raised edge
	fillRect(img, image.Rect(win.Min.X-4, win.Min.Y-4, win.Max.X+4, win.Min.Y), paper)
	fillRect(img, image.Rect(win.Min.X-4, win.Min.Y-4, win.Min.X, win.Max.Y+4), paper)
	fillRect(img, image.Rect(win.Min.X-4, win.Max.Y, win.Max.X+4, win.Max.Y+4), ink)
	fillRect(img, image.Rect(win.Max.X, win.Min.Y-4, win.Max.X+4, win.Max.Y+4), ink)
	// the title bar: dark, with the name in white and _ □ ✕
	title := image.Rect(win.Min.X, win.Min.Y, win.Max.X, win.Min.Y+30)
	fillRect(img, title, grey2)
	drawText(img, face("uiBold", 16), title.Min.X+12, title.Min.Y+21, "时钟", paper)
	for i, g := range []string{"_", "□", "✕"} {
		b := image.Rect(title.Max.X-30-(2-i)*28, title.Min.Y+5, title.Max.X-8-(2-i)*28, title.Max.Y-5)
		bevel(img, b, false)
		cx, cy := float64(b.Min.X+b.Max.X)/2, float64(b.Min.Y+b.Max.Y)/2
		switch g {
		case "_":
			line(img, cx-5, cy+4, cx+4, cy+4, 2.4, ink)
		case "□":
			strokeRoundRect(img, image.Rect(int(cx)-5, int(cy)-5, int(cx)+5, int(cy)+5), 0, 1.6, ink)
		default:
			line(img, cx-4, cy-4, cx+4, cy+4, 2, ink)
			line(img, cx+4, cy-4, cx-4, cy+4, 2, ink)
		}
	}
	// the taskbar along the bottom, with Start
	bar := image.Rect(0, LH-44, LW, LH)
	fillRect(img, bar, grey4)
	fillRect(img, image.Rect(0, bar.Min.Y, LW, bar.Min.Y+2), paper)
	start := image.Rect(6, bar.Min.Y+6, 100, bar.Max.Y-6)
	bevel(img, start, false)
	flagGlyph(img, float64(start.Min.X+18), float64(start.Min.Y+start.Max.Y)/2)
	drawText(img, face("uiBold", 15), start.Min.X+34, (start.Min.Y+start.Max.Y)/2+6, "开始", ink)
}

// flagGlyph: four small squares (a plain window, not anyone's logo).
func flagGlyph(img *image.Gray, cx, cy float64) {
	for i, c := range []uint8{grey2, ink, ink, grey2} {
		x, y := int(cx)-7+(i%2)*8, int(cy)-7+(i/2)*8
		fillRect(img, image.Rect(x, y, x+6, y+6), c)
	}
}

// notebookClock: ruled paper, a margin, punched holes and two strips of tape.
func notebookClock(img *image.Gray) {
	for y := 70; y < LH-10; y += 38 {
		for x := 0; x < LW; x++ {
			onPaper(img, x, y, grey4)
		}
	}
	for y := 0; y < LH; y++ {
		onPaper(img, 92, y, grey3)
		onPaper(img, 95, y, grey3)
	}
	for _, y := range []int{140, LH / 2, LH - 140} {
		fillCircle(img, 40, float64(y), 13, paper)
		ringCircle(img, 40, float64(y), 13, 2, grey3)
	}
	tape := func(cx, cy, a float64) { // a strip of tape, its short ends torn (zigzag)
		ca, sa := math.Cos(a), math.Sin(a)
		at := func(u, v float64) [2]float64 { return [2]float64{cx + u*ca - v*sa, cy + u*sa + v*ca} }
		const L, W = 80.0, 20.0
		var p [][2]float64
		p = append(p, at(-L, -W), at(L, -W))
		for k := 0; k <= 6; k++ { // the right end, torn
			p = append(p, at(L+float64(k%2)*5, -W+float64(k)*W/3))
		}
		p = append(p, at(-L, W))
		for k := 6; k >= 0; k-- { // the left end, torn
			p = append(p, at(-L-float64(k%2)*5, -W+float64(k)*W/3))
		}
		fillPolygon(img, p, grey4)
		outline(img, p, 1, grey3)
	}
	tape(LW-90, 40, 0.6)
	tape(150, float64(LH-40), 0.5)
}

// sketchClock: a hand-drawn frame with doodles in the corners.
func sketchClock(img *image.Gray) {
	sketchRect(img, image.Rect(40, 40, LW-40, LH-60), ink, 21)
	star := func(cx, cy, r float64, seed int64) {
		var p [][2]float64
		for i := 0; i <= 10; i++ {
			a := -math.Pi/2 + float64(i)*math.Pi/5
			rr := r
			if i%2 == 1 {
				rr = r * 0.45
			}
			p = append(p, [2]float64{cx + rr*math.Cos(a), cy + rr*math.Sin(a)})
		}
		for i := 1; i < len(p); i++ {
			wobble(img, p[i-1][0], p[i-1][1], p[i][0], p[i][1], 1.8, ink, seed+int64(i))
		}
	}
	star(90, 95, 24, 30)
	star(LW-95, float64(LH-120), 18, 40)
	sketchCircle(img, LW-100, 100, 26, ink, 50) // a little sun with rays
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		wobble(img, LW-100+math.Cos(a)*34, 100+math.Sin(a)*34, LW-100+math.Cos(a)*46, 100+math.Sin(a)*46, 1.8, ink, int64(60+i))
	}
	for i := 0; i < 3; i++ { // a squiggle
		x := 70.0 + float64(i)*28
		wobble(img, x, float64(LH-110), x+14, float64(LH-130), 1.8, ink, int64(70+i))
		wobble(img, x+14, float64(LH-130), x+28, float64(LH-110), 1.8, ink, int64(80+i))
	}
}

// themePreview: a small picture of a theme for its card in Settings › Themes: a header, two
// tasks (one ticked) and a button, drawn with the theme's own hooks.
func themePreview(img *image.Gray, r image.Rectangle, id string) {
	was := themeName
	themeName = id
	defer func() { themeName = was }()
	fillRect(img, r, paper)
	switch id {
	case "classic":
		for y := r.Min.Y + 4; y < r.Min.Y+20; y += 3 {
			fillRect(img, image.Rect(r.Min.X+4, y, r.Max.X-4, y+1), ink)
		}
	case "retro":
		fillRect(img, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+22), grey2)
	case "notebook":
		for y := r.Min.Y + 32; y < r.Max.Y; y += 22 {
			fillRect(img, image.Rect(r.Min.X, y, r.Max.X, y+1), grey4)
		}
		fillRect(img, image.Rect(r.Min.X+16, r.Min.Y, r.Min.X+17, r.Max.Y), grey3)
	case "gameboy":
		for x := r.Min.X + 4; x < r.Max.X-4; x += 8 {
			fillRect(img, image.Rect(x, r.Min.Y+21, x+4, r.Min.Y+25), ink)
		}
	case "garden":
		rng := rand.New(rand.NewSource(9))
		vine(img, rng, func(t float64) (float64, float64) {
			return float64(r.Max.X) - 10 + 4*math.Sin(t*8), float64(r.Max.Y) - 4 - t*float64(r.Dy()-8)
		}, 60, 16, true, func(x, y float64) bool { return image.Pt(int(x), int(y)).In(r.Inset(1)) })
	}
	if id != "classic" && id != "retro" && id != "gameboy" {
		fillRect(img, image.Rect(r.Min.X+4, r.Min.Y+22, r.Max.X-4, r.Min.Y+23), ink)
		if id == "sketch" {
			wobble(img, float64(r.Min.X+4), float64(r.Min.Y+25), float64(r.Max.X-4), float64(r.Min.Y+24), 1.2, ink, 3)
		}
	}
	f := face("ui", 13)
	for i := 0; i < 2; i++ {
		y := float64(r.Min.Y + 44 + i*26)
		tickMark(img, float64(r.Min.X+26), y, 8, i == 0)
		c := ink
		if i == 0 {
			c = grey3
		}
		fillRect(img, image.Rect(r.Min.X+42, int(y)-2, r.Min.X+42+[]int{70, 52}[i], int(y)+2), c)
		_ = f
	}
	b := image.Rect(r.Min.X+20, r.Max.Y-30, r.Min.X+90, r.Max.Y-10)
	buttonFace(img, b, 5, false)
	b2 := image.Rect(b.Max.X+8, b.Min.Y, b.Max.X+60, b.Max.Y)
	buttonFace(img, b2, 5, true)
}

// flowerGlyph: Settings › Themes in the sidebar (a small open flower).
func flowerGlyph(img *image.Gray, cx, cy float64, c uint8) {
	for i := 0; i < 5; i++ {
		a := -math.Pi/2 + 2*math.Pi*float64(i)/5
		p := ellipsePts(cx+math.Cos(a)*5.5, cy+math.Sin(a)*5.5, 5.2, 3.4, a, 14)
		outline(img, p, 1.6, c)
	}
	fillCircle(img, cx, cy, 2.6, c)
}

// themesPane: Settings › Themes: a card per theme with a little picture of it; tap to use it.
// The standby clock's second style shows the chosen theme's decorations.
func themesPane(v *View, x0, x1, y int) {
	img := v.Img
	label(img, x0, y, "主题", grey2)
	drawText(img, face("ui", 13), x0, y+22, "应用于各个页面，也用于待机时钟的第二种样式。", grey2)
	const cols, gap = 4, 12
	cw := (x1 - x0 - (cols-1)*gap) / cols
	ch := (LH - 16 - (y + 40) - gap) / 2
	cur := themeName
	if cur == "" {
		cur = "plain"
	}
	for i, t := range themes {
		col, row := i%cols, i/cols
		r := image.Rect(x0+col*(cw+gap), y+40+row*(ch+gap), x0+col*(cw+gap)+cw, y+40+row*(ch+gap)+ch)
		on := t.id == cur
		if on {
			strokeRoundRect(img, r, 10, 3, ink)
		} else {
			strokeRoundRect(img, r, 10, 1.5, grey4)
		}
		pic := image.Rect(r.Min.X+8, r.Min.Y+8, r.Max.X-8, r.Min.Y+8+ch*55/100)
		themePreview(img, pic, t.id)
		strokeRoundRect(img, pic.Inset(-1), 4, 1, grey3)
		nf := face("uiBold", 16)
		ny := pic.Max.Y + 26
		if on {
			fillCircle(img, float64(r.Min.X+16), float64(ny-6), 5.5, ink)
		} else {
			ringCircle(img, float64(r.Min.X+16), float64(ny-6), 5.5, 1.6, grey3)
		}
		drawText(img, nf, r.Min.X+28, ny, t.name, ink)
		tf := face("ui", 12)
		for k, l := range wrap(tf, t.about, cw-20, 3) {
			drawText(img, tf, r.Min.X+10, ny+20+k*16, l, grey2)
		}
		v.add(Target{R: r, Show: r, Act: "settheme2", Arg: t.id})
	}
}
