package main

// "Send feedback" (in the Work / Home menu): who made this, and where to send ideas,
// bugs and feature requests, as a QR code to scan with a phone.

import (
	"fmt"
	"image"
	"math"

	qrcode "github.com/skip2/go-qrcode"
)

// feedbackEmail is a placeholder: replace it with the real feedback address before the
// public release (noted in HANDOVER.md and PUBLISHING.md).
const feedbackEmail = "feedback@example.com"

// helperURL is a placeholder too: where to download Desk Buddy (shown as a QR code on
// the Deck of a computer that isn't connected). Replace before the public release.
const helperURL = "https://github.com/deskbuddy-app/desk-buddy/releases"

// aboutPane: Settings › About: who made this, where to send feedback (with a QR code to
// scan), then version, updates and battery.
func aboutPane(v *View, x0, x1, y int) {
	img := v.Img
	// Desk Buddy himself says hello (the mascot, brand/desk-buddy.svg)
	b := drawBuddy(img, x0+26, y-2, 64, ink)
	drawText(img, face("serifB", 28), b.Max.X+16, y+40, "你好，我是 Desk Buddy", ink)
	f := face("serif", 19)
	ty := y + 100
	for _, l := range []string{
		"我是一个业余项目，起因是",
		"在家里找到一台闲置了",
		"十多年的 Kindle，如今由我陪伴你。",
		"",
		"想法、问题、功能建议，欢迎",
		"告诉作者，扫码或写信至",
	} {
		drawText(img, f, x0, ty, l, ink)
		ty += 27
	}
	drawText(img, face("uiBold", 17), x0, ty+2, feedbackEmail, ink)
	if q, err := qrcode.New("mailto:"+feedbackEmail+"?subject=Desk%20Buddy%20feedback", qrcode.Medium); err == nil {
		q.DisableBorder = true
		bm := q.Bitmap()
		cell := 170 / len(bm)
		size := cell * len(bm)
		qx, qy := x1-size, y+14
		for r, row := range bm {
			for c, on := range row {
				if on {
					fillRect(img, image.Rect(qx+c*cell, qy+r*cell, qx+(c+1)*cell, qy+(r+1)*cell), ink)
				}
			}
		}
		drawCentered(img, face("ui", 13), qx+size/2, qy+size+22, "用手机扫码", grey2)
	}
	// the first-run tour, again
	ty += 40
	fillRect(img, image.Rect(x0, ty, x1, ty+1), grey4)
	drawText(img, face("serifB", 20), x0, ty+36, "使用导览", ink)
	drawText(img, face("ui", 13), x0, ty+58, "了解屏幕上的各项功能", grey2)
	tr := image.Rect(x1-180, ty+16, x1, ty+62)
	pillAt(img, tr, "查看导览", false)
	v.add(Target{R: tr.Inset(-4), Show: tr, Act: "tour", Arg: "start"})
}

// updatesPane: Settings › Updates: version, battery, and checking for (then installing) a new
// version. Updates only install when tapped.
func updatesPane(v *View, ui *UI, st Status, x0, x1, y int) {
	img := v.Img
	label(img, x0, y, "更新", grey2)
	y += 18
	row := func(name, hint, value string) {
		drawText(img, face("serifB", 20), x0, y+30, name, ink)
		drawText(img, face("ui", 13), x0, y+52, hint, grey2)
		drawRight(img, face("ui", 16), x1, y+38, value, grey2)
		fillRect(img, image.Rect(x0, y+68, x1, y+69), grey4)
		y += 74
	}
	row("版本", "Kindle 应用版本", buildVersion)
	if st.Battery >= 0 {
		state := ""
		if st.Charging {
			state = " · charging"
		}
		row("电量", "接通电源可持续运行", fmt.Sprintf("%d%%%s", st.Battery, state))
	}
	// Show on main screen, and how often to look (greyed out when it's off)
	every := ui.UpdEvery
	if every == 0 {
		every = 12
	}
	for _, r := range []struct {
		name, hint   string
		labels, args []string
		sel          int
		act          string
		off          bool
	}{
		{"在主屏幕显示", "有新版时在顶部显示更新按钮", []string{"开启", "关闭"}, []string{"1", "0"}, map[bool]int{false: 0, true: 1}[ui.NoUpdShow], "updshow", false},
		{"检查间隔", "通过 Wi-Fi 下载，点击后才安装", []string{"1 hour", "12 hours", "24 hours"}, []string{"1", "12", "24"}, map[int]int{1: 0, 12: 1, 24: 2}[every], "updevery", ui.NoUpdShow},
	} {
		w := map[int]int{2: 144, 3: 300}[len(r.labels)]
		drawText(img, face("serifB", 20), x0, y+30, r.name, ink)
		drawText(img, face("ui", 13), x0, y+52, r.hint, grey2)
		n := len(v.Targets)
		segmented(v, image.Rect(x1-w, y+10, x1, y+54), r.labels, r.args, r.sel, r.act)
		if r.off {
			fade(img, image.Rect(x0, y, x1, y+64))
			v.Targets = v.Targets[:n]
		}
		fillRect(img, image.Rect(x0, y+68, x1, y+69), grey4)
		y += 74
	}
	btn := image.Rect(x0, y+20, x0+230, y+66)
	if ui.UpdateReady { // Install, and What's new? (the update's changes, in a box)
		upd := pillAt(img, btn, "安装更新", true)
		v.add(Target{R: upd.Inset(-4), Show: upd, Act: "update"})
		wn := pillAt(img, image.Rect(btn.Max.X+12, btn.Min.Y, btn.Max.X+12+170, btn.Max.Y), "更新内容", false)
		v.add(Target{R: wn.Inset(-4), Show: wn, Act: "notesmodal"})
		drawText(img, face("uiBold", 15), wn.Max.X+20, (btn.Min.Y+btn.Max.Y)/2+5, "已可安装", ink)
	} else {
		upd := pillAt(img, btn, "检查更新", false)
		v.add(Target{R: upd.Inset(-4), Show: upd, Act: "checkupdate"})
		if ui.UpdateNote != "" {
			drawText(img, face("ui", 15), btn.Max.X+20, (btn.Min.Y+btn.Max.Y)/2+5, ui.UpdateNote, grey2)
		}
	}
	drawText(img, face("ui", 14), x0, btn.Max.Y+30, ellipsize(face("ui", 14), helperReminder, x1-x0), grey2)
	fillRect(img, image.Rect(x0, btn.Max.Y+46, x1, btn.Max.Y+47), grey4)
	patchNotesPane(v, ui, x0, x1, btn.Max.Y+84)
}

