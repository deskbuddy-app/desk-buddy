//go:build !windows && !linux

package main

import "os/exec"

// Stand-ins so the rest can be built and tested on a Mac (the real ones are in winapi_windows.go).

func idleSeconds() float64           { return 5 }
func darkMode() (bool, bool)         { return false, true }
func chimeSound()                    {}
func shellOpen(string) error         { return nil }
func onlyOne() bool                  { return true }
func startsWithWindows() bool        { return false }
func setStartWithWindows(bool) error { return nil }

const serverExe = "desk-buddy-server"

func hideWindow(*exec.Cmd) {}

func ejectDrive(string) bool { return false }

func openWindow() { openPage() }

func usage(int) (float64, float64, bool) { return 0, 40, true }
