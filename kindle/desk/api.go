package main

// Talking to the Desk Buddy backend (backend/server.py).

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
)

// Desk is the /api/desk response: everything needed to draw one screen.
type Desk struct {
	Rev    string `json:"rev"`     // changes whenever anything changes (newer backends only)
	NewKey string `json:"new_key"` // the server's key was changed: use this one from now on
	Cloud  string `json:"cloud"`   // the cloud's address (remembered, so Cloud can always be picked)
	MoveTo *struct {
		Server string `json:"server"`
		Name   string `json:"name"`
	} `json:"move_to"` // the computer moved Desk Buddy to this server: follow it
	Screen    string          `json:"screen"` // tasks, night, message
	Message   string          `json:"message"`
	Profile   string          `json:"profile"`
	Reason    string          `json:"reason"`
	Mode      string          `json:"mode"`     // work, home, auto
	PCLabel   string          `json:"pc_label"` // "Linux" when the connected PC runs Linux, else "Windows"
	Tasks     []Task          `json:"tasks"`
	OpenTotal int             `json:"open_total"`
	Claude    *Claude         `json:"claude"`
	Agenda    []Event         `json:"agenda"`          // now and next, until the end of tomorrow
	CalList   []CalShow       `json:"calendar_list"`   // every calendar, and which list it shows on
	Computers []string        `json:"computers"`       // computers with the helper ("mac", "pc"); nil on older backends
	KindleSet *KindleSettings `json:"kindle_settings"` // this Kindle's settings as last changed anywhere (ksync.go)
	Calendars int             `json:"calendars"`       // calendars connected for this profile
	Deck      *Deck           `json:"deck"`            // missing on the old Mac backend
	Pomodoro  *Pomo           `json:"pomodoro"`
	Music     *Music          `json:"music"`
	Settings  map[string]any  `json:"settings"`
	// for the Auto theme: the computer in use's appearance ("dark", "light", "" = unknown)
	// and today's sunrise / sunset
	Appearance string `json:"appearance"`
	Sun        *struct {
		Rise int64 `json:"rise"`
		Set  int64 `json:"set"`
	} `json:"sun"`
	Now       int64 `json:"now"`
	UTCOffset int   `json:"utc_offset"`
}

// CalShow: a calendar and where it shows (Settings › Calendar): both, work, home or off.
type CalShow struct {
	Name string `json:"name"`
	Show string `json:"show"`
}

type Event struct {
	Title       string `json:"title"`
	Start       int64  `json:"start"`
	End         int64  `json:"end"`
	AllDay      bool   `json:"all_day"` // then Start/End are UTC midnights, End exclusive
	Location    string `json:"location"`
	Description string `json:"description"`
	Cal         string `json:"cal"`
	ID          int    `json:"id"`   // set on events added on the Kindle (they can be deleted)
	Join        string `json:"join"` // a Zoom / Meet / … link in it (the backend finds it)
}

type Deck struct {
	Profile      string     `json:"profile"`
	Machine      string     `json:"machine"`
	MachineLabel string     `json:"machine_label"`
	Online       bool       `json:"online"`
	Connected    bool       `json:"connected"` // the computer has the helper (else: "install it" with a QR code)
	Tiles        []DeckTile `json:"tiles"`
	Last         *DeckLast  `json:"last"`
	IconsVersion string     `json:"icons_version"` // changes when the computer uploads new app icons
	Decks        []Deck     `json:"decks"`         // every computer's deck (work → MacBook, home → PC)
	Apps         []string   `json:"apps"`          // every app on that computer ("All apps")
}

type DeckTile struct {
	Label string `json:"label"`
	App   string `json:"app"` // the app it opens (to tell which apps are favourites)
}

type DeckLast struct {
	Label string `json:"label"`
	State string `json:"state"` // sent, waiting, opening, done, failed
	Error string `json:"error"`
}

type Task struct {
	ID     int    `json:"id"`
	Text   string `json:"text"`
	Done   bool   `json:"done"`
	Pinned bool   `json:"pinned"`
	Tag    string `json:"tag"`
	Parent *int   `json:"parent_id"`
	Notes  string `json:"notes"`
}

