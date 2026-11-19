package main

// The Calendar tab: a month grid on the left; the chosen day's events (or one event's
// details) on the right. Read-only.

import (
	"image"
	"sort"
	"strings"
	"time"
)

// gridStart is the Monday on or before the 1st of the month.
func gridStart(month time.Time) time.Time {
	wd := (int(month.Weekday()) + 6) % 7 // Monday = 0
	return month.AddDate(0, 0, -wd)
}

// eventDays lists the days (YYYY-MM-DD, local) an event covers.
func eventDays(e Event, loc *time.Location) []string {
	var s, end time.Time
	if e.AllDay { // UTC midnights, end exclusive
		s, end = time.Unix(e.Start, 0).UTC(), time.Unix(e.End, 0).UTC().Add(-time.Second)
	} else {
		s, end = time.Unix(e.Start, 0).In(loc), time.Unix(e.End, 0).In(loc).Add(-time.Second)
		if end.Before(s) {
			end = s
		}
	}
	var out []string
	for d := time.Date(s.Year(), s.Month(), s.Day(), 0, 0, 0, 0, time.UTC); len(out) < 62; d = d.AddDate(0, 0, 1) {
		k := d.Format("2006-01-02")
		out = append(out, k)
		if k >= end.Format("2006-01-02") {
			break
		}
	}
	return out
}

// dayEvents returns a day's events: all-day first, then by start time.
func dayEvents(evs []Event, day time.Time, loc *time.Location) []Event {
	k := day.Format("2006-01-02")
	var out []Event
	for _, e := range evs {
		for _, d := range eventDays(e, loc) {
			if d == k {
				out = append(out, e)
				break
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].AllDay != out[j].AllDay {
			return out[i].AllDay
		}
		return out[i].Start < out[j].Start
	})
	return out
}

func calendarView(v *View, d *Desk, st Status, ui *UI) {
	img := v.Img
	loc := st.Now.Location()
	x0, x1 := pad, sideX-2-pad
	if ui.CalHidden { // the day panel is collapsed: the month takes the whole width
		x1 = LW - pad
	} else {
		fillRect(img, image.Rect(sideX-2, barH+2, sideX, LH), ink)
	}
	y := barH + 2 + 16

	// Month title and navigation
	drawText(img, face("serifB", 28), x0, y+32, displayDate(ui.Month, "January 2006"), ink)
	navR := x1
	if ui.CalHidden { // « brings the day panel back
		collapseButton(v, true, image.Rect(x1-44, y+9, x1, y+43), "calhide")
		navR = x1 - 44 - 16
	}
	today := pillAt(img, image.Rect(navR-textW(face("uiBold", 16), "今天")-32, y+4, navR, y+48), "今天", false)
	next := image.Rect(today.Min.X-10-48, y+4, today.Min.X-10, y+48)
	prev := image.Rect(next.Min.X-8-48, y+4, next.Min.X-8, y+48)
	iconButton(img, prev, "left", true)
	iconButton(img, next, "right", true)
	v.add(Target{R: prev.Inset(-4), Show: prev, Act: "month", Arg: "-1"})
	v.add(Target{R: next.Inset(-4), Show: next, Act: "month", Arg: "1"})
	v.add(Target{R: today.Inset(-4), Show: today, Act: "month", Arg: "0"})
	if ui.CalErr != "" {
		drawText(img, face("uiBold", 13), x0, y+56, ellipsize(face("uiBold", 13), "日历："+ui.CalErr, prev.Min.X-x0-10), ink)
	} else if ui.CalKey == "" {
		drawText(img, face("ui", 13), x0, y+56, "加载中…", grey2)
	}
	y += 66

	// Weekday names
	cw := (x1 - x0) / 7
	wf := face("uiBold", 12)
	for i, n := range []string{"周一", "周二", "周三", "周四", "周五", "周六", "周日"} {
		drawSpaced(img, wf, x0+i*cw+8, y+14, n, 1, grey2)
	}
	y += 24
	ch := (LH - 22 - y) / 6

	start := gridStart(ui.Month)
	todayKey, selKey := st.Now.Format("2006-01-02"), ui.Day.Format("2006-01-02")
	byDay := map[string][]Event{}
	for _, e := range ui.Cal {
		for _, k := range eventDays(e, loc) {
			byDay[k] = append(byDay[k], e)
		}
	}
	nf, ef := face("uiBold", 16), face("ui", 12)
	for i := 0; i < 42; i++ {
		day := start.AddDate(0, 0, i)
		k := day.Format("2006-01-02")
		cell := image.Rect(x0+(i%7)*cw, y+(i/7)*ch, x0+(i%7+1)*cw, y+(i/7+1)*ch)
		fillRect(img, image.Rect(cell.Min.X, cell.Min.Y, cell.Max.X, cell.Min.Y+1), grey4)
		if i%7 > 0 {
			fillRect(img, image.Rect(cell.Min.X, cell.Min.Y, cell.Min.X+1, cell.Max.Y), grey4)
		}
		c := ink
		if day.Month() != ui.Month.Month() {
			c = grey3
		}
		num := itoa(day.Day())
		if k == todayKey {
			fillCircle(img, float64(cell.Min.X+20), float64(cell.Min.Y+20), 15, ink)
			drawCentered(img, nf, cell.Min.X+20, cell.Min.Y+20+capHeight(nf)/2, num, paper)
		} else {
			drawCentered(img, nf, cell.Min.X+20, cell.Min.Y+20+capHeight(nf)/2, num, c)
		}
		evs := dayEvents(byDay[k], day, loc)
		const lineH = 15
		ey := cell.Min.Y + 50
		fit := max(1, (cell.Max.Y-4-(ey-11))/lineH) // lines that fit under the date
		for j, e := range evs {
			if j == fit-1 && len(evs) > fit { // the last line says how many more
				drawText(img, ef, cell.Min.X+6, ey, "+"+itoa(len(evs)-j)+" more", grey2)
				break
			}
			fillRect(img, image.Rect(cell.Min.X+6, ey-9, cell.Min.X+9, ey+1), c)
			drawText(img, ef, cell.Min.X+12, ey, ellipsize(ef, e.Title, cw-16), c)
			ey += lineH
		}
		if k == selKey {
			strokeRoundRect(img, cell.Inset(2), 6, 3, ink)
		}
		v.add(Target{R: cell, Show: cell.Inset(3), Act: "day", Arg: k})
	}

	if !ui.CalHidden {
		dayPanel(v, st, ui)
	}
}

