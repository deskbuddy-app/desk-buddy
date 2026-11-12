//go:build linux

package main

// Performance (the Performance page): CPU time and resident memory of this app and, on
// "On this PC", of its server, read from /proc.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// usage: CPU seconds so far and resident memory in MiB for a process (0 = this one).
func usage(pid int) (cpu float64, mem float64, ok bool) {
	proc := "self"
	if pid != 0 {
		proc = strconv.Itoa(pid)
	}
	statBytes, err := os.ReadFile(fmt.Sprintf("/proc/%s/stat", proc))
	if err != nil {
		return 0, 0, false
	}
	s := string(statBytes)
	rparen := strings.LastIndex(s, ")")
	if rparen < 0 || rparen+2 >= len(s) {
		return 0, 0, false
	}
	fields := strings.Fields(s[rparen+2:])
	// After "(comm) ", field[0] is state (field 3), field[11] is utime (field 14), field[12] is stime (field 15)
	if len(fields) < 22 {
		return 0, 0, false
	}
	utime, err1 := strconv.ParseUint(fields[11], 10, 64)
	stime, err2 := strconv.ParseUint(fields[12], 10, 64)
	rssPages, err3 := strconv.ParseInt(fields[21], 10, 64)
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, false
	}
	// USER_HZ is 100 ticks/sec on Linux kernels
	cpu = float64(utime+stime) / 100.0
	pageSize := int64(os.Getpagesize())
	mem = float64(rssPages*pageSize) / (1 << 20)
	return cpu, mem, true
}
