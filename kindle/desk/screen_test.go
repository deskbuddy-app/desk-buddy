package main

import (
	"image"
	"testing"
	"time"
)

// Tapping a Deck tile changes the tile and the status line: two areas, not one big one.
func TestChangedKeepsSeparateAreasApart(t *testing.T) {
	a, b := newCanvas(LW, LH), newCanvas(LW, LH)
	fillRect(b, image.Rect(520, 552, 750, 732), ink) // a bottom tile
	fillRect(b, image.Rect(700, 100, 996, 130), ink) // the status text, top right
	got := changed(a, b, nil, nil)
	if len(got) != 2 {
		t.Fatalf("want 2 areas, got %v", got)
	}
	for _, r := range got {
		if r.Dx()*r.Dy() > LW*LH/4 {
			t.Errorf("area %v is too big", r)
		}
	}
	if len(changed(a, a, nil, nil)) != 0 {
		t.Error("identical pictures should have no changes")
	}
}

func TestBasicKindleDetectionAndPanels(t *testing.T) {
	defer func() {
		setPanel(1024, 758)
		isBasicKindle = false
		setCompact(false)
	}()

	// 1. Kindle Basic 2022 (11th gen, Bellatrix4) & 2024 (Matcha): 1072×1448 portrait
	isBasicKindle = false
	w, h := parseFBInkEnv("viewWidth=1072;viewHeight=1448;deviceName='Basic 4 (2022)';codename='Bellatrix4';")
	if w != 1072 || h != 1448 || !isBasicKindle {
		t.Fatalf("2022 Basic Kindle: got w=%d h=%d isBasic=%v", w, h, isBasicKindle)
	}
	isBasicKindle = false
	w, h = parseFBInkEnv("viewWidth='1072';viewHeight='1448';deviceName='Kindle Basic 5 (2024)';codename='Matcha';")
	if w != 1072 || h != 1448 || !isBasicKindle {
		t.Fatalf("2024 Basic Kindle (quoted): got w=%d h=%d isBasic=%v", w, h, isBasicKindle)
	}
	isBasicKindle = false
	w, h = parseFBInkEnv("screenWidth=1072;screenHeight=1448;deviceName='Unknown';")
	if w != 1072 || h != 1448 || !isBasicKindle {
		t.Fatalf("1072x1448 screenWidth fallback: got w=%d h=%d isBasic=%v", w, h, isBasicKindle)
	}

	// 2. Older Kindle Basic (167 ppi, 600×800 portrait / 800×600 landscape)
	isBasicKindle = false
	w, h = parseFBInkEnv("viewWidth=600;viewHeight=800;deviceName='Kindle Basic 3';")
	if w != 600 || h != 800 || !isBasicKindle {
		t.Fatalf("600x800 Basic Kindle: got w=%d h=%d isBasic=%v", w, h, isBasicKindle)
	}

	// 3. Downscaling on 800×600 keeps 1px horizontal/vertical lines visible and circles gap-free
	setPanel(800, 600)
	if cw != 800 || ch != 600 || PW != 768 || PH != 1024 {
		t.Fatalf("setPanel(800, 600): got cw=%d ch=%d PW=%d PH=%d", cw, ch, PW, PH)
	}
	img := newCanvas(LW, LH)
	if img.Rect.Dx() != 800 || img.Rect.Dy() != 600 {
		t.Fatalf("canvas size: got %v, want 800x600", img.Rect)
	}
	r := pxRect(img, image.Rect(100, 200, 500, 201))
	if r.Dy() < 1 {
		t.Errorf("1px horizontal rule disappeared at 800x600: %v", r)
	}

	// 4. Compact mode renders all tabs cleanly at both 1448×1072 (Basic 2022/2024) and 800×600
	if err := loadFonts(); err != nil {
		t.Fatalf("loadFonts: %v", err)
	}
	parentID := 1
	d := &Desk{
		Profile: "work",
		Tasks: []Task{
			{ID: 1, Text: "Test compact task"},
			{ID: 2, Text: "Subtask", Parent: &parentID},
		},
		Deck: &Deck{
			Decks: []Deck{{
				Machine:      "mac",
				MachineLabel: "Linux PC",
				Tiles:        []DeckTile{{Label: "Terminal", App: "terminal"}, {Label: "Browser", App: "firefox"}},
				Apps:         []string{"Terminal", "Firefox", "Files", "Code"},
			}},
		},
	}
	st := Status{Now: time.Date(2026, 11, 9, 10, 30, 0, 0, time.UTC), LastOK: time.Now(), Battery: 85}
	for _, sz := range [][2]int{{1448, 1072}, {800, 600}} {
		setPanel(sz[0], sz[1])
		setCompact(true)
		for _, tab := range []string{"tasks", "deck", "calendar", "weather"} {
			ui := &UI{
				Tab: tab, Widget: "day", Collapsed: map[int]bool{},
				Event: -1, CalHour: -1, ConfirmRemove: -1, NightLight: -1, DayLight: -1, Refresh: 1,
				Dismissed: map[string]bool{}, Icons: map[string]map[string]*image.Gray{}, IconsKey: map[string]string{},
				Month: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
				Day:   time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
			}
			v := render(d, st, ui)
			if v == nil || v.Img == nil {
				t.Fatalf("render(%s) at %dx%d returned nil", tab, sz[0], sz[1])
			}
		}
		// Settings › Screen with Compact mode
		ui := &UI{
			Tab: "tasks", Settings: true, SettingsTab: "screen",
			Collapsed: map[int]bool{}, Event: -1, CalHour: -1, ConfirmRemove: -1,
			NightLight: -1, DayLight: -1, Refresh: 1,
			Dismissed: map[string]bool{}, Icons: map[string]map[string]*image.Gray{}, IconsKey: map[string]string{},
		}
		v := render(d, st, ui)
		foundCompact := false
		for _, tg := range v.Targets {
			if tg.Act == "setcompact" {
				foundCompact = true
			}
		}
		if !foundCompact {
			t.Errorf("Settings › Screen at %dx%d missing setcompact target", sz[0], sz[1])
		}
		// Top-left menu includes Exit to Kindle (quitapp)
		uiMenu := &UI{
			Tab: "tasks", Menu: true,
			Collapsed: map[int]bool{}, Event: -1, CalHour: -1, ConfirmRemove: -1,
			NightLight: -1, DayLight: -1, Refresh: 1,
			Dismissed: map[string]bool{}, Icons: map[string]map[string]*image.Gray{}, IconsKey: map[string]string{},
		}
		vm := render(d, st, uiMenu)
		foundQuit := false
		for _, tg := range vm.Targets {
			if tg.Act == "quitapp" {
				foundQuit = true
			}
		}
		if !foundQuit {
			t.Errorf("Menu at %dx%d missing quitapp target", sz[0], sz[1])
		}
	}
}