type Claude struct {
	State string  `json:"state"` // working, input, done, idle
	Text  string  `json:"text"`
	At    float64 `json:"at"`
}

type Client struct {
	base, key string
	hc        *http.Client
	long      *http.Client // for a desk request the server may hold for up to 25 s
}

// certs.pem: an up-to-date list of trusted certificate authorities (copied from macOS's
// /etc/ssl/cert.pem), added to the Kindle's own 2021 list so HTTPS hosts like Cloudflare work.
//
//go:embed certs.pem
var certsPEM []byte

func NewClient(base, key string) *Client {
	roots, err := x509.SystemCertPool()
	if err != nil || roots == nil {
		roots = x509.NewCertPool()
	}
	roots.AppendCertsFromPEM(certsPEM)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.TLSClientConfig = &tls.Config{RootCAs: roots}
	c := &Client{base: base, key: key}
	kt := keyTransport{tr, c}
	c.hc = &http.Client{Timeout: 20 * time.Second, Transport: kt} // patient: on a weak signal requests are slow
	c.long = &http.Client{Timeout: 50 * time.Second, Transport: kt}
	return c
}

// keyTransport sends the key as an X-Key header on every request (it used to ride in the
// address as ?key=, where it ends up in logs).
type keyTransport struct {
	next http.RoundTripper
	c    *Client
}

func (t keyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if k := t.c.Key(); k != "" && r.Header.Get("X-Key") == "" {
		r = r.Clone(r.Context())
		r.Header.Set("X-Key", k)
	}
	return t.next.RoundTrip(r)
}

func (c *Client) url(path string) string {
	return c.Base() + path // the key goes in a header (keyTransport)
}

// Art downloads the current album cover (only kept in memory).
func (c *Client) Art() (string, string, error) {
	resp, err := c.hc.Get(c.url("/api/music/art"))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	var r struct{ Key, Image string }
	err = json.NewDecoder(resp.Body).Decode(&r)
	return r.Key, r.Image, err
}

