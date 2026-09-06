package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"pomo/internal/db"
)

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

func writePidfile(pid int) error {
	if err := os.MkdirAll(db.Dir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(PidfilePath(), []byte(strconv.Itoa(pid)), 0o644)
}

func removePidfile() { _ = os.Remove(PidfilePath()) }

// Running returns the daemon pid if the pidfile points at a live process.
func Running() (pid int, ok bool) {
	pid, ok = ReadPid()
	if !ok || !PidAlive(pid) {
		return 0, false
	}
	return pid, true
}
