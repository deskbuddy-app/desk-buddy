package main

// The Weather tab: a big picture of now, the next 12 hours, and the week. All the
// weather pictures are drawn here (no image files), so they're sharp at any size.

import (
	"fmt"
	"image"
	"math"
	"strings"
	"time"
)

type Weather struct {
	Error   string  `json:"error"`
	At      float64 `json:"at"`
	Place   string  `json:"place"`
	Current struct {
		Temp     int  `json:"temp"`
		Feels    int  `json:"feels"`
		Code     int  `json:"code"`
		Wind     int  `json:"wind"`
		Humidity int  `json:"humidity"`
		Day      bool `json:"day"`
	} `json:"current"`
	Hours []struct {
		T    int64 `json:"t"`
		Temp int   `json:"temp"`
		Rain int   `json:"rain"`
		Code int   `json:"code"`
		Day  bool  `json:"day"`
	} `json:"hours"`
	Days []struct {
		T       int64 `json:"t"`
		Code    int   `json:"code"`
		Max     int   `json:"max"`
		Min     int   `json:"min"`
		Rain    int   `json:"rain"`
		Sunrise int64 `json:"sunrise"`
		Sunset  int64 `json:"sunset"`
	} `json:"days"`
}

// WMO weather codes, as Open-Meteo sends them.
func weatherKind(code int) (kind, words string) {
	switch {
	case code == 0:
		return "clear", "晴"
	case code == 1:
		return "clear", "大部晴朗"
	case code == 2:
		return "partly", "局部多云"
	case code == 3:
		return "cloudy", "阴"
	case code == 45 || code == 48:
		return "fog", "雾"
	case code >= 51 && code <= 57:
		return "drizzle", "毛毛雨"
	case code >= 61 && code <= 67:
		if code == 65 {
			return "rain", "大雨"
		}
		return "rain", "雨"
	case code >= 71 && code <= 77, code == 85 || code == 86:
		return "snow", "雪"
	case code >= 80 && code <= 82:
		return "rain", "阵雨"
	case code >= 95:
		return "thunder", "雷暴"
	}
	return "cloudy", "多云"
}

// tempF: show temperatures in Fahrenheit (Settings › Temperature; Celsius by default).
var tempF bool

// deg is a temperature (the backend sends Celsius) as "18°" in the chosen unit.
func deg(c int) string {
	if tempF {
		return itoa(int(math.Round(float64(c)*9/5+32))) + "°"
	}
	return itoa(c) + "°"
}

// drawWeather draws the picture for a weather code in a box of size s centred on (cx, cy).
func drawWeather(img *image.Gray, code int, day bool, cx, cy, s float64) {
	kind, _ := weatherKind(code)
	t := math.Max(1.6, s*0.055) // line thickness
	sunOrMoon := func(x, y, r float64) {
		if day {
			ringCircle(img, x, y, r*0.5, t, ink)
			for i := 0; i < 8; i++ {
				a := float64(i) * math.Pi / 4
				line(img, x+math.Cos(a)*r*0.72, y+math.Sin(a)*r*0.72, x+math.Cos(a)*r*0.95, y+math.Sin(a)*r*0.95, t, ink)
			}
		} else { // crescent
			fillCircle(img, x, y, r*0.62, ink)
			fillCircle(img, x+r*0.32, y-r*0.22, r*0.55, paper)
		}
	}
	switch kind {
	case "clear":
		sunOrMoon(cx, cy, s*0.45)
		return
	case "partly":
		sunOrMoon(cx-s*0.17, cy-s*0.17, s*0.3)
		cloud(img, cx+s*0.08, cy+s*0.08, s*0.68, t)
		return
	case "fog":
		cloud(img, cx, cy-s*0.12, s*0.7, t)
		for i, w := range []float64{0.8, 0.6, 0.75} {
			y := cy + s*0.2 + float64(i)*s*0.11
			line(img, cx-s*w/2, y, cx+s*w/2, y, t, ink)
		}
		return
	}
	cloud(img, cx, cy-s*0.12, s*0.82, t)
	below := cy + s*0.2
	switch kind {
	case "rain", "drizzle":
		n, long := 4, s*0.2
		if kind == "drizzle" {
			n, long = 3, s*0.1
		}
		for i := 0; i < n; i++ {
			x := cx - s*0.24 + float64(i)*s*0.48/float64(n-1)
			line(img, x+s*0.05, below, x-s*0.03, below+long, t, ink)
		}
	case "snow":
		for i := 0; i < 3; i++ {
			x := cx - s*0.22 + float64(i)*s*0.22
			y := below + s*0.1 + float64(i%2)*s*0.08
			for a := 0; a < 3; a++ {
				ang := float64(a) * math.Pi / 3
				dx, dy := math.Cos(ang)*s*0.06, math.Sin(ang)*s*0.06
				line(img, x-dx, y-dy, x+dx, y+dy, t*0.8, ink)
			}
		}
	case "thunder":
		fillTriangle(img, cx+s*0.06, below-s*0.04, cx-s*0.14, below+s*0.16, cx+s*0.02, below+s*0.16, ink)
		fillTriangle(img, cx-s*0.04, below+s*0.32, cx+s*0.14, below+s*0.1, cx-s*0.02, below+s*0.1, ink)
	}
}