// refreshGlyph: the refresh arrow in a colour (Settings › Updates).
func refreshGlyph(img *image.Gray, cx, cy float64, c uint8) {
	if c == ink {
		refreshIcon(img, cx, cy, 10)
		return
	}
	// on the dark (selected) row: draw it on a scratch canvas and copy it over, lightened
	tmp := newScratch(30, 30)
	defer dropScratch(tmp)
	refreshIcon(tmp, 15, 15, 10)
	n, x0, y0 := tmp.Rect.Dx(), px(int(cx)-15, scaleOf(img)), px(int(cy)-15, scaleOf(img))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if p := tmp.Pix[y*tmp.Stride+x]; p < 255 {
				blendPx(img, x0+x, y0+y, c, float64(255-p)/255)
			}
		}
	}
}

// smiley: a round face, outlined, with two eyes and a smile.
func smiley(img *image.Gray, cx, cy, r float64) {
	ringCircle(img, cx, cy, r, 2.6, ink)
	fillCircle(img, cx-r*0.35, cy-r*0.25, r*0.11, ink)
	fillCircle(img, cx+r*0.35, cy-r*0.25, r*0.11, ink)
	px, py := 0.0, 0.0
	for i := 0; i <= 12; i++ {
		a := math.Pi*0.2 + math.Pi*0.6*float64(i)/12
		x, y := cx+math.Cos(a)*r*0.55, cy+math.Sin(a)*r*0.55
		if i > 0 {
			line(img, px, py, x, y, 2.6, ink)
		}
		px, py = x, y
	}
}

// focusGlyph: a timer (Settings › Focus).
func focusGlyph(img *image.Gray, cx, cy float64, c uint8) {
	ringCircle(img, cx, cy+1, 10, 2.2, c)
	line(img, cx, cy+1, cx, cy-5, 2.2, c)
	line(img, cx, cy+1, cx+4, cy+4, 2.2, c)
	fillRect(img, image.Rect(int(cx)-3, int(cy)-14, int(cx)+3, int(cy)-11), c)
}

// cloudGlyph: a filled cloud (Settings › Server).
func cloudGlyph(img *image.Gray, cx, cy float64, c uint8) {
	fillRoundRect(img, image.Rect(int(cx)-12, int(cy)-1, int(cx)+12, int(cy)+9), 5, c)
	fillCircle(img, cx-4, cy, 5.5, c)
	fillCircle(img, cx+3, cy-3, 7.5, c)
}

// feedbackGlyph: a speech bubble (the menu's "Send feedback").
func feedbackGlyph(img *image.Gray, cx, cy float64, c uint8) {
	strokeRoundRect(img, image.Rect(int(cx)-12, int(cy)-10, int(cx)+12, int(cy)+7), 5, 2.4, c)
	fillPolygon(img, [][2]float64{{cx - 6, cy + 6}, {cx - 7, cy + 13}, {cx + 1, cy + 6}}, c)
}
