package main

// The Settings screen (from the Work / Home / Auto dropdown). Look settings (theme, font)
// live on the Kindle; the rest live on the backend because they affect the computers.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	xdraw "golang.org/x/image/draw"
)

// fontStyle: "sans" (Atkinson Hyperlegible, the default), "serif" (Literata), "inter" or
// "bitter" for task text and headings.
var fontStyle = "sans"

var fontStyles = []string{"sans", "serif", "inter", "bitter"}

func settingsView(v *View, d *Desk, st Status, ui *UI) {
	img := v.Img
	bf := face("uiBold", 17)
	back := image.Rect(pad-8, 16, pad+120, 60)
	chevron(img, float64(back.Min.X+18), 38, 7, "left", ink)
	drawText(img, bf, back.Min.X+34, 38+capHeight(bf)/2, "返回", ink)
	v.add(Target{R: back.Inset(-6), Show: back, Act: "settings"})
	drawCentered(img, face("serifB", 30), LW/2, 48, "设置", ink)
	// top right, as on the main screens: refresh, then light / dark
	lightDark := image.Rect(LW-pad-44, 14, LW-pad, 58)
	contrastIcon(img, float64(lightDark.Min.X+lightDark.Max.X)/2, 36, 11)
	v.add(Target{R: image.Rect(lightDark.Min.X, 0, LW, barH), Show: lightDark, Act: "dark"})
	ref := image.Rect(lightDark.Min.X-6-44, 14, lightDark.Min.X-6, 58)
	refreshIcon(img, float64(ref.Min.X+ref.Max.X)/2, 36, 10)
	v.add(Target{R: image.Rect(ref.Min.X, 0, ref.Max.X, barH), Show: ref, Act: "refresh"})
	fillRect(img, image.Rect(0, barH, LW, barH+2), ink)

	set := d.Settings
	bval := func(k string, def bool) bool {
		if v, ok := set[k].(bool); ok {
			return v
		}
		return def
	}
	sval := func(k, def string) string {
		if v, ok := set[k].(string); ok && v != "" {
			return v
		}
		return def
	}
	nval := func(k string, def int) int {
		if v, ok := set[k].(float64); ok {
			return int(v)
		}
		return def
	}
	onOff := func(on bool) int {
		if on {
			return 0
		}
		return 1
	}
	pick := func(vals []int, cur int) int {
		for i, x := range vals {
			if x == cur {
				return i
			}
		}
		return 0
	}

	type row struct {
		title, hint string
		labels      []string
		args        []string
		sel         int
		act         string
		off         bool // greyed out and not tappable (what it depends on is off)
	}
	col := func(x0, x1, y int, heading string, rows []row) int {
		label(img, x0, y, heading, grey2)
		y += 18
		step := 74
		if len(rows) >= 6 {
			step = 54
		} else if len(rows) >= 5 {
			step = 62
		}
		for _, r := range rows {
			cw := 72 // each choice at least 72 wide, wider for longer words ("Per computer")
			for _, l := range r.labels {
				cw = max(cw, textW(face("uiBold", 16), l)+28)
			}
			w := min(320, cw*len(r.labels))
			ty, hy, sy, bh := y+30, y+52, y+10, 44
			if step == 54 {
				ty, hy, sy, bh = y+23, y+41, y+5, 40
			} else if step < 74 {
				ty, hy, sy = y+26, y+46, y+8
			}
			drawText(img, face("serifB", 20), x0, ty, r.title, ink)
			if r.hint != "" {
				drawText(img, face("ui", 13), x0, hy, ellipsize(face("ui", 13), r.hint, x1-x0-w-16), grey2)
			}
			n := len(v.Targets)
			segmented(v, image.Rect(x1-w, sy, x1, sy+bh), r.labels, r.args, r.sel, r.act)
			if r.off {
				fade(img, image.Rect(x0, y, x1, y+step-8))
				v.Targets = v.Targets[:n]
			}
			fillRect(img, image.Rect(x0, y+step-6, x1, y+step-5), grey4)
			y += step
		}
		return y
	}
	dark := 0
	if ui.Dark {
		dark = 1
	}
	font := 0
	for i, f := range fontStyles {
		if f == fontStyle {
			font = i
		}
	}
	unit := 0
	if tempF {
		unit = 1
	}
	focus := []int{25, 45, 50}
	brk := []int{5, 10, 15}
	sound := d.Pomodoro != nil && d.Pomodoro.Sound
	what := listName(d) + "列表" // for Clear: "the Mac list", or "the list"
	if listName(d) == "" {
		what = "列表"
	}

	// Left: the sections. Right: the chosen one's options.
	const sideW = 260
	fillRect(img, image.Rect(sideW-2, barH+2, sideW, LH), ink)
	sections := []struct {
		id, name, hint string
		glyph          func(*image.Gray, float64, float64, uint8)
	}{
		{"screen", "屏幕", "主题、尺寸、字体", sunGlyph},
		{"focus", "专注", "番茄钟与休息", focusGlyph},
		{"music", "音乐", "应用、同步与封面", noteGlyph},
		{"tasks", "任务", "列表与清空", tasksGlyph},
		{"calendar", "日历", "日历与会议提醒", calendarGlyph},
		{"server", "服务器", "云端或本地", cloudGlyph},
		{"updates", "更新", "版本与电量", refreshGlyph},
		{"themes", "主题", "花园、复古、手绘…", flowerGlyph},
		{"about", "关于", "作者与反馈", infoGlyph},
	}
	cur := ui.SettingsTab
	if cur == "" {
		cur = "screen"
	}
	const rowH = 68
	for i, sec := range sections {
		r := image.Rect(0, barH+2+i*rowH, sideW-2, barH+2+(i+1)*rowH)
		fg, soft := ink, grey2
		if sec.id == cur {
			fillRect(img, r, ink) // the open section, inverted (like the chosen day on the Weather tab)
			fg, soft = paper, grey4
		} else if i > 0 && sections[i-1].id != cur {
			fillRect(img, image.Rect(pad, r.Min.Y, sideW-2-pad, r.Min.Y+1), grey4)
		}
		sec.glyph(img, float64(pad+12), float64(r.Min.Y+rowH/2), fg)
		drawText(img, face("serifB", 22), pad+40, r.Min.Y+32, sec.name, fg)
		drawText(img, face("ui", 14), pad+40, r.Min.Y+52, sec.hint, soft)
		if sec.id != cur {
			chevron(img, float64(sideW-2-pad), float64(r.Min.Y+rowH/2), 5, "right", grey2)
		}
		v.add(Target{R: r, Show: r, Act: "settab", Arg: sec.id})
	}

	x0, x1 := sideW+40, LW-pad
	y := barH + 26
	switch cur {
	case "screen":
		theme, themeHint := dark, "深色：黑底白字"
		if ui.AutoDark {
			theme, themeHint = 2, "跟随 Mac 深色模式，否则日落后开启"
		}
		compSel := 0
		if compactMode {
			compSel = 1
		}
		rotSel := 0
		if ui.Rotation == 270 {
			rotSel = 1
		}
		rows := []row{
			{"主题", themeHint, []string{"浅色", "深色", "自动"}, []string{"light", "dark", "auto"}, theme, "settheme", false},
			{"屏幕尺寸", "紧凑：适合 6 英寸入门款 Kindle", []string{"标准", "紧凑"}, []string{"0", "1"}, compSel, "setcompact", false},
			{"方向", "Kindle 底边所在的一侧", []string{"左侧", "右侧"}, []string{"90", "270"}, rotSel, "setrot", false},
			{"字体", "用于任务与标题", []string{"无衬线", "衬线", "Inter", "Bitter"}, fontStyles, font, "setfont", false},
			{"温度", "用于天气", []string{"°C", "°F"}, []string{"c", "f"}, unit, "setunit", false},
		}
		// the front light while Auto is dark (greyed out unless the theme is Auto)
		levels := []int{-1, 0, 4, 10}
		sel := 0
		for i, l := range levels {
			if l == ui.NightLight {
				sel = i
			}
		}
		nightHint := "自动深色模式下的前光"
		if !ui.AutoDark {
			nightHint = "先在上方选择自动模式"
		}
		night := row{"夜间亮度", nightHint, []string{"相同", "关闭", "低", "中"}, []string{"-1", "0", "4", "10"}, sel, "nightlight", !ui.AutoDark}
		rows = append(rows[:1], append([]row{night}, rows[1:]...)...)
		ny := col(x0, x1, y, "屏幕", rows)
		refreshCards(v, ui, x0, x1, ny)
	case "focus":
		col(x0, x1, y, "专注", []row{
			{"专注时长", "每次番茄钟的分钟数", []string{"25", "45", "50"}, []string{"focus_min=25", "focus_min=45", "focus_min=50"}, pick(focus, nval("focus_min", 25)), "setting", false},
			{"休息时长", "两次专注间的休息分钟数", []string{"5", "10", "15"}, []string{"break_min=5", "break_min=10", "break_min=15"}, pick(brk, nval("break_min", 5)), "setting", false},
			{"电脑提示音", "专注或休息结束时在当前电脑播放", []string{"开启", "关闭"}, []string{"sound:1", "sound:0"}, onOff(sound), "pomo", false},
		})
	case "music":
		col(x0, x1, y, "音乐", []row{
			{"音乐同步", "显示电脑正在播放的音乐", []string{"开启", "关闭"}, []string{"music_sync=true", "music_sync=false"}, onOff(bval("music_sync", true)), "setting", false},
			{"音乐应用", map[string]string{"auto": "跟随正在播放的应用", "spotify": "仅 Spotify", "music": "仅 Apple Music"}[sval("music_app", "auto")], []string{"自动", "Spotify", "Apple Music"}, []string{"music_app=auto", "music_app=spotify", "music_app=music"}, map[string]int{"auto": 0, "spotify": 1, "music": 2}[sval("music_app", "auto")], "setting", !bval("music_sync", true)},
			{"音乐应用标志", "在曲目旁显示音乐应用标志", []string{"开启", "关闭"}, []string{"1", "0"}, map[bool]int{true: 0, false: 1}[ui.ShowMusicLogo], "musiclogo", !bval("music_sync", true)},
			{"专辑封面", "在音乐页显示有封面的曲目", []string{"开启", "关闭"}, []string{"album_covers=true", "album_covers=false"}, onOff(bval("album_covers", true)), "setting", !bval("music_sync", true)},
			{"封面质量", "清晰：大封面使用双倍细节", []string{"标准", "清晰"}, []string{"cover_sharp=false", "cover_sharp=true"}, map[bool]int{false: 0, true: 1}[bval("cover_sharp", true)], "setting", !bval("music_sync", true) || !bval("album_covers", true)},
		})
	case "tasks":
		pc := pcName(d)
		sep, sepHint := 0, "Mac 与 "+pc+" 电脑分别使用独立任务列表"
		if searching(d) {
			sepHint = "正在搜索其他设备…请先连接另一台电脑"
		}
		if oneList(d) {
			sep, sepHint = 1, "关闭：所有设备共用一个列表"
		}
		col(x0, x1, y, "任务", []row{
			{"独立任务列表", sepHint, []string{"开启", "关闭"}, []string{"task_lists=per_computer", "task_lists=one"}, sep, "setting", searching(d)},
			{"自动删除已完成任务", "自动清理勾选完成的任务", []string{"开启", "关闭"}, []string{"auto_clear=true", "auto_clear=false"}, onOff(bval("auto_clear", false)), "setting", false},
			{"删除延迟", "勾选完成后等待多久", []string{"立即", "1 min", "5 min", "10 min"}, []string{"auto_clear_min=0", "auto_clear_min=1", "auto_clear_min=5", "auto_clear_min=10"}, pick([]int{0, 1, 5, 10}, nval("auto_clear_min", 5)), "setting", !bval("auto_clear", false)},
		})
		y += 18 + 3*74
		clearHint := "清空当前显示的" + what + "，操作前会确认。"
		if oneList(d) {
			clearHint = "清空任务列表，操作前会确认。"
		}
		drawText(img, face("serifB", 20), x0, y+30, "清空任务", ink)
		drawText(img, face("ui", 13), x0, y+52, clearHint, grey2)
		cb := image.Rect(x1-144, y+10, x1, y+54) // same width as On | Off above
		pillAt(img, cb, "清空…", false)
		v.add(Target{R: cb.Inset(-4), Show: cb, Act: "clearask"})
		fillRect(img, image.Rect(x0, y+68, x1, y+69), grey4)
		nf := face("ui", 15)
		notes := []string{"在左上角菜单选择 Mac、" + pc + " 或自动模式。", "自动模式跟随当前使用的电脑。", "", "关闭独立列表后，会将 " + pc + " 的任务", "合并到 Mac 的同一个列表中。"}
		if oneList(d) {
			notes = []string{"开启独立列表可让每台电脑分别使用自己的", "列表。原有任务保留在 Mac 列表中。"}
		} else if searching(d) {
			notes = []string{"要连接另一台电脑，请在该电脑安装 Desk Buddy。", "然后在左上角菜单选择 Kindle 跟随的电脑，", "或选择自动模式跟随当前使用的电脑。"}
		}
		for i, l := range notes {
			drawText(img, nf, x0, y+108+i*22, l, grey2)
		}
	case "calendar":
		on := 0
		if ui.NoHeadsUp {
			on = 1
		}
		early := ui.HeadsUpMin
		if early == 0 {
			early = 5
		}
		col(x0, x1, y, "会议提醒", []row{
			{"会议前提醒", "会前显示提醒卡，可打开 Zoom / Meet 链接", []string{"开启", "关闭"}, []string{"1", "0"}, on, "headsup", false},
			{"提前多久", "会议开始前的分钟数", []string{"2", "5", "10"}, []string{"2", "5", "10"}, pick([]int{2, 5, 10}, early), "headsupmin", ui.NoHeadsUp},
		})
		calendarList(v, d, ui, x0, x1, y+18+2*74+44)
	case "server":
		serverRow(v, ui, x0, x1, y)
	case "updates":
		updatesPane(v, ui, st, x0, x1, y)
	case "themes":
		themesPane(v, x0, x1, y)
	case "about":
		aboutPane(v, x0, x1, y)
	}
	if ui.ClearAsk {
		clearModal(v, d, what)
	}
	if ui.NotesModal && cur == "updates" {
		notesModal(v, ui)
	}
}

