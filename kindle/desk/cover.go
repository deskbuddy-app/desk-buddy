package main

// The cover screen: tap the album cover in the Music widget for a whole screen of it.
// The cover is big on the left; the song, the controls, Refresh and Back on the right.
//
// The backend only holds a 320 px cover, so it's enlarged here (about 2×). The panel
// shows 16 greys, so the enlarged picture is Bayer-dithered down to those 16 levels:
// smooth shading (skies, skin) then looks smooth instead of banded.

import (
	"bytes"
	_ "embed"
	"image"
	"image/png"
	"math"
	"strings"
	"sync"

	xdraw "golang.org/x/image/draw"
)

const coverSize = 640 // the cover's side on the cover screen

func coverView(v *View, d *Desk, st Status, ui *UI) {
	img := v.Img
	m := d.Music
	hasArt := m != nil && m.Running && m.Title != "" && ui.Art != nil && m.ArtKey == ui.ArtKey
	if ui.CoverFull && hasArt {
		v.Kind = "coverfull"
		coverFullView(v, d, ui)
		return
	}
	y0 := (LH - coverSize) / 2
	cr := image.Rect(y0, y0, y0+coverSize, y0+coverSize) // same margin left as top and bottom
	x0, x1 := cr.Max.X+44, LW-y0                         // the same margin all round: left, right, top and bottom

	// top: back and refresh, their tops level with the cover's top edge
	cy := y0 + 10
	bf := face("uiBold", 17)
	back := image.Rect(x0-8, cy-22, x0+90, cy+22)
	chevron(img, float64(back.Min.X+18), float64(cy), 7, "left", ink)
	drawText(img, bf, back.Min.X+34, cy+capHeight(bf)/2, "Back", ink)
	v.add(Target{R: back.Inset(-6), Show: back, Act: "cover"})
	ref := image.Rect(x1-44, cy-22, x1, cy+22)
	refreshIcon(img, float64(ref.Min.X+ref.Max.X)/2, float64(cy), 10)
	v.add(Target{R: ref.Inset(-6), Show: ref, Act: "refresh"})
	left := ref.Min.X
	if hasArt {
		full := image.Rect(ref.Min.X-52, cy-22, ref.Min.X-8, cy+22)
		cornersGlyph(img, float64(full.Min.X+full.Max.X)/2, float64(cy), 11, false, ink)
		v.add(Target{R: full.Inset(-6), Show: full, Act: "coverfull"})
		left = full.Min.X
	}
	if m != nil && m.Running && m.Title != "" && m.Volume != nil { // the volume, as in the header
		sp := image.Rect(left-52, cy-22, left-8, cy+22)
		speakerGlyph(img, float64(sp.Min.X+sp.Max.X)/2+2, float64(cy), *m.Volume > 0, ink)
		v.add(Target{R: sp.Inset(-6), Show: sp, Act: "volopen"})
		ui.VolumeAnchor = sp
	}

	if m == nil || !m.Running || m.Title == "" {
		drawRecord(img, cr)
		v.add(Target{R: cr, Act: "cover"})
		drawText(img, face("serifB", 30), x0, cy+110, "Nothing playing", ink)
		hint := "Open Spotify on your " + musicComputer(m)
		if m != nil && m.Running {
			hint = "Spotify is open, but not playing on this " + musicComputer(m)
		}
		for i, l := range wrap(face("ui", 17), hint, x1-x0, 3) {
			drawText(img, face("ui", 17), x0, cy+142+i*24, l, grey2)
		}
		return
	}

	// the cover (tap it to fill the whole screen with it)
	if art := bigArt(ui); hasArt {
		draw := art
		if ui.Dark { // the whole screen gets inverted for dark mode: pre-invert so the cover comes out right
			draw = image.NewGray(art.Rect)
			copy(draw.Pix, art.Pix)
			invertRect(draw, draw.Rect)
		}
		drawImage(img, cr, draw)
		strokeRoundRect(img, cr, 2, 1.5, grey3)
		v.add(Target{R: cr, Act: "coverfull"})
	} else {
		drawRecord(img, cr)
		v.add(Target{R: cr, Act: "cover"})
	}

	// the song
	state := "PAUSED"
	if m.Playing {
		state = "PLAYING"
	}
	y := cy + 80
	switch known := m.App == "Spotify" || m.App == "Music"; {
	case known && !ui.ShowMusicLogo: // Settings › Music › Music app logo: Off (the default)
	case known: // where the sound comes from: the app's mark, nothing more
		appMark(img, m.App, float64(x0+14), float64(y-5), 14, grey3)
		y += 12
	default:
		label(img, x0, y, strings.ToUpper(m.App)+" · "+state, grey2)
	}
	tf := face("serifB", 30)
	title := wrap(tf, m.Title, x1-x0, 4)
	y += 26 + capHeight(tf)
	for i, l := range title {
		drawText(img, tf, x0, y+i*38, l, ink)
	}
	y += 38*(len(title)-1) + 36
	af := face("ui", 19)
	for i, l := range wrap(af, m.Artist, x1-x0, 2) {
		drawText(img, af, x0, y+i*25, l, grey2)
		if i > 0 {
			y += 25
		}
	}
	if m.Album != "" && m.Album != m.Title {
		bf := face("ui", 16)
		for i, l := range wrap(bf, m.Album, x1-x0, 2) {
			drawText(img, bf, x0, y+30+i*22, l, grey3)
			y += 22
		}
		y += 8
	}
	bw := (x1 - x0 - 2*12) / 3 // square buttons, their bottom level with the cover's
	musicControls(v, m, x0, x1, cr.Max.Y-bw, bw)
	outputPicker(v, m, ui, x0, x1, cr.Max.Y-bw-12)
	if ui.VolumeOpen {
		volumePanel(v, m, ui.VolumeAnchor)
	}
}

