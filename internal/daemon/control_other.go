//go:build !darwin

package daemon

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"pomo/internal/db"
)

var errMacOnly = errors.New("daemon control is macOS-only")

// PidfilePath is the path to the daemon pidfile.
func PidfilePath() string { return filepath.Join(db.Dir(), "daemon.pid") }

// LockPath is the path to the daemon flock file.
func LockPath() string { return filepath.Join(db.Dir(), "daemon.lock") }

// LogPath is the path to the daemon log file.
func LogPath() string { return filepath.Join(db.Dir(), "daemon.log") }

// ReadPid parses the pidfile.
func ReadPid() (int, bool) {
	b, err := os.ReadFile(PidfilePath())
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return 0, false
	}
	return pid, true
}

// PidAlive is unsupported off darwin.
func PidAlive(pid int) bool { return false }

// Running is unsupported off darwin.
func Running() (pid int, ok bool) { return 0, false }

// Spawn is unsupported off darwin.
func Spawn() (int, error) { return 0, errMacOnly }

// Stop is unsupported off darwin.
func Stop() (int, error) { return 0, errMacOnly }
