package main

// The Kindle's own settings (screen, clock, heads-up, updates) are mirrored on the backend
// (kv kindle_settings, KINDLE_SETTINGS in worker/src/index.js) so Desk Buddy on the
// Mac can show and change them. Both sides keep a copy; the newest change wins:
//   - a change here: saved locally, then sent (POST /api/kindle/settings, which answers with
//     its time); while offline it waits (dirty) and goes on the next good fetch
//   - a change elsewhere: the desk carries kindle_settings; newer than ours → applied here
// The first time (nothing synced yet) this Kindle's settings are sent up.
//
// The Kindle also says how it is with each desk request (battery, charging, version, build),
// for the helper's Home screen.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"golang.org/x/image/font"
	"net/url"
)

// KindleSettings: the synced settings and when they last changed (server time, seconds).
type KindleSettings struct {
	Values map[string]any `json:"values"`
	At     float64        `json:"at"`
}

func kindleSettings(ui *UI) map[string]any {
	theme := "light"
	if ui.AutoDark {
		theme = "auto"
	} else if ui.Dark {
		theme = "dark"
	}
	hu, ue := ui.HeadsUpMin, ui.UpdEvery
	if hu == 0 {
		hu = 5
	}
	if ue == 0 {
		ue = 12
	}
	rot := ui.Rotation
	if rot != 270 {
		rot = 90
	}
	return map[string]any{
		"theme": theme, "night_light": ui.NightLight, "font": fontStyle, "temp_f": tempF,
		"refresh": ui.Refresh, "heads_up": !ui.NoHeadsUp, "heads_up_min": hu,
		"clock_12h": ui.Clock12, "clock_quote": !ui.NoQuote, "clock_event": !ui.NoEvent,
		"update_show": !ui.NoUpdShow, "update_every": ue, "music_logo": ui.ShowMusicLogo, "style": map[bool]string{true: "plain", false: themeName}[themeName == ""],
		"compact": compactMode, "rotation": rot,
	}
}

// settingsKey: the synced settings as a string, to notice changes.
func settingsKey(ui *UI) string {
	b, _ := json.Marshal(kindleSettings(ui))
	return string(b)
}

// applyKindleSettings sets what came from the backend. Returns whether the theme or font
// changed (the screen needs a full redraw).
func applyKindleSettings(ui *UI, v map[string]any) (look bool) {
	str := func(k string) (string, bool) { s, ok := v[k].(string); return s, ok }
	num := func(k string) (int, bool) { f, ok := v[k].(float64); return int(f), ok }
	flag := func(k string) (bool, bool) { b, ok := v[k].(bool); return b, ok }
	if t, ok := str("theme"); ok {
		auto, dark := t == "auto", t == "dark"
		look = look || auto != ui.AutoDark || (!auto && dark != ui.Dark)
		ui.AutoDark = auto
		if !auto {
			ui.Dark = dark
		}
	}
	if f, ok := str("font"); ok && f != fontStyle {
		for _, s := range fontStyles {
			if s == f {
				fontStyle, look = f, true
			}
		}
	}
	if n, ok := num("night_light"); ok {
		ui.NightLight = n
	}
	if b, ok := flag("temp_f"); ok {
		tempF = b
	}
	if n, ok := num("refresh"); ok {
		ui.Refresh = max(0, min(2, n))
	}
	if b, ok := flag("heads_up"); ok {
		ui.NoHeadsUp = !b
	}
	if n, ok := num("heads_up_min"); ok {
		ui.HeadsUpMin = n
	}
	if b, ok := flag("clock_12h"); ok {
		ui.Clock12 = b
	}
	if b, ok := flag("clock_quote"); ok {
		ui.NoQuote = !b
	}
	if b, ok := flag("clock_event"); ok {
		ui.NoEvent = !b
	}
	if b, ok := flag("update_show"); ok {
		ui.NoUpdShow = !b
	}
	if n, ok := num("update_every"); ok {
		ui.UpdEvery = n
	}
	if b, ok := flag("music_logo"); ok {
		ui.ShowMusicLogo = b
	}
	if b, ok := flag("compact"); ok && b != compactMode {
		setCompact(b)
		look = true
	}
	if n, ok := num("rotation"); ok && (n == 90 || n == 270) {
		cur := ui.Rotation
		if cur != 270 {
			cur = 90
		}
		if n != cur {
			ui.Rotation = n
			look = true
		}
	}
	if t, ok := str("style"); ok {
		want := map[bool]string{true: "", false: t}[t == "plain"]
		for _, th := range themes {
			if th.id == t && want != themeName {
				themeName, look = want, true
				faces = map[string]font.Face{} // a theme may bring its own fonts
			}
		}
	}
	return look
}

// pushSettings sends this Kindle's settings; returns the backend's time for them.
func (c *Client) pushSettings(v map[string]any) (float64, error) {
	b, _ := json.Marshal(map[string]any{"values": v})
	resp, err := c.hc.Post(c.url("/api/kindle/settings"), "application/json", bytes.NewReader(b))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("settings: %s", resp.Status)
	}
	var ks KindleSettings
	err = json.NewDecoder(resp.Body).Decode(&ks)
	return ks.At, err
}

// settingsChanged: saveUI noticed a synced setting changed here (main sends it).
var settingsChanged = make(chan struct{}, 1)

// statusQuery: how this Kindle is, added to each desk request ("&kb=86&kc=1&kv=…&ks=…").
var statusQuery string

func setStatusQuery(battery int, charging bool, sha string) {
	c := "0"
	if charging {
		c = "1"
	}
	statusQuery = fmt.Sprintf("kb=%d&kc=%s&kv=%s&ks=%s", battery, c, url.QueryEscape(buildVersion), url.QueryEscape(sha))
}
