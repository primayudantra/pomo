package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPidfileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(filepath.Join(dir, ".pomo"), 0o755)

	if _, ok := readPidfile(); ok {
		t.Fatal("no pidfile yet, want ok=false")
	}
	if err := writePidfile(4242); err != nil {
		t.Fatal(err)
	}
	pid, ok := readPidfile()
	if !ok || pid != 4242 {
		t.Fatalf("readPidfile = %d, %v", pid, ok)
	}
	removePidfile()
	if _, ok := readPidfile(); ok {
		t.Fatal("pidfile should be gone after removePidfile")
	}
}

func TestPidAliveForSelf(t *testing.T) {
	if !pidAlive(os.Getpid()) {
		t.Fatal("current process should be alive")
	}
	if pidAlive(1 << 30) {
		t.Fatal("absurd pid should not be alive")
	}
}
