package main

// Drawing on an 8-bit grey canvas: rectangles, anti-aliased round shapes, and text.
//
// Every screen is laid out in Paperwhite 2 units: LW (1024) wide and LH tall (758 on most
// Kindles, a little more on the wider ones). The canvas holds that at the panel's own
// resolution, sc times larger (1 on a Paperwhite 2, about 1.41 on a Paperwhite 3 / 4, 1.61 on
// a Paperwhite 5), and the functions below scale as they draw, so text and shapes stay sharp.
// Small scratch images (icons being prepared) are drawn 1:1.

import (
	"embed"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// E-ink greys, matching the mockup's --e-ink / --e-2 / --e-3 / --e-4.
const (
	ink   uint8 = 0
	grey2 uint8 = 77
	grey3 uint8 = 139
	grey4 uint8 = 196
	paper uint8 = 255
)

//go:embed fonts/AtkinsonHyperlegible-Regular.ttf fonts/AtkinsonHyperlegible-Bold.ttf fonts/Literata-Medium.ttf fonts/Literata-Bold.ttf fonts/Literata-Display.ttf fonts/Inter-Medium.ttf fonts/Inter-Bold.ttf fonts/Bitter-Medium.ttf fonts/Bitter-Bold.ttf fonts/CourierPrime-Regular.ttf fonts/CourierPrime-Bold.ttf fonts/PressStart2P-Regular.ttf
var fontFiles embed.FS

var (
	fonts = map[string]*opentype.Font{}
	faces = map[string]font.Face{}
)

var (
	LH          = 758     // the layout's height in Paperwhite 2 units (setPanel)
	sc          = 1.0     // canvas pixels per layout unit
	cw, ch      = LW, 758 // the canvas (the panel, landscape) in pixels
	compactMode bool      // Settings › Screen › Screen size: Compact (for 6" Basic Kindles)
)

// setCompact switches between the standard layout and the compact 6" Basic Kindle layout.
func setCompact(on bool) {
	if compactMode != on {
		compactMode = on
		faces = map[string]font.Face{}
	}
}

// setPanel fits the layout to a panel of long × short pixels (landscape): the width stays LW
// units, the height follows the panel's shape. A Paperwhite 2 (1024 × 758) changes nothing.
func setPanel(long, short int) {
	if long < 780 || short < 580 {
		return
	}
	if long <= 800 {
		isBasicKindle = true
	}
	sc = float64(long) / LW
	LH = int(math.Round(LW * float64(short) / float64(long)))
	cw, ch = long, short
	PW, PH = LH, LW // touches are mapped to the layout (portrait)
	kbTop = LH - 318
}

// scaleOf: how many pixels per layout unit img has (the canvas, and scratch images made with
// newScratch: sc; any other image: 1).
func scaleOf(img *image.Gray) float64 {
	if sc != 1 && (img.Rect.Dx() == cw && img.Rect.Dy() == ch || scratch[img]) {
		return sc
	}
	return 1
}

// scratch: small images being drawn at the canvas's resolution (newScratch, then dropScratch).
var scratch = map[*image.Gray]bool{}

// newScratch is a white w × h (layout units) image at the canvas's resolution, drawn on like
// the canvas; dropScratch when done with it.
func newScratch(w, h int) *image.Gray {
	img := newCanvas(px(w, sc), px(h, sc))
	if sc != 1 {
		scratch[img] = true
	}
	return img
}

func dropScratch(img *image.Gray) { delete(scratch, img) }

// px turns a layout coordinate into a pixel one at scale s.
func px(v int, s float64) int {
	if s == 1 {
		return v
	}
	return int(math.Round(float64(v) * s))
}

// pxRect turns a layout rectangle into pixels on img (edges round the same way everywhere, so
// neighbouring areas still meet exactly; 1-unit lines stay at least 1 pixel thick on smaller panels).
func pxRect(img *image.Gray, r image.Rectangle) image.Rectangle {
	s := scaleOf(img)
	if s == 1 {
		return r
	}
	x0, y0, x1, y1 := px(r.Min.X, s), px(r.Min.Y, s), px(r.Max.X, s), px(r.Max.Y, s)
	if r.Dx() > 0 && x1 <= x0 {
		x1 = x0 + 1
	}
	if r.Dy() > 0 && y1 <= y0 {
		y1 = y0 + 1
	}
	return image.Rect(x0, y0, x1, y1)
}

// drawImage draws src filling the layout rectangle r, at the canvas's resolution: copied as it
// is when the sizes match, else scaled (smoothly).
func drawImage(img *image.Gray, r image.Rectangle, src image.Image) {
	r = pxRect(img, r)
	if r.Size() == src.Bounds().Size() {
		draw.Draw(img, r, src, src.Bounds().Min, draw.Src)
		return
	}
	xdraw.ApproxBiLinear.Scale(img, r, src, src.Bounds(), xdraw.Src, nil)
}

// sface is a font at its layout size, carrying the same font at the canvas's size for drawing.
type sface struct {
	font.Face           // at the layout size (measures for the layout)
	big       font.Face // at sc × the size (draws on the canvas)
}

func loadFonts() error {
	for name, file := range map[string]string{
		"ui":      "AtkinsonHyperlegible-Regular.ttf",
		"uiBold":  "AtkinsonHyperlegible-Bold.ttf",
		"serif":   "Literata-Medium.ttf",
		"serifB":  "Literata-Bold.ttf",
		"display": "Literata-Display.ttf",
		"inter":   "Inter-Medium.ttf",
		"interB":  "Inter-Bold.ttf",
		"bitter":  "Bitter-Medium.ttf",
		"bitterB": "Bitter-Bold.ttf",
		"mono":    "CourierPrime-Regular.ttf", // the Typewriter theme
		"monoB":   "CourierPrime-Bold.ttf",
		"pixel":   "PressStart2P-Regular.ttf", // the Game Boy theme
	} {
		b, err := fontFiles.ReadFile("fonts/" + file)
		if err != nil {
			return err
		}
		f, err := opentype.Parse(b)
		if err != nil {
			return err
		}
		fonts[name] = f
	}
	return nil
}

// face returns a cached font face; size is in screen pixels.
func face(name string, size float64) font.Face {
	if compactMode && size <= 23 {
		size += 2 // larger text on 6" Basic Kindles
	}
	switch themeName { // two themes bring their own type, everywhere
	case "typewriter":
		bold := name == "uiBold" || name == "serifB" || name == "display" || name == "monoB"
		name = map[bool]string{false: "mono", true: "monoB"}[bold]
		size *= 0.9 // Courier is wide
	case "gameboy":
		name = "pixel"
		size = math.Max(8, math.Round(size*0.58/4)*4) // a pixel font is crisp in steps of 4
	}
	if name == "serif" || name == "serifB" { // the Font setting, for text and headings
		bold := name == "serifB"
		switch fontStyle {
		case "sans": // Atkinson Hyperlegible
			name = map[bool]string{false: "ui", true: "uiBold"}[bold]
		case "inter", "bitter":
			name = fontStyle + map[bool]string{false: "", true: "B"}[bold]
		}
	}
	key := name + "/" + strconv.FormatFloat(size, 'f', -1, 64)
	if f, ok := faces[key]; ok {
		return f
	}
	mk := func(size float64) font.Face {
		f, err := opentype.NewFace(fonts[name], &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
		if err != nil {
			panic(err)
		}
		return f
	}
	var f font.Face = mk(size)
	if sc != 1 {
		f = &sface{f, mk(size * sc)}
	}
	faces[key] = f
	return f
}

// newCanvas makes a white image; a screen-sized one (LW × LH) is made at the panel's resolution.
func newCanvas(w, h int) *image.Gray {
	if w == LW && h == LH {
		w, h = cw, ch
	}
	img := image.NewGray(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = paper
	}
	return img
}

func fillRect(img *image.Gray, r image.Rectangle, c uint8) {
	r = pxRect(img, r).Intersect(img.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := img.Pix[img.PixOffset(r.Min.X, y):img.PixOffset(r.Max.X, y)]
		for i := range row {
			row[i] = c
		}
	}
}

func invertRect(img *image.Gray, r image.Rectangle) {
	r = pxRect(img, r).Intersect(img.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := img.Pix[img.PixOffset(r.Min.X, y):img.PixOffset(r.Max.X, y)]
		for i := range row {
			row[i] = 255 - row[i]
		}
	}
}

// blend mixes c into the layout point (x, y) with opacity a: on a larger canvas, the whole
// block of pixels it covers (for code that draws dot by dot).
func blend(img *image.Gray, x, y int, c uint8, a float64) {
	s := scaleOf(img)
	if s == 1 {
		blendPx(img, x, y, c, a)
		return
	}
	x0, y0 := px(x, s), px(y, s)
	x1, y1 := max(x0+1, px(x+1, s)), max(y0+1, px(y+1, s))
	for py := y0; py < y1; py++ {
		for pxx := x0; pxx < x1; pxx++ {
			blendPx(img, pxx, py, c, a)
		}
	}
}

// blendPx mixes c into the pixel (x, y) with opacity a.
func blendPx(img *image.Gray, x, y int, c uint8, a float64) {
	if a <= 0 || !image.Pt(x, y).In(img.Rect) {
		return
	}
	i := img.PixOffset(x, y)
	if a >= 1 {
		img.Pix[i] = c
		return
	}
	img.Pix[i] = uint8(float64(img.Pix[i])*(1-a) + float64(c)*a + 0.5)
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// roundRectDist is the signed distance from point (px,py) to the rounded rectangle r.
func roundRectDist(r image.Rectangle, rad, px, py float64) float64 {
	cx, cy := float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2
	hx, hy := float64(r.Dx())/2, float64(r.Dy())/2
	qx, qy := math.Abs(px-cx)-hx+rad, math.Abs(py-cy)-hy+rad
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - rad
}

// fillRoundRect fills r with rounded corners, only touching pixels inside clip.
func rawFillRoundRect(img *image.Gray, r image.Rectangle, rad float64, c uint8, clip ...image.Rectangle) {
	rad *= scaleOf(img)
	r = pxRect(img, r)
	area := r.Inset(-1)
	if len(clip) > 0 {
		area = area.Intersect(pxRect(img, clip[0]))
	}
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			blendPx(img, x, y, c, clamp01(0.5-roundRectDist(r, rad, float64(x)+0.5, float64(y)+0.5)))
		}
	}
}

// strokeRoundRect draws a border of thickness t just inside r.
func rawStrokeRoundRect(img *image.Gray, r image.Rectangle, rad, t float64, c uint8) {
	s := scaleOf(img)
	rad, t, r = rad*s, t*s, pxRect(img, r)
	area := r.Inset(-1)
	for y := area.Min.Y; y < area.Max.Y; y++ {
		for x := area.Min.X; x < area.Max.X; x++ {
			d := roundRectDist(r, rad, float64(x)+0.5, float64(y)+0.5)
			blendPx(img, x, y, c, clamp01(0.5-(math.Abs(d+t/2)-t/2)))
		}
	}
}

func fillCircle(img *image.Gray, cx, cy, rad float64, c uint8) {
	s := scaleOf(img)
	cx, cy, rad = cx*s, cy*s, rad*s
	r := image.Rect(int(cx-rad), int(cy-rad), int(math.Ceil(cx+rad)), int(math.Ceil(cy+rad)))
	for y := r.Min.Y - 1; y <= r.Max.Y; y++ {
		for x := r.Min.X - 1; x <= r.Max.X; x++ {
			blendPx(img, x, y, c, clamp01(0.5-(math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)-rad)))
		}
	}
}

