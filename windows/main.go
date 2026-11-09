// Desk Buddy for Windows: connects this PC to Desk Buddy. A tray icon (bottom right) with a
// small menu; the window is a page in your browser. It tells the Kindle when you're on this PC
// (so it shows the Windows list), opens apps from the Kindle's Deck, and chimes when a focus or
// break ends. Built on the Mac: GOOS=windows GOARCH=amd64 (see build.sh).
package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"
)

const version = "1.2"

//go:embed icon.ico
var iconICO []byte

var (
	stateMu  sync.Mutex
	status   = "Starting…"
	lastDeck string
	mStatus  *systray.MenuItem
)

func setStatus(s string) {
	if s == "" {
		return
	}
	stateMu.Lock()
	status = s
	stateMu.Unlock()
	if mStatus != nil {
		mStatus.SetTitle(s)
	}
}

func getStatus() (string, string) {
	stateMu.Lock()
	defer stateMu.Unlock()
	return status, lastDeck
}

func setLast(label, err string) {
	stateMu.Lock()
	if err == "" {
		lastDeck = label + " · opened " + time.Now().Format("15:04")
	} else {
		lastDeck = label + " · " + err
	}
	stateMu.Unlock()
	if err != "" {
		report("deck: " + label + ": " + err)
	}
}

func logf(format string, a ...any) { log.Printf(format, a...) }

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--after-update" {
		// just updated: the old copy is still closing, so wait for it (up to 15 s)
		for i := 0; i < 30 && !onlyOne(); i++ {
			time.Sleep(500 * time.Millisecond)
		}
		go cleanAfterUpdate()
	} else if !onlyOne() {
		// already running: ask it to show its window
		if b, err := os.ReadFile(filepath.Join(configDir(), "running.url")); err == nil {
			if resp, err := quick.Get(strings.Replace(string(b), "/?t=", "/show?t=", 1)); err == nil {
				resp.Body.Close()
			}
		}
		return
	}
	os.MkdirAll(configDir(), 0o700)
	logPath := filepath.Join(configDir(), "desk-buddy.log")
	if st, err := os.Stat(logPath); err == nil && st.Size() > 500_000 {
		os.Remove(logPath)
	}
	if f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
		log.SetOutput(f)
	}
	defer func() {
		if r := recover(); r != nil {
			report(fmt.Sprintf("crashed: %v", r))
			time.Sleep(2 * time.Second)
		}
	}()
	loadConfig()
	startPage()
	go measure()
	go updates()
	go keepServer()
	go watchPairing()
	go heartbeats()
	go deck()
	go appList()
	go watchMusic()
	systray.Run(onReady, stopServer)
}

func onReady() {
	systray.SetIcon(iconICO)
	systray.SetTitle("Desk Buddy")
	systray.SetTooltip("Desk Buddy")
	mStatus = systray.AddMenuItem(status, "")
	mStatus.Disable()
	systray.AddSeparator()
	mOpen := systray.AddMenuItem("Open Desk Buddy…", "Your tasks, the Kindle's settings, and more")
	mPause := systray.AddMenuItemCheckbox("Pause", "Stop telling the Kindle about this PC", current().Paused)
	loginLabel := "Start with Windows"
	if runtime.GOOS == "linux" {
		loginLabel = "Start at login"
	}
	mLogin := systray.AddMenuItemCheckbox(loginLabel, "", startsWithWindows())
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit Desk Buddy", "Stops Desk Buddy on this PC (and its server, if it runs here)")
	if !connected() {
		setStatus("Not set up yet")
	}
	openWindow() // like opening an app: show the window (it also says how to set up)
	for {
		select {
		case <-mOpen.ClickedCh:
			openWindow()
		case <-mPause.ClickedCh:
			setPaused(!current().Paused)
			if current().Paused {
				mPause.Check()
			} else {
				mPause.Uncheck()
			}
		case <-mLogin.ClickedCh:
			on := !startsWithWindows()
			if err := setStartWithWindows(on); err != nil {
				report("start with Windows: " + err.Error())
			}
			if startsWithWindows() {
				mLogin.Check()
			} else {
				mLogin.Uncheck()
			}
		case <-mQuit.ClickedCh:
			quitApp()
			return
		}
	}
}

func quitApp() {
	stopServer()
	systray.Quit()
}

func setPaused(p bool) {
	c := current()
	c.Paused = p
	saveConfig(c)
	if p {
		setStatus("Paused · the Kindle won't follow this PC")
	} else {
		setStatus("Running")
	}
}
