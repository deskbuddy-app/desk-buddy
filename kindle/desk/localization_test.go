package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
)

func TestChineseFallbackKeepsLatinMetrics(t *testing.T) {
	if err := loadFonts(); err != nil {
		t.Fatal(err)
	}
	themeName, fontStyle = "", "serif"
	f := face("ui", 20)
	original, err := opentype.NewFace(fonts["ui"], &opentype.FaceOptions{Size: 20, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		t.Fatal(err)
	}
	defer original.Close()
	for _, s := range []string{"Desk Buddy", "Wi-Fi 10:32", "AV To 123"} {
		if font.MeasureString(f, s) != font.MeasureString(original, s) {
			t.Fatalf("Latin widths changed: %q", s)
		}
	}
	for _, r := range "简体中文任务日历设置，。" {
		rect, _, ok := f.GlyphBounds(r)
		if !ok || rect.Empty() {
			t.Errorf("missing Chinese glyph %U", r)
		}
	}
}

func TestChineseWrapPreservesTextAndWidth(t *testing.T) {
	if err := loadFonts(); err != nil {
		t.Fatal(err)
	}
	themeName, fontStyle = "", "serif"
	f := face("ui", 20)
	text := "今天完成汉化测试并核对日历任务"
	lines := wrap(f, text, 85, 12)
	if len(lines) < 2 || strings.Join(lines, "") != text {
		t.Fatalf("lost Chinese text: %#v", lines)
	}
	for _, line := range lines {
		if textW(f, line) > 85 {
			t.Fatalf("line too wide: %q", line)
		}
	}
	clipped := wrap(f, text, 85, 2)
	if len(clipped) != 2 || !strings.HasSuffix(clipped[1], "…") {
		t.Fatalf("last line must signal truncation: %#v", clipped)
	}
}

func TestEnglishWrapRemainsUnchanged(t *testing.T) {
	if err := loadFonts(); err != nil {
		t.Fatal(err)
	}
	f := face("ui", 20)
	lines := wrap(f, "one two three four", 90, 3)
	if strings.Join(lines, "|") != "one two|three four" {
		t.Fatalf("English word wrapping changed: %#v", lines)
	}
}

func TestChineseDateKeepsLocalDay(t *testing.T) {
	tm := time.Date(2026, 11, 16, 0, 30, 0, 0, time.FixedZone("fixture", 8*3600))
	if got := displayDate(tm, "Monday 2 January"); got != "2026年11月16日 周一" {
		t.Fatalf("wrong local date: %s", got)
	}
	if got := displayDate(tm, "Mon"); got != "周一" {
		t.Fatalf("wrong weekday: %s", got)
	}
	if got := displayDate(tm, "15:04"); got != "00:30" {
		t.Fatalf("clock changed: %s", got)
	}
}

func TestChineseFinalLineDoesNotPaintNewlineGlyph(t *testing.T) {
	if err := loadFonts(); err != nil {
		t.Fatal(err)
	}
	f := face("ui", 20)
	lines := wrap(f, "第一行\n第二行\n第三行", 300, 2)
	if len(lines) != 2 || strings.ContainsAny(strings.Join(lines, ""), "\r\n") {
		t.Fatalf("newline glyph leaked into final display line: %#v", lines)
	}
}

func TestAppleMusicMarkKeepsItsAppIdentifier(t *testing.T) {
	got, want := newCanvas(80, 80), newCanvas(80, 80)
	appMark(got, "Music", 40, 40, 18, ink)
	appleMusicMark(want, 40, 40, 18, ink)
	if !bytes.Equal(got.Pix, want.Pix) {
		t.Fatal("Apple Music's wire identifier no longer selects its original mark")
	}
}