func ringCircle(img *image.Gray, cx, cy, rad, t float64, c uint8) {
	s := scaleOf(img)
	cx, cy, rad, t = cx*s, cy*s, rad*s, t*s
	r := image.Rect(int(cx-rad), int(cy-rad), int(math.Ceil(cx+rad)), int(math.Ceil(cy+rad)))
	for y := r.Min.Y - 1; y <= r.Max.Y; y++ {
		for x := r.Min.X - 1; x <= r.Max.X; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) - rad
			blendPx(img, x, y, c, clamp01(0.5-(math.Abs(d+t/2)-t/2)))
		}
	}
}

// line draws a round-capped segment of width w.
func line(img *image.Gray, ax, ay, bx, by, w float64, c uint8) {
	s := scaleOf(img)
	ax, ay, bx, by, w = ax*s, ay*s, bx*s, by*s, w*s
	minX, maxX := int(math.Min(ax, bx)-w), int(math.Max(ax, bx)+w)+1
	minY, maxY := int(math.Min(ay, by)-w), int(math.Max(ay, by)+w)+1
	dx, dy := bx-ax, by-ay
	l2 := dx*dx + dy*dy
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			t := 0.0
			if l2 > 0 {
				t = clamp01(((px-ax)*dx + (py-ay)*dy) / l2)
			}
			d := math.Hypot(px-(ax+t*dx), py-(ay+t*dy)) - w/2
			blendPx(img, x, y, c, clamp01(0.5-d))
		}
	}
}

