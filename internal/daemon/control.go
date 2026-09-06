//go:build darwin

package daemon

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"pomo/internal/db"
	"pomo/internal/pomoexec"
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

// PidAlive reports whether the given pid is a live process.
func PidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

// Running returns the daemon pid if the pidfile points at a live process.
func Running() (pid int, ok bool) {
	pid, ok = ReadPid()
	if !ok || !PidAlive(pid) {
		return 0, false
	}
	return pid, true
}

// Spawn starts the drift daemon as a detached background process. It is
// idempotent: if the daemon is already running, it returns the existing pid.
func Spawn() (int, error) {
	if pid, ok := Running(); ok {
		return pid, nil
	}
	bin, err := pomoexec.Find("pomo")
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(db.Dir(), 0o755); err != nil {
		return 0, err
	}
	lf, err := os.OpenFile(LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer lf.Close()

	child := exec.Command(bin, "daemon", "run")
	child.Stdout = lf
	child.Stderr = lf
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		return 0, err
	}
	pid := child.Process.Pid
	if err := writePidfile(pid); err != nil {
		return 0, err
	}
	_ = child.Process.Release()
	return pid, nil
}

// Stop sends SIGTERM to the running daemon and removes the pidfile.
func Stop() (int, error) {
	pid, ok := Running()
	if !ok {
		removePidfile()
		return 0, nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return pid, err
	}
	removePidfile()
	return pid, nil
}
