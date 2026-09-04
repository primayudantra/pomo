//go:build darwin

package watch

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

func New() Watcher { return &darwinWatcher{} }

type darwinWatcher struct {
	titleDegraded bool
}

func (w *darwinWatcher) Supported() bool { return true }
func (w *darwinWatcher) Name() string    { return "darwin/lsappinfo" }

func run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return strings.TrimSpace(string(out)), err
}

func (w *darwinWatcher) Foreground() (ForegroundApp, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	name, err := run(ctx, "osascript", "-e",
		`tell application "System Events" to get name of first application process whose frontmost is true`)
	if err != nil || name == "" {
		return ForegroundApp{}, ErrUnsupported
	}
	app := ForegroundApp{Name: name}

	if bid, e := run(ctx, "osascript", "-e",
		`tell application "System Events" to get bundle identifier of first application process whose frontmost is true`); e == nil {
		app.BundleID = bid
	}

	app.Title = w.title(ctx, name)
	return app, nil
}

func (w *darwinWatcher) title(ctx context.Context, appName string) string {
	var script string
	switch {
	case strings.Contains(appName, "Chrome"), strings.Contains(appName, "Brave"),
		strings.Contains(appName, "Edge"), strings.Contains(appName, "Arc"):
		script = `tell application "` + appName + `" to get title of active tab of front window`
	case strings.Contains(appName, "Safari"):
		script = `tell application "Safari" to get name of current tab of front window`
	default:
		script = `tell application "System Events" to get name of front window of (first application process whose frontmost is true)`
	}
	title, err := run(ctx, "osascript", "-e", script)
	if err != nil {
		w.titleDegraded = true
		return ""
	}
	return title
}