// DeckIcons downloads a computer's app icons, ready to draw (iconSize square, greyscale).
func (c *Client) DeckIcons(machine string) (map[string]*image.Gray, error) {
	resp, err := c.hc.Get(c.url("/api/deck/icons?machine=" + url.QueryEscape(machine)))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("icons: server said %s", resp.Status)
	}
	var r struct {
		Icons map[string]string `json:"icons"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	out := map[string]*image.Gray{}
	for label, b64 := range r.Icons {
		raw, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			continue
		}
		src, err := png.Decode(bytes.NewReader(raw))
		if err != nil {
			continue
		}
		n := px(iconSize, sc) // at the panel's resolution, so the Deck's icons are sharp
		dst := image.NewGray(image.Rect(0, 0, n, n))
		xdraw.CatmullRom.Scale(dst, dst.Rect, src, src.Bounds(), xdraw.Src, nil)
		trimIcon(dst)
		out[label] = dst
	}
	return out, nil
}

// Weather returns the forecast (the backend caches it for 20 minutes).
func (c *Client) Weather() (*Weather, error) {
	resp, err := c.hc.Get(c.url("/api/weather"))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("the weather needs the new backend")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("server said %s", resp.Status)
	}
	var w Weather
	return &w, json.NewDecoder(resp.Body).Decode(&w)
}

// Calendar returns the events between two times for a profile.
func (c *Client) Calendar(from, to int64, profile string) ([]Event, error) {
	resp, err := c.hc.Get(c.url(fmt.Sprintf("/api/calendar?from=%d&to=%d&profile=%s", from, to, url.QueryEscape(profile))))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("the calendar needs the new backend")
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("server said %s", resp.Status)
	}
	var r struct {
		Events []Event `json:"events"`
	}
	return r.Events, json.NewDecoder(resp.Body).Decode(&r)
}

// Desk fetches one screen's worth of data. music: the Music widget is showing, so the
// computers should keep track of what's playing (they don't otherwise).
func (c *Client) Desk(music ...bool) (*Desk, error) {
	return c.DeskWait(context.Background(), len(music) > 0 && music[0], "")
}

// DeskWait is Desk, but with rev (the last reply's) the server holds the request until
// something changes, up to 25 s. ctx cancels it when the Kindle needs an answer now.
func (c *Client) DeskWait(ctx context.Context, music bool, rev string) (*Desk, error) {
	path := "/api/desk"
	if music {
		path += "?music=1"
	}
	hc := c.hc
	if rev != "" {
		if strings.Contains(path, "?") {
			path += "&"
		} else {
			path += "?"
		}
		path += "wait=25&rev=" + url.QueryEscape(rev)
		hc = c.long
	}
	if statusQuery != "" { // how this Kindle is (ksync.go)
		if strings.Contains(path, "?") {
			path += "&" + statusQuery
		} else {
			path += "?" + statusQuery
		}
	}
	req, err := http.NewRequestWithContext(ctx, "GET", c.url(path), nil)
	if err != nil {
		return nil, err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return nil, fmt.Errorf("server said %s: %s", resp.Status, b)
	}
	var d Desk
	return &d, json.NewDecoder(resp.Body).Decode(&d)
}

func (c *Client) Post(path string, body any) error { return c.do("POST", path, body) }

// do sends a JSON request and only cares whether it worked.
func (c *Client) do(method, path string, body any) error {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.url(path), r)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s: %s", path, resp.Status)
	}
	return nil
}

// report sends an error message to the backend (GET /api/kindle/log shows them), so
// problems can be seen without plugging the Kindle in. Best effort.
func (c *Client) report(text string) {
	go c.Post("/api/kindle/log", map[string]string{"text": text})
}

// send performs a tap's action on the backend.
func (c *Client) send(t Target) error {
	switch t.Act {
	case "toggle":
		return c.Post(fmt.Sprintf("/api/tasks/%d/toggle", t.ID), nil)
	case "pin":
		return c.Post(fmt.Sprintf("/api/tasks/%d/pin", t.ID), nil)
	case "unpin":
		return c.Post("/api/unpin/"+t.Arg, nil)
	case "mode":
		return c.Post("/api/profile", map[string]string{"mode": t.Arg})
	case "wake":
		return c.Post("/api/wake", nil)
	case "move":
		return c.Post(fmt.Sprintf("/api/tasks/%d/move/%s", t.ID, t.Arg), nil)
	case "deck":
		return c.Post("/api/deck/run", map[string]any{"profile": t.Arg, "index": t.ID})
	case "deckapp":
		profile, app, _ := strings.Cut(t.Arg, "|")
		return c.Post("/api/deck/run", map[string]any{"profile": profile, "app": app})
	case "favadd":
		profile, app, _ := strings.Cut(t.Arg, "|")
		return c.Post("/api/deck/favourite", map[string]any{"profile": profile, "app": app})
	case "music":
		return c.Post("/api/music", map[string]string{"action": t.Arg})
	case "addtask": // Arg "profile|text", ID the parent (0: none)
		profile, text, _ := strings.Cut(t.Arg, "|")
		b := map[string]any{"profile": profile, "text": text}
		if t.ID > 0 {
			b["parent_id"] = t.ID
		}
		return c.Post("/api/tasks", b)
	case "addevent": // Arg: the JSON body
		var b map[string]any
		json.Unmarshal([]byte(t.Arg), &b)
		return c.Post("/api/events", b)
	case "cleartasks": // Arg "profile|all" or "profile|done"
		profile, which, _ := strings.Cut(t.Arg, "|")
		return c.Post("/api/tasks/clear", map[string]any{"profile": profile, "done_only": which == "done"})
	case "calshow": // Arg "show|calendar name"
		show, name, _ := strings.Cut(t.Arg, "|")
		return c.Post("/api/calendars/show", map[string]string{"name": name, "show": show})
	case "join": // open a meeting link on the computer in use
		return c.Post("/api/join", map[string]string{"url": t.Arg})
	case "delevent":
		return c.do("DELETE", fmt.Sprintf("/api/events/%d", t.ID), nil)
	case "setplace": // Arg "lat|lon|name"
		p := strings.SplitN(t.Arg, "|", 3)
		if len(p) < 3 {
			return nil
		}
		lat, _ := strconv.ParseFloat(p[0], 64)
		lon, _ := strconv.ParseFloat(p[1], 64)
		return c.do("PUT", "/api/weather/location", map[string]any{"lat": lat, "lon": lon, "name": p[2]})
	case "deltask":
		return c.do("DELETE", fmt.Sprintf("/api/tasks/%d", t.ID), nil)
	case "edittask":
		return c.do("PATCH", fmt.Sprintf("/api/tasks/%d", t.ID), map[string]string{"text": t.Arg})
	case "setting": // Arg "key=value"
		k, val, _ := strings.Cut(t.Arg, "=")
		var v any = val
		if val == "true" || val == "false" {
			v = val == "true"
		} else if n, err := strconv.Atoi(val); err == nil {
			v = n
		}
		return c.Post("/api/settings", map[string]any{"key": k, "value": v})
	case "pomo":
		if on, ok := strings.CutPrefix(t.Arg, "sound:"); ok {
			return c.Post("/api/pomodoro", map[string]any{"action": "sound", "on": on == "1"})
		}
		b := map[string]any{"action": t.Arg}
		if (t.Arg == "start" || t.Arg == "switch") && t.ID > 0 {
			b["task"] = t.ID
		}
		return c.Post("/api/pomodoro", b)
	case "favremove":
		profile, _, _ := strings.Cut(t.Arg, "|")
		return c.Post("/api/deck/favourite", map[string]any{"profile": profile, "remove": t.ID})
	case "favmove":
		profile, to, _ := strings.Cut(t.Arg, "|")
		n, _ := strconv.Atoi(to)
		return c.Post("/api/deck/favourite", map[string]any{"profile": profile, "move": t.ID, "to": n})
	}
	return nil
}

// apply changes the local copy straight away, as the backend will, so the screen
// can update before the reply arrives. Returns false if only the server can say what comes next.
// fetchedAt is the Kindle clock when the last /api/desk reply arrived (main.go sets it),
// so optimistic timer changes use the server's idea of now.
var fetchedAt int64

// addTask sends a new task and returns the id the server gave it.
func (c *Client) addTask(t Target) (int, error) {
	profile, text, _ := strings.Cut(t.Arg, "|")
	b := map[string]any{"profile": profile, "text": text}
	if t.ID > 0 {
		b["parent_id"] = t.ID
	}
	body, _ := json.Marshal(b)
	req, err := http.NewRequest("POST", c.url("/api/tasks"), bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return 0, fmt.Errorf("/api/tasks: %s", resp.Status)
	}
	var r struct {
		ID int `json:"id"`
	}
	return r.ID, json.NewDecoder(resp.Body).Decode(&r)
}

func apply(d *Desk, t Target) bool {
	switch t.Act {
	case "calshow":
		show, name, _ := strings.Cut(t.Arg, "|")
		for i := range d.CalList {
			if d.CalList[i].Name == name {
				d.CalList[i].Show = show
			}
		}
		return true
	case "addtask": // only shown before the server has it when it was added offline (Temp)
		profile, text, _ := strings.Cut(t.Arg, "|")
		if t.Temp == 0 || (profile != d.Profile && t.ID == 0) {
			return false
		}
		task := Task{ID: t.Temp, Text: text}
		if t.ID != 0 {
			parent := t.ID
			task.Parent = &parent
		}
		d.Tasks = append(d.Tasks, task)
		d.OpenTotal++
		return true
	case "toggle":
		done := false
		for i := range d.Tasks {
			if d.Tasks[i].ID == t.ID {
				d.Tasks[i].Done = !d.Tasks[i].Done
				done = d.Tasks[i].Done
				if d.Tasks[i].Done {
					d.Tasks[i].Pinned = false
					d.OpenTotal--
				} else {
					d.OpenTotal++
				}
			}
		}
		for i := range d.Tasks { // its subtasks follow (as the backend does)
			if p := d.Tasks[i].Parent; p != nil && *p == t.ID {
				d.Tasks[i].Done = done
			}
		}
		return true
	case "pin":
		for i := range d.Tasks {
			d.Tasks[i].Pinned = d.Tasks[i].ID == t.ID
		}
		return true
	case "unpin":
		for i := range d.Tasks {
			d.Tasks[i].Pinned = false
		}
		return true
	case "move":
		moveLocal(d, t.ID, t.Arg == "up")
		return true
	case "deltask": // the task and its subtasks
		var keep []Task
		for _, x := range d.Tasks {
			if x.ID != t.ID && (x.Parent == nil || *x.Parent != t.ID) {
				keep = append(keep, x)
			}
		}
		d.Tasks = keep
		return true
	case "cleartasks":
		_, which, _ := strings.Cut(t.Arg, "|")
		gone := map[int]bool{}
		for _, x := range d.Tasks {
			if x.Parent == nil && (which == "all" || x.Done) {
				gone[x.ID] = true
			}
		}
		var keep []Task // what's left: nothing, or the open tasks and their subtasks
		for _, x := range d.Tasks {
			if which != "all" && !gone[x.ID] && (x.Parent == nil || !gone[*x.Parent]) {
				keep = append(keep, x)
			}
		}
		d.Tasks = keep
		return true
	case "edittask":
		for i := range d.Tasks {
			if d.Tasks[i].ID == t.ID {
				d.Tasks[i].Text = t.Arg
			}
		}
		return true
	case "deck":
		if d.Deck != nil {
			for i := range d.Deck.Decks {
				if dk := &d.Deck.Decks[i]; dk.Profile == t.Arg && t.ID < len(dk.Tiles) {
					dk.Last = &DeckLast{Label: dk.Tiles[t.ID].Label, State: "sent"}
				}
			}
		}
		return true
	case "deckapp":
		profile, app, _ := strings.Cut(t.Arg, "|")
		if d.Deck != nil {
			for i := range d.Deck.Decks {
				if d.Deck.Decks[i].Profile == profile {
					d.Deck.Decks[i].Last = &DeckLast{Label: app, State: "sent"}
				}
			}
		}
		return true
	case "setting":
		k, val, _ := strings.Cut(t.Arg, "=")
		if d.Settings == nil {
			d.Settings = map[string]any{}
		}
		if val == "true" || val == "false" {
			d.Settings[k] = val == "true"
		} else if n, err := strconv.Atoi(val); err == nil {
			d.Settings[k] = float64(n)
		} else {
			d.Settings[k] = val
		}
		return true
	case "music":
		if d.Music != nil && t.Arg == "playpause" {
			d.Music.Playing = !d.Music.Playing
		}
		if d.Music != nil && d.Music.Volume != nil && strings.HasPrefix(t.Arg, "volume:") {
			v := *d.Music.Volume + map[bool]int{true: 10, false: -10}[t.Arg == "volume:up"]
			v = max(0, min(100, v))
			d.Music.Volume = &v
		}
		if id, ok := strings.CutPrefix(t.Arg, "output:"); ok && d.Music != nil {
			for i := range d.Music.Outputs {
				d.Music.Outputs[i].Current = d.Music.Outputs[i].ID == id
			}
		}
		return true
	case "pomo":
		p := d.Pomodoro
		if p == nil {
			return false
		}
		now := time.Now().Unix() + (d.Now - fetchedAt)
		switch t.Arg {
		case "start":
			if t.ID > 0 {
				p.Task = t.ID
				for i := range d.Tasks {
					d.Tasks[i].Pinned = d.Tasks[i].ID == t.ID
					if d.Tasks[i].ID == t.ID {
						p.Title = d.Tasks[i].Text
					}
				}
			}
			mins := int64(25)
			if v, ok := d.Settings["focus_min"].(float64); ok {
				mins = int64(v)
			}
			p.State, p.Length, p.Ends = "focus", mins*60, now+mins*60
		case "switch":
			p.Task = t.ID
			for i := range d.Tasks {
				d.Tasks[i].Pinned = d.Tasks[i].ID == t.ID
				if d.Tasks[i].ID == t.ID {
					p.Title = d.Tasks[i].Text
				}
			}
		case "break":
			p.State, p.Length, p.Ends = "break", 5*60, now+5*60
		case "pause":
			p.Was, p.Left, p.State = p.State, max(0, p.Ends-now), "paused"
		case "resume":
			p.State, p.Ends = p.Was, now+p.Left
		case "stop":
			p.State = "idle"
		case "sound:1", "sound:0":
			p.Sound = t.Arg == "sound:1"
		}
		return true
	case "favadd", "favremove", "favmove":
		profile, app, _ := strings.Cut(t.Arg, "|")
		if d.Deck != nil {
			for i := range d.Deck.Decks {
				dk := &d.Deck.Decks[i]
				if dk.Profile != profile {
					continue
				}
				if t.Act == "favmove" {
					from, to := t.ID, 0
					fmt.Sscan(app, &to) // Arg is "profile|to"
					if from < len(dk.Tiles) && to >= 0 && to < len(dk.Tiles) {
						tile := dk.Tiles[from]
						rest := append(dk.Tiles[:from:from], dk.Tiles[from+1:]...)
						dk.Tiles = append(rest[:to:to], append([]DeckTile{tile}, rest[to:]...)...)
					}
				}
				if t.Act == "favremove" && t.ID < len(dk.Tiles) {
					dk.Tiles = append(dk.Tiles[:t.ID:t.ID], dk.Tiles[t.ID+1:]...)
				}
				if t.Act == "favadd" {
					dk.Tiles = append(dk.Tiles, DeckTile{Label: app, App: app})
				}
			}
		}
		return true
	case "mode":
		d.Mode = t.Arg
		if t.Arg != d.Profile {
			return false // switching profile needs the other list from the server
		}
		d.Reason = "manual · until 4am"
		return true
	}
	return false
}

// moveLocal swaps a task with the sibling above or below it, as the backend will.
func moveLocal(d *Desk, id int, up bool) {
	same := func(a, b *int) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }
	var me *Task
	for i := range d.Tasks {
		if d.Tasks[i].ID == id {
			me = &d.Tasks[i]
		}
	}
	if me == nil {
		return
	}
	var idx []int
	at := -1
	for i, t := range d.Tasks {
		if same(t.Parent, me.Parent) {
			if t.ID == id {
				at = len(idx)
			}
			idx = append(idx, i)
		}
	}
	j := at + 1
	if up {
		j = at - 1
	}
	if at < 0 || j < 0 || j >= len(idx) {
		return
	}
	d.Tasks[idx[at]], d.Tasks[idx[j]] = d.Tasks[idx[j]], d.Tasks[idx[at]]
}

// iconBox is the rounded square an app icon occupies in an n×n image: macOS icons fill
// ~83% of the canvas with smooth corners. We trim just inside it.
func iconBox(n int) (image.Rectangle, float64) {
	in := int(float64(n)*0.095 + 0.5)
	box := image.Rect(in, in, n-in, n-in)
	return box, float64(box.Dx()) * 0.27
}

// trimIcon keeps only the app icon's rounded square, dropping the margin, the icon's own
// faint edge and its soft shadow (which showed as a grey box on white).
func trimIcon(img *image.Gray) {
	n := img.Rect.Dx()
	box, rad := iconBox(n)
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			a := clamp01(0.5 - roundRectDist(box, rad, float64(x)+0.5, float64(y)+0.5))
			i := img.PixOffset(x, y)
			img.Pix[i] = uint8(float64(img.Pix[i])*a + 255*(1-a) + 0.5)
		}
	}
}