// outputPicker: where the sound goes, as a quiet ghost button just above the controls
// ("🎧 My AirPods ⌄"); tapping it opens a dropdown of the other outputs upwards.
// Only shown when there's a choice.
func outputPicker(v *View, m *Music, ui *UI, x0, x1, bottom int) {
	img := v.Img
	var cur *Output
	for i := range m.Outputs {
		if m.Outputs[i].Current {
			cur = &m.Outputs[i]
		}
	}
	if cur == nil || len(m.Outputs) < 2 {
		return
	}
	pf := face("uiBold", 15)
	btn := image.Rect(x0-10, bottom-44, x1, bottom)
	cy := (btn.Min.Y + btn.Max.Y) / 2
	outputGlyph(img, cur.Kind, float64(x0+10), float64(cy), ink)
	end := drawText(img, pf, x0+28, cy+capHeight(pf)/2, ellipsize(pf, cur.Name, x1-x0-28-24), ink)
	dir := "down"
	if ui.Outputs {
		dir = "up"
	}
	chevron(img, float64(end+12), float64(cy), 5, dir, ink)
	v.add(Target{R: btn, Show: image.Rect(x0-10, btn.Min.Y, end+22, btn.Max.Y), Act: "outputs"})
	if !ui.Outputs {
		return
	}

	// the dropdown, opening upwards from the button
	const rowH = 52
	n := len(m.Outputs)
	box := image.Rect(x0, btn.Min.Y-6-n*rowH-12, x1, btn.Min.Y-6)
	fillRoundRect(img, box.Inset(-4), 14, paper)
	strokeRoundRect(img, box, 12, 2, ink)
	y := box.Min.Y + 6
	for i, o := range m.Outputs {
		r := image.Rect(box.Min.X+4, y, box.Max.X-4, y+rowH)
		if i > 0 {
			fillRect(img, image.Rect(box.Min.X+16, y, box.Max.X-16, y+1), grey4)
		}
		ry := y + rowH/2
		f, c := face("ui", 17), ink
		if o.Current {
			f = face("uiBold", 17)
		}
		if !o.Connected {
			c = grey2
		}
		lx := box.Min.X + 16
		outputGlyph(img, o.Kind, float64(lx+10), float64(ry), c)
		right := box.Max.X - 16
		switch {
		case o.Current:
			line(img, float64(right-18), float64(ry), float64(right-11), float64(ry+7), 2.6, ink) // tick
			line(img, float64(right-11), float64(ry+7), float64(right), float64(ry-7), 2.6, ink)
			right -= 28
		case !o.Connected:
			sf := face("ui", 13)
			right = drawRight(img, sf, right, ry+capHeight(sf)/2, "Connect", grey2) - 10
		}
		drawText(img, f, lx+30, ry+capHeight(f)/2, ellipsize(f, o.Name, right-lx-30), c)
		if !o.Current {
			act := "output:" + o.ID
			if !o.Connected {
				act = "connect:" + o.ID
			}
			v.add(Target{R: r, Show: r, Act: "music", Arg: act})
		}
		y += rowH
	}
}