// cloud: a flat-bottomed pill with fully rounded ends and two soft bumps on top
// (a small one left, a big one right of centre), outlined, w wide.
func cloud(img *image.Gray, cx, cy, w, t float64) {
	type circ struct{ x, y, r float64 }
	rb := w * 0.15                 // radius of the pill's rounded ends
	by := cy + w*0.12              // the pill's centre line
	lx, rx := cx-w*0.30, cx+w*0.30 // centres of its ends
	cs := []circ{
		{lx, by, rb}, {rx, by, rb}, // rounded ends
		{cx - w*0.17, cy, w * 0.19},          // small bump
		{cx + w*0.08, cy - w*0.06, w * 0.27}, // big bump
	}
	inside := func(px, py, grow float64) bool {
		for _, c := range cs {
			if math.Hypot(px-c.x, py-c.y) <= c.r+grow {
				return true
			}
		}
		return px >= lx && px <= rx && py >= by-rb-grow && py <= by+rb+grow
	}
	left, right, baseBot := cx-w*0.45, cx+w*0.45, by+rb
	s := scaleOf(img) // drawn at the canvas's resolution
	x0, x1 := int((left-t-2)*s), int((right+t+2)*s)
	y0, y1 := int((cy-w*0.33-t-2)*s), int((baseBot+t+2)*s)
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			// 2×2 samples for smooth edges: outline ring, white inside.
			outer, inner := 0, 0
			for _, o := range [4][2]float64{{.25, .25}, {.75, .25}, {.25, .75}, {.75, .75}} {
				px, py := (float64(x)+o[0])/s, (float64(y)+o[1])/s
				if inside(px, py, 0) {
					inner++
				}
				if inside(px, py, t) {
					outer++
				}
			}
			if outer > 0 {
				blendPx(img, x, y, ink, float64(outer)/4)
			}
			if inner > 0 {
				blendPx(img, x, y, paper, float64(inner)/4)
			}
		}
	}
}

