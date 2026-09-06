// Package pomoexec locates pomo's sibling binaries (the daemon reuses the
// main pomo binary; `pomo desktop` launches pomo-desktop). It looks next to
// the running executable first so a self-contained install works, then falls
// back to $PATH.
package pomoexec

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Find returns an absolute path to the named binary.
func Find(name string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return findIn(filepath.Dir(exe), name)
}

func findIn(dir, name string) (string, error) {
	cand := filepath.Join(dir, name)
	if fi, err := os.Stat(cand); err == nil && !fi.IsDir() {
		return cand, nil
	}
	if p, err := exec.LookPath(name); err == nil {
		abs, aerr := filepath.Abs(p)
		if aerr != nil {
			return p, nil
		}
		return abs, nil
	}
	return "", fmt.Errorf("%s not found next to %s or on $PATH", name, dir)
}