// bigArt is the cover enlarged and dithered for the cover screen, made once per song.
func bigArt(ui *UI) *image.Gray {
	if ui.Art == nil {
		return nil
	}
	if ui.ArtBigKey != ui.ArtKey || ui.ArtBig == nil {
		n := px(coverSize, sc) // at the panel's resolution, so the dither pattern stays fine
		g := image.NewGray(image.Rect(0, 0, n, n))
		xdraw.CatmullRom.Scale(g, g.Rect, ui.Art, ui.Art.Rect, xdraw.Src, nil)
		bayer16(g)
		ui.ArtBig, ui.ArtBigKey = g, ui.ArtKey
	}
	return ui.ArtBig
}

// smallArt is the cover shrunk smoothly to n pixels for the Music widget, made once per
// song (drawImage's quick scaling would make a big cover look grainy).
func smallArt(ui *UI, n int) *image.Gray {
	if ui.Art.Rect.Dx() == n {
		return ui.Art
	}
	if ui.ArtSmallKey != ui.ArtKey || ui.ArtSmall == nil || ui.ArtSmall.Rect.Dx() != n {
		g := image.NewGray(image.Rect(0, 0, n, n))
		xdraw.CatmullRom.Scale(g, g.Rect, ui.Art, ui.Art.Rect, xdraw.Src, nil)
		ui.ArtSmall, ui.ArtSmallKey = g, ui.ArtKey
	}
	return ui.ArtSmall
}

// bayer8 is the 8×8 ordered-dither threshold map (values 0–63).
var bayer8 = [8][8]int{
	{0, 32, 8, 40, 2, 34, 10, 42},
	{48, 16, 56, 24, 50, 18, 58, 26},
	{12, 44, 4, 36, 14, 46, 6, 38},
	{60, 28, 52, 20, 62, 30, 54, 22},
	{3, 35, 11, 43, 1, 33, 9, 41},
	{51, 19, 59, 27, 49, 17, 57, 25},
	{15, 47, 7, 39, 13, 45, 5, 37},
	{63, 31, 55, 23, 61, 29, 53, 21},
}

// bayer16 reduces a picture to the panel's 16 greys (0, 17, 34 … 255) with ordered
// dithering, so in-between shades become a fine, even pattern instead of bands.
func bayer16(g *image.Gray) {
	for y := 0; y < g.Rect.Dy(); y++ {
		row := g.Pix[y*g.Stride : y*g.Stride+g.Rect.Dx()]
		for x, p := range row {
			// p/17 is the level; the threshold decides whether its remainder rounds up
			lvl := (int(p)*64 + bayer8[y&7][x&7]*17) / (17 * 64)
			row[x] = uint8(min(lvl, 15) * 17)
		}
	}
}

// drawRecord: a vinyl record, for when there's no cover.
func drawRecord(img *image.Gray, r image.Rectangle) {
	cx, cy, rad := float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2, float64(r.Dx())/2*0.9
	fillCircle(img, cx, cy, rad, ink)
	for _, k := range []float64{0.87, 0.72, 0.57} {
		ringCircle(img, cx, cy, rad*k, 1, grey2)
	}
	fillCircle(img, cx, cy, rad*0.3, paper)
	fillCircle(img, cx, cy, 4, ink)
}

