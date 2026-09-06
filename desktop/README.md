# pomo-desktop

A macOS desktop shell for `pomo`, built with [Wails v2](https://wails.io) +
Svelte + TypeScript. macOS-only for v1.

## Prerequisites

- Go 1.25
- Node 20+ (dev machine has 22.x) and npm
- Xcode command line tools
- The Wails CLI, pinned:

  ```bash
  go install github.com/wailsapp/wails/v2/cmd/wails@v2.10.1
  # ensure $(go env GOPATH)/bin is on PATH
  ```

Run `wails doctor` to verify the environment. Optional tools (`upx`, `nsis`) are
not required.

## Develop / build

```bash
make desktop-dev     # cd desktop && wails dev  — live-reload dev window
make desktop-build   # cd desktop && wails build — produces build/bin/pomo-desktop.app
```

`make desktop-build` also copies the inner Mach-O binary to
`desktop/build/bin/pomo-desktop` for convenience.

## Notes

- Single Go module: this directory has **no** `go.mod`. The Wails dependency
  lives in the repo-root `go.mod` (`module pomo`); this package is `pomo/desktop`.
- **Single-instance lock:** `main.go` takes a non-blocking `flock` on
  `~/.pomo/desktop.lock` before `wails.Run`. A second launch prints
  `pomo-desktop is already running` and exits 0. Focusing the existing window
  from a second launch needs IPC and is deferred past v1.
- `frontend/dist/` and `frontend/node_modules/` and `build/bin/` are gitignored;
  `wails build` / `wails dev` regenerate them.
- **Window lifecycle (v1):** closing the window quits the app. If a session is
  running/paused, `OnBeforeClose` (`app.onBeforeClose`) first prompts
  (Leave it running / Cancel session / Stay); "Stay" keeps the window open,
  the other two let the close proceed. No hide-on-close in v1 — that returns
  with the menubar tray in v1.1.

## v1.1 backlog

- Menubar tray: live countdown in the menubar plus quick controls
  (pause/resume/stop). Deferred — v1 is window-only. The tray also brings back
  hide-on-close (closing the window leaves the app resident in the menubar).

## Manual checklist (run on a Mac)

- [ ] launch, run a 1-min session, quit mid-run, relaunch → timer rehydrates
- [ ] pause, quit, relaunch → still paused, correct remaining
- [ ] complete a session → chime + notification + break prompt
- [ ] Today totals update after a completed session
- [ ] tracking toggle on/off ↔ `pomo daemon status`
- [ ] external `pomo daemon stop` reflected in the UI within ~5s
- [ ] second `open pomo-desktop.app` → exits quietly, first instance intact
- [ ] `pomo start` in a terminal → desktop QuickStart shows the running-session error
