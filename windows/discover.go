package main

// Joining Desk Buddy on the same Wi-Fi without a code: ask the network who runs Desk Buddy
// (the same question the Kindle asks, UDP port 8788), then ask that computer to let this PC in.
// Its owner clicks Allow there (the code on both screens should match), and the key comes back.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
)

type found struct {
	Name string `json:"name"`
	Addr string `json:"addr"`
}

// findDesks: computers on this Wi-Fi running Desk Buddy (not this PC itself), for about 1.5 s.
func findDesks() []found {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{})
	if err != nil {
		return nil
	}
	defer conn.Close()
	targets := []string{"255.255.255.255"}
	mine := map[string]bool{}
	if ifs, err := net.InterfaceAddrs(); err == nil { // also each network's own broadcast address
		for _, a := range ifs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil && !n.IP.IsLoopback() {
				ip, mask := n.IP.To4(), n.Mask
				mine[ip.String()] = true
				b := make(net.IP, 4)
				for i := range b {
					b[i] = ip[i] | ^mask[len(mask)-4+i]
				}
				targets = append(targets, b.String())
			}
		}
	}
	for _, t := range targets {
		for _, q := range []string{"desk-buddy?", "kindle-desk?"} { // the new question, and the older one
			conn.WriteToUDP([]byte(q), &net.UDPAddr{IP: net.ParseIP(t), Port: 8788})
		}
	}
	seen := map[string]found{}
	conn.SetReadDeadline(time.Now().Add(1500 * time.Millisecond))
	buf := make([]byte, 512)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			break
		}
		var r struct {
			Name string `json:"name"`
			Port int    `json:"port"`
		}
		if json.Unmarshal(buf[:n], &r) != nil || r.Port == 0 || mine[from.IP.String()] {
			continue
		}
		addr := fmt.Sprintf("http://%s:%d", from.IP, r.Port)
		seen[addr] = found{Name: r.Name, Addr: addr}
	}
	if len(seen) == 0 { // no answer (often Windows Firewall blocks it): check the addresses directly
		for _, f := range probeSubnets(mine) {
			seen[f.Addr] = f
		}
	}
	out := make([]found, 0, len(seen))
	for _, f := range seen {
		out = append(out, f)
	}
	return out
}

// probeSubnets asks every address on this PC's own networks (up to /24 each) for Desk Buddy's
// /api/where on port 8787, a few at a time. A normal outgoing connection, like a browser's, so
// firewalls let it through. Answers 200 or 401 (needs the key) mean Desk Buddy is there.
func probeSubnets(mine map[string]bool) []found {
	var ips []string
	ifs, _ := net.InterfaceAddrs()
	for _, a := range ifs {
		n, ok := a.(*net.IPNet)
		if !ok || n.IP.To4() == nil || n.IP.IsLoopback() || !n.IP.IsPrivate() {
			continue
		}
		base := n.IP.To4().Mask(net.CIDRMask(24, 32))
		for i := 1; i < 255; i++ {
			ip := net.IPv4(base[0], base[1], base[2], byte(i)).String()
			if !mine[ip] {
				ips = append(ips, ip)
			}
		}
	}
	hc := &http.Client{Timeout: 1200 * time.Millisecond}
	var mu sync.Mutex
	var out []found
	sem := make(chan struct{}, 64)
	var wg sync.WaitGroup
	for _, ip := range ips {
		wg.Add(1)
		sem <- struct{}{}
		go func(ip string) {
			defer wg.Done()
			defer func() { <-sem }()
			addr := "http://" + ip + ":8787"
			req, _ := http.NewRequest("GET", addr+"/api/where", nil)
			req.Header.Set("User-Agent", userAgent)
			resp, err := hc.Do(req)
			if err != nil {
				return
			}
			resp.Body.Close()
			if resp.StatusCode == 200 || resp.StatusCode == 401 {
				mu.Lock()
				out = append(out, found{Name: "Desk Buddy at " + ip, Addr: addr})
				mu.Unlock()
			}
		}(ip)
	}
	wg.Wait()
	return out
}

// ---- joining one of them ----

type joining struct {
	Name   string `json:"name"`
	Addr   string `json:"addr"`
	Code   string `json:"code"`
	State  string `json:"state"` // asking, waiting (for Allow), denied, failed
	Reason string `json:"reason,omitempty"`
}

var (
	joinMu   sync.Mutex
	joinNow  *joining
	foundNow []found
	scanning bool
	declined = map[string]bool{} // computers the user said no to (or that said no): don't join by themselves again
)

// scan looks on the Wi-Fi, and joins by itself when there's exactly one Desk Buddy to join.
func scan(auto bool) {
	joinMu.Lock()
	if scanning {
		joinMu.Unlock()
		return
	}
	scanning = true
	joinMu.Unlock()
	f := findDesks()
	joinMu.Lock()
	scanning, foundNow = false, f
	busy := joinNow != nil && (joinNow.State == "asking" || joinNow.State == "waiting")
	joinMu.Unlock()
	if auto && !busy && !connected() && len(f) == 1 && !declined[f[0].Addr] {
		go join(f[0])
	}
}

func pcName() string {
	base := osLabel() + " PC"
	h, _ := os.Hostname()
	if h == "" {
		return base
	}
	return base + " (" + h + ")"
}

// join asks that computer to let this PC in and waits (up to 5 minutes) for Allow there.
func join(f found) {
	j := &joining{Name: f.Name, Addr: f.Addr, State: "asking"}
	joinMu.Lock()
	joinNow = j
	joinMu.Unlock()
	set := func(state, reason string) {
		joinMu.Lock()
		j.State, j.Reason = state, reason
		joinMu.Unlock()
	}
	b, _ := json.Marshal(map[string]string{"name": pcName()})
	req, _ := http.NewRequest("POST", f.Addr+"/api/pair", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	resp, err := quick.Do(req)
	if err != nil {
		set("failed", "Couldn't reach "+f.Name+".")
		return
	}
	var r struct{ ID, Code string }
	json.NewDecoder(resp.Body).Decode(&r)
	resp.Body.Close()
	if r.ID == "" {
		set("failed", f.Name+" isn't letting new devices in right now.")
		return
	}
	joinMu.Lock()
	j.Code, j.State = r.Code, "waiting"
	joinMu.Unlock()
	for end := time.Now().Add(5 * time.Minute); time.Now().Before(end); time.Sleep(time.Second) {
		g, err := http.NewRequest("GET", f.Addr+"/api/pair/"+r.ID, nil)
		if err != nil {
			break
		}
		g.Header.Set("User-Agent", userAgent)
		resp, err := quick.Do(g)
		if err != nil {
			continue
		}
		var s struct{ State, Key string }
		json.NewDecoder(resp.Body).Decode(&s)
		resp.Body.Close()
		switch s.State {
		case "ok":
			c := current()
			c.Mode, c.Server, c.Key, c.Paused = "remote", f.Addr, s.Key, false
			saveConfig(c)
			setStatus("Connected to " + f.Name)
			joinMu.Lock()
			joinNow = nil
			joinMu.Unlock()
			return
		case "denied":
			declined[f.Addr] = true
			set("denied", f.Name+" said no.")
			return
		case "expired":
			set("failed", "It took too long. Try again.")
			return
		}
	}
	set("failed", "Nobody clicked Allow. Try again.")
}

func joinState() (*joining, []found, bool) {
	joinMu.Lock()
	defer joinMu.Unlock()
	var j *joining
	if joinNow != nil {
		c := *joinNow
		j = &c
	}
	return j, append([]found(nil), foundNow...), scanning
}
