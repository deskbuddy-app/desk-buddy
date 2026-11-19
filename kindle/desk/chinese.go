package main

// Display-only support. The original font owns Latin metrics; Source Han Sans
// supplies glyphs it lacks. Measuring and painting share this font.Face.
import (
	"fmt"
	"image"
	"strings"
	"time"
	"unicode"

	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

type fallbackFace struct {
	font.Face
	chinese font.Face
}

func (f *fallbackFace) selected(r rune) font.Face {
	if _, ok := f.Face.GlyphAdvance(r); ok {
		return f.Face
	}
	return f.chinese
}
func (f *fallbackFace) Glyph(dot fixed.Point26_6, r rune) (image.Rectangle, image.Image, image.Point, fixed.Int26_6, bool) {
	return f.selected(r).Glyph(dot, r)
}
func (f *fallbackFace) GlyphBounds(r rune) (fixed.Rectangle26_6, fixed.Int26_6, bool) {
	return f.selected(r).GlyphBounds(r)
}
func (f *fallbackFace) GlyphAdvance(r rune) (fixed.Int26_6, bool) {
	return f.selected(r).GlyphAdvance(r)
}
func (f *fallbackFace) Kern(a, b rune) fixed.Int26_6 {
	left, right := f.selected(a), f.selected(b)
	if left != right {
		return 0
	}
	return left.Kern(a, b)
}
func (f *fallbackFace) Close() error {
	err := f.Face.Close()
	if other := f.chinese.Close(); err == nil {
		err = other
	}
	return err
}

func hasCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) || (r >= 0x3000 && r <= 0x303f) || (r >= 0xff00 && r <= 0xffef) {
			return true
		}
	}
	return false
}

// Chinese has no inter-word spaces. Only Chinese-containing display text takes
// this path; the upstream English word wrapper is left intact.
func wrapChinese(f font.Face, s string, maxW, maxLines int) []string {
	if maxLines <= 0 {
		return nil
	}
	runes := []rune(s)
	var lines []string
	start := 0
	for start < len(runes) && len(lines) < maxLines {
		if len(lines) == maxLines-1 {
			lines = append(lines, ellipsize(f, strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(string(runes[start:])), "\r", " "), "\n", " "), maxW))
			break
		}
		end := start
		for end < len(runes) && runes[end] != '\n' && textW(f, string(runes[start:end+1])) <= maxW {
			end++
		}
		if end == start && runes[start] != '\n' {
			end++
		}
		lines = append(lines, ellipsize(f, string(runes[start:end]), maxW))
		start = end
		if start < len(runes) && runes[start] == '\n' {
			start++
		}
	}
	return lines
}

func displayDate(t time.Time, layout string) string {
	weekday := [...]string{"周日", "周一", "周二", "周三", "周四", "周五", "周六"}[t.Weekday()]
	switch layout {
	case "Monday 2 January":
		return fmt.Sprintf("%d年%d月%d日 %s", t.Year(), t.Month(), t.Day(), weekday)
	case "Mon 2 Jan":
		return fmt.Sprintf("%d月%d日 %s", t.Month(), t.Day(), weekday)
	case "January 2006":
		return fmt.Sprintf("%d年%d月", t.Year(), t.Month())
	case "2 January":
		return fmt.Sprintf("%d月%d日", t.Month(), t.Day())
	case "Mon":
		return weekday
	case "PM":
		if t.Hour() < 12 {
			return "上午"
		}
		return "下午"
	}
	return t.Format(layout)
}

// The confirmation arrives as an English display description from upstream.
// Only the rendered label changes; the task/profile/action values stay intact.
func displayList(s string) string {
	if s == "the list" {
		return "列表"
	}
	return strings.TrimSuffix(strings.TrimPrefix(s, "the "), " list") + "列表"
}
