package main

// Putting Desk Buddy on a Kindle over USB (Kindle page): plug in a jailbroken Kindle, click
// Put Desk Buddy on it; it copies the desk folder and the library item, already set up for
// this PC's Desk Buddy server (server + key, so the Kindle connects without Find it / Allow),
// reads every file back to check it landed, and ejects the Kindle. The manual way: Save the
// files writes the same thing as a zip in Downloads, with the steps inside. As the Mac's
// KindleSetup.swift. The Kindle files: public/kindle/desk-<sha>.bin and run-<sha>.sh next to
// the exe (the current release) and kindle-launcher.sh (a copy of kindle-files/launcher.sh,
// refreshed by build.sh).

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

//go:embed kindle-launcher.sh
var kindleLauncher []byte

type kSetup struct {
	State string `json:"state"` // waiting, found, copying, checking, done, failed
	Drive string `json:"drive"`
	Has   bool   `json:"has"` // Desk Buddy is on it already
	Note  string `json:"note"`
}

var (
	ksMu      sync.Mutex
	ks        = kSetup{State: "waiting"}
	ksRoots   = kindleRoots // tests point these elsewhere
	kindlePub = func() string { return filepath.Join(exeDir(), "public", "kindle") }
)

// kindleRoots: where drives are mounted (D: … Z: on Windows, /run/media/*/*, /media/*, /mnt/* on Linux, /Volumes/* on macOS).
func kindleRoots() []string {
	if runtime.GOOS == "windows" {
		var r []string
		for c := 'D'; c <= 'Z'; c++ {
			r = append(r, string(c)+`:\`)
		}
		return r
	}
	var out []string
	for _, pat := range []string{"/Volumes/*", "/run/media/*/*", "/media/*/*", "/media/*", "/mnt/*"} {
		if m, err := filepath.Glob(pat); err == nil {
			out = append(out, m...)
		}
	}
	return out
}

// findKindle: the first drive with the Kindle's own documents and system folders.
func findKindle() string {
	for _, r := range ksRoots() {
		if st, err := os.Stat(filepath.Join(r, "documents")); err == nil && st.IsDir() {
			if _, err := os.Stat(filepath.Join(r, "system")); err == nil {
				return r
			}
		}
	}
	return ""
}

// kindleSetupState looks for a Kindle (unless it's busy) and says where things are.
func kindleSetupState() kSetup {
	ksMu.Lock()
	defer ksMu.Unlock()
	if ks.State == "copying" || ks.State == "checking" {
		return ks
	}
	if d := findKindle(); d != "" {
		if ks.State != "done" || ks.Drive != d {
			_, err := os.Stat(filepath.Join(d, "desk", "desk"))
			ks = kSetup{State: "found", Drive: d, Has: err == nil}
		}
	} else if ks.State != "done" {
		ks = kSetup{State: "waiting"}
	}
	return ks
}

func setKS(state, note string) {
	ksMu.Lock()
	ks.State, ks.Note = state, note
	ksMu.Unlock()
}

type kfile struct {
	path string // on the Kindle, with /
	data []byte
}

// kindleFiles: what goes onto the Kindle, set up for this PC's server. drive (may be "")
// keeps which way a Kindle that had Desk Buddy before stands.
func kindleFiles(drive string) ([]kfile, error) {
	pub := kindlePub()
	bins, _ := filepath.Glob(filepath.Join(pub, "desk-*.bin"))
	runs, _ := filepath.Glob(filepath.Join(pub, "run-*.sh"))
	if len(bins) == 0 || len(runs) == 0 {
		return nil, fmt.Errorf("this copy of Desk Buddy doesn't have the Kindle files in it")
	}
	bin, err := os.ReadFile(bins[0])
	if err != nil {
		return nil, err
	}
	run, err := os.ReadFile(runs[0])
	if err != nil {
		return nil, err
	}
	c := current()
	server := c.Server
	if local() {
		server = lanAddress()
	}
	cfg := map[string]any{"server": server, "key": c.Key, "rotation": 90, "full_refresh_every": 40}
	if drive != "" {
		var old map[string]any
		if b, err := os.ReadFile(filepath.Join(drive, "desk", "config.json")); err == nil && json.Unmarshal(b, &old) == nil {
			for _, k := range []string{"rotation", "full_refresh_every"} {
				if v, ok := old[k]; ok {
					cfg[k] = v
				}
			}
		}
	}
	cj, _ := json.MarshalIndent(cfg, "", "  ")
	// the library item; on a Kindle that had Desk Buddy before, with its cover line (as the
	// Kindle app writes it, buddy.go ensureLibraryItem), so the book keeps its cover
	launcher := kindleLauncher
	if drive != "" {
		if _, err := os.Stat(filepath.Join(drive, "desk", "cover.png")); err == nil {
			launcher = bytes.Replace(kindleLauncher, []byte("# DontUseFBInk"), []byte("# Icon: /mnt/us/desk/cover.png\n# DontUseFBInk"), 1)
		}
	}
	return []kfile{{"desk/desk", bin}, {"desk/run.sh", run}, {"desk/config.json", cj}, {"documents/Desk Buddy.sh", launcher}}, nil
}

func writeSynced(path string, data []byte) error {
	os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil { // onto the Kindle, not just the PC's cache
		f.Close()
		return err
	}
	return f.Close()
}

// installKindle copies, reads back (and copies again what didn't land), then ejects.
func installKindle() {
	ksMu.Lock()
	d := ks.Drive
	if d == "" || ks.State == "copying" || ks.State == "checking" {
		ksMu.Unlock()
		return
	}
	ks.State, ks.Note = "copying", ""
	ksMu.Unlock()
	fs, err := kindleFiles(d)
	if err != nil {
		setKS("failed", "Couldn't copy onto the Kindle: "+err.Error()+".")
		return
	}
	bad := fs
	for attempt := 0; attempt < 2 && len(bad) > 0; attempt++ {
		setKS("copying", "")
		for _, f := range bad {
			if err := writeSynced(filepath.Join(d, filepath.FromSlash(f.path)), f.data); err != nil {
				setKS("failed", "Couldn't copy onto the Kindle: "+err.Error()+". Unplug it, plug it in again and try again.")
				return
			}
		}
		setKS("checking", "")
		var still []kfile
		for _, f := range bad {
			got, err := os.ReadFile(filepath.Join(d, filepath.FromSlash(f.path)))
			if err != nil || sha256.Sum256(got) != sha256.Sum256(f.data) {
				still = append(still, f)
			}
		}
		bad = still
	}
	if len(bad) > 0 {
		var names []string
		for _, f := range bad {
			names = append(names, f.path)
		}
		setKS("failed", "Some files didn't stick on the Kindle ("+strings.Join(names, ", ")+"). Unplug it, plug it straight into this PC (not a hub), wait 20 seconds, and try again.")
		return
	}
	note := ""
	if !ejectDrive(d) {
		note = "Eject the Kindle before unplugging it (the USB icon in the taskbar › Eject Kindle)."
	}
	setKS("done", note)
}

const kindleSteps = `Putting Desk Buddy on your Kindle by hand

Your Kindle needs to be jailbroken first, with the hotfix (kindlemodding.org).

1. Connect the Kindle to your computer with USB. It shows up as a drive called Kindle.
2. Copy the "desk" folder to the top level of the Kindle drive (next to "documents").
3. Copy "Desk Buddy.sh" from the "documents" folder here into the Kindle's "documents" folder.
4. Eject the Kindle before unplugging it.
5. On the Kindle, open Desk Buddy from the library. It connects to your computer by itself.

If files go missing: plug the Kindle straight into the computer (not a hub), wait 20 seconds
before copying, and check the files are there after plugging it back in.

desk/config.json holds your Desk Buddy key: don't share these files.
`

// saveKindleFiles writes "Desk Buddy for Kindle.zip" to Downloads and shows it.
func saveKindleFiles() (string, error) {
	fs, err := kindleFiles("")
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, f := range append(fs, kfile{"How to install.txt", []byte(strings.ReplaceAll(kindleSteps, "\n", "\r\n"))}) {
		w, err := z.Create("Desk Buddy for Kindle/" + f.path)
		if err != nil {
			return "", err
		}
		w.Write(f.data)
	}
	if err := z.Close(); err != nil {
		return "", err
	}
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, "Downloads")
	if _, err := os.Stat(dir); err != nil {
		dir = home
	}
	out := filepath.Join(dir, "Desk Buddy for Kindle.zip")
	if err := os.WriteFile(out, buf.Bytes(), 0o600); err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("explorer", "/select,"+out)
		cmd.Start()
	}
	return out, nil
}
