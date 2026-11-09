package main

// A Wi-Fi diary, to find out why the Kindle drops off the network (2026-10-09: it connects,
// then the Kindle's own Wi-Fi goes to no bars within half a minute). Every 10 s it notes
// wifid's state and the signal (from /proc/net/wireless); every change is kept with the time,
// plus a line every 2 minutes. When the server answers again, the unsent lines go to
// /api/kindle/log ("wifi: …") so they can be read on the computer. Also kept in
// /mnt/us/desk/wifi.log (small).
//
// 2026-10-09: wifid said CONNECTED all through a drop, so each line also says whether the
// Kindle has an address, and whether the router and the server answer (pings / a TCP
// connect), and failed requests are noted with their error: which layer stops?

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

var (
	diaryMu   sync.Mutex
	diary     []string // unsent lines
	diaryLast string
	diaryAt   time.Time
)

// wifiBars: the signal the Kindle's own top bar shows, from wifid ("" when it won't say).
// wifid's signalStrength is a string, e.g. "3/5".
func wifiBars() string {
	out, err := exec.Command("lipc-get-prop", "-s", "com.lab126.wifid", "signalStrength").Output()
	if v := strings.TrimSpace(string(out)); err == nil && v != "" {
		return "bars " + v
	}
	return ""
}

// wifiSignal: the Wi-Fi link quality and level, e.g. "q 54/70, -56 dBm", from /proc/net/wireless.
func wifiSignal() string {
	b, err := os.ReadFile("/proc/net/wireless")
	if err != nil {
		return "?"
	}
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) >= 4 && strings.HasSuffix(f[0], ":") {
			return fmt.Sprintf("q %s/70, %s dBm", strings.TrimSuffix(f[2], "."), strings.TrimSuffix(f[3], "."))
		}
	}
	return "no link"
}

// wlanAddr: the Kindle's own address on wlan0, "no address" without one.
func wlanAddr() string {
	if i, err := net.InterfaceByName("wlan0"); err == nil {
		if as, err := i.Addrs(); err == nil {
			for _, a := range as {
				if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
					return n.IP.String()
				}
			}
		}
	}
	return "no address"
}

// gateway: the router's address, from /proc/net/route (hex, little-endian).
func gateway() string {
	b, _ := os.ReadFile("/proc/net/route")
	for _, l := range strings.Split(string(b), "\n") {
		f := strings.Fields(l)
		if len(f) > 2 && f[1] == "00000000" && len(f[2]) == 8 {
			var x uint32
			fmt.Sscanf(f[2], "%x", &x)
			return fmt.Sprintf("%d.%d.%d.%d", x&255, x>>8&255, x>>16&255, x>>24)
		}
	}
	return ""
}

// pingOK: one ping (busybox) answered within a second.
func pingOK(host string) bool {
	return exec.Command("ping", "-c", "1", "-W", "1", host).Run() == nil
}

// serverOK: a TCP connection to the server opens within 2 s ("" when there's no server).
func serverOK(base string) string {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return ""
	}
	host := u.Host
	if u.Port() == "" {
		host += map[bool]string{true: ":443", false: ":80"}[u.Scheme == "https"]
	}
	t := time.Now()
	c, err := net.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		e := err.Error()
		if i := strings.LastIndex(e, ": "); i >= 0 {
			e = e[i+2:]
		}
		return "server ✗ " + e
	}
	c.Close()
	return fmt.Sprintf("server ok %dms", time.Since(t).Milliseconds())
}

// diaryNote adds a line to the diary now (e.g. a failed request).
func diaryNote(dir, line string) {
	entry := time.Now().UTC().Format("15:04:05") + "Z " + line
	diaryMu.Lock()
	diary = append(diary, entry)
	if len(diary) > 200 {
		diary = diary[len(diary)-200:]
	}
	diaryMu.Unlock()
	if f, err := os.OpenFile(dir+"/wifi.log", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
		if st, _ := f.Stat(); st != nil && st.Size() > 200_000 {
			f.Truncate(0)
		}
		fmt.Fprintln(f, entry)
		f.Close()
	}
}

var lastFetchErr string

// noteFetchErr: a failed request goes in the diary (only when the error changes).
func noteFetchErr(err error) {
	if diaryDir == "" || err == nil || err.Error() == lastFetchErr {
		return
	}
	lastFetchErr = err.Error()
	diaryNote(diaryDir, "request failed: "+lastFetchErr)
}

var diaryDir string

