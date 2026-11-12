package main

// Updating itself: the Desk Buddy server this PC is connected to (your Mac, another computer, or
// your own cloud) offers the newest Windows app at /windows/release.json (windows/release.sh puts
// it there). When it's newer, the window shows "Update Desk Buddy"; clicking it downloads the
// new Desk Buddy.exe, checks it, swaps it in (Windows lets a running program be renamed) and
// starts it. It also asks GitHub for the newest public release (desk-buddy-latest.json, a file in
// every release since 1.2) and takes whichever is newer; a GitHub update is the whole zip, so the
// server and the Kindle's files are updated too.

import (
	"archive/zip"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type release struct {
	Version    string      `json:"version"`
	SHA256     string      `json:"sha256"`
	Size       int64       `json:"size"`
	File       string      `json:"file"`       // a path on the Desk Buddy server (Desk Buddy.exe only)
	URL        string      `json:"url"`        // or a GitHub download (the whole zip)
	Notes      []string    `json:"notes"`      // the newest version's items (what 0.4 shows)
	PatchNotes []noteEntry `json:"patchnotes"` // the newest few versions (release.sh, from patchnotes.json)
}

// noteEntry: one version's patch notes (patchnotes.json, newest first; the first is this version).
type noteEntry struct {
	Version string   `json:"version"`
	Date    string   `json:"date"`
	Items   []string `json:"items"`
}

//go:embed patchnotes.json
var patchNotesJSON []byte

// patchNotes: what came in each version, newest first.
func patchNotes() []noteEntry {
	var n []noteEntry
	json.Unmarshal(patchNotesJSON, &n)
	return n
}

var (
	updMu      sync.Mutex
	updReady   *release
	updNote    string
	updBusy    bool
	updChecked time.Time // the last check that got an answer
)

// newer: "0.10" > "0.9" > "0.3".
func newer(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x > y
		}
	}
	return false
}

// latestURL: a small file in every GitHub release since 1.2 saying what's in it
// ({"windows": release, "mac": …}); /releases/latest/ always points at the newest full release.
var latestURL = "https://github.com/deskbuddy-app/desk-buddy/releases/latest/download/desk-buddy-latest.json"

func fromGitHub() (*release, error) { return fromURL(latestURL) }

// tagURL: the same file in an older release (for "Go back"; releases have it since 1.2).
func tagURL(version string) string {
	return "https://github.com/deskbuddy-app/desk-buddy/releases/download/v" + version + "/desk-buddy-latest.json"
}

