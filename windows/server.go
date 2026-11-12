package main

// "On this PC": Desk Buddy's own server (the same code as on the Mac and in the cloud, built
// for Windows) runs next to this app, on port 8787 for the whole Wi-Fi (the Kindle and the phone
// page), with its data in %APPDATA%\DeskBuddy\server. A Kindle that wants to connect waits
// for Allow on this PC.

import (
	"crypto/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const port = 8787

var (
	srvMu sync.Mutex
	srv   *exec.Cmd
	stop  bool
)

func newKey() string {
	const abc = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	b := make([]byte, 32)
	rand.Read(b)
	for i := range b {
		b[i] = abc[int(b[i])%len(abc)]
	}
	return string(b)
}

func exeDir() string {
	exe, _ := os.Executable()
	return filepath.Dir(exe)
}

// serverAnswers: our server is up (maybe still running from before this app was restarted).
func serverAnswers() bool {
	return call(quick, "GET", "/api/where", nil, nil) == nil
}

// keepServer runs the server while in "On this PC" mode, and starts it again if it stops.
func keepServer() {
	for {
		srvMu.Lock()
		running := srv != nil
		srvMu.Unlock()
		if local() && !running && !stop && !serverAnswers() {
			startServer()
		}
		time.Sleep(5 * time.Second)
	}
}

func startServer() {
	c := current()
	bin := filepath.Join(exeDir(), serverExe)
	if _, err := os.Stat(bin); err != nil {
		report("server: " + serverExe + " is missing next to Desk Buddy.exe (unzip the whole folder)")
		setStatus("The server file is missing: unzip the whole Desk Buddy folder")
		return
	}
	data := filepath.Join(configDir(), "server")
	os.MkdirAll(data, 0o700)
	cmd := exec.Command(bin, "--data", data, "--key", c.Key, "--port", "8787", "--assets", filepath.Join(exeDir(), "public"))
	hideWindow(cmd)
	if err := cmd.Start(); err != nil {
		report("server: " + err.Error())
		return
	}
	srvMu.Lock()
	srv = cmd
	srvMu.Unlock()
	go func() {
		cmd.Wait()
		srvMu.Lock()
		if srv == cmd {
			srv = nil
		}
		srvMu.Unlock()
	}()
}

func stopServer() {
	srvMu.Lock()
	stop = true
	if srv != nil && srv.Process != nil {
		srv.Process.Kill()
	}
	srv = nil
	srvMu.Unlock()
}

// ---- a Kindle (or another device) asking to connect: Allow on this PC ----

type pairReq struct {
	ID   string `json:"id"`
	Code string `json:"code"`
	Name string `json:"name"`
}

var (
	pairMu  sync.Mutex
	pending []pairReq
	asked   = map[string]bool{}
)

func watchPairing() {
	for {
		if local() {
			var list []pairReq
			if call(quick, "GET", "/api/pair/pending", nil, &list) == nil {
				pairMu.Lock()
				pending = list
				fresh := false
				for _, p := range list {
					if !asked[p.ID] {
						asked[p.ID], fresh = true, true
					}
				}
				pairMu.Unlock()
				if fresh {
					openWindow() // the window shows Allow / Don't Allow with the code
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
}

func answerPair(id string, allow bool) {
	call(quick, "POST", "/api/pair/"+id, map[string]bool{"allow": allow}, nil)
	pairMu.Lock()
	for i, p := range pending {
		if p.ID == id {
			pending = append(pending[:i], pending[i+1:]...)
			break
		}
	}
	pairMu.Unlock()
}

func pairs() []pairReq {
	pairMu.Lock()
	defer pairMu.Unlock()
	return append([]pairReq(nil), pending...)
}

// lanAddress: this PC's address on the Wi-Fi, for the phone page and other devices.
func lanAddress() string {
	ifaces, _ := net.Interfaces()
	for _, i := range ifaces {
		if i.Flags&net.FlagUp == 0 || i.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := i.Addrs()
		for _, a := range addrs {
			if ip, ok := a.(*net.IPNet); ok && ip.IP.To4() != nil && ip.IP.IsPrivate() {
				return "http://" + ip.IP.String() + ":8787"
			}
		}
	}
	return "http://127.0.0.1:8787"
}