// deleteModal: "Delete this task?" (after a swipe and the bin).
func deleteModal(v *View, d *Desk, id int) {
	img := v.Img
	var t *Task
	kids := 0
	for i := range d.Tasks {
		if d.Tasks[i].ID == id {
			t = &d.Tasks[i]
		}
		if d.Tasks[i].Parent != nil && *d.Tasks[i].Parent == id {
			kids++
		}
	}
	if t == nil {
		return
	}
	for i, p := range img.Pix {
		img.Pix[i] = uint8((int(p)*35 + 172*65) / 100)
	}
	v.Targets = nil
	box := image.Rect(LW/2-300, LH/2-110, LW/2+300, LH/2+110)
	fillRoundRect(img, box.Inset(-3), 16, paper)
	strokeRoundRect(img, box, 14, 2, ink)
	x0, x1 := box.Min.X+32, box.Max.X-32
	drawText(img, face("serifB", 28), x0, box.Min.Y+56, "删除此任务？", ink)
	f := face("serif", 20)
	drawText(img, f, x0, box.Min.Y+94, ellipsize(f, "“"+t.Text+"”", x1-x0), grey2)
	if kids > 0 {
		sub := "子任务也会被删除。"
		if kids > 1 {
			sub = fmt.Sprintf("其 %d 个子任务也会被删除。", kids)
		}
		drawText(img, face("ui", 16), x0, box.Min.Y+122, sub, grey2)
	}
	by := box.Max.Y - 28 - 48
	cancel := pillAt(img, image.Rect(x0, by, x0+120, by+48), "取消", false)
	v.add(Target{R: img.Rect, Show: cancel, Act: "binask", ID: 0}) // Cancel, or tap outside
	del := pillAt(img, image.Rect(x1-130, by, x1, by+48), "删除", true)
	v.add(Target{R: del.Inset(-4), Show: del, Act: "deltask", ID: id})
}

