// Package gitinfo resolves the git repository and branch for a directory,
// shelling out to the git binary. Every function returns "" rather than an
// error when git is missing or the directory is not a repo.
package gitinfo

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

func gitOut(dir string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Toplevel is the repo root containing dir, or "".
func Toplevel(dir string) string {
	if dir == "" {
		return ""
	}
	return gitOut(dir, "rev-parse", "--show-toplevel")
}

// Branch is the current branch name for dir, or "".
func Branch(dir string) string {
	if dir == "" {
		return ""
	}
	b := gitOut(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if b == "HEAD" { // detached
		return ""
	}
	return b
}

// Describe returns Toplevel and Branch together.
func Describe(dir string) (repoPath, branch string) {
	return Toplevel(dir), Branch(dir)
}