func fromURL(u string) (*release, error) {
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", userAgent)
	resp, err := quick.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("github: %s", resp.Status)
	}
	var m struct {
		Windows release `json:"windows"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&m); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(m.Windows.URL, "https://github.com/deskbuddy-app/") {
		return nil, fmt.Errorf("github: no windows download")
	}
	return &m.Windows, nil
}

// selfUpdate: the releases only carry a Windows download, so the Linux build doesn't update
// itself (it would install the Windows files).
var selfUpdate = runtime.GOOS != "linux"

// checkUpdate asks GitHub and the Desk Buddy server this PC is connected to, and keeps the newest.
func checkUpdate() error {
	if !selfUpdate {
		return fmt.Errorf("no self-update on %s", runtime.GOOS)
	}
	var best *release
	answered := false
	if r, err := fromGitHub(); err == nil {
		answered = true
		best = r
	}
	if connected() {
		var r release
		if call(quick, "GET", "/windows/release.json", nil, &r) == nil {
			answered = true
			if r.File != "" && (best == nil || newer(r.Version, best.Version)) {
				r.URL = ""
				best = &r
			}
		}
	}
	if !answered {
		return fmt.Errorf("couldn't reach GitHub or the server")
	}
	updMu.Lock()
	defer updMu.Unlock()
	updChecked = time.Now()
	if best != nil && best.Version != "" && best.SHA256 != "" && newer(best.Version, version) {
		updReady = best
	} else {
		updReady = nil
	}
	return nil
}

// updates looks a minute after starting and then every 6 hours.
func updates() {
	time.Sleep(time.Minute)
	for {
		checkUpdate()
		time.Sleep(6 * time.Hour)
	}
}

func updateState() (*release, string, bool) {
	updMu.Lock()
	defer updMu.Unlock()
	return updReady, updNote, updBusy
}

// lastChecked: when the last check got an answer (zero: not yet).
func lastChecked() time.Time {
	updMu.Lock()
	defer updMu.Unlock()
	return updChecked
}

func setUpdNote(s string, busy bool) {
	updMu.Lock()
	updNote, updBusy = s, busy
	updMu.Unlock()
}

// applyUpdate downloads and checks the new version, swaps it in, then restarts.
func applyUpdate() {
	r, _, busy := updateState()
	if r == nil || busy || !selfUpdate {
		return
	}
	setUpdNote("Downloading…", true)
	installRelease(r)
}

// goBack installs the version before this one again (About › Updates › Go back), from its
// GitHub release. Settings and tasks stay; the newer version is offered again as usual.
func goBack() {
	notes := patchNotes()
	if _, _, busy := updateState(); busy || len(notes) < 2 || !selfUpdate {
		return
	}
	v := notes[1].Version
	setUpdNote("Finding version "+v+"…", true)
	r, err := fromURL(tagURL(v))
	if err != nil || r.Version != v || r.SHA256 == "" {
		setUpdNote("Couldn't find version "+v+" on GitHub. Download it from github.com/deskbuddy-app/desk-buddy/releases instead.", false)
		return
	}
	setUpdNote("Downloading version "+v+"…", true)
	installRelease(r)
}

// installRelease downloads and checks a version, swaps it in, then restarts.
func installRelease(r *release) {
	fail := func(msg string) { setUpdNote(msg, false); report("update: " + msg) }
	var req *http.Request
	if r.URL != "" {
		req, _ = http.NewRequest("GET", r.URL, nil) // GitHub: no key
	} else {
		c := current()
		req, _ = http.NewRequest("GET", strings.TrimRight(c.Server, "/")+r.File, nil)
		req.Header.Set("X-Key", c.Key)
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := (&http.Client{Timeout: 10 * time.Minute}).Do(req)
	if err != nil || resp.StatusCode != 200 {
		fail("Couldn't download it. Try again in a moment.")
		return
	}
	defer resp.Body.Close()
	dir := filepath.Join(configDir(), "update")
	os.RemoveAll(dir)
	os.MkdirAll(dir, 0o700)
	tmp := filepath.Join(dir, "download.part")
	f, err := os.Create(tmp)
	if err != nil {
		fail("Couldn't save it: " + err.Error())
		return
	}
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), resp.Body)
	f.Close()
	if err != nil || (r.Size > 0 && n != r.Size) || hex.EncodeToString(h.Sum(nil)) != r.SHA256 {
		os.Remove(tmp)
		fail("The download didn't check out. Try again.")
		return
	}
	setUpdNote("Installing…", true)
	exe, _ := os.Executable()
	if r.URL != "" {
		err = swapFolder(tmp, filepath.Join(dir, "new"), filepath.Dir(exe))
	} else {
		err = swapFiles(map[string]string{exe: tmp})
	}
	if err != nil {
		fail("Couldn't replace Desk Buddy: " + err.Error())
		return
	}
	os.RemoveAll(dir)
	setUpdNote("Restarting…", true)
	if err := exec.Command(exe, "--after-update").Start(); err != nil {
		fail("Updated, but couldn't restart: open Desk Buddy again")
		return
	}
	quitApp()
}

// swapFolder unpacks the release zip (one folder, "Desk Buddy for Windows") into staging, then
// puts each of its files in place of the ones next to Desk Buddy.exe.
func swapFolder(zipPath, staging, into string) error {
	z, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer z.Close()
	files := map[string]string{} // where it goes → where it was unpacked
	for _, f := range z.File {
		_, rel, ok := strings.Cut(f.Name, "/") // drop the top folder
		if !ok || rel == "" || strings.HasSuffix(f.Name, "/") {
			continue
		}
		rel = filepath.FromSlash(rel)
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("unexpected file in the download: %s", f.Name)
		}
		src := filepath.Join(staging, rel)
		os.MkdirAll(filepath.Dir(src), 0o700)
		in, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.Create(src)
		if err == nil {
			_, err = io.Copy(out, in)
			if cerr := out.Close(); err == nil {
				err = cerr
			}
		}
		in.Close()
		if err != nil {
			return err
		}
		files[filepath.Join(into, rel)] = src
	}
	if _, ok := files[filepath.Join(into, "Desk Buddy.exe")]; !ok {
		return fmt.Errorf("the download didn't have Desk Buddy in it")
	}
	return swapFiles(files)
}

// swapFiles puts each new file in place. Programs (.exe) may be running: Windows lets them be
// renamed, not overwritten, so the old one becomes .old.exe (cleanAfterUpdate removes it).
// If anything fails, the programs already moved aside are put back.
func swapFiles(files map[string]string) error {
	var moved []string
	undo := func() {
		for _, to := range moved {
			os.Remove(to)
			os.Rename(oldName(to), to)
		}
	}
	for to, from := range files {
		os.MkdirAll(filepath.Dir(to), 0o755)
		if strings.EqualFold(filepath.Ext(to), ".exe") {
			if _, err := os.Stat(to); err == nil {
				os.Remove(oldName(to))
				if err := os.Rename(to, oldName(to)); err != nil {
					undo()
					return err
				}
				moved = append(moved, to)
			}
		}
		if err := copyFile(from, to); err != nil {
			undo()
			return err
		}
	}
	return nil
}

func oldName(exe string) string { return strings.TrimSuffix(exe, filepath.Ext(exe)) + ".old.exe" }

func copyFile(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// cleanAfterUpdate removes the previous version's programs once they have stopped.
func cleanAfterUpdate() {
	for i := 0; i < 30; i++ {
		old, _ := filepath.Glob(filepath.Join(exeDir(), "*.old.exe"))
		left := 0
		for _, f := range old {
			if os.Remove(f) != nil {
				left++
			}
		}
		if left == 0 {
			return
		}
		time.Sleep(2 * time.Second)
	}
}