// clearModal: "Clear the Work list?" over the faded Settings screen.
func clearModal(v *View, d *Desk, what string) { // what: "the Mac list", "the list"
	img := v.Img
	for i, p := range img.Pix {
		img.Pix[i] = 255 - uint8(int(255-p)*28/100)
	}
	v.Targets = nil
	all, done := 0, 0
	for _, t := range d.Tasks {
		if t.Parent == nil {
			all++
			if t.Done {
				done++
			}
		}
	}
	box := image.Rect(LW/2-300, LH/2-120, LW/2+300, LH/2+120)
	fillRoundRect(img, box.Inset(-3), 16, paper)
	strokeRoundRect(img, box, 14, 2, ink)
	x0, x1 := box.Min.X+32, box.Max.X-32
	drawText(img, face("serifB", 30), x0, box.Min.Y+60, "清空"+displayList(what)+"？", ink)
	f := face("ui", 17)
	drawText(img, f, x0, box.Min.Y+98, fmt.Sprintf("共 %d 项任务，已完成 %d 项。子任务也会删除。", all, done), grey2)
	drawText(img, f, x0, box.Min.Y+124, "此操作无法撤销。", grey2)
	by := box.Max.Y - 32 - 50
	cancel := pillAt(img, image.Rect(x0, by, x0+120, by+50), "取消", false)
	v.add(Target{R: cancel.Inset(-4), Show: cancel, Act: "clearask"})
	bf := face("uiBold", 16)
	everything := fmt.Sprintf("全部（%d）", all)
	er := image.Rect(x1-textW(bf, everything)-36, by, x1, by+50)
	pillAt(img, er, everything, true)
	if all > 0 {
		v.add(Target{R: er.Inset(-4), Show: er, Act: "cleartasks", Arg: d.Profile + "|all"})
	}
	doneS := fmt.Sprintf("已完成（%d）", done)
	dr := image.Rect(er.Min.X-12-textW(bf, doneS)-36, by, er.Min.X-12, by+50)
	if done > 0 {
		pillAt(img, dr, doneS, false)
		v.add(Target{R: dr.Inset(-4), Show: dr, Act: "cleartasks", Arg: d.Profile + "|done"})
	} else {
		strokeRoundRect(img, dr, 7, 2, grey4)
		drawCentered(img, bf, (dr.Min.X+dr.Max.X)/2, (dr.Min.Y+dr.Max.Y)/2+capHeight(bf)/2, doneS, grey3)
	}
}

