//go:build !windows && !linux

package main

// Stand-ins so the music code can be built and tested on a Mac (the real ones are in
// music_windows.go and audio_windows.go).

func nowPlaying(pref, last string) (musicState, error) { return musicState{}, nil }
func coverArt() ([]byte, error)                        { return nil, errNoMusic }
func mediaControl(cmd, pref, last string) error        { return errNoMusic }
func soundVolume() (int, bool)                         { return 0, false }
func changeVolume(up bool) error                       { return nil }
func soundOutputs() []output                           { return nil }
func setOutput(id string) error                        { return nil }
