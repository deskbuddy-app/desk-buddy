package main

// Server › Your own cloud server (optional): like the Mac app's guide. The setup (setup.js, with
// the server's code, from the cloud-kit folder next to the app) is copied to
// %APPDATA%\DeskBuddy\cloud-setup with the key in a private file `key`, and run in a window
// where you can watch it. Connect checks the address, copies your tasks over (when Desk Buddy
// runs on this PC), leaves a "moved" note so the Kindle follows, and switches.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func nodeInstalled() bool { _, err := exec.LookPath("node"); return err == nil }

func kitDir() string { return filepath.Join(configDir(), "cloud-setup") }

// prepareKit copies the setup and the server's code (and the Kindle's update files) next to a
// private key file, so the setup (or an AI helping) never needs the key typed.
func prepareKit() error {
	src := filepath.Join(exeDir(), "cloud-kit")
	if _, err := os.Stat(src); err != nil {
		return fmt.Errorf("the cloud-kit folder is missing next to Desk Buddy.exe (unzip the whole folder)")
	}
	dst := kitDir()
	for _, name := range []string{"src", "public", "schema.sql", "setup.js", "package.json"} {
		os.RemoveAll(filepath.Join(dst, name))
	}
	if err := copyTree(src, dst); err != nil {
		return err
	}
	if err := copyTree(filepath.Join(exeDir(), "public"), filepath.Join(dst, "public")); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dst, "key"), []byte(current().Key), 0o600)
}

func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		to := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(to, 0o700)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.Create(to)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
}

// runSetup opens a window (Command Prompt or Linux terminal) running `node setup.js` in the kit.
func runSetup() error {
	if err := prepareKit(); err != nil {
		return err
	}
	dir := kitDir()
	if runtime.GOOS != "windows" {
		shCmd := "node setup.js; echo ''; read -p 'Press Enter to close...'"
		for _, term := range []struct {
			bin  string
			args []string
		}{
			{"x-terminal-emulator", []string{"-e", "sh", "-c", shCmd}},
			{"gnome-terminal", []string{"--", "sh", "-c", shCmd}},
			{"konsole", []string{"-e", "sh", "-c", shCmd}},
			{"xfce4-terminal", []string{"-e", "sh -c \"" + shCmd + "\""}},
			{"kitty", []string{"sh", "-c", shCmd}},
			{"alacritty", []string{"-e", "sh", "-c", shCmd}},
			{"xterm", []string{"-e", "sh", "-c", shCmd}},
		} {
			if p, err := exec.LookPath(term.bin); err == nil {
				cmd := exec.Command(p, term.args...)
				cmd.Dir = dir
				if cmd.Start() == nil {
					return nil
				}
			}
		}
		cmd := exec.Command("node", "setup.js")
		cmd.Dir = dir
		return cmd.Start()
	}
	cmd := exec.Command("cmd.exe", "/k", "node setup.js")
	cmd.Dir = dir
	return cmd.Start()
}

func cloudPromptText() string {
	if runtime.GOOS != "windows" {
		return `I'm setting up Desk Buddy (an app that turns a Kindle into a desk display) with its own free cloud server on Cloudflare. Please help me do it on this Linux PC.

The Desk Buddy app has prepared everything in this folder:
` + kitDir() + `
It holds setup.js (the setup script), the server's code, and a private file named "key". Don't open, print or copy the key file: setup.js reads it by itself.

Steps:
1. Check Node.js is installed (node --version). If it isn't, tell me and help me install the LTS version from nodejs.org. Ask before installing anything.
2. Make sure I have a Cloudflare account. If I don't, I'll make a free one at https://dash.cloudflare.com/sign-up
3. In that folder, run: node setup.js
   It logs in to Cloudflare (my browser opens once and I click Allow), makes a database, puts the server online and locks it with my key.
4. When it finishes it prints an address ending in .workers.dev. Give me that address: I'll paste it into Desk Buddy › Server › Your own cloud server and click Connect.

Only work in that folder, and only run setup.js and the commands it needs. If something fails, explain it in plain words and suggest the fix.
If you can't run commands on my PC, walk me through these steps one at a time and tell me exactly what to type into the terminal.`
	}
	return cloudPrompt
}

const cloudPrompt = `I'm setting up Desk Buddy (an app that turns a Kindle into a desk display) with its own free cloud server on Cloudflare. Please help me do it on this Windows PC.

The Desk Buddy app has prepared everything in this folder:
%APPDATA%\DeskBuddy\cloud-setup
It holds setup.js (the setup script), the server's code, and a private file named "key". Don't open, print or copy the key file: setup.js reads it by itself.

Steps:
1. Check Node.js is installed (node --version). If it isn't, tell me and help me install the LTS version from nodejs.org. Ask before installing anything.
2. Make sure I have a Cloudflare account. If I don't, I'll make a free one at https://dash.cloudflare.com/sign-up
3. In that folder, run: node setup.js
   It logs in to Cloudflare (my browser opens once and I click Allow), makes a database, puts the server online and locks it with my key.
4. When it finishes it prints an address ending in .workers.dev. Give me that address: I'll paste it into Desk Buddy › Server › Your own cloud server and click Connect.

Only work in that folder, and only run setup.js and the commands it needs. If something fails, explain it in plain words and suggest the fix.
If you can't run commands on my PC, walk me through these steps one at a time and tell me exactly what to type into Command Prompt.`