func weatherView(v *View, st Status, ui *UI) {
	img := v.Img
	x0, x1 := pad, sideX-2-pad
	pinR := x1            // the place button sits at the right of the place line
	if ui.WeatherHidden { // the week is collapsed: today takes the whole width
		x1 = LW - pad
		collapseButton(v, true, image.Rect(x1-44, barH+2+12, x1, barH+2+46), "weatherhide")
		pinR = x1 - 44 - 8
	} else {
		fillRect(img, image.Rect(sideX-2, barH+2, sideX, LH), ink)
	}
	pr := image.Rect(pinR-48, barH+2+7, pinR, barH+2+51)
	mapPinGlyph(img, float64(pr.Min.X+pr.Max.X)/2, float64(pr.Min.Y+pr.Max.Y)/2+2, ink)
	v.add(Target{R: pr.Inset(-4), Show: pr, Act: "placenew"})
	w := ui.Weather
	if w == nil || w.Error != "" || len(w.Hours) == 0 {
		msg := "正在加载天气…"
		if ui.WeatherErr != "" {
			msg = "天气：" + ui.WeatherErr
		} else if w != nil && w.Error != "" {
			msg = "天气：" + w.Error
		}
		drawText(img, face("serif", 22), x0, barH+80, msg, grey2)
		return
	}
	loc := st.Now.Location()
	at := time.Unix(int64(w.At), 0).In(loc)
	sel := max(0, min(ui.WeatherDay, len(w.Days)-1))
	df := face("ui", 17)
	sun := func(i int) string {
		if i >= len(w.Days) {
			return ""
		}
		d := w.Days[i]
		return "日出 " + time.Unix(d.Sunrise, 0).In(loc).Format("15:04") + " · 日落 " + time.Unix(d.Sunset, 0).In(loc).Format("15:04")
	}

	type slot struct {
		label      string
		temp, rain int
		code       int
		day        bool
	}
	var slots []slot
	cols := 12
	if ui.WeatherHidden {
		cols = 18 // the wider strip shows more hours
	}
	stripTitle := fmt.Sprintf("未来 %d 小时", cols)
	if sel == 0 {
		// Now: big picture, big temperature.
		label(img, x0, barH+2+30, strings.ToUpper(w.Place)+" · 更新于 "+at.Format("15:04"), grey2)
		c := w.Current
		drawWeather(img, c.Code, c.Day, 150, 230, 210)
		drawText(img, face("display", 130), 280, 262, deg(c.Temp), ink)
		_, words := weatherKind(c.Code)
		drawText(img, face("serifB", 30), 284, 310, words, ink)
		drawText(img, df, 286, 344, fmt.Sprintf("体感 %s · 风速 %d mph · 湿度 %d%%", deg(c.Feels), c.Wind, c.Humidity), grey2)
		drawText(img, df, 286, 370, sun(0), grey2)
		for i, hr := range w.Hours {
			if i == cols {
				break
			}
			l := time.Unix(hr.T, 0).In(loc).Format("15")
			if i == 0 {
				l = "现在"
			}
			slots = append(slots, slot{l, hr.Temp, hr.Rain, hr.Code, hr.Day})
		}
	} else {
		// A chosen day: its picture, high / low, and the day every two hours.
		d := w.Days[sel]
		date := time.Unix(d.T, 0).In(loc)
		label(img, x0, barH+2+30, strings.ToUpper(w.Place+" · "+displayDate(date, "Monday 2 January")), grey2)
		drawWeather(img, d.Code, true, 150, 230, 210)
		x := drawText(img, face("display", 130), 280, 262, deg(d.Max), ink)
		drawText(img, face("serif", 40), x+10, 262, "/ "+deg(d.Min), grey2)
		_, words := weatherKind(d.Code)
		drawText(img, face("serifB", 30), 284, 310, words, ink)
		drawText(img, df, 286, 344, fmt.Sprintf("降雨概率 %d%% · 最高 %s，最低 %s", d.Rain, deg(d.Max), deg(d.Min)), grey2)
		drawText(img, df, 286, 370, sun(sel), grey2)
		stripTitle = "全天天气"
		key := date.Format("2006-01-02")
		for _, hr := range w.Hours {
			t := time.Unix(hr.T, 0).In(loc)
			if t.Format("2006-01-02") == key && t.Hour()%2 == 0 {
				slots = append(slots, slot{t.Format("15"), hr.Temp, hr.Rain, hr.Code, hr.Day})
			}
		}
	}

	// Hour strip: time, picture, temperature, chance of rain.
	y := 408
	fillRect(img, image.Rect(x0, y, x1, y+2), grey4)
	label(img, x0, y+30, stripTitle, grey2)
	n := 12
	if len(slots) > 12 {
		n = len(slots)
	}
	colW := (x1 - x0) / n
	hf, tf2, rf := face("uiBold", 14), face("serifB", 19), face("ui", 13)
	for i, sl := range slots {
		cx := x0 + i*colW + colW/2
		drawCentered(img, hf, cx, y+66, sl.label, grey2)
		drawWeather(img, sl.code, sl.day, float64(cx), float64(y+104), 40)
		drawCentered(img, tf2, cx, y+156, deg(sl.temp), ink)
		bh := sl.rain * 70 / 100 // rain chance as a small bar, bottom-aligned
		if bh > 0 {
			fillRoundRect(img, image.Rect(cx-7, y+250-bh, cx+7, y+250), 3, grey3)
		}
		fillRect(img, image.Rect(cx-12, y+250, cx+12, y+252), grey4)
		if sl.rain >= 10 {
			drawCentered(img, rf, cx, y+272, fmt.Sprintf("%d%%", sl.rain), grey2)
		}
	}
	hint := "柱形表示降雨概率 · 点击右侧日期查看"
	if ui.WeatherHidden {
		hint = "柱形表示降雨概率 · « 查看本周"
	}
	drawText(img, face("ui", 13), x0, y+300, hint, grey3)
	if ui.WeatherHidden {
		return
	}

	// The week, with each day's range on a shared scale. Tap a day to see it.
	sx0, sx1 := sideX+22, LW-22
	label(img, sx0, barH+2+30, "本周", grey2)
	collapseButton(v, false, image.Rect(sx1-44, barH+2+12, sx1, barH+2+46), "weatherhide") // » hides the week
	lo, hi := 99, -99
	for _, d := range w.Days {
		lo, hi = min(lo, d.Min), max(hi, d.Max)
	}
	rowH := (LH - (barH + 50) - 16) / max(1, len(w.Days))
	df2, nf := face("serifB", 19), face("uiBold", 15)
	for i, d := range w.Days {
		ry := barH + 50 + i*rowH
		cy := ry + rowH/2
		row := image.Rect(sideX, ry, LW, ry+rowH) // edge to edge: divider to the screen's right edge
		if i == sel {
			fillRect(img, row, ink) // the chosen day, inverted
		} else if i > 0 && i != sel+1 {
			fillRect(img, image.Rect(sx0, ry, sx1, ry+1), grey4)
		}
		fg, soft := ink, grey2
		if i == sel {
			fg, soft = paper, grey4
		}
		name := displayDate(time.Unix(d.T, 0).In(loc), "Mon")
		if i == 0 {
			name = "今天"
		}
		drawText(img, df2, sx0, cy+2, name, fg)
		if d.Rain >= 20 {
			drawText(img, face("ui", 13), sx0, cy+20, fmt.Sprintf("降雨 %d%%", d.Rain), soft)
		}
		if i == sel {
			// draw the picture on a white chip so it reads on the black row
			fillRoundRect(img, image.Rect(sx0+68, cy-24, sx0+116, cy+24), 10, paper)
		}
		drawWeather(img, d.Code, true, float64(sx0+92), float64(cy), 40)
		drawRight(img, nf, sx0+156, cy+capHeight(nf)/2, deg(d.Min), soft)
		bx0, bx1 := sx0+164, sx1-42
		trackC, fillC := grey4, ink
		if i == sel {
			trackC, fillC = grey2, paper
		}
		fillRoundRect(img, image.Rect(bx0, cy-4, bx1, cy+4), 4, trackC)
		if hi > lo {
			a := bx0 + (bx1-bx0)*(d.Min-lo)/(hi-lo)
			b := bx0 + (bx1-bx0)*(d.Max-lo)/(hi-lo)
			fillRoundRect(img, image.Rect(a, cy-4, max(b, a+8), cy+4), 4, fillC)
		}
		drawRight(img, nf, sx1, cy+capHeight(nf)/2, deg(d.Max), fg)
		v.add(Target{R: image.Rect(sideX, ry, LW, ry+rowH), Show: row, Act: "wday", ID: i})
	}
}

func weatherGlyph(img *image.Gray, cx, cy float64, c uint8) {
	// A small sun peeking out behind a cloud (the same shape as the big ones), in the tab's colour.
	sx, sy := cx-5, cy-5
	ringCircle(img, sx, sy, 4.5, 1.8, c)
	for i := 0; i < 8; i++ {
		a := float64(i) * math.Pi / 4
		if a > 0.2 && a < 2.0 { // hidden behind the cloud
			continue
		}
		line(img, sx+math.Cos(a)*7, sy+math.Sin(a)*7, sx+math.Cos(a)*9.5, sy+math.Sin(a)*9.5, 1.8, c)
	}
	// the cloud, filled: a pill with round ends and two bumps
	fillRoundRect(img, image.Rect(int(cx)-8, int(cy)+2, int(cx)+13, int(cy)+11), 4.5, c)
	fillCircle(img, cx-2, cy+3.5, 4.5, c)
	fillCircle(img, cx+5, cy+1, 6.5, c)
}