// decodeArt turns the album cover from the backend into a small, punchy greyscale picture.
func decodeArt(b64 string, size int) *image.Gray {
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil
	}
	src, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil
	}
	size = min(size, max(src.Bounds().Dx(), 1)) // keep a small cover small (Standard is 320 px)
	g := image.NewGray(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(g, g.Rect, src, src.Bounds(), xdraw.Src, nil)
	photoTone(g) // so it doesn't look flat and dark on e-ink
	return g
}

var _ = strings.Cut

// fade greys out an area (a setting that's switched off): black becomes mid grey, white stays.
func fade(img *image.Gray, r image.Rectangle) {
	r = pxRect(img, r).Intersect(img.Rect)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := img.Pix[img.PixOffset(r.Min.X, y):img.PixOffset(r.Max.X, y)]
		for i, p := range row {
			row[i] = uint8(255 - (255-int(p))*40/100)
		}
	}
}

// refreshCards: Settings › Screen › Screen refresh as three cards, each with a small picture
// of what flashes: nothing (Gentle), big changes where they are (Normal), every change (Clean).
func refreshCards(v *View, ui *UI, x0, x1, y int) {
	img := v.Img
	drawText(img, face("serifB", 20), x0, y+30, "屏幕刷新", ink)
	drawText(img, face("ui", 13), x0, y+52, "画面变化时如何刷新", grey2)
	cards := []struct{ name, text string }{
		{"柔和", "不闪屏，可能留下轻微残影。"},
		{"标准", "较大变化时，仅在变化区域闪屏。"},
		{"清晰", "每次变化都闪屏，显示最清晰。"},
	}
	const gap = 16
	cw := (x1 - x0 - 2*gap) / 3
	top, bottom := y+62, LH-18
	for i, c := range cards {
		r := image.Rect(x0+i*(cw+gap), top, x0+i*(cw+gap)+cw, bottom)
		if i == ui.Refresh {
			strokeRoundRect(img, r, 12, 3, ink)
		} else {
			strokeRoundRect(img, r, 12, 2, grey4)
		}
		// the picture: a little screen with a list on it
		pw, ph := 150, min(96, max(64, (bottom-top)-136))
		p := image.Rect((r.Min.X+r.Max.X-pw)/2, r.Min.Y+16, (r.Min.X+r.Max.X+pw)/2, r.Min.Y+16+ph)
		fillRect(img, p, paper)
		rowGap := max(13, (ph-20)/4)
		for k := 0; k < 4; k++ { // the list's lines
			ly := p.Min.Y + 12 + k*rowGap
			if i == 0 && k == 1 { // gentle: a faint ghost of what was there before
				fillRect(img, image.Rect(p.Min.X+18, ly+5, p.Min.X+18+70, ly+9), grey4)
			}
			fillCircle(img, float64(p.Min.X+16), float64(ly+2), 4, ink)
			fillRect(img, image.Rect(p.Min.X+26, ly, p.Min.X+26+[]int{92, 70, 100, 56}[k], ly+5), ink)
		}
		switch i {
		case 1: // normal: only the changed row flashes
			fl := image.Rect(p.Min.X+6, p.Min.Y+12+rowGap-4, p.Max.X-6, p.Min.Y+12+rowGap+12)
			invertRect(img, fl)
		case 2: // clean: every change flashes, the small ones too
			invertRect(img, image.Rect(p.Min.X+6, p.Min.Y+12+rowGap-4, p.Max.X-6, p.Min.Y+12+rowGap+12))
			invertRect(img, image.Rect(p.Min.X+8, p.Max.Y-24, p.Min.X+24, p.Max.Y-10))
			invertRect(img, image.Rect(p.Max.X-30, p.Min.Y+6, p.Max.X-8, p.Min.Y+18))
		}
		strokeRoundRect(img, p.Inset(-2), 6, 2, ink)
		// name, with a filled dot when chosen, and what it does
		nf := face("uiBold", 18)
		ny := p.Max.Y + 30
		if i == ui.Refresh {
			fillCircle(img, float64(r.Min.X+24), float64(ny-6), 6, ink)
		} else {
			ringCircle(img, float64(r.Min.X+24), float64(ny-6), 6, 2, grey3)
		}
		drawText(img, nf, r.Min.X+38, ny, c.name, ink)
		tf := face("ui", 14)
		for k, l := range wrap(tf, c.text, cw-32, 3) {
			drawText(img, tf, r.Min.X+16, ny+26+k*19, l, grey2)
		}
		v.add(Target{R: r, Show: r, Act: "setrefresh", Arg: itoa(i)})
	}
}

