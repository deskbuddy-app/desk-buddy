package main

// The meeting heads-up (a card a few minutes before a meeting, with Join when the event has
// a Zoom / Meet / … link) and the Auto theme (dark when the Mac is, else sunset to sunrise).

import (
	"fmt"
	"image"
	"time"
)

// headsUpEvent is the meeting to warn about now: a timed event starting within the chosen
// minutes, until 5 minutes after it started, not dismissed. nil when there's none.
func headsUpEvent(d *Desk, ui *UI, now time.Time) *Event {
	if ui.NoHeadsUp || d == nil {
		return nil
	}
	mins := ui.HeadsUpMin
	if mins == 0 {
		mins = 5
	}
	t := now.Unix()
	for i, e := range d.Agenda {
		if e.AllDay || ui.Dismissed[eventKey(e)] {
			continue
		}
		if e.Start-t <= int64(mins)*60 && t < e.Start+300 {
			return &d.Agenda[i]
		}
	}
	return nil
}

// headsUp draws the card near the top of whatever screen is showing.
func headsUp(v *View, e *Event, ui *UI, st Status) {
	img := v.Img
	ui.HeadsUpKey = eventKey(*e)
	box := image.Rect(LW/2-430, barH+20, LW/2+430, barH+20+150)
	if v.Kind == "standby" { // under the clock, not over it
		box = box.Add(image.Pt(0, LH-60-150-box.Min.Y))
	}
	// a grey veil over the screen behind (as in the tour), so the card reads as a
	// notification; a tap anywhere off the card dismisses it
	for i, p := range img.Pix {
		img.Pix[i] = uint8((int(p)*35 + 172*65) / 100)
	}
	v.Targets = nil
	v.add(Target{R: img.Rect, Show: box, Act: "dismiss"})
	fillRoundRect(img, box.Inset(-4), 16, paper)
	strokeRoundRect(img, box, 14, 3, ink)
	x0, x1 := box.Min.X+28, box.Max.X-28
	left := (e.Start - st.Now.Unix() + 59) / 60
	when := fmt.Sprintf("%d 分钟后", left)
	switch {
	case left <= 0 && st.Now.Unix() < e.Start+60:
		when = "即将开始"
	case left <= 0:
		when = fmt.Sprintf("已开始 %d 分钟", (st.Now.Unix()-e.Start)/60)
	case left == 1:
		when = "1 分钟后"
	}
	label(img, x0, box.Min.Y+32, when, grey2)
	// buttons on the right: Join (solid, when there's a link) and Dismiss
	bx := x1
	dis := image.Rect(bx-120, box.Min.Y+52, bx, box.Min.Y+98)
	pillAt(img, dis, "关闭提醒", false)
	v.add(Target{R: dis.Inset(-6), Show: dis, Act: "dismiss"})
	bx = dis.Min.X - 12
	if e.Join != "" {
		jr := image.Rect(bx-120, box.Min.Y+52, bx, box.Min.Y+98)
		pillAt(img, jr, "加入会议", true)
		v.add(Target{R: jr.Inset(-6), Show: jr, Act: "join", Arg: e.Join})
		bx = jr.Min.X - 12
	}
	tf := face("serifB", 28)
	drawText(img, tf, x0, box.Min.Y+84, ellipsize(tf, e.Title, bx-16-x0), ink)
	sub := hm(e.Start, st) + "–" + hm(e.End, st)
	if e.Join != "" {
		sub += " · 加入会议将在电脑打开链接"
	} else if e.Location != "" {
		sub += " · " + e.Location
	}
	sf := face("ui", 16)
	drawText(img, sf, x0, box.Min.Y+122, ellipsize(sf, sub, x1-x0), grey2)
}

// autoDark: should the Auto theme be dark now? The computer's own appearance while it's on,
// else between sunset and sunrise, else 20:00–07:00.
func autoDark(d *Desk, now time.Time) bool {
	if d != nil && d.Appearance != "" {
		return d.Appearance == "dark"
	}
	if d != nil && d.Sun != nil && d.Sun.Rise > 0 {
		t := now.Unix()
		rise, set := d.Sun.Rise, d.Sun.Set
		for rise+86400 <= t { // yesterday's times: move them on a day (close enough)
			rise, set = rise+86400, set+86400
		}
		return t < rise || t >= set
	}
	return now.Hour() >= 20 || now.Hour() < 7
}
