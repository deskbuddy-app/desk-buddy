//go:build linux

package main

// Music on Linux via D-Bus MPRIS2 (org.mpris.MediaPlayer2.*): works with Spotify, Rhythmbox,
// Elisa, Amberol, VLC, Firefox, Chromium/Chrome, Brave, and any other MPRIS2 media player.

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

type mprisPlayer struct {
	dest    string
	app     string
	playing bool
	title   string
	artist  string
	album   string
	artURL  string
}

var (
	artMu        sync.Mutex
	lastArtURL   string
	mprisPlayers = func() []mprisPlayer {
		conn, err := dbus.SessionBus()
		if err != nil {
			return nil
		}
		return listMPRISPlayers(conn)
	}
	mprisSend = func(dest, method string) error {
		conn, err := dbus.SessionBus()
		if err != nil {
			return errNoMusic
		}
		return conn.Object(dest, "/org/mpris/MediaPlayer2").Call(method, 0).Err
	}
)

func listMPRISPlayers(conn *dbus.Conn) []mprisPlayer {
	var names []string
	if err := conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names); err != nil {
		return nil
	}
	var out []mprisPlayer
	for _, name := range names {
		if !strings.HasPrefix(name, "org.mpris.MediaPlayer2.") {
			continue
		}
		obj := conn.Object(name, "/org/mpris/MediaPlayer2")
		idStr := strings.TrimPrefix(name, "org.mpris.MediaPlayer2.")
		if v, err := obj.GetProperty("org.mpris.MediaPlayer2.Identity"); err == nil {
			if s, ok := v.Value().(string); ok && s != "" {
				idStr = s
			}
		}
		p := mprisPlayer{dest: name, app: appName(idStr)}
		if v, err := obj.GetProperty("org.mpris.MediaPlayer2.Player.PlaybackStatus"); err == nil {
			if s, ok := v.Value().(string); ok {
				p.playing = strings.EqualFold(s, "Playing")
			}
		}
		if v, err := obj.GetProperty("org.mpris.MediaPlayer2.Player.Metadata"); err == nil {
			if meta, ok := v.Value().(map[string]dbus.Variant); ok {
				if t, ok := meta["xesam:title"]; ok {
					if s, ok := t.Value().(string); ok {
						p.title = s
					}
				}
				if a, ok := meta["xesam:album"]; ok {
					if s, ok := a.Value().(string); ok {
						p.album = s
					}
				}
				if ar, ok := meta["xesam:artist"]; ok {
					switch val := ar.Value().(type) {
					case []string:
						if len(val) > 0 {
							p.artist = strings.Join(val, ", ")
						}
					case string:
						p.artist = val
					}
				}
				if u, ok := meta["mpris:artUrl"]; ok {
					if s, ok := u.Value().(string); ok {
						p.artURL = s
					}
				}
			}
		}
		out = append(out, p)
	}
	return out
}

func nowPlaying(pref, last string) (musicState, error) {
	players := mprisPlayers()
	if len(players) == 0 {
		return musicState{}, nil
	}
	infos := make([]sessionInfo, len(players))
	for i, p := range players {
		infos[i] = sessionInfo{App: p.app, Playing: p.playing}
	}
	idx := pickSession(infos, 0, pref, last)
	if idx < 0 {
		return musicState{}, nil
	}
	chosen := players[idx]
	artMu.Lock()
	lastArtURL = chosen.artURL
	artMu.Unlock()
	return musicState{
		App:     chosen.app,
		Running: true,
		Playing: chosen.playing,
		Title:   chosen.title,
		Artist:  chosen.artist,
		Album:   chosen.album,
	}, nil
}

func mediaControl(cmd, pref, last string) error {
	players := mprisPlayers()
	if len(players) == 0 {
		return errNoMusic
	}
	infos := make([]sessionInfo, len(players))
	for i, p := range players {
		infos[i] = sessionInfo{App: p.app, Playing: p.playing}
	}
	idx := pickSession(infos, 0, pref, last)
	if idx < 0 {
		return errNoMusic
	}
	method := ""
	switch cmd {
	case "playpause":
		method = "org.mpris.MediaPlayer2.Player.PlayPause"
	case "next":
		method = "org.mpris.MediaPlayer2.Player.Next"
	case "previous":
		method = "org.mpris.MediaPlayer2.Player.Previous"
	default:
		return fmt.Errorf("unknown media command: %s", cmd)
	}
	return mprisSend(players[idx].dest, method)
}

func coverArt() ([]byte, error) {
	artMu.Lock()
	u := lastArtURL
	artMu.Unlock()
	if u == "" {
		return nil, errNoMusic
	}
	if strings.HasPrefix(u, "file://") {
		parsed, err := url.Parse(u)
		if err != nil {
			return nil, err
		}
		return os.ReadFile(parsed.Path)
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(u)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, errNoMusic
		}
		return io.ReadAll(io.LimitReader(resp.Body, 5<<20))
	}
	return nil, errNoMusic
}