// musicControls: previous, play/pause (solid) and next, side by side.
func musicControls(v *View, m *Music, x0, x1, by, h int) {
	img := v.Img
	bw := (x1 - x0 - 2*12) / 3
	for i, b := range []struct{ icon, act string }{{"prev", "previous"}, {"playpause", "playpause"}, {"next", "next"}} {
		r := image.Rect(x0+i*(bw+12), by, x0+i*(bw+12)+bw, by+h)
		solid := b.act == "playpause"
		c := ink
		if solid {
			fillRoundRect(img, r, 10, ink)
			c = paper
		} else {
			strokeRoundRect(img, r, 10, 2, ink)
		}
		transportIcon(img, b.icon, float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2, m.Playing, c)
		v.add(Target{R: r.Inset(-4), Show: r, Act: "music", Arg: b.act})
	}
}

// volumePanel: the computer's volume, dropping down from the header's speaker (or the cover
// screen's), like the brightness panel: − ━━━━━━── + and the level. Tapping elsewhere closes it.
func volumePanel(v *View, m *Music, anchor image.Rectangle) {
	img := v.Img
	if m == nil || m.Volume == nil {
		return
	}
	v.add(Target{R: img.Rect, Act: "volopen"}) // tapping elsewhere closes it
	const w = 400
	x := min(max(pad, anchor.Max.X-w), LW-pad-w)
	box := image.Rect(x, anchor.Max.Y+6, x+w, anchor.Max.Y+6+122)
	fillRoundRect(img, box.Inset(-3), 12, paper)
	strokeRoundRect(img, box, 10, 3, ink)
	x0, x1 := box.Min.X+20, box.Max.X-20
	label(img, x0, box.Min.Y+34, "VOLUME", grey2)
	where := map[string]string{"Music": "Apple Music"}[m.App]
	if where == "" {
		where = m.App
	}
	for _, o := range m.Outputs {
		if o.Current {
			where += " · " + o.Name
		}
	}
	lf := face("ui", 14)
	drawRight(img, lf, x1, box.Min.Y+34, ellipsize(lf, where, x1-x0-100), grey2)
	vol := *m.Volume
	y0, y1 := box.Min.Y+50, box.Min.Y+102
	minus := image.Rect(x0, y0, x0+60, y1)
	plus := image.Rect(x1-60, y0, x1, y1)
	for i, r := range []image.Rectangle{minus, plus} {
		strokeRoundRect(img, r, 10, 2, ink)
		cx, cy := float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2
		line(img, cx-10, cy, cx+10, cy, 3, ink)
		if i == 1 {
			line(img, cx, cy-10, cx, cy+10, 3, ink)
		}
	}
	v.add(Target{R: minus.Inset(-4), Show: minus, Act: "music", Arg: "volume:down"})
	v.add(Target{R: plus.Inset(-4), Show: plus, Act: "music", Arg: "volume:up"})
	pf := face("uiBold", 18)
	pct := itoa(vol) + "%"
	cy := (y0 + y1) / 2
	bar := image.Rect(minus.Max.X+18, cy-4, plus.Min.X-18-textW(pf, "100%")-12, cy+4)
	fillRoundRect(img, bar, 4, grey4)
	if fill := bar.Min.X + bar.Dx()*vol/100; fill > bar.Min.X+4 {
		fillRoundRect(img, image.Rect(bar.Min.X, bar.Min.Y, fill, bar.Max.Y), 4, ink)
	}
	drawRight(img, pf, plus.Min.X-18, cy+capHeight(pf)/2, pct, ink)
}

// transportIcon draws ⏮, ⏭ or play/pause (pause while playing) centred on (mx, my).
func transportIcon(img *image.Gray, icon string, mx, my float64, playing bool, c uint8) {
	switch icon {
	case "prev":
		fillPolygon(img, [][2]float64{{mx + 8, my - 10}, {mx - 4, my}, {mx + 8, my + 10}}, c)
		fillRect(img, image.Rect(int(mx)-9, int(my)-10, int(mx)-5, int(my)+10), c)
	case "next":
		fillPolygon(img, [][2]float64{{mx - 8, my - 10}, {mx + 4, my}, {mx - 8, my + 10}}, c)
		fillRect(img, image.Rect(int(mx)+5, int(my)-10, int(mx)+9, int(my)+10), c)
	default:
		if playing { // pause
			fillRect(img, image.Rect(int(mx)-9, int(my)-11, int(mx)-3, int(my)+11), c)
			fillRect(img, image.Rect(int(mx)+3, int(my)-11, int(mx)+9, int(my)+11), c)
		} else {
			fillPolygon(img, [][2]float64{{mx - 7, my - 12}, {mx + 11, my}, {mx - 7, my + 12}}, c)
		}
	}
}

