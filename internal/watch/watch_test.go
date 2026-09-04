package watch_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"pomo/internal/pomoconfig"
	"pomo/internal/watch"
)

func cfg() pomoconfig.DriftConfig {
	return pomoconfig.DriftConfig{
		FocusApps:       pomoconfig.DefaultFocusApps(),
		DistractApps:    pomoconfig.DefaultDistractApps(),
		FocusTitleHints: pomoconfig.DefaultFocusTitleHints(),
	}
}

func TestClassify(t *testing.T) {
	c := cfg()
	cases := []struct {
		app  watch.ForegroundApp
		want watch.Class
	}{
		{watch.ForegroundApp{Name: "iTerm2"}, watch.Focus},
		{watch.ForegroundApp{Name: "Visual Studio Code"}, watch.Focus},
		{watch.ForegroundApp{Name: "Slack"}, watch.Distract},
		{watch.ForegroundApp{Name: "Google Chrome", Title: "Reddit - dive into anything"}, watch.Distract},
		{watch.ForegroundApp{Name: "Google Chrome", Title: "pomo/plans at main · github.com"}, watch.Neutral},
		{watch.ForegroundApp{Name: "SomeUnknownApp"}, watch.Neutral},
	}
	for _, tc := range cases {
		if got := watch.Classify(tc.app, c); got != tc.want {
			t.Errorf("Classify(%q / %q) = %q, want %q", tc.app.Name, tc.app.Title, got, tc.want)
		}
	}
}

func TestClassifyUserOverrideWins(t *testing.T) {
	c := cfg()
	c.FocusApps = append(c.FocusApps, "figma")
	if got := watch.Classify(watch.ForegroundApp{Name: "Figma"}, c); got != watch.Focus {
		t.Errorf("user focus override: got %q", got)
	}
}

func TestRepoActive(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel string) {
		t.Helper()
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("main.go")
	past := time.Now().Add(-time.Hour)
	if !watch.RepoActive(dir, past) {
		t.Error("RepoActive should be true for a file newer than 1h ago")
	}
	future := time.Now().Add(time.Hour)
	if watch.RepoActive(dir, future) {
		t.Error("RepoActive should be false when nothing is newer than 1h ahead")
	}

	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	mustWrite(".git/COMMIT_EDITMSG")
	if watch.RepoActive(dir, future) {
		t.Error("changes under .git must be ignored")
	}
	if watch.RepoActive("", past) {
		t.Error("empty path must be false")
	}
}
