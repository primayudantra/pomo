package main

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"pomo/internal/db"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Single-instance lock: a second launch exits quietly. Focusing the
	// existing window would need IPC; that is deferred past v1.
	if lock, err := os.OpenFile(
		filepath.Join(db.Dir(), "desktop.lock"),
		os.O_CREATE|os.O_RDWR, 0o644); err == nil {
		if ferr := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); ferr != nil {
			fmt.Fprintln(os.Stderr, "pomo-desktop is already running")
			os.Exit(0)
		}
	}

	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "pomo-desktop",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		// Closing the window hides it instead of quitting; the real exit
		// path is App.Quit (which guards a running session). In v1 the
		// user gets the window back by clicking the dock icon — Wails'
		// default applicationShouldHandleReopen shows the hidden window.
		// A menubar tray with quick controls comes in v1.1.
		OnBeforeClose: func(ctx context.Context) (prevent bool) {
			if app.reallyQuit {
				return false
			}
			wr.WindowHide(ctx)
			return true
		},
		Bind: []interface{}{
			app,
			app.timer,
			app.session,
			app.daemonSvc,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