// cornersGlyph: four corner marks, pointing out (go full screen) or in (leave it).
func cornersGlyph(img *image.Gray, cx, cy, s float64, in bool, c uint8) {
	for _, d := range [][2]float64{{-1, -1}, {1, -1}, {-1, 1}, {1, 1}} {
		x, y := cx+d[0]*s, cy+d[1]*s // the corner point
		k := -0.55 * s               // arms run back towards the middle
		if in {
			x, y = cx+d[0]*s*0.35, cy+d[1]*s*0.35
			k = -k
		}
		line(img, x, y, x+d[0]*k, y, 2.4, c)
		line(img, x, y, x, y+d[1]*k, 2.4, c)
	}
}

// outputGlyph: headphones, a laptop, a monitor or a speaker, about 20 px, centred.
func outputGlyph(img *image.Gray, kind string, cx, cy float64, c uint8) {
	switch kind {
	case "headphones":
		for i := 0; i < 10; i++ { // the band: a half circle over the top
			a1, a2 := math.Pi+float64(i)*math.Pi/10, math.Pi+float64(i+1)*math.Pi/10
			line(img, cx+8*math.Cos(a1), cy+1+8*math.Sin(a1), cx+8*math.Cos(a2), cy+1+8*math.Sin(a2), 2, c)
		}
		fillRoundRect(img, image.Rect(int(cx)-10, int(cy), int(cx)-5, int(cy)+9), 2, c)
		fillRoundRect(img, image.Rect(int(cx)+5, int(cy), int(cx)+10, int(cy)+9), 2, c)
	case "laptop":
		strokeRoundRect(img, image.Rect(int(cx)-8, int(cy)-8, int(cx)+8, int(cy)+4), 2, 2, c)
		fillRect(img, image.Rect(int(cx)-11, int(cy)+6, int(cx)+11, int(cy)+8), c)
	case "display":
		strokeRoundRect(img, image.Rect(int(cx)-10, int(cy)-9, int(cx)+10, int(cy)+5), 2, 2, c)
		fillRect(img, image.Rect(int(cx)-4, int(cy)+7, int(cx)+4, int(cy)+9), c)
	default: // speaker
		strokeRoundRect(img, image.Rect(int(cx)-7, int(cy)-10, int(cx)+7, int(cy)+10), 3, 2, c)
		ringCircle(img, cx, cy+3, 3.5, 1.8, c)
		fillCircle(img, cx, cy-5, 1.5, c)
	}
}

// spotifyMark: Spotify's logomark (from the official icon: a disc with three bars cut out),
// radius r, centred, in colour c.
func spotifyMark(img *image.Gray, cx, cy, r float64, c uint8) {
	spotifyOnce.Do(func() {
		if m, err := png.Decode(bytes.NewReader(spotifyPNG)); err == nil {
			spotifyImg = image.NewGray(m.Bounds())
			xdraw.Copy(spotifyImg, image.Point{}, m, m.Bounds(), xdraw.Src, nil)
		}
	})
	if spotifyImg == nil {
		return
	}
	s := scaleOf(img) // at the canvas's resolution
	n := int(2 * r * s)
	mask := image.NewGray(image.Rect(0, 0, n, n))
	xdraw.CatmullRom.Scale(mask, mask.Rect, spotifyImg, spotifyImg.Rect, xdraw.Src, nil)
	x0, y0 := int((cx-r)*s), int((cy-r)*s)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if a := mask.Pix[y*mask.Stride+x]; a > 0 {
				blendPx(img, x0+x, y0+y, c, float64(a)/255)
			}
		}
	}
}

//go:embed spotify-mark.png
var spotifyPNG []byte

var (
	spotifyOnce sync.Once
	spotifyImg  *image.Gray
)