// ---------- text ----------

// textW: the width of s in layout units (measured at the size it's drawn, so drawing and
// layout agree on every screen).
func textW(f font.Face, s string) int {
	if sf, ok := f.(*sface); ok {
		return int(math.Ceil(float64(font.MeasureString(sf.big, s)) / 64 / sc))
	}
	return font.MeasureString(f, s).Ceil()
}

func capHeight(f font.Face) int {
	m := f.Metrics()
	if m.CapHeight > 0 {
		return m.CapHeight.Round()
	}
	return (m.Ascent * 7 / 10).Round()
}

// drawText draws s with its baseline at y and returns the x after the last glyph.
func drawText(img *image.Gray, f font.Face, x, y int, s string, c uint8) int {
	if sf, ok := f.(*sface); ok {
		if k := scaleOf(img); k != 1 { // the canvas: the big font, at the point in pixels
			d := font.Drawer{Dst: img, Src: image.NewUniform(color.Gray{c}), Face: sf.big,
				Dot: fixed.Point26_6{X: fixed.Int26_6(math.Round(float64(x) * k * 64)), Y: fixed.Int26_6(math.Round(float64(y) * k * 64))}}
			d.DrawString(s)
			return int(math.Ceil(float64(d.Dot.X) / 64 / k))
		}
		f = sf.Face
	}
	d := font.Drawer{Dst: img, Src: image.NewUniform(color.Gray{c}), Face: f, Dot: fixed.P(x, y)}
	d.DrawString(s)
	return d.Dot.X.Ceil()
}

