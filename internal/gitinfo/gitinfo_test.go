package gitinfo_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"pomo/internal/gitinfo"
)

func TestDescribeInRepo(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("config", "user.email", "t@example.com")
	run("config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(dir, "f"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "init")

	sub := filepath.Join(dir, "pkg")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	repo, branch := gitinfo.Describe(sub)
	if filepath.Base(repo) != filepath.Base(dir) {
		t.Errorf("repo = %q, want basename %q", repo, filepath.Base(dir))
	}
	if branch != "main" {
		t.Errorf("branch = %q, want main", branch)
	}
}

func TestDescribeOutsideRepo(t *testing.T) {
	repo, branch := gitinfo.Describe(t.TempDir())
	if repo != "" || branch != "" {
		t.Errorf("outside repo: got %q / %q, want empty", repo, branch)
	}
}
