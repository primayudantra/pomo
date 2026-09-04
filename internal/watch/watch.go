// Package watch reads the current foreground application and repo file
// activity for the drift daemon. Foreground detection is darwin-only for now
// (watch_other.go stubs the rest); Classify and RepoActive are portable.
package watch

import (
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"time"

	"pomo/internal/pomoconfig"
)

var ErrUnsupported = errors.New("watch: foreground detection not supported on this platform")

type ForegroundApp struct {
	Name     string
	BundleID string
	Title    string
}

type Watcher interface {
	Foreground() (ForegroundApp, error)
	Supported() bool
	Name() string
}

type Class string

const (
	Focus    Class = "focus"
	Neutral  Class = "neutral"
	Distract Class = "distract"
)

func matchesAny(hay string, needles []string) bool {
	hay = strings.ToLower(hay)
	for _, n := range needles {
		if n != "" && strings.Contains(hay, n) {
			return true
		}
	}
	return false
}

// Classify buckets a foreground reading. Precedence: explicit focus rule >
// distract rule (with title-hint downgrade) > neutral.
func Classify(app ForegroundApp, cfg pomoconfig.DriftConfig) Class {
	nameAndTitle := app.Name + ": " + app.Title
	id := app.BundleID

	focusHit := matchesAny(app.Name, cfg.FocusApps) || matchesAny(id, cfg.FocusApps)
	distractHit := matchesAny(app.Name, cfg.DistractApps) || matchesAny(id, cfg.DistractApps) ||
		matchesAny(nameAndTitle, cfg.DistractApps)

	if focusHit {
		return Focus
	}
	if distractHit {
		if app.Title != "" && matchesAny(app.Title, cfg.FocusTitleHints) {
			return Neutral
		}
		return Distract
	}
	return Neutral
}

var skipDirs = map[string]bool{
	".git": true, "node_modules": true, "vendor": true, ".venv": true,
	"venv": true, "dist": true, "build": true, "target": true, ".next": true,
	".idea": true, ".vscode": true,
}

// RepoActive reports whether any non-skipped file under repoPath has an mtime
// >= since. Early-exits on the first hit.
func RepoActive(repoPath string, since time.Time) bool {
	if repoPath == "" {
		return false
	}
	found := false
	_ = filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path != repoPath && skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if !info.ModTime().Before(since) {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return found
}