func drawRight(img *image.Gray, f font.Face, right, y int, s string, c uint8) int {
	x := right - textW(f, s)
	drawText(img, f, x, y, s, c)
	return x
}

func drawCentered(img *image.Gray, f font.Face, cx, y int, s string, c uint8) {
	drawText(img, f, cx-textW(f, s)/2, y, s, c)
}

// drawSpaced draws letter-spaced text (the small uppercase labels).
func drawSpaced(img *image.Gray, f font.Face, x, y int, s string, spacing int, c uint8) int {
	sf, big := f.(*sface)
	for _, r := range s {
		if big && sc != 1 { // step by the same widths spacedW adds up
			drawText(img, f, x, y, string(r), c)
			x += int(math.Ceil(float64(font.MeasureString(sf.big, string(r)))/64/sc)) + spacing
			continue
		}
		x = drawText(img, f, x, y, string(r), c) + spacing
	}
	return x - spacing
}

func spacedW(f font.Face, s string, spacing int) int {
	n := len([]rune(s))
	if n == 0 {
		return 0
	}
	if sf, ok := f.(*sface); ok && sc != 1 { // drawSpaced places letter by letter: add them up the same way
		x := 0
		for _, r := range s {
			x += int(math.Ceil(float64(font.MeasureString(sf.big, string(r))) / 64 / sc))
		}
		return x + spacing*(n-1)
	}
	return textW(f, s) + spacing*(n-1)
}

