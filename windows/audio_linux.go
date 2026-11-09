//go:build linux

package main

// System volume and sound outputs on Linux via PulseAudio / PipeWire (`pactl` or `wpctl`).

import (
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var pctRe = regexp.MustCompile(`(\d+)%`)

func soundVolume() (int, bool) {
	if p, err := exec.LookPath("pactl"); err == nil {
		if out, err := exec.Command(p, "get-sink-volume", "@DEFAULT_SINK@").Output(); err == nil {
			if m := pctRe.FindStringSubmatch(string(out)); len(m) == 2 {
				if v, err := strconv.Atoi(m[1]); err == nil {
					if muteOut, err := exec.Command(p, "get-sink-mute", "@DEFAULT_SINK@").Output(); err == nil {
						if strings.Contains(strings.ToLower(string(muteOut)), "yes") {
							return 0, true
						}
					}
					return max(0, min(100, v)), true
				}
			}
		}
	}
	if p, err := exec.LookPath("wpctl"); err == nil {
		if out, err := exec.Command(p, "get-volume", "@DEFAULT_AUDIO_SINK@").Output(); err == nil {
			s := strings.TrimSpace(string(out))
			if strings.Contains(s, "[MUTED]") {
				return 0, true
			}
			fields := strings.Fields(s)
			if len(fields) >= 2 {
				if f, err := strconv.ParseFloat(fields[1], 64); err == nil {
					return max(0, min(100, int(f*100+0.5))), true
				}
			}
		}
	}
	return 0, false
}

func changeVolume(up bool) error {
	cur, ok := soundVolume()
	if !ok {
		cur = 50
	}
	next := stepVolume(cur, up)
	if p, err := exec.LookPath("pactl"); err == nil {
		_ = exec.Command(p, "set-sink-mute", "@DEFAULT_SINK@", "0").Run()
		return exec.Command(p, "set-sink-volume", "@DEFAULT_SINK@", strconv.Itoa(next)+"%").Run()
	}
	if p, err := exec.LookPath("wpctl"); err == nil {
		_ = exec.Command(p, "set-mute", "@DEFAULT_AUDIO_SINK@", "0").Run()
		return exec.Command(p, "set-volume", "@DEFAULT_AUDIO_SINK@", strconv.Itoa(next)+"%").Run()
	}
	return nil
}

func soundOutputs() []output {
	p, err := exec.LookPath("pactl")
	if err != nil {
		return nil
	}
	defBytes, _ := exec.Command(p, "get-default-sink").Output()
	defSink := strings.TrimSpace(string(defBytes))

	outBytes, err := exec.Command(p, "list", "sinks").Output()
	if err != nil {
		return nil
	}
	var res []output
	blocks := strings.Split(string(outBytes), "Sink #")
	for _, b := range blocks[1:] {
		var name, desc string
		for _, line := range strings.Split(b, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "Name:") {
				name = strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
			} else if strings.HasPrefix(line, "Description:") {
				desc = strings.TrimSpace(strings.TrimPrefix(line, "Description:"))
			}
		}
		if name == "" {
			continue
		}
		if desc == "" {
			desc = name
		}
		var ff uint32
		low := strings.ToLower(name + " " + desc)
		if strings.Contains(low, "hdmi") || strings.Contains(low, "displayport") {
			ff = 9
		} else if strings.Contains(low, "headphone") || strings.Contains(low, "headset") || strings.Contains(low, "bluez") {
			ff = 3
		}
		res = append(res, output{
			ID:        name,
			Name:      desc,
			Kind:      outputKind(ff, desc),
			Current:   name == defSink,
			Connected: true,
		})
	}
	if len(res) < 2 {
		return nil
	}
	return res
}

func setOutput(id string) error {
	p, err := exec.LookPath("pactl")
	if err != nil {
		return err
	}
	return exec.Command(p, "set-default-sink", id).Run()
}