func dayPanel(v *View, st Status, ui *UI) {
	img := v.Img
	x0, x1 := sideX+22, LW-22
	w := x1 - x0
	// Rows of 44 px buttons, level with the month's ‹ › Today on the left, 12 px apart.
	const bh, gap = 44, 12
	y := barH + 2 + 16 + 4
	loc := st.Now.Location()
	evs := dayEvents(ui.Cal, ui.Day, loc)
	collapseButton(v, false, image.Rect(x1-48, y, x1, y+bh), "calhide") // » hides the panel: a button without the outline
	if ui.Event < 0 && ui.EventKey == "" {
		addGlyphButton(v, image.Rect(x1-48-8-48, y, x1-48-8, y+bh), "eventnew") // + a new event on this day
	}

	when := func(e Event) string {
		if e.AllDay {
			return "全天"
		}
		return hm(e.Start, st) + "–" + hm(e.End, st)
	}

	if ui.EventKey != "" { // opened from the Day widget: find that event once the month has loaded
		for i, e := range evs {
			if eventKey(e) == ui.EventKey {
				ui.Event, ui.EventKey = i, ""
			}
		}
	}
	if ui.Event >= 0 && ui.Event < len(evs) {
		e := evs[ui.Event]
		back := pillAt(img, image.Rect(x0, y, x0+textW(face("uiBold", 16), "返回")+52, y+bh), "", false)
		chevron(img, float64(back.Min.X+18), float64(back.Min.Y+bh/2), 6, "left", ink)
		drawText(img, face("uiBold", 16), back.Min.X+30, back.Min.Y+bh/2+capHeight(face("uiBold", 16))/2, "返回", ink)
		v.add(Target{R: back.Inset(-6), Show: back, Act: "event", ID: -1})
		y += bh + gap + 8
		label(img, x0, y+12, strings.ToUpper(displayDate(ui.Day, "Monday 2 January")), grey2)
		y += 26
		tf := face("serifB", 24)
		for _, l := range wrap(tf, e.Title, w, 3) {
			drawText(img, tf, x0, y+24, l, ink)
			y += 30
		}
		y += 8
		f := face("ui", 16)
		drawText(img, face("uiBold", 16), x0, y+16, when(e), ink)
		y += 26
		if e.Location != "" {
			for _, l := range wrap(f, e.Location, w, 2) {
				drawText(img, f, x0, y+16, l, grey2)
				y += 22
			}
		}
		if e.Cal != "" {
			drawText(img, face("ui", 14), x0, y+16, e.Cal, grey3)
			y += 22
		}
		if e.ID > 0 { // added on the Kindle: it can be deleted here
			y += 16
			if ui.ConfirmEvent == e.ID {
				drawText(img, face("uiBold", 16), x0, y+16, "删除此日程？", ink)
				y += 30
				del := pillAt(img, image.Rect(x0, y, x0+110, y+bh), "删除", true)
				keep := pillAt(img, image.Rect(del.Max.X+10, y, del.Max.X+10+100, y+bh), "保留", false)
				v.add(Target{R: del.Inset(-4), Show: del, Act: "delevent", ID: e.ID})
				v.add(Target{R: keep.Inset(-4), Show: keep, Act: "delask", ID: 0})
			} else {
				del := pillAt(img, image.Rect(x0, y, x0+110, y+bh), "删除", false)
				v.add(Target{R: del.Inset(-4), Show: del, Act: "delask", ID: e.ID})
			}
			y += bh
		}
		if e.Description != "" {
			y += 10
			fillRect(img, image.Rect(x0, y, x1, y+2), grey4)
			y += 12
			df := face("ui", 15)
			var lines []string
			for _, para := range strings.Split(e.Description, "\n") {
				if strings.TrimSpace(para) == "" {
					continue
				}
				lines = append(lines, wrap(df, para, w, 6)...)
			}
			for _, l := range lines {
				if y+20 > LH-20 {
					break
				}
				drawText(img, df, x0, y+16, l, ink)
				y += 21
			}
		}
		return
	}

	lf := face("uiBold", 13)
	label(img, x0, y+bh/2+capHeight(lf)/2, strings.ToUpper(displayDate(ui.Day, "Monday 2 January")), grey2) // centred on »
	y += bh + gap
	// List | Timeline (remembered), and earlier / later for the timeline
	mode := 0
	if ui.CalTimeline {
		mode = 1
	}
	segmented(v, image.Rect(x0, y, x0+190, y+bh), []string{"列表", "时间轴"}, []string{"0", "1"}, mode, "calmode")
	if ui.CalTimeline {
		dn := image.Rect(x1-48, y, x1, y+bh)
		up := image.Rect(dn.Min.X-8-48, y, dn.Min.X-8, y+bh)
		iconButton(img, up, "up", true)
		iconButton(img, dn, "down", true)
		v.add(Target{R: up.Inset(-4), Show: up, Act: "calhour", Arg: "-3"})
		v.add(Target{R: dn.Inset(-4), Show: dn, Act: "calhour", Arg: "3"})
	}
	y += bh + gap + 4
	if ui.CalTimeline {
		dayTimeline(v, st, ui, evs, x0, x1, y)
		return
	}
	if len(evs) == 0 {
		drawText(img, face("serif", 21), x0, y+28, "当天没有安排", grey2)
		return
	}
	tf, sf := face("serif", 20), face("uiBold", 15)
	for i, e := range evs {
		lines := wrap(tf, e.Title, w-4, 2)
		h := 30 + 25*len(lines)
		if e.Location != "" {
			h += 20
		}
		if y+h > LH-16 {
			drawText(img, face("ui", 15), x0, y+20, itoa(len(evs)-i)+" more", grey2)
			break
		}
		r := image.Rect(x0-8, y, x1+8, y+h)
		drawText(img, sf, x0, y+20, when(e), grey2)
		ly := y + 20
		for _, l := range lines {
			ly += 25
			drawText(img, tf, x0, ly, l, ink)
		}
		if e.Location != "" {
			drawText(img, face("ui", 14), x0, ly+20, ellipsize(face("ui", 14), e.Location, w), grey2)
		}
		fillRect(img, image.Rect(x0, y+h-1, x1, y+h), grey4)
		v.add(Target{R: r, Act: "event", ID: i})
		y += h
	}
}

