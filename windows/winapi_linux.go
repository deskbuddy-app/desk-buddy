//go:build linux

package main

// Linux desktop integration: idle time over D-Bus (GNOME Mutter IdleMonitor / freedesktop
// ScreenSaver), dark mode via XDG Desktop Portal / gsettings, notification sound, opening URLs
// and the app window, single-instance file lock, XDG autostart, and USB Kindle drive ejection.

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/godbus/dbus/v5"
)

const serverExe = "desk-buddy-server"

func hideWindow(*exec.Cmd) {}

var lockFile *os.File

// onlyOne: false when Desk Buddy is already running on this user account.
func onlyOne() bool {
	os.MkdirAll(configDir(), 0o700)
	f, err := os.OpenFile(filepath.Join(configDir(), "desk-buddy.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return true
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return false
	}
	lockFile = f
	return true
}

// idleSeconds: time since the last keyboard or mouse input, via session D-Bus.
func idleSeconds() float64 {
	conn, err := dbus.SessionBus()
	if err != nil {
		return 0
	}
	// 1. GNOME / Mutter IdleMonitor (returns uint64 milliseconds)
	var ms uint64
	if err := conn.Object("org.gnome.Mutter.IdleMonitor", "/org/gnome/Mutter/IdleMonitor/Core").
		Call("org.gnome.Mutter.IdleMonitor.GetIdletime", 0).Store(&ms); err == nil {
		return float64(ms) / 1000.0
	}
	// 2. freedesktop / KDE ScreenSaver (returns uint32 milliseconds)
	for _, path := range []dbus.ObjectPath{"/org/freedesktop/ScreenSaver", "/ScreenSaver"} {
		var ms32 uint32
		if err := conn.Object("org.freedesktop.ScreenSaver", path).
			Call("org.freedesktop.ScreenSaver.GetSessionIdleTime", 0).Store(&ms32); err == nil {
			return float64(ms32) / 1000.0
		}
	}
	return 0
}

// darkMode: Linux desktop dark colour-scheme preference via XDG Desktop Portal or gsettings.
func darkMode() (bool, bool) {
	if conn, err := dbus.SessionBus(); err == nil {
		var outer dbus.Variant
		err := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop").
			Call("org.freedesktop.portal.Settings.Read", 0, "org.freedesktop.appearance", "color-scheme").
			Store(&outer)
		if err == nil {
			val := outer.Value()
			if v, ok := val.(dbus.Variant); ok {
				val = v.Value()
			}
			if u, ok := val.(uint32); ok && (u == 1 || u == 2) {
				// 1 = prefer dark, 2 = prefer light
				return u == 1, true
			}
		}
	}
	if out, err := exec.Command("gsettings", "get", "org.gnome.desktop.interface", "color-scheme").Output(); err == nil {
		s := strings.ToLower(strings.TrimSpace(string(out)))
		if strings.Contains(s, "prefer-dark") {
			return true, true
		}
		if strings.Contains(s, "default") || strings.Contains(s, "prefer-light") {
			return false, true
		}
	}
	return false, false
}

// chimeSound plays the desktop's notification chime when a focus or break ends.
func chimeSound() {
	go func() {
		if p, err := exec.LookPath("canberra-gtk-play"); err == nil {
			if exec.Command(p, "-i", "complete").Run() == nil {
				return
			}
		}
		for _, snd := range []string{
			"/usr/share/sounds/freedesktop/stereo/complete.oga",
			"/usr/share/sounds/freedesktop/stereo/message.oga",
			"/usr/share/sounds/ubuntu/stereo/message.ogg",
		} {
			if _, err := os.Stat(snd); err == nil {
				for _, player := range []string{"pw-play", "paplay", "ogg123"} {
					if p, err := exec.LookPath(player); err == nil {
						_ = exec.Command(p, snd).Run()
						return
					}
				}
			}
		}
	}()
}

// shellOpen opens a file, directory, or URL with the user's default handler.
func shellOpen(target string) error {
	return exec.Command("xdg-open", target).Start()
}

// openWindow opens Desk Buddy's UI (ui.html) in a dedicated app window when a Chromium-based
// browser is available, or in the default browser via xdg-open.
func openWindow() {
	u := uiURL()
	if u == "" {
		return
	}
	for _, bin := range []string{"chromium", "chromium-browser", "google-chrome", "google-chrome-stable", "brave-browser", "microsoft-edge"} {
		if p, err := exec.LookPath(bin); err == nil {
			if exec.Command(p, "--app="+u).Start() == nil {
				return
			}
		}
	}
	_ = shellOpen(u)
}

func autostartPath() string {
	d, err := os.UserConfigDir()
	if err != nil || d == "" {
		home, _ := os.UserHomeDir()
		d = filepath.Join(home, ".config")
	}
	return filepath.Join(d, "autostart", "desk-buddy.desktop")
}

func startsWithWindows() bool {
	_, err := os.Stat(autostartPath())
	return err == nil
}

func setStartWithWindows(on bool) error {
	p := autostartPath()
	if !on {
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	desktop := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=Desk Buddy
Comment=Kindle desk display companion
Exec="%s"
Terminal=false
Categories=Utility;
StartupNotify=false
X-GNOME-Autostart-enabled=true
`, exe)
	return os.WriteFile(p, []byte(desktop), 0o644)
}

// ejectDrive flushes and unmounts a mounted USB Kindle volume on Linux.
func ejectDrive(d string) bool {
	syscall.Sync()
	if p, err := exec.LookPath("gio"); err == nil {
		if exec.Command(p, "mount", "-u", d).Run() == nil {
			return true
		}
	}
	if p, err := exec.LookPath("udisksctl"); err == nil {
		if out, err := exec.Command("findmnt", "-n", "-o", "SOURCE", "--target", d).Output(); err == nil {
			if dev := strings.TrimSpace(string(out)); dev != "" {
				if exec.Command(p, "unmount", "-b", dev, "--no-user-interaction").Run() == nil {
					_ = exec.Command(p, "power-off", "-b", dev, "--no-user-interaction").Run()
					return true
				}
			}
		}
	}
	if p, err := exec.LookPath("umount"); err == nil {
		if exec.Command(p, d).Run() == nil {
			return true
		}
	}
	return false
}
