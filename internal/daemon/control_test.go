package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestReadPidRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir) // db.Dir() derives from HOME
	if err := os.MkdirAll(filepath.Join(dir, ".pomo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(PidfilePath(), []byte("4321\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pid, ok := ReadPid()
	if !ok || pid != 4321 {
		t.Fatalf("ReadPid() = %d, %v", pid, ok)
	}
}

func TestRunningFalseWhenPidDead(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(filepath.Join(dir, ".pomo"), 0o755)
	// A pid that is almost certainly not alive.
	_ = os.WriteFile(PidfilePath(), []byte(strconv.Itoa(999999)), 0o644)
	if _, ok := Running(); ok {
		t.Fatal("Running() should be false for a dead pid")
	}
}
