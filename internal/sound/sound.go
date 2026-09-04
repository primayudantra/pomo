// Package sound plays the short mp3 clips bundled with pomo. Playback shells
// out to a platform audio player against a temp file — no cgo audio decoder
// needed for four tiny clips.
package sound

import (
	"embed"
	"os"
	"os/exec"
	"runtime"
	"sync"
)

//go:embed start.mp3 finish.mp3 in-progress-1.mp3 in-progress-2.mp3
var files embed.FS

// ID names a bundled clip, or None to mean "play nothing".
type ID string

const (
	None        ID = "none"
	Start       ID = "start"
	Finish      ID = "finish"
	InProgress1 ID = "in-progress-1"
	InProgress2 ID = "in-progress-2"
)

// Option is a clip offered in the settings picker.
type Option struct {
	ID    ID
	Label string
}

// StartOptions lists the clips a user can pick as their session-start sound.
func StartOptions() []Option {
	return []Option{
		{None, "None"},
		{Start, "Classic"},
		{InProgress1, "Chime 1"},
		{InProgress2, "Chime 2"},
	}
}

var (
	mu      sync.Mutex
	current *exec.Cmd
	gen     int // bumped by Stop/Play/Loop to invalidate any earlier loop goroutine
)

// Play fires id once, asynchronously, first stopping whatever clip is still
// playing — otherwise switching sounds quickly (e.g. cycling the settings
// picker) stacks overlapping afplay processes instead of replacing the old
// one. Playback failures are swallowed since sound is a non-critical nicety
// and the TUI has no stderr to surface them to.
func Play(id ID) {
	myGen := Stop()
	if id == "" || id == None {
		return
	}
	data, err := files.ReadFile(string(id) + ".mp3")
	if err != nil {
		return
	}
	go play(data, myGen)
}

// Loop repeats id back-to-back until Stop is called (or Play/Loop starts a
// different clip) — used to keep an ambient sound going for the length of a
// running Pomodoro instead of firing once at the start.
func Loop(id ID) {
	myGen := Stop()
	if id == "" || id == None {
		return
	}
	data, err := files.ReadFile(string(id) + ".mp3")
	if err != nil {
		return
	}
	go func() {
		for stillCurrent(myGen) {
			play(data, myGen)
		}
	}()
}

// Stop kills whatever clip is currently playing (if any) and invalidates
// any Loop still running, then returns the new generation for a caller
// about to start its own playback.
func Stop() int {
	mu.Lock()
	defer mu.Unlock()
	gen++
	if current != nil && current.Process != nil {
		_ = current.Process.Kill()
	}
	current = nil
	return gen
}

func stillCurrent(myGen int) bool {
	mu.Lock()
	defer mu.Unlock()
	return myGen == gen
}

// PlayFinish always plays the fixed completion chime, independent of the
// user's chosen start sound.
func PlayFinish() {
	Play(Finish)
}

func play(data []byte, myGen int) {
	tmp, err := os.CreateTemp("", "pomo-sound-*.mp3")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return
	}
	tmp.Close()

	name, args := player()
	if name == "" {
		return
	}

	cmd := exec.Command(name, append(args, tmp.Name())...)
	mu.Lock()
	if myGen != gen {
		mu.Unlock()
		return
	}
	current = cmd
	mu.Unlock()

	_ = cmd.Run()

	mu.Lock()
	if current == cmd {
		current = nil
	}
	mu.Unlock()
}

// player picks a CLI audio player available on the current OS. Only macOS
// is guaranteed (afplay ships with the OS); other platforms fall back to
// whichever common player is on PATH, best-effort.
func player() (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "afplay", nil
	case "linux":
		for _, cand := range []string{"mpg123", "ffplay", "paplay"} {
			if _, err := exec.LookPath(cand); err == nil {
				if cand == "ffplay" {
					return cand, []string{"-nodisp", "-autoexit", "-loglevel", "quiet"}
				}
				return cand, nil
			}
		}
	}
	return "", nil
}