// connectCloud switches to the cloud server at url (after checking it has our key).
func connectCloud(url string) error {
	url = normalizeServer(url)
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("paste the https://… address the setup printed")
	}
	c := current()
	req, _ := http.NewRequest("GET", url+"/api/where", nil)
	req.Header.Set("X-Key", c.Key)
	req.Header.Set("User-Agent", userAgent)
	resp, err := quick.Do(req)
	if err != nil {
		return fmt.Errorf("couldn't reach it: check the address, or wait a minute and try again")
	}
	resp.Body.Close()
	if resp.StatusCode == 401 {
		return fmt.Errorf("that server has a different key: run the setup from this app again")
	}
	if resp.StatusCode != 200 {
		return fmt.Errorf("it said %s", resp.Status)
	}
	if c.Mode == "local" {
		// copy everything over, and tell the Kindle where Desk Buddy went
		var all json.RawMessage
		if err := call(quick, "GET", "/api/export", nil, &all); err != nil {
			return fmt.Errorf("couldn't read your tasks here: %v", err)
		}
		imp, _ := http.NewRequest("POST", url+"/api/import", strings.NewReader(string(all)))
		imp.Header.Set("X-Key", c.Key)
		imp.Header.Set("User-Agent", userAgent)
		imp.Header.Set("Content-Type", "application/json")
		r2, err := quick.Do(imp)
		if err != nil || r2.StatusCode != 200 {
			return fmt.Errorf("couldn't copy your tasks to the cloud")
		}
		r2.Body.Close()
		call(quick, "POST", "/api/kindle/move", map[string]string{"server": url, "name": ""}, nil)
		time.AfterFunc(60*time.Second, func() { // keep this PC's server a minute, so the Kindle reads the note
			if current().Mode == "remote" {
				stopServer()
			}
		})
	}
	c.Mode, c.Server = "remote", url
	if err := saveConfig(c); err != nil {
		return err
	}
	os.Remove(filepath.Join(kitDir(), "key")) // done with it
	setStatus("Connected to your cloud server")
	return nil
}

// uninstall: no more starting with Windows, Desk Buddy's server stopped, its settings removed;
// the window then says to delete the folder. The tasks stay in %APPDATA%\DeskBuddy\server.
func uninstall() {
	setStartWithWindows(false)
	stopServer()
	os.Remove(filepath.Join(configDir(), "config.json"))
	os.Remove(filepath.Join(configDir(), "running.url"))
	os.RemoveAll(kitDir())
	shellOpen(exeDir()) // the folder to delete
	go func() { time.Sleep(time.Second); quitApp() }()
}

// switchToLocal moves Desk Buddy from the cloud (or another computer) to this PC: same key,
// everything copied down, a "moved" note left there so the Kindle follows.
func switchToLocal() error {
	c := current()
	if c.Mode == "local" {
		return nil
	}
	var all json.RawMessage
	if err := call(quick, "GET", "/api/export", nil, &all); err != nil {
		return fmt.Errorf("couldn't read your tasks there: %v", err)
	}
	from := c.Server
	c.Mode, c.Server = "local", "http://127.0.0.1:8787"
	if err := saveConfig(c); err != nil {
		return err
	}
	srvMu.Lock()
	stop = false
	srvMu.Unlock()
	up := false
	for i := 0; i < 40 && !up; i++ { // keepServer starts it within seconds
		time.Sleep(500 * time.Millisecond)
		up = serverAnswers()
	}
	if !up {
		return fmt.Errorf("Desk Buddy's server didn't start on this PC")
	}
	if err := call(quick, "POST", "/api/import", all, nil); err != nil {
		return fmt.Errorf("couldn't copy your tasks here: %v", err)
	}
	// the note on the old server, and none on this one
	call(quick, "POST", "/api/kindle/move", map[string]string{"server": ""}, nil)
	req, _ := http.NewRequest("POST", from+"/api/kindle/move", strings.NewReader(`{"server":"`+lanAddress()+`","name":"`+pcName()+`"}`))
	req.Header.Set("X-Key", c.Key)
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json")
	if r, err := quick.Do(req); err == nil {
		r.Body.Close()
	}
	setStatus("Desk Buddy runs on this PC")
	return nil
}
