package pomoexec

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestFindSibling(t *testing.T) {
	dir := t.TempDir()
	name := "pomo-fake"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	sib := filepath.Join(dir, name)
	if err := os.WriteFile(sib, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := findIn(dir, name)
	if err != nil {
		t.Fatal(err)
	}
	if got != sib {
		t.Errorf("got %q, want %q", got, sib)
	}
}

func TestFindMissing(t *testing.T) {
	if _, err := findIn(t.TempDir(), "definitely-not-here-xyz"); err == nil {
		t.Fatal("expected error for missing binary")
	}
}
