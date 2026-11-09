package main

// The window's interface (ui.html) and the small local API it uses: /ui/info (how Desk Buddy is
// set up here), /ui/do (actions: set up, join, Allow, pause, start with Windows, a code for
// another computer, the AI instructions) and /ui/api/… (passed to the Desk Buddy server with the
// key added here, so the key never reaches the page). All need the window's token.

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/skip2/go-qrcode"
)

//go:embed ui.html
var uiHTML string

// the same font as the Mac app and the Kindle (SIL Open Font License)
//
//go:embed AtkinsonHyperlegible-Regular.ttf
var fontRegular []byte

//go:embed AtkinsonHyperlegible-Bold.ttf
var fontBold []byte

func uiURL() string { return strings.Replace(pageURL, "/?t=", "/ui?t=", 1) }

func uiRoutes(mux *http.ServeMux) {
	ok := func(r *http.Request) bool { return r.URL.Query().Get("t") == token }
	mux.HandleFunc("/ui", func(w http.ResponseWriter, r *http.Request) {
		if !ok(r) {
			http.Error(w, "Open Desk Buddy from its icon in the taskbar.", 403)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		page := strings.ReplaceAll(uiHTML, "{{BUDDY}}", "data:image/png;base64,"+base64.StdEncoding.EncodeToString(buddyPNG))
		io.WriteString(w, strings.Replace(page, "{{VERSION}}", version, 1))
	})
	mux.HandleFunc("/ui/font/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "font/ttf")
		w.Header().Set("Cache-Control", "max-age=86400")
		if strings.HasSuffix(r.URL.Path, "bold.ttf") {
			w.Write(fontBold)
		} else {
			w.Write(fontRegular)
		}
	})
	mux.HandleFunc("/ui/info", func(w http.ResponseWriter, r *http.Request) {
		if !ok(r) {
			http.Error(w, "", 403)
			return
		}
		c := current()
		st, last := getStatus()
		info := map[string]any{"mode": c.Mode, "server": c.Server, "connected": connected(), "paused": c.Paused,
			"startup": startsWithWindows(), "status": st, "last": last, "refused": keyRefused.Load(), "version": version, "pairs": pairs(), "perf": perfInfo(), "node": nodeInstalled(), "os": runtime.GOOS, "config_dir": configDir()}
		if r, n, busy := updateState(); r != nil || n != "" {
			info["update"] = map[string]any{"ready": r, "note": n, "busy": busy}
		}
		info["notes"] = patchNotes() // About › Updates
		if connected() {
			info["ksetup"] = kindleSetupState() // Kindle page: put Desk Buddy on a Kindle (kindlesetup.go)
		}
		if t := lastChecked(); !t.IsZero() {
			info["checked"] = t.Format("15:04")
		}
		if !connected() {
			j, f, busy := joinState()
			info["joining"], info["found"], info["scanning"] = j, f, busy
		}
		if connected() {
			phone := c.Server
			if c.Mode == "local" {
				phone = lanAddress()
				info["lan"] = phone
			}
			if png, err := qrcode.Encode(phone+"/app?key="+c.Key, qrcode.Medium, 240); err == nil {
				info["qr"] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
			}
		}
		writeJSON(w, info)
	})
	mux.HandleFunc("/ui/do", func(w http.ResponseWriter, r *http.Request) {
		if !ok(r) || r.Method != "POST" {
			http.Error(w, "", 403)
			return
		}
		var b map[string]string
		json.NewDecoder(r.Body).Decode(&b)
		res := map[string]any{"ok": true}
		switch b["do"] {
		case "scan": // look for Desk Buddy on the Wi-Fi (and join by itself if there's exactly one)
			go scan(b["auto"] == "1")
		case "joinfound":
			go join(found{Name: b["name"], Addr: b["addr"]})
		case "cancel":
			joinMu.Lock()
			if joinNow != nil {
				declined[joinNow.Addr] = true
			}
			joinNow = nil
			joinMu.Unlock()
		case "local":
			c := current()
			if c.Mode != "local" || c.Key == "" {
				c.Key = newKey()
			}
			c.Mode, c.Server, c.Paused = "local", "http://127.0.0.1:8787", false
			saveConfig(c)
			stop = false
			setStatus("Starting Desk Buddy on this PC…")
		case "join":
			server := normalizeServer(b["server"])
			key, err := claim(server, b["code"])
			if err != nil {
				res = map[string]any{"error": err.Error()}
				break
			}
			c := current()
			c.Mode, c.Server, c.Key, c.Paused = "remote", server, key, false
			saveConfig(c)
			setStatus("Connected")
		case "allow", "deny":
			answerPair(b["id"], b["do"] == "allow")
		case "pause":
			setPaused(b["on"] == "1")
		case "startup":
			setStartWithWindows(b["on"] == "1")
		case "reset":
			if local() {
				stopServer()
			}
			saveConfig(Config{})
			keyRefused.Store(false)
			setStatus("Not set up")
		case "offer": // a one-time code for another computer
			var o struct{ Code string }
			if err := call(quick, "POST", "/api/connect/offer", map[string]any{}, &o); err != nil {
				res = map[string]any{"error": "Couldn't make a code: " + err.Error()}
				break
			}
			srv := current().Server
			if local() {
				srv = lanAddress()
			}
			res = map[string]any{"code": o.Code, "server": srv}
		case "kindleinstall": // put Desk Buddy on the plugged-in Kindle
			go installKindle()
		case "kindlesave": // the manual way: a zip in Downloads
			if p, err := saveKindleFiles(); err != nil {
				res = map[string]any{"error": "Couldn't save the Kindle files: " + err.Error()}
			} else {
				res = map[string]any{"saved": p}
			}
		case "update":
			go applyUpdate()
		case "checkupdate":
			if err := checkUpdate(); err != nil {
				setUpdNote("Couldn't check just now", false)
			} else if r, _, _ := updateState(); r == nil {
				setUpdNote("Up to date", false)
			} else {
				setUpdNote("", false)
			}
		case "openurl": // in the browser, for these addresses only
			if u := b["url"]; u == "https://dash.cloudflare.com/sign-up" || u == "https://nodejs.org" {
				shellOpen(u)
			}
		case "openpage": // the phone page in the browser
			c := current()
			base := c.Server
			if local() {
				base = lanAddress()
			}
			shellOpen(base + "/app?key=" + c.Key)
		case "uninstall":
			uninstall()
		case "cloudsetup":
			if err := runSetup(); err != nil {
				res = map[string]any{"error": err.Error()}
			}
		case "cloudconnect":
			if err := connectCloud(b["url"]); err != nil {
				res = map[string]any{"error": err.Error()}
			}
		case "tolocal":
			if err := switchToLocal(); err != nil {
				res = map[string]any{"error": err.Error()}
			}
		case "cloudprompt":
			prepareKit()
			res = map[string]any{"text": cloudPromptText()}
		case "ai":
			res = map[string]any{"text": aiInstructions()}
		case "quit":
			go quitApp()
		}
		writeJSON(w, res)
	})
	// the Desk Buddy server's API, for the window (only what it needs)
	mux.HandleFunc("/ui/api/", func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/ui")
		allowed := false
		for _, p := range []string{"/api/state", "/api/desk", "/api/tasks", "/api/unpin/", "/api/settings", "/api/kindle/settings", "/api/kindle/status", "/api/pomodoro", "/api/calendars", "/api/calendar"} {
			if path == p || strings.HasPrefix(path, p+"/") || (strings.HasSuffix(p, "/") && strings.HasPrefix(path, p)) {
				allowed = true
			}
		}
		if !ok(r) || !allowed {
			http.Error(w, "", 403)
			return
		}
		var body any
		if r.Method != "GET" {
			raw, _ := io.ReadAll(io.LimitReader(r.Body, 100_000))
			if len(bytes.TrimSpace(raw)) > 0 {
				json.Unmarshal(raw, &body)
			} else {
				body = map[string]any{}
			}
		}
		q := r.URL.Query() // e.g. ?from=…&to=… (not the window's token)
		q.Del("t")
		if len(q) > 0 {
			path += "?" + q.Encode()
		}
		var out any
		if err := call(quick, r.Method, path, body, &out); err != nil {
			w.WriteHeader(502)
			writeJSON(w, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, out)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func normalizeServer(server string) string {
	server = strings.TrimSpace(server)
	if server != "" && !strings.HasPrefix(server, "http") {
		if strings.Contains(server, ".workers.dev") || !strings.Contains(server, ":") {
			server = "https://" + server
		} else {
			server = "http://" + server
		}
	}
	return strings.TrimRight(server, "/")
}

// aiInstructions: for Claude Code / Codex on this PC, like the Mac app's. They read the address
// and key from Desk Buddy's own settings file when needed, so the key is never pasted.
func aiInstructions() string {
	cfgFile := `%APPDATA%\DeskBuddy\config.json`
	if runtime.GOOS != "windows" {
		cfgFile = filepath.Join(configDir(), "config.json")
	}
	return `# Desk Buddy tasks

My task list lives in Desk Buddy (shown on a Kindle on my desk). When I ask to add, change, tick off, pin, reorder or remove a task, do it through Desk Buddy's API. The Kindle updates within a few seconds.

- Settings: ` + "`" + cfgFile + "`" + ` (JSON): the server's address is ` + "`server`" + `, the key is ` + "`key`" + `. Send the key as an ` + "`X-Key`" + ` header. Never print, log or repeat the key.
- Always send a User-Agent (the cloud server blocks some default ones).
- Lists ("profile"): ` + "`home`" + ` = the ` + osLabel() + ` list, ` + "`work`" + ` = the Mac list. With one list it's always ` + "`work`" + `. If it's unclear which list, ask.
- ` + "`GET /api/state`" + `: everything, with each task's ` + "`id`" + `.
- ` + "`POST /api/tasks`" + ` ` + "`{\"profile\", \"text\", \"notes\"?, \"parent_id\"?}`" + `: add (` + "`parent_id`" + ` makes it a subtask).
- ` + "`PATCH /api/tasks/<id>`" + ` ` + "`{\"text\"?, \"notes\"?}`" + `: edit.
- ` + "`POST /api/tasks/<id>/toggle`" + `: tick / untick (subtasks follow). ` + "`POST /api/tasks/<id>/pin`" + `: "Now working on". ` + "`POST /api/unpin/<profile>`" + `.
- ` + "`POST /api/tasks/<id>/move/up`" + ` or ` + "`/move/down`" + `. ` + "`DELETE /api/tasks/<id>`" + ` (with its subtasks; ask first).

To add a task: read ` + "`server`" + ` and ` + "`key`" + ` from that settings file (without printing the key), then POST ` + "`{\"profile\":\"home\",\"text\":\"Call Sam\"}`" + ` to ` + "`<server>/api/tasks`" + ` with the key in an ` + "`X-Key`" + ` header and the User-Agent ` + "`DeskBuddy`" + `.
`
}