// eventKey identifies an event across the Day widget and the Calendar tab.
func eventKey(e Event) string { return itoa(int(e.Start%1e9)) + "|" + e.Title }

// dayTimeline: the day's events on an hour timeline (all-day ones above it). It starts
// an hour before now (today) or before the first event, and scrolls with ↑ ↓ or a swipe.
func dayTimeline(v *View, st Status, ui *UI, evs []Event, x0, x1, y int) {
	img := v.Img
	for i, e := range evs {
		if !e.AllDay || y > barH+260 {
			continue
		}
		r := image.Rect(x0, y, x1, y+28)
		fillRoundRect(img, r, 6, grey4)
		f := face("uiBold", 14)
		drawText(img, f, x0+10, y+14+capHeight(f)/2, ellipsize(f, e.Title, x1-x0-20), ink)
		v.add(Target{R: r, Act: "event", ID: i})
		y += 34
	}
	const perHour = 56
	top, bottom := y+10, LH-20
	hours := (bottom - top) / perHour
	h := ui.CalHour
	if h < 0 { // not scrolled yet: start somewhere useful
		h = 8
		if ui.Day.Format("2006-01-02") == st.Now.Format("2006-01-02") {
			h = st.Now.Hour() - 1
		} else {
			for _, e := range evs {
				if !e.AllDay {
					h = time.Unix(e.Start, 0).In(st.Now.Location()).Hour() - 1
					break
				}
			}
		}
	}
	h = max(0, min(h, 24-hours))
	ui.CalHourShown = h
	start := time.Date(ui.Day.Year(), ui.Day.Month(), ui.Day.Day(), h, 0, 0, 0, st.Now.Location())
	n := timeline(v, evs, st, x0, x1, top, bottom, start, perHour, func(_ Event, i int) Target {
		return Target{Act: "event", ID: i}
	})
	if n == 0 {
		drawText(img, face("ui", 15), x0+56, top+perHour/2+6, "这些时段没有安排", grey2)
	}
}