func ellipsize(f font.Face, s string, maxW int) string {
	if textW(f, s) <= maxW {
		return s
	}
	r := []rune(s)
	for len(r) > 0 {
		r = r[:len(r)-1]
		t := strings.TrimRight(string(r), " ") + "…"
		if textW(f, t) <= maxW {
			return t
		}
	}
	return "…"
}

// wrap breaks s into at most maxLines lines of maxW, ellipsizing the last one.
func wrap(f font.Face, s string, maxW, maxLines int) []string {
	words := strings.Fields(s)
	var lines []string
	cur := ""
	for i, w := range words {
		try := strings.TrimSpace(cur + " " + w)
		if cur == "" || textW(f, try) <= maxW {
			cur = try
			continue
		}
		lines = append(lines, cur)
		cur = w
		if len(lines) == maxLines-1 {
			cur = strings.Join(words[i:], " ")
			break
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	for i := range lines {
		lines[i] = ellipsize(f, lines[i], maxW)
	}
	return lines
}

// fillTriangle fills a triangle, smoothing its edges.
func fillTriangle(img *image.Gray, ax, ay, bx, by, cx, cy float64, c uint8) {
	s := scaleOf(img)
	ax, ay, bx, by, cx, cy = ax*s, ay*s, bx*s, by*s, cx*s, cy*s
	edge := func(px, py, x0, y0, x1, y1 float64) float64 { // signed distance to the edge line
		return ((x1-x0)*(py-y0) - (y1-y0)*(px-x0)) / math.Hypot(x1-x0, y1-y0)
	}
	if edge(cx, cy, ax, ay, bx, by) < 0 { // make the winding consistent
		bx, by, cx, cy = cx, cy, bx, by
	}
	for y := int(math.Min(ay, math.Min(by, cy))) - 1; y <= int(math.Max(ay, math.Max(by, cy)))+1; y++ {
		for x := int(math.Min(ax, math.Min(bx, cx))) - 1; x <= int(math.Max(ax, math.Max(bx, cx)))+1; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			d := math.Min(edge(px, py, ax, ay, bx, by), math.Min(edge(px, py, bx, by, cx, cy), edge(px, py, cx, cy, ax, ay)))
			blendPx(img, x, y, c, clamp01(d+0.5))
		}
	}
}

// fillPolygon fills a simple polygon, smoothing its edges (4 samples per pixel).
func fillPolygon(img *image.Gray, pts [][2]float64, c uint8) {
	if s := scaleOf(img); s != 1 {
		big := make([][2]float64, len(pts))
		for i, p := range pts {
			big[i] = [2]float64{p[0] * s, p[1] * s}
		}
		pts = big
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range pts {
		minX, maxX = math.Min(minX, p[0]), math.Max(maxX, p[0])
		minY, maxY = math.Min(minY, p[1]), math.Max(maxY, p[1])
	}
	in := func(x, y float64) bool { // even-odd rule
		inside := false
		for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
			a, b := pts[i], pts[j]
			if (a[1] > y) != (b[1] > y) && x < (b[0]-a[0])*(y-a[1])/(b[1]-a[1])+a[0] {
				inside = !inside
			}
		}
		return inside
	}
	for y := int(minY) - 1; y <= int(maxY)+1; y++ {
		for x := int(minX) - 1; x <= int(maxX)+1; x++ {
			n := 0
			for _, o := range [4][2]float64{{.25, .25}, {.75, .25}, {.25, .75}, {.75, .75}} {
				if in(float64(x)+o[0], float64(y)+o[1]) {
					n++
				}
			}
			blendPx(img, x, y, c, float64(n)/4)
		}
	}
}
