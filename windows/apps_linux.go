//go:build linux

package main

// Linux desktop applications for the Kindle's Deck: scans XDG .desktop files across system,
// user, Flatpak, and Snap directories, extracts and scales their PNG icons for e-ink, and
// launches selected apps or URI schemes.

import (
	"bufio"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func desktopDirs() []string {
	home, _ := os.UserHomeDir()
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" && home != "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	dirs := []string{}
	if dataHome != "" {
		dirs = append(dirs,
			filepath.Join(dataHome, "applications"),
			filepath.Join(dataHome, "flatpak", "exports", "share", "applications"),
		)
	}
	dataDirs := os.Getenv("XDG_DATA_DIRS")
	if dataDirs == "" {
		dataDirs = "/usr/local/share:/usr/share"
	}
	for _, d := range strings.Split(dataDirs, ":") {
		if d = strings.TrimSpace(d); d != "" {
			dirs = append(dirs, filepath.Join(d, "applications"))
		}
	}
	dirs = append(dirs,
		"/var/lib/flatpak/exports/share/applications",
		"/var/lib/snapd/desktop/applications",
	)
	return dirs
}

type desktopEntry struct {
	name      string
	exec      string
	icon      string
	noDisplay bool
	hidden    bool
	isApp     bool
}

func parseDesktopFile(path string) (desktopEntry, bool) {
	f, err := os.Open(path)
	if err != nil {
		return desktopEntry{}, false
	}
	defer f.Close()

	var e desktopEntry
	inMain := false
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inMain = line == "[Desktop Entry]"
			continue
		}
		if !inMain {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "Type":
			e.isApp = v == "Application"
		case "Name":
			if e.name == "" {
				e.name = v
			}
		case "Exec":
			if e.exec == "" {
				e.exec = v
			}
		case "Icon":
			if e.icon == "" {
				e.icon = v
			}
		case "NoDisplay":
			e.noDisplay = strings.EqualFold(v, "true")
		case "Hidden":
			e.hidden = strings.EqualFold(v, "true")
		}
	}
	if !e.isApp || e.noDisplay || e.hidden || e.name == "" || e.exec == "" {
		return desktopEntry{}, false
	}
	return e, true
}

// listApps scans XDG .desktop files and returns name → .desktop file path.
func listApps() (map[string]string, error) {
	out := map[string]string{}
	for _, dir := range desktopDirs() {
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(strings.ToLower(d.Name()), ".desktop") {
				return nil
			}
			e, ok := parseDesktopFile(p)
			if !ok {
				return nil
			}
			name := strings.TrimSpace(e.name)
			if _, dup := out[name]; !dup && keepApp(name, p) {
				out[name] = p
			}
			return nil
		})
	}
	return out, nil
}

func findIconFile(icon string) string {
	if icon == "" {
		return ""
	}
	if filepath.IsAbs(icon) {
		if _, err := os.Stat(icon); err == nil {
			return icon
		}
		return ""
	}
	name := strings.TrimSuffix(icon, ".png")
	home, _ := os.UserHomeDir()
	var bases []string
	if home != "" {
		bases = append(bases,
			filepath.Join(home, ".local", "share", "icons"),
			filepath.Join(home, ".local", "share", "flatpak", "exports", "share", "icons"),
		)
	}
	bases = append(bases,
		"/var/lib/flatpak/exports/share/icons",
		"/usr/local/share/icons",
		"/usr/share/icons",
	)
	sizes := []string{"128x128", "96x96", "64x64", "256x256", "48x48", "512x512"}
	themes := []string{"hicolor", "Adwaita", "breeze", "Papirus", "Yaru", "gnome"}
	for _, base := range bases {
		for _, th := range themes {
			for _, sz := range sizes {
				for _, cat := range []string{"apps", "devices", "places"} {
					p := filepath.Join(base, th, sz, cat, name+".png")
					if _, err := os.Stat(p); err == nil {
						return p
					}
				}
			}
		}
	}
	for _, pixDir := range []string{"/usr/share/pixmaps", "/usr/local/share/pixmaps"} {
		p := filepath.Join(pixDir, name+".png")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// appIcon loads the PNG icon for a .desktop file path and converts it to a 96×96 e-ink PNG.
func appIcon(target string) []byte {
	e, ok := parseDesktopFile(target)
	if !ok || e.icon == "" {
		return nil
	}
	iconPath := findIconFile(e.icon)
	if iconPath == "" {
		return nil
	}
	f, err := os.Open(iconPath)
	if err != nil {
		return nil
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	return iconPNG(img)
}

// splitDesktopExec splits a Desktop Entry Exec= line into command arguments and removes %f/%u/etc.
func splitDesktopExec(raw string) []string {
	var args []string
	var cur strings.Builder
	inSingle, inDouble, escaped := false, false, false
	flush := func() {
		if cur.Len() > 0 {
			s := cur.String()
			cur.Reset()
			if len(s) == 2 && s[0] == '%' && strings.ContainsRune("fFuUdDnNickvm", rune(s[1])) {
				return
			}
			args = append(args, s)
		}
	}
	for _, r := range raw {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\' && !inSingle:
			escaped = true
		case r == '\'' && !inDouble:
			inSingle = !inSingle
		case r == '"' && !inSingle:
			inDouble = !inDouble
		case (r == ' ' || r == '\t') && !inSingle && !inDouble:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return args
}

// launch starts an app from the Deck (either a .desktop path or an allowed URI scheme).
func launch(target string) error {
	if strings.HasSuffix(strings.ToLower(target), ".desktop") {
		if p, err := exec.LookPath("gtk-launch"); err == nil {
			id := strings.TrimSuffix(filepath.Base(target), ".desktop")
			if exec.Command(p, id).Start() == nil {
				return nil
			}
		}
		if e, ok := parseDesktopFile(target); ok {
			if args := splitDesktopExec(e.exec); len(args) > 0 {
				return exec.Command(args[0], args[1:]...).Start()
			}
		}
	}
	return shellOpen(target)
}
