package main

// Offline screenshots and action snapshots use only fictitious data. This file
// also runs against a temporary pristine v1.3 checkout for differential review.
import (
	"encoding/json"
	"fmt"
	"golang.org/x/image/font"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRenderProtocolAndPreviews(t *testing.T) {
	out := os.Getenv("DESK_BUDDY_RENDER_OUT")
	if out == "" {
		t.Skip("set DESK_BUDDY_RENDER_OUT to keep the offline evidence")
	}
	fixture := os.Getenv("DESK_BUDDY_RENDER_FIXTURE")
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err = loadFonts(); err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(out, 0755); err != nil {
		t.Fatal(err)
	}
	type action struct {
		Act  string
		ID   int
		Arg  string
		Temp int
	}
	type snapshot struct {
		Name, Kind string
		Actions    []action
		Data       json.RawMessage
	}
	var result []snapshot
	panels := [][2]int{{1024, 758}, {800, 600}, {1448, 1072}}
	cases := []string{"tasks", "task-open", "delete-task", "menu", "light", "calendar", "deck", "settings-screen", "settings-focus", "settings-music", "settings-tasks", "settings-calendar", "settings-server", "settings-updates", "settings-themes", "settings-about", "clear-list", "standby", "tour", "connecting", "offline"}
	for _, panel := range panels {
		setPanel(panel[0], panel[1])
		setCompact(panel[0] == 800)
		for _, name := range cases {
			faces = map[string]font.Face{}
			themeName, fontStyle = "plain", "sans"
			var d Desk
			if err = json.Unmarshal(data, &d); err != nil {
				t.Fatal(err)
			}
			now := time.Unix(d.Now, 0).In(time.FixedZone("fixture", d.UTCOffset))
			ui := &UI{Tab: "tasks", Widget: "day", Collapsed: map[int]bool{}, Event: -1, CalHour: -1, ConfirmRemove: -1, NightLight: -1, DayLight: -1, Refresh: 1, Rotation: 90, Dismissed: map[string]bool{}, Icons: map[string]map[string]*image.Gray{}, Month: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()), Day: now, Cal: d.Agenda}
			st := Status{Now: now, Server: "http://fixture.invalid", Battery: 86, Light: 12, OnSince: now.Add(-time.Hour)}
			if name == "calendar" || name == "deck" {
				ui.Tab = name
			}
			if len(name) > 9 && name[:9] == "settings-" {
				ui.Settings = true
				ui.SettingsTab = name[9:]
			}
			switch name {
			case "task-open":
				ui.Selected = 1
			case "delete-task":
				ui.DeleteAsk = 1
			case "clear-list":
				ui.Settings = true
				ui.SettingsTab = "tasks"
				ui.ClearAsk = true
			case "menu":
				ui.Menu = true
			case "light":
				ui.LightMenu = true
			case "standby":
				ui.Standby = true
			case "tour":
				ui.Tour = 1
			case "offline":
				ui.OfflineInfo = true
				st.Offline = true
				st.LastOK = now.Add(-time.Hour)
			}
			var view *View
			if name == "connecting" {
				view = render(nil, st, ui)
			} else {
				view = render(&d, st, ui)
			}
			rec := snapshot{Name: fmt.Sprintf("%dx%d-%s", panel[0], panel[1], name), Kind: view.Kind}
			for _, target := range view.Targets {
				rec.Actions = append(rec.Actions, action{target.Act, target.ID, target.Arg, target.Temp})
			}
			rec.Data, _ = json.Marshal(d)
			result = append(result, rec)
			if os.Getenv("DESK_BUDDY_RENDER_IMAGES") != "0" {
				f, e := os.Create(filepath.Join(out, rec.Name+".png"))
				if e != nil {
					t.Fatal(e)
				}
				e = png.Encode(f, view.Img)
				f.Close()
				if e != nil {
					t.Fatal(e)
				}
			}
		}
	}
	// The two themes that replace every font must retain Chinese glyphs as well.
	if os.Getenv("DESK_BUDDY_RENDER_IMAGES") != "0" {
		setPanel(1024, 758)
		setCompact(false)
		for _, theme := range themes {
			for _, dark := range []bool{false, true} {
				faces = map[string]font.Face{}
				themeName = theme.id
				var d Desk
				json.Unmarshal(data, &d)
				now := time.Unix(d.Now, 0).In(time.FixedZone("fixture", d.UTCOffset))
				ui := &UI{Tab: "tasks", Widget: "day", Dark: dark, Selected: 1, Collapsed: map[int]bool{}, ConfirmRemove: -1}
				view := render(&d, Status{Now: now, Battery: 86}, ui)
				f, e := os.Create(filepath.Join(out, fmt.Sprintf("theme-%s-dark-%v.png", theme.id, dark)))
				if e != nil {
					t.Fatal(e)
				}
				e = png.Encode(f, view.Img)
				f.Close()
				if e != nil {
					t.Fatal(e)
				}
			}
		}
	}
	b, _ := json.MarshalIndent(result, "", "  ")
	if err = os.WriteFile(filepath.Join(out, "actions.json"), b, 0644); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d protocol snapshots; 63 panel views and 16 theme views when images enabled", len(result))
}
