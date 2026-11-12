//go:build !windows && !linux

package main

import "errors"

// Stand-ins so the apps code can be built and tested on a Mac (the real ones are in
// apps_windows.go).

func listApps() (map[string]string, error) { return nil, errors.New("only on Windows") }
func appIcon(string) []byte                { return nil }
func launch(string) error                  { return nil }
