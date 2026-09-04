package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pomo/internal/db/dbtest"
)

func TestRunDigestWritesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	database = dbtest.NewTemp(t)
	t.Cleanup(func() { database = nil })

	var buf bytes.Buffer
	if err := runDigest("", false, &buf); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSpace(buf.String())
	if !strings.HasSuffix(out, ".md") {
		t.Fatalf("expected a path, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".pomo", "reviews")); err != nil {
		t.Fatalf("reviews dir not created: %v", err)
	}
}
