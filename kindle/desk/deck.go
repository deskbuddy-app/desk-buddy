package main

// The Deck tab: big tiles; each asks the helper on the profile's computer to open an app.

import (
	"image"
	"math"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// placeholderDeck stands in until the backend has a deck (the old Mac server doesn't):
// the same tiles the new backend starts with, so the layout can be tried out.
func placeholderDeck(profile string) *Deck {
	labels := []string{"Slack", "Figma", "Notion", "Linear", "Chrome", "Spotify", "Mail", "Calendar", "Claude", "Terminal", "Finder", "Notes"}
	dk := &Deck{MachineLabel: "MacBook"}
	if profile == "home" {
		labels = []string{"Spotify", "Discord", "Steam", "Chrome", "WhatsApp", "Files"}
		dk.MachineLabel = "PC"
	}
	for _, l := range labels {
		dk.Tiles = append(dk.Tiles, DeckTile{Label: l})
	}
	return dk
}

const iconSize = 84

// shownDeck is the computer's deck being looked at: the one picked with the computer
// switch (only there with two computers), else the one in use (the backend's choice).
func shownDeck(d *Desk, ui *UI) (dk *Deck, placeholder bool) {
	if d.Deck == nil {
		return placeholderDeck(d.Profile), true
	}
	want := ui.DeckProfile
	if want == "" {
		want = d.Deck.Profile // one list: the computer in use
		if !oneList(d) {
			want = d.Profile // a list per computer: that list's computer
		}
	}
	for i := range d.Deck.Decks {
		if d.Deck.Decks[i].Profile == want {
			return &d.Deck.Decks[i], false
		}
	}
	return d.Deck, false
}

func deckView(v *View, d *Desk, ui *UI) {
	img := v.Img
	x0, x1 := pad, LW-pad
	y := barH + 2 + 16
	dk, placeholder := shownDeck(d, ui)
	if !placeholder && !dk.Connected {
		notConnected(v, dk)
		return
	}

	// Left: which computer (a switch only with one list and two connected computers; with a
	// list per computer the Deck follows the list), then Favourites | All apps.
	var conn []Deck
	for _, o := range d.Deck.Decks {
		if o.Connected {
			conn = append(conn, o)
		}
	}
	tx := x0 - 12
	if !placeholder && oneList(d) && len(conn) > 1 {
		f := face("uiBold", 16)
		const bw = 112
		seg := image.Rect(x0, y+2, x0+bw*len(conn), y+46)
		for i, o := range conn {
			cell := image.Rect(seg.Min.X+i*bw, seg.Min.Y, seg.Min.X+(i+1)*bw, seg.Max.Y)
			c := ink
			if o.Profile == dk.Profile {
				fillRoundRect(img, seg, 8, ink, cell)
				c = paper
			}
			if i > 0 {
				fillRect(img, image.Rect(cell.Min.X-1, cell.Min.Y, cell.Min.X+1, cell.Max.Y), ink)
			}
			drawCentered(img, f, (cell.Min.X+cell.Max.X)/2, (cell.Min.Y+cell.Max.Y)/2+capHeight(f)/2, o.MachineLabel, c)
			v.add(Target{R: cell.Inset(-3), Show: cell, Act: "deckfor", Arg: o.Profile})
		}
		strokeRoundRect(img, seg, 8, 2, ink)
		tx = seg.Max.X
	}
	if !placeholder && len(dk.Apps) > 0 { // Favourites | All apps
		all := 0
		if ui.DeckAll {
			all = 1
		}
		// two icon buttons: a star (favourites) and a grid (all apps)
		for i, g := range []func(*image.Gray, float64, float64, uint8){starGlyph, gridGlyph} {
			r := image.Rect(tx+12+i*58, y+2, tx+12+i*58+52, y+46)
			c := buttonFace(img, r, 8, i == all) // in the theme's style
			g(img, float64(r.Min.X+r.Max.X)/2, float64(r.Min.Y+r.Max.Y)/2, c)
			v.add(Target{R: r.Inset(-3), Show: r, Act: "deckall", Arg: itoa(i)})
		}
		tx += 12 + 2*58 - 6
	} else if tx < x0 {
		tx = drawText(img, face("serifB", 26), x0, y+32, dk.MachineLabel, ink)
	}

	// Favourites: big tiles, 12 a page (6 in Compact mode). All apps: smaller tiles, 24 a page (12 in Compact mode).
	all := ui.DeckAll && !placeholder && len(dk.Apps) > 0
	cols, rows, gap := 4, 3, 16
	if compactMode {
		cols, rows = 3, 2
	}
	names := make([]string, len(dk.Tiles))
	for i, t := range dk.Tiles {
		names[i] = t.Label
	}
	if all {
		cols, rows, gap, names = 6, 4, 10, dk.Apps
		if compactMode {
			cols, rows, gap = 4, 3, 12
		}
	}
	per := cols * rows
	pages := max(1, (len(names)+per-1)/per)
	ui.DeckPage = max(0, min(ui.DeckPage, pages-1))
	bottom := LH - 24
	if !placeholder { // Edit / Done, bottom right
		bottom = LH - 74
		f := face("uiBold", 16)
		label := "Edit"
		if ui.DeckEdit {
			label = "Done"
		}
		r := image.Rect(x1-textW(f, label)-62, LH-62, x1, LH-18)
		c := ink
		if ui.DeckEdit {
			fillRoundRect(img, r, 8, ink)
			c = paper
		} else {
			strokeRoundRect(img, r, 8, 2, ink)
		}
		pencilGlyph(img, float64(r.Min.X+22), float64(r.Min.Y+r.Max.Y)/2, c)
		drawText(img, f, r.Min.X+38, (r.Min.Y+r.Max.Y)/2+capHeight(f)/2, label, c)
		v.add(Target{R: r.Inset(-6), Show: r, Act: "deckedit"})
	}
	if pages > 1 { // ‹ 2 / 5 › along the bottom (or swipe sideways)
		bottom = LH - 74
		cy := LH - 40
		pf := face("uiBold", 16)
		txt := itoa(ui.DeckPage+1) + " / " + itoa(pages)
		prev := image.Rect(LW/2-60-56, cy-22, LW/2-60, cy+22)
		next := image.Rect(LW/2+60, cy-22, LW/2+60+56, cy+22)
		iconButton(img, prev, "left", ui.DeckPage > 0)
		iconButton(img, next, "right", ui.DeckPage < pages-1)
		drawCentered(img, pf, LW/2, cy+capHeight(pf)/2, txt, grey2)
		if ui.DeckPage > 0 {
			v.add(Target{R: prev.Inset(-6), Show: prev, Act: "deckpage", Arg: "-1"})
		}
		if ui.DeckPage < pages-1 {
			v.add(Target{R: next.Inset(-6), Show: next, Act: "deckpage", Arg: "1"})
		}
	}

	// Right: how the last tap went, or whether the computer is listening.
	sf := face("uiBold", 13) // the badge is the size the LIVE badge was: 26 px, fully rounded
	status, solid := dk.MachineLabel+" isn't listening", false
	if dk.Online {
		status = dk.MachineLabel + " is listening"
	}
	if placeholder {
		status = "Placeholders"
		if ui.Note != "" {
			status, solid = ui.Note, true
		}
	} else if ui.Note != "" {
		status, solid = ui.Note, true
	} else if l := dk.Last; l != nil {
		solid = true
		switch l.State {
		case "sent":
			status = "Sending " + l.Label + "…"
		case "waiting":
			status = "Waiting for the " + dk.MachineLabel + "…"
			solid = false
		case "opening":
			status = "Opening " + l.Label + "…"
		case "done":
			status = "Opened " + l.Label
		case "failed":
			status = "Couldn't open " + l.Label
		}
	}
	dot := !solid && dk.Online && !placeholder // a dot in the badge while the computer is listening
	sw := textW(sf, status) + 22
	if dot {
		sw += 12
	}
	sr := image.Rect(x1-sw, y+11, x1, y+37)
	if !placeholder && ui.DeckEdit { // how editing works
		hint := "× removes · arrows move"
		if ui.DeckAll {
			hint = "Tap an app to add or remove it"
		}
		hf := face("ui", 13)
		if room := sr.Min.X - 24 - (tx + 16); room > 80 {
			drawText(img, hf, tx+16, y+24+capHeight(hf)/2, ellipsize(hf, hint, room), grey3)
		}
	}
	if solid {
		fillRoundRect(img, sr, 13, ink)
		drawCentered(img, sf, (sr.Min.X+sr.Max.X)/2, y+24+capHeight(sf)/2, status, paper)
	} else { // an outlined badge, fully rounded
		strokeRoundRect(img, sr, 13, 1.5, ink)
		tx := sr.Min.X + 11
		if dot {
			fillCircle(img, float64(tx+3), float64(y+24), 3.5, ink)
			tx += 12
		}
		drawText(img, sf, tx, y+24+capHeight(sf)/2, status, ink)
	}
	y += 64

	if len(names) == 0 {
		drawText(img, face("ui", 18), x0, y+40, "No apps on this deck yet.", grey2)
		return
	}
	tw := (x1 - x0 - (cols-1)*gap) / cols
	th := (bottom - y - (rows-1)*gap) / rows
	lf, mf := face("uiBold", 19), face("serifB", 34)
	isz, msz := iconSize, 72
	if all {
		lf, mf, isz, msz = face("uiBold", 14), face("serifB", 24), 60, 52
	}
	icons := ui.Icons[dk.Machine]
	for k := 0; k < per; k++ {
		i := ui.DeckPage*per + k
		if i >= len(names) {
			break
		}
		name := names[i]
		r := image.Rect(x0+(k%cols)*(tw+gap), y+(k/cols)*(th+gap), x0+(k%cols)*(tw+gap)+tw, y+(k/cols)*(th+gap)+th)
		buttonFace(img, r, 12, false) // a tile, in the theme's style
		if themeName == "retro" {
			fillRect(img, r.Inset(4), paper) // a white face inside the 3D edge (the icons are on white)
		}
		cx := (r.Min.X + r.Max.X) / 2
		labelH := 40
		if all {
			labelH = 30
		}
		var m image.Rectangle
		if icon := icons[name]; icon != nil && !placeholder {
			m = image.Rect(cx-isz/2, r.Min.Y+(th-isz-labelH)/2, cx+isz/2, r.Min.Y+(th-isz-labelH)/2+isz)
			drawImage(img, m, icon)
			box, rad := iconBox(isz) // one thin outline, so white icons keep their shape
			strokeRoundRect(img, box.Add(m.Min), rad, 1.5, grey3)
		} else {
			m = image.Rect(cx-msz/2, r.Min.Y+(th-msz-labelH)/2, cx+msz/2, r.Min.Y+(th-msz-labelH)/2+msz)
			fillRoundRect(img, m, float64(msz)/5, ink)
			drawCentered(img, mf, cx, (m.Min.Y+m.Max.Y)/2+capHeight(mf)/2, initials(name), paper)
		}
		drawCentered(img, lf, cx, m.Max.Y+labelH-6, ellipsize(lf, name, tw-12), ink)
		if ui.ConfirmRemove == i && !all { // this tile is the one being asked about
			strokeRoundRect(img, r.Inset(-3), 14, 4, ink)
		}
		if ui.DeckEdit && !placeholder {
			editControls(v, dk, r, i, name, all)
			continue
		}
		switch {
		case placeholder:
			v.add(Target{R: r, Show: r.Inset(3), Act: "note", Arg: name + " will open once the new backend is live"})
		case all:
			v.add(Target{R: r, Show: r.Inset(3), Act: "deckapp", Arg: dk.Profile + "|" + name})
		default:
			v.add(Target{R: r, Show: r.Inset(3), Act: "deck", ID: i, Arg: dk.Profile})
		}
	}
}

// confirmRemove: "Remove Slack from favourites?" with Remove / Cancel.
func confirmRemove(v *View, d *Desk, ui *UI) {
	dk, _ := shownDeck(d, ui)
	i := ui.ConfirmRemove
	if dk == nil || i < 0 || i >= len(dk.Tiles) {
		return
	}
	img := v.Img
	v.add(Target{R: img.Rect, Act: "favcancel"}) // tapping elsewhere cancels
	box := image.Rect(LW/2-280, LH/2-90, LW/2+280, LH/2+90)
	fillRoundRect(img, box.Inset(-4), 14, paper)
	strokeRoundRect(img, box, 12, 3, ink)
	f := face("serifB", 26)
	drawCentered(img, f, LW/2, box.Min.Y+62, ellipsize(f, "Remove "+dk.Tiles[i].Label+" from favourites?", box.Dx()-40), ink)
	cancel := pillAt(img, image.Rect(LW/2-190, box.Max.Y-74, LW/2-10, box.Max.Y-26), "Cancel", false)
	remove := pillAt(img, image.Rect(LW/2+10, box.Max.Y-74, LW/2+190, box.Max.Y-26), "Remove", true)
	v.add(Target{R: cancel.Inset(-4), Show: cancel, Act: "favcancel"})
	v.add(Target{R: remove.Inset(-4), Show: remove, Act: "favremove", ID: i, Arg: dk.Profile})
}

// editControls: on a favourite, ✕ (remove) and ‹ › (move); on All apps, a star to toggle.
func editControls(v *View, dk *Deck, r image.Rectangle, i int, name string, all bool) {
	img := v.Img
	if all {
		fav := -1
		for k, t := range dk.Tiles {
			if t.Label == name || t.App == name {
				fav = k
			}
		}
		b := image.Rect(r.Max.X-40, r.Min.Y+6, r.Max.X-6, r.Min.Y+40)
		if fav >= 0 {
			fillCircle(img, float64(b.Min.X+b.Max.X)/2, float64(b.Min.Y+b.Max.Y)/2, 16, ink)
			starGlyph(img, float64(b.Min.X+b.Max.X)/2, float64(b.Min.Y+b.Max.Y)/2, paper)
			v.add(Target{R: r, Show: r.Inset(3), Act: "favremove", ID: fav, Arg: dk.Profile})
		} else {
			ringCircle(img, float64(b.Min.X+b.Max.X)/2, float64(b.Min.Y+b.Max.Y)/2, 16, 2, grey3)
			starGlyph(img, float64(b.Min.X+b.Max.X)/2, float64(b.Min.Y+b.Max.Y)/2, grey3)
			v.add(Target{R: r, Show: r.Inset(3), Act: "favadd", Arg: dk.Profile + "|" + name})
		}
		return
	}
	// ✕ in the top right corner
	x := image.Rect(r.Max.X-46, r.Min.Y+6, r.Max.X-6, r.Min.Y+46)
	cx, cy := float64(x.Min.X+x.Max.X)/2, float64(x.Min.Y+x.Max.Y)/2
	fillCircle(img, cx, cy, 18, ink)
	line(img, cx-6, cy-6, cx+6, cy+6, 3, paper)
	line(img, cx+6, cy-6, cx-6, cy+6, 3, paper)
	v.add(Target{R: x.Inset(-6), Show: x, Act: "favremove", ID: i, Arg: dk.Profile})
	// ‹ › along the bottom of the tile
	for k, dir := range []string{"left", "right"} {
		to := i - 1 + 2*k
		if to < 0 || to >= len(dk.Tiles) {
			continue
		}
		b := image.Rect(r.Min.X+8, r.Max.Y-48, r.Min.X+48, r.Max.Y-8)
		if k == 1 {
			b = image.Rect(r.Max.X-48, r.Max.Y-48, r.Max.X-8, r.Max.Y-8)
		}
		iconButton(img, b, dir, true)
		v.add(Target{R: b.Inset(-4), Show: b, Act: "favmove", ID: i, Arg: dk.Profile + "|" + itoa(to)})
	}
}

func pencilGlyph(img *image.Gray, cx, cy float64, c uint8) {
	line(img, cx-6, cy+6, cx+5, cy-5, 4, c)
	fillTriangle(img, cx-9, cy+9, cx-7.5, cy+3, cx-3, cy+7.5, c)
}

func starGlyph(img *image.Gray, cx, cy float64, c uint8) {
	var pts [][2]float64
	for i := 0; i < 10; i++ {
		r := 11.0
		if i%2 == 1 {
			r = 4.6
		}
		a := -math.Pi/2 + float64(i)*math.Pi/5
		pts = append(pts, [2]float64{cx + r*math.Cos(a), cy + 1 + r*math.Sin(a)})
	}
	fillPolygon(img, pts, c)
}

func gridGlyph(img *image.Gray, cx, cy float64, c uint8) {
	for row := -1; row <= 1; row++ {
		for col := -1; col <= 1; col++ {
			x, y := int(cx)+col*7, int(cy)+row*7
			fillRoundRect(img, image.Rect(x-2, y-2, x+3, y+3), 1, c)
		}
	}
}

// initials: "Slack" → "S", "VS Code" → "VC".
func initials(s string) string {
	w := strings.Fields(s)
	if len(w) == 0 {
		return "?"
	}
	out := strings.ToUpper(string([]rune(w[0])[:1]))
	if len(w) > 1 {
		out += strings.ToUpper(string([]rune(w[1])[:1]))
	}
	return out
}

// notConnected: the Deck for a computer without the helper: what it would do, and a QR code
// to get the helper (helperURL).
func notConnected(v *View, dk *Deck) {
	img := v.Img
	name := dk.MachineLabel
	cx := LW / 2
	top := barH + 70
	drawCentered(img, face("serifB", 34), cx, top+30, name+" isn't connected yet", ink)
	bf := face("serif", 21)
	what := "Mac"
	if dk.Machine == "pc" {
		if dk.MachineLabel != "" && dk.MachineLabel != "PC" {
			what = dk.MachineLabel + " PC"
		} else {
			what = "Windows PC"
		}
	}
	for i, l := range []string{
		"Install Desk Buddy on your " + what + " to open its apps from here,",
		"show its calendar and music, and let Auto follow it.",
	} {
		drawCentered(img, bf, cx, top+80+i*30, l, grey2)
	}
	if q, err := qrcode.New(helperURL, qrcode.Medium); err == nil {
		q.DisableBorder = true
		bm := q.Bitmap()
		cell := 200 / len(bm)
		size := cell * len(bm)
		qx, qy := cx-size/2, top+150
		for r, row := range bm {
			for c, on := range row {
				if on {
					fillRect(img, image.Rect(qx+c*cell, qy+r*cell, qx+(c+1)*cell, qy+(r+1)*cell), ink)
				}
			}
		}
		drawCentered(img, face("ui", 15), cx, qy+size+30, "Scan with your phone to get it", grey2)
		drawCentered(img, face("uiBold", 15), cx, qy+size+54, strings.TrimPrefix(helperURL, "https://"), ink)
	}
}