func wifiDiary(dir string, c *Client) {
	if _, err := os.Stat("/mnt/us"); err != nil {
		return // not on a Kindle
	}
	diaryDir = dir
	// what earlier runs wrote (an outage ends with the app closed, which loses the unsent
	// lines; wifi.log keeps them): the file's last 100 lines go first
	if b, err := os.ReadFile(dir + "/wifi.log"); err == nil {
		lines := strings.Split(strings.TrimSpace(string(b)), "\n")
		if len(lines) > 100 {
			lines = lines[len(lines)-100:]
		}
		diaryMu.Lock()
		diary = append([]string{"earlier, from wifi.log:"}, lines...)
		diaryMu.Unlock()
	}
	// once: the raw signal file (to fix "no link") and the chip's power saving
	w, _ := os.ReadFile("/proc/net/wireless")
	pm, _ := exec.Command("sh", "-c", "iwconfig wlan0 2>&1 | grep -i power; cat /sys/module/*/parameters/*power* 2>/dev/null | head -3").Output()
	diaryNote(dir, "start · wireless: "+strings.Join(strings.Fields(string(w)), " ")+" · power: "+strings.Join(strings.Fields(string(pm)), " "))
	var deadSince, kicked time.Time
	for {
		st := wifiState()
		off := wifiOff()
		if off {
			st += " (Wi-Fi switched off)"
		}
		gw := gateway()
		router := "no router"
		if gw != "" {
			router = "router ✗"
			if pingOK(gw) {
				router = "router ok"
			}
		}
		srv := serverOK(c.Base())
		key := strings.Join([]string{st, wlanAddr(), router, strings.SplitN(srv, " ", 3)[0] + " " + strings.SplitN(srv+" ", " ", 3)[1]}, " · ") // without the ms
		line := strings.Join([]string{st, wlanAddr(), router, srv}, " · ")
		if sig := wifiSignal(); sig != "no link" { // this Kindle's driver doesn't report it
			line += " · " + sig
		}
		if b := wifiBars(); b != "" {
			line += " · " + b
		}
		if key != diaryLast || time.Since(diaryAt) > 2*time.Minute {
			diaryNote(dir, line)
			diaryLast, diaryAt = key, time.Now()
		}
		// Stuck: wifid says CONNECTED but neither the router nor the server answers (seen
		// 2026-10-09/10: it stays like that until Wi-Fi is reconnected by hand), or it has been
		// off the network for a while. Reconnect it, as the user did: Wi-Fi off and on.
		// Not when the user switched Wi-Fi off, and at most every 3 minutes.
		// 2026-10-10: the diary showed wifid stuck "PENDING" (joining) for 42 minutes while the
		// app ran, and joining at once after the user quit it: joining needs the Kindle's own
		// software (cvm), which run.sh pauses so it can't draw over us. So: wake it while
		// Wi-Fi rejoins, then pause it again and redraw our screen.
		stuck := !off && st == "CONNECTED" && router != "router ok" && strings.HasPrefix(srv, "server ✗")
		lost := !off && st != "CONNECTED" && st != ""
		switch {
		case !stuck && !lost:
			deadSince = time.Time{}
		case deadSince.IsZero():
			deadSince = time.Now()
		case time.Since(kicked) > 2*time.Minute && (stuck && time.Since(deadSince) >= 30*time.Second || lost && time.Since(deadSince) >= 20*time.Second):
			why := map[bool]string{true: "connected but nothing answers", false: "not connected"}[stuck]
			took, ok := rejoinWifi(stuck)
			diaryNote(dir, fmt.Sprintf("rejoined Wi-Fi with the Kindle's software awake (%s for %ds): %s after %ds",
				why, int(time.Since(deadSince).Seconds()), map[bool]string{true: "connected", false: "still not connected"}[ok], took))
			kicked, deadSince = time.Now(), time.Time{}
		}
		time.Sleep(10 * time.Second)
	}
}

// reconnectWifi switches Wi-Fi off and on again, so wifid joins the saved network afresh.
func reconnectWifi() {
	exec.Command("lipc-set-prop", "-i", "com.lab126.wifid", "enable", "0").Run()
	time.Sleep(3 * time.Second)
	wifiOn()
}

// wifiRedraw: the Kindle's software was awake and may have drawn over the screen (main.go
// redraws everything when it gets this).
var wifiRedraw = make(chan struct{}, 1)

// rejoinWifi wakes the Kindle's software (cvm, paused by run.sh), restarts Wi-Fi when it
// says connected but nothing gets through, waits up to a minute for it to connect, then
// pauses the software again and asks for a full redraw. Returns the seconds it took.
func rejoinWifi(restart bool) (int, bool) {
	t := time.Now()
	exec.Command("killall", "-CONT", "cvm", "kppMainApp", "mesquite", "webreader").Run()
	if restart {
		reconnectWifi()
	}
	ok := false
	for time.Since(t) < 60*time.Second {
		time.Sleep(2 * time.Second)
		if wifiState() == "CONNECTED" && wlanAddr() != "no address" {
			ok = true
			time.Sleep(3 * time.Second) // let it finish (address, routes)
			break
		}
	}
	exec.Command("killall", "-STOP", "cvm", "kppMainApp", "mesquite", "webreader").Run()
	select {
	case wifiRedraw <- struct{}{}:
	default:
	}
	return int(time.Since(t).Seconds()), ok
}

// sendDiary posts the unsent lines (called after a successful fetch).
func sendDiary(c *Client) {
	diaryMu.Lock()
	if len(diary) == 0 {
		diaryMu.Unlock()
		return
	}
	lines := diary
	diary = nil
	diaryMu.Unlock()
	go func() {
		// in pieces the server keeps whole (it cuts a log entry at 4000 characters)
		for len(lines) > 0 {
			n, size := 0, 0
			for n < len(lines) && (n == 0 || size+len(lines[n])+3 < 3800) {
				size += len(lines[n]) + 3
				n++
			}
			if err := c.Post("/api/kindle/log", map[string]string{"text": "wifi: " + strings.Join(lines[:n], " | ")}); err != nil {
				diaryMu.Lock()
				diary = append(lines, diary...) // try again next time
				diaryMu.Unlock()
				return
			}
			lines = lines[n:]
		}
	}()
}