// calendarList: Settings › Calendar: every calendar, and which list it shows on (or off).
// Long lists get pages (Newer / Older style arrows by the heading).
func calendarList(v *View, d *Desk, ui *UI, x0, x1, y int) {
	img := v.Img
	label(img, x0, y, "日历", grey2)
	if len(d.CalList) == 0 {
		drawText(img, face("ui", 15), x0, y+34, "暂无日历，Desk Buddy 会同步电脑的日历。", grey2)
		return
	}
	pc := pcName(d)
	labels, args := []string{"两者", "Mac", pc, "关闭"}, []string{"both", "work", "home", "off"}
	hint := map[string]string{"both": "两个列表均显示", "work": "仅在 Mac 列表显示", "home": "仅在 " + pc + " 列表显示", "off": "不显示"}
	if oneList(d) || searching(d) {
		labels, args = []string{"开启", "关闭"}, []string{"both", "off"}
		hint = map[string]string{"both": "显示", "work": "显示", "home": "显示", "off": "不显示"}
	}
	const rowH = 64
	per := max(1, (LH-16-(y+14))/rowH)
	pages := (len(d.CalList) + per - 1) / per
	ui.CalListPage = max(0, min(ui.CalListPage, pages-1))
	if pages > 1 {
		pf := face("uiBold", 15)
		older := image.Rect(x1-80, y-24, x1, y+10)
		newer := image.Rect(older.Min.X-92, y-24, older.Min.X-12, y+10)
		for _, b := range []struct {
			r   image.Rectangle
			s   string
			on  bool
			arg string
		}{{newer, "‹ 返回", ui.CalListPage > 0, "-1"}, {older, "更多 ›", ui.CalListPage < pages-1, "1"}} {
			c := ink
			if !b.on {
				c = grey4
			}
			drawCentered(img, pf, (b.r.Min.X+b.r.Max.X)/2, (b.r.Min.Y+b.r.Max.Y)/2+5, b.s, c)
			if b.on {
				v.add(Target{R: b.r.Inset(-6), Show: b.r, Act: "calpage", Arg: b.arg})
			}
		}
	}
	yy := y + 14
	end := min(len(d.CalList), (ui.CalListPage+1)*per)
	for _, c := range d.CalList[ui.CalListPage*per : end] {
		sel := 0
		for i, a := range args {
			if a == c.Show || (len(args) == 2 && c.Show != "off" && a == "both") {
				sel = i
			}
		}
		w := min(320, 80*len(labels))
		tf := face("serifB", 18)
		drawText(img, tf, x0, yy+26, ellipsize(tf, c.Name, x1-x0-w-16), ink)
		drawText(img, face("ui", 13), x0, yy+46, hint[c.Show], grey2)
		cargs := make([]string, len(args))
		for i, a := range args {
			cargs[i] = a + "|" + c.Name
		}
		segmented(v, image.Rect(x1-w, yy+8, x1, yy+50), labels, cargs, sel, "calshow")
		fillRect(img, image.Rect(x0, yy+rowH-4, x1, yy+rowH-3), grey4)
		yy += rowH
	}
}
