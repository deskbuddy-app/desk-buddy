package main

// Pomodoro: the timer lives on the backend (so it survives restarts and the Mac can
// chime); here we show it: on the "Now working on" card, as a small header timer, in
// standby, and a full "Time's up" screen when a focus or break ends.

import (
	"fmt"
	"image"
	"math"
	"strings"
)

type Pomo struct {
	State  string `json:"state"` // idle, focus, break, paused
	Task   int    `json:"task"`
	Title  string `json:"title"`
	Length int64  `json:"length"` // seconds
	Ends   int64  `json:"ends"`
	Left   int64  `json:"left"` // seconds left while paused
	Was    string `json:"was"`  // what was paused: focus or break
	Sound  bool   `json:"sound"`
}

func (p *Pomo) running() bool { return p != nil && (p.State == "focus" || p.State == "break") }
func (p *Pomo) active() bool  { return p.running() || (p != nil && p.State == "paused") }

// remaining is the seconds left (0 when time's up).
func (p *Pomo) remaining(now int64) int64 {
	if p == nil {
		return 0
	}
	if p.State == "paused" {
		return p.Left
	}
	return max(0, p.Ends-now)
}

// finished: a focus or break whose time has run out (the Time's up screen).
func (p *Pomo) finished(now int64) bool { return p.running() && now >= p.Ends }

func minsLeft(sec int64) string {
	if sec < 60 {
		return "<1 min"
	}
	return fmt.Sprintf("%d 分钟", (sec+59)/60)
}

// pomoCardLine is the label on the "Now working on" card while a timer is on.
func pomoCardLine(p *Pomo, now int64) string { return strings.ToUpper(pomoLine(p, now)) }

func pomoLine(p *Pomo, now int64) string {
	switch {
	case p.State == "paused":
		return "已暂停 · " + minsLeft(p.Left) + " left"
	case p.State == "break":
		return "休息 · " + minsLeft(p.remaining(now)) + " left"
	}
	return "专注 · " + minsLeft(p.remaining(now)) + " left"
}

// pomoBar draws how much of the session has gone.
func pomoBar(img *image.Gray, r image.Rectangle, p *Pomo, now int64) {
	strokeRoundRect(img, r, 5, 2, ink)
	if p.Length <= 0 {
		return
	}
	done := float64(p.Length-p.remaining(now)) / float64(p.Length)
	fill := r
	fill.Max.X = r.Min.X + int(float64(r.Dx())*math.Max(0, math.Min(1, done)))
	fillRoundRect(img, r, 5, ink, fill)
}

// speakerGlyph: a little speaker, with waves when the sound is on.
func speakerGlyph(img *image.Gray, cx, cy float64, on bool, c uint8) {
	fillPolygon(img, [][2]float64{{cx - 9, cy - 4}, {cx - 5, cy - 4}, {cx + 1, cy - 9}, {cx + 1, cy + 9}, {cx - 5, cy + 4}, {cx - 9, cy + 4}}, c)
	if on {
		for i, r := range []float64{5, 9} {
			for a := -0.8; a < 0.8; a += 0.2 {
				line(img, cx+1+r*math.Cos(a), cy+r*math.Sin(a), cx+1+r*math.Cos(a+0.2), cy+r*math.Sin(a+0.2), 1.8, c)
			}
			_ = i
		}
	} else {
		line(img, cx+4, cy-4, cx+11, cy+4, 2, c)
		line(img, cx+11, cy-4, cx+4, cy+4, 2, c)
	}
}

// pomoHeader: a small timer in the header while one is on (tap: go to Tasks).
func pomoHeader(v *View, p *Pomo, now int64, right int) int {
	if !p.active() {
		return right
	}
	img := v.Img
	f := face("uiBold", 15)
	s := minsLeft(p.remaining(now))
	if p.State == "paused" {
		s = "已暂停"
	} else if p.State == "break" {
		s = "休息 " + s
	}
	w := textW(f, s) + 44
	r := image.Rect(right-w, 18, right, 54)
	strokeRoundRect(img, r, 18, 2, ink)
	cx, cy := float64(r.Min.X+20), 36.0
	ringCircle(img, cx, cy, 8, 2, ink)
	line(img, cx, cy, cx, cy-5, 2, ink)
	line(img, cx, cy, cx+4, cy, 2, ink)
	drawText(img, f, r.Min.X+34, 36+capHeight(f)/2, s, ink)
	v.add(Target{R: r.Inset(-4), Show: r, Act: "focusview"})
	return r.Min.X - 10
}

// timesUp: the full screen when a focus or break ends.
func timesUp(v *View, p *Pomo) {
	img := v.Img
	fillRect(img, img.Rect, paper)
	v.Targets = nil // only this screen's buttons
	title, sub := "时间到", "专注得不错，休息一下？"
	if p.Title != "" {
		sub = fmt.Sprintf("专注了 %d 分钟，任务：%s", p.Length/60, p.Title)
	}
	type btn struct{ label, arg string }
	buttons := []btn{{"休息 5 分钟", "break"}, {"再专注 25 分钟", "start"}, {"停止", "stop"}}
	if p.State == "break" {
		title, sub = "休息结束", "准备好下一轮了吗？"
		if p.Title != "" {
			sub = "接下来：" + p.Title
		}
		buttons = []btn{{"开始 25 分钟", "start"}, {"再休息 5 分钟", "break"}, {"停止", "stop"}}
	}
	drawCentered(img, face("display", 120), LW/2, 300, title, ink)
	sf := face("serif", 28)
	drawCentered(img, sf, LW/2, 370, ellipsize(sf, sub, LW-160), grey2)
	bf := face("uiBold", 18)
	x := LW/2 - (3*230+2*20)/2
	for i, b := range buttons {
		r := image.Rect(x+i*250, 450, x+i*250+230, 512)
		pillAt(img, r, b.label, i == 0)
		_ = bf
		v.add(Target{R: r.Inset(-6), Show: r, Act: "pomo", Arg: b.arg})
	}
	// sound on the computer
	sr := image.Rect(LW/2-110, 560, LW/2+110, 604)
	strokeRoundRect(img, sr, 22, 2, ink)
	speakerGlyph(img, float64(sr.Min.X+30), float64(sr.Min.Y+sr.Max.Y)/2, p.Sound, ink)
	lbl := "关闭提示音"
	if p.Sound {
		lbl = "开启提示音"
	}
	f := face("uiBold", 16)
	drawText(img, f, sr.Min.X+52, (sr.Min.Y+sr.Max.Y)/2+capHeight(f)/2, lbl+" (Mac)", ink)
	arg := "sound:1"
	if p.Sound {
		arg = "sound:0"
	}
	v.add(Target{R: sr.Inset(-4), Show: sr, Act: "pomo", Arg: arg})
}
