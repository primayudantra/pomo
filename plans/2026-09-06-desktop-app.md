# Desktop App Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a native macOS desktop app for pomo — quick-start a task, run a
Pomodoro with a visible countdown, see today's sessions, and toggle the drift
daemon — built on the existing Go core.

**Architecture:** A Wails v2 app in a new top-level `desktop/` directory. The Go
backend owns the timer (a state machine + 1s ticker goroutine) and writes
`sessions` rows through `internal/db`, exactly like the CLI. A Svelte frontend
in the native webview renders three screens and reacts to backend events. The
app is a third independent client of `~/.pomo/pomo.db` (no IPC). A new
`pomo desktop` Cobra command locates and execs the built app.

**Tech Stack:** Go 1.25, Wails v2 (`github.com/wailsapp/wails/v2`), Svelte +
TypeScript + Vite (Wails `svelte-ts` template), `fyne.io/systray` for the
menubar, `modernc.org/sqlite` (already vendored, pure Go, no cgo).

**Spec:** `specs/2026-09-06-desktop-app.md`

## Global Constraints

- Go 1.25 (`go.mod` says `go 1.25.0`). No cgo anywhere — keep the pure-Go build.
- macOS only for v1. `//go:build darwin` on any platform-specific file; provide
  a `_other.go` stub so `go build ./...` still passes on Linux CI.
- Only one session may be `status = 'running'` at a time. Every start path
  calls `database.LastRunningSession()` first and refuses if one exists.
- Session status values are exactly: `running`, `completed`, `cancelled`,
  `skipped`, `interrupted` (`internal/model`). Do not invent new ones.
- Focus time in reports comes from `sessions.actual_duration` (seconds, int),
  set when a session finishes. The timer must pass a pause-excluded value.
- Additive schema changes only: new columns via `ensureColumn` in
  `db.OpenAt` (never edit the `CREATE TABLE` string for existing tables).
- `cmd/` stays Cobra-only. Root `main.go` is untouched. The desktop app is a
  separate binary built from `desktop/`.
- Durations are stored as int **seconds** everywhere in the DB.
- Frontend build output and `node_modules` are gitignored; end users get a
  self-contained `.app`.

---

## File Structure

**New files:**

| Path | Responsibility |
|---|---|
| `internal/daemon/control.go` | `Spawn()` / `Stop()` / `Running()` — process lifecycle for the drift daemon, moved out of `cmd/daemon.go` |
| `internal/daemon/control_test.go` | tests for the control functions with a fake spawn |
| `internal/pomoexec/pomoexec.go` | `Find(name string) (string, error)` — locate a sibling binary next to the running executable, then `$PATH` |
| `cmd/desktop.go` | `pomo desktop` — exec the `pomo-desktop` app |
| `desktop/main.go` | Wails bootstrap, bind services, start tray, single-instance lock |
| `desktop/app.go` | `App` struct — holds `context.Context`, `*db.DB`, service refs; startup/shutdown hooks |
| `desktop/timer.go` | `TimerService` — state machine, ticker goroutine, DB writes, rehydrate |
| `desktop/timer_test.go` | `TimerService` table tests with an injected clock |
| `desktop/session.go` | `SessionService` — `Today()`, thin read wrapper over `internal/db` |
| `desktop/session_test.go` | `SessionService` tests against `db.OpenAt(tmpfile)` |
| `desktop/daemonsvc.go` | `DaemonService` — `Status()` / `SetTracking(bool)` over `internal/daemon` |
| `desktop/daemonsvc_test.go` | `DaemonService` tests with a fake controller |
| `desktop/tray.go` | menubar title updater + menu, driven by timer state changes |
| `desktop/events.go` | event-name constants shared backend↔frontend |
| `desktop/clock.go` | `Clock` interface + `realClock` + `fakeClock` (test helper) |
| `desktop/frontend/src/App.svelte` | root component, derives `view` from timer phase |
| `desktop/frontend/src/lib/stores.ts` | `timer`, `today`, `daemon` Svelte stores + event wiring |
| `desktop/frontend/src/lib/format.ts` | `mmss(seconds)` helper |
| `desktop/frontend/src/screens/QuickStart.svelte` | task input + duration chips |
| `desktop/frontend/src/screens/Timer.svelte` | countdown, ring, pause/cancel, break prompt |
| `desktop/frontend/src/screens/Today.svelte` | session list, totals, tracking toggle |
| `desktop/frontend/src/app.css` | dark-first theme tokens |
| `desktop/README.md` | how to dev/build the desktop app |

**Modified files:**

| Path | Change |
|---|---|
| `internal/db/db.go` | add `paused_at` + `pause_accum_secs` columns in `OpenAt`; add `SetPaused`, `ClearPaused`, `RunningSessionAny` helpers |
| `cmd/daemon.go` | delete the moved helpers; call `internal/daemon` control functions |
| `Makefile` | `desktop-dev`, `desktop-build` targets |
| `.gitignore` | `desktop/frontend/node_modules`, `desktop/frontend/dist`, `desktop/build/bin` |
| `go.mod` / `go.sum` | add Wails v2 + `fyne.io/systray` |
| `README.md` | one line pointing at `desktop/README.md` |

---

## Task 1: Pause-tracking schema columns + DB helpers

**Files:**
- Modify: `internal/db/db.go`
- Test: `internal/db/session_repo_test.go`

**Interfaces:**
- Consumes: existing `db.OpenAt(path string) (*DB, error)`, `ensureColumn`.
- Produces:
  - column `sessions.paused_at TEXT` (nullable, RFC3339 or empty)
  - column `sessions.pause_accum_secs INTEGER NOT NULL DEFAULT 0`
  - `func (d *DB) SetPaused(id int64, at time.Time) error`
  - `func (d *DB) ClearPaused(id int64, addSecs int) error` — clears `paused_at`, adds `addSecs` to `pause_accum_secs`
  - `model.Session` gains `PausedAt *time.Time` and `PauseAccumSecs int`; all
    `SELECT` column lists and `scanSession` / `ListSessions` scan loop include
    the two new columns (append them last, after `repo_branch`).

- [ ] **Step 1: Write the failing test**

Add to `internal/db/session_repo_test.go`:

```go
func TestPauseTracking(t *testing.T) {
	d := openTestDB(t) // existing helper; if absent, use: d, _ := db.OpenAt(filepath.Join(t.TempDir(), "p.db"))
	id, err := d.CreateSession(model.Session{
		TaskName: "x", PlannedDuration: 1500,
		Status: model.StatusRunning, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	pausedAt := time.Now()
	if err := d.SetPaused(id, pausedAt); err != nil {
		t.Fatal(err)
	}
	got, err := d.LastRunningSession()
	if err != nil {
		t.Fatal(err)
	}
	if got.PausedAt == nil {
		t.Fatal("PausedAt should be set")
	}

	if err := d.ClearPaused(id, 42); err != nil {
		t.Fatal(err)
	}
	got, _ = d.LastRunningSession()
	if got.PausedAt != nil {
		t.Errorf("PausedAt should be cleared, got %v", got.PausedAt)
	}
	if got.PauseAccumSecs != 42 {
		t.Errorf("PauseAccumSecs = %d, want 42", got.PauseAccumSecs)
	}

	// ClearPaused accumulates.
	_ = d.SetPaused(id, time.Now())
	_ = d.ClearPaused(id, 8)
	got, _ = d.LastRunningSession()
	if got.PauseAccumSecs != 50 {
		t.Errorf("PauseAccumSecs = %d, want 50", got.PauseAccumSecs)
	}
}
```

If `openTestDB` does not exist in the package, check the top of
`session_repo_test.go` for the pattern the other tests use and match it.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/db/ -run TestPauseTracking -v`
Expected: FAIL — `got.PausedAt undefined` / `SetPaused undefined`.

- [ ] **Step 3: Add the columns**

In `internal/db/db.go`, `OpenAt`, extend the `ensureColumn` slice:

```go
	for _, c := range []struct{ table, col, ddl string }{
		{"sessions", "repo_path", "repo_path TEXT DEFAULT ''"},
		{"sessions", "repo_branch", "repo_branch TEXT DEFAULT ''"},
		{"sessions", "paused_at", "paused_at TEXT DEFAULT ''"},
		{"sessions", "pause_accum_secs", "pause_accum_secs INTEGER NOT NULL DEFAULT 0"},
	} {
```

- [ ] **Step 4: Extend the model**

In `internal/model/session.go` (the file holding `Session`), add fields:

```go
	RepoPath       string
	RepoBranch     string
	PausedAt       *time.Time
	PauseAccumSecs int
```

- [ ] **Step 5: Scan the new columns**

In `internal/db/db.go`, every `SELECT ... FROM sessions` that feeds
`scanSession` or the `ListSessions` loop: append `, paused_at, pause_accum_secs`
to the column list (last, after `repo_branch`). Update `scanSession`:

```go
func scanSession(row *sql.Row) (*model.Session, error) {
	var s model.Session
	var completedAt sql.NullTime
	var pausedAt sql.NullString
	if err := row.Scan(&s.ID, &s.TaskID, &s.TaskName, &s.Tag, &s.PlannedDuration, &s.ActualDuration,
		&s.Status, &s.Note, &s.StartedAt, &completedAt, &s.CreatedAt, &s.RepoPath, &s.RepoBranch,
		&pausedAt, &s.PauseAccumSecs); err != nil {
		return nil, err
	}
	if completedAt.Valid {
		s.CompletedAt = &completedAt.Time
	}
	if pausedAt.Valid && pausedAt.String != "" {
		if t, err := time.Parse(time.RFC3339, pausedAt.String); err == nil {
			s.PausedAt = &t
		}
	}
	return &s, nil
}
```

Apply the same two-column addition and parse to the `ListSessions` row loop
(it has its own inline scan).

- [ ] **Step 6: Add the helper methods**

In `internal/db/db.go`, in the `--- Sessions ---` block:

```go
func (d *DB) SetPaused(id int64, at time.Time) error {
	_, err := d.Exec(`UPDATE sessions SET paused_at = ? WHERE id = ?`,
		at.Format(time.RFC3339), id)
	return err
}

func (d *DB) ClearPaused(id int64, addSecs int) error {
	_, err := d.Exec(`UPDATE sessions
		SET paused_at = '', pause_accum_secs = pause_accum_secs + ?
		WHERE id = ?`, addSecs, id)
	return err
}
```

- [ ] **Step 7: Run tests**

Run: `go test ./internal/db/ ./internal/model/ ./internal/report/ -v`
Expected: PASS. (Report tests must still pass — the new columns are additive
and default safely.)

- [ ] **Step 8: Commit**

```bash
git add internal/db/db.go internal/model/session.go internal/db/session_repo_test.go
git commit -m "feat(db): track per-session pause time for the desktop timer"
```

---

## Task 2: `pomoexec.Find` — sibling binary locator

**Files:**
- Create: `internal/pomoexec/pomoexec.go`
- Test: `internal/pomoexec/pomoexec_test.go`

**Interfaces:**
- Produces: `func Find(name string) (string, error)` — returns an absolute path
  to `name`, searching (1) the directory of `os.Executable()`, then (2)
  `exec.LookPath(name)`. Error text: `fmt.Errorf("%s not found next to %s or on $PATH", name, exeDir)`.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/pomoexec/ -v`
Expected: FAIL — package/`findIn` does not exist.

- [ ] **Step 3: Implement**

```go
// Package pomoexec locates pomo's sibling binaries (the daemon reuses the
// main pomo binary; `pomo desktop` launches pomo-desktop). It looks next to
// the running executable first so a self-contained install works, then falls
// back to $PATH.
package pomoexec

import (
	"errors"
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

var _ = errors.New
```

(Drop the `var _ = errors.New` line — it is only there to remind you not to
add an unused import; remove `errors` from imports too.)

- [ ] **Step 4: Run tests**

Run: `go test ./internal/pomoexec/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/pomoexec/
git commit -m "feat: pomoexec.Find locates sibling pomo binaries"
```

---

## Task 3: Extract daemon process control into `internal/daemon`

**Files:**
- Create: `internal/daemon/control.go`
- Create: `internal/daemon/control_test.go`
- Modify: `cmd/daemon.go`

**Interfaces:**
- Consumes: `db.Dir()`, `pomoexec.Find`.
- Produces (all in package `daemon`):
  - `func PidfilePath() string`, `func LockPath() string`, `func LogPath() string`
  - `func ReadPid() (int, bool)` — parse the pidfile
  - `func PidAlive(pid int) bool`
  - `func Running() (pid int, ok bool)` — `ReadPid` + `PidAlive`
  - `func Spawn() (int, error)` — if already `Running()`, returns that pid, nil.
    Else finds the `pomo` binary via `pomoexec.Find("pomo")`, starts
    `pomo daemon run` detached (`Setsid`, stdio → `LogPath()`), writes the
    pidfile, returns the new pid.
  - `func Stop() (int, error)` — `SIGTERM` the running pid, remove the pidfile.
    Returns the pid it signalled, or `(0, nil)` if not running.
  - keep `//go:build darwin` on the `syscall.Kill` / `Setsid` parts; add
    `control_other.go` with `//go:build !darwin` stubs returning
    `errors.New("daemon control is macOS-only")` so `go build ./...` passes.

- [ ] **Step 1: Write the failing test**

`internal/daemon/control_test.go`:

```go
package daemon

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestReadPidRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir) // db.Dir() derives from HOME
	if err := os.MkdirAll(filepath.Join(dir, ".pomo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(PidfilePath(), []byte("4321\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	pid, ok := ReadPid()
	if !ok || pid != 4321 {
		t.Fatalf("ReadPid() = %d, %v", pid, ok)
	}
}

func TestRunningFalseWhenPidDead(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(filepath.Join(dir, ".pomo"), 0o755)
	// A pid that is almost certainly not alive.
	_ = os.WriteFile(PidfilePath(), []byte(strconv.Itoa(999999)), 0o644)
	if _, ok := Running(); ok {
		t.Fatal("Running() should be false for a dead pid")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/daemon/ -run 'TestReadPid|TestRunningFalse' -v`
Expected: FAIL — `PidfilePath` / `ReadPid` / `Running` undefined.

- [ ] **Step 3: Create `control.go`**

Move the bodies of `pidfilePath`, `lockPath`, `logPath`, `readPidfile`,
`writePidfile`, `removePidfile`, `pidAlive` out of `cmd/daemon.go` into
`internal/daemon/control.go`, renaming the exported ones as listed in
Interfaces. Then add:

```go
// Spawn starts the drift daemon as a detached background process. It is
// idempotent: if the daemon is already running, it returns the existing pid.
func Spawn() (int, error) {
	if pid, ok := Running(); ok {
		return pid, nil
	}
	bin, err := pomoexec.Find("pomo")
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(db.Dir(), 0o755); err != nil {
		return 0, err
	}
	lf, err := os.OpenFile(LogPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, err
	}
	defer lf.Close()

	child := exec.Command(bin, "daemon", "run")
	child.Stdout = lf
	child.Stderr = lf
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		return 0, err
	}
	pid := child.Process.Pid
	if err := writePidfile(pid); err != nil {
		return 0, err
	}
	_ = child.Process.Release()
	return pid, nil
}

// Stop sends SIGTERM to the running daemon and removes the pidfile.
func Stop() (int, error) {
	pid, ok := Running()
	if !ok {
		removePidfile()
		return 0, nil
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		return pid, err
	}
	removePidfile()
	return pid, nil
}
```

Put `//go:build darwin` at the top of `control.go`. Create
`internal/daemon/control_other.go` with `//go:build !darwin` and stub
`Spawn`/`Stop`/`PidAlive`/`Running` returning zero values + an error where the
signature allows.

- [ ] **Step 4: Repoint `cmd/daemon.go`**

Delete the moved helper funcs from `cmd/daemon.go`. Rewrite the command
bodies to call the package:

```go
var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the daemon as a detached background process",
	RunE: func(c *cobra.Command, args []string) error {
		pid, err := daemon.Spawn()
		if err != nil {
			return err
		}
		fmt.Printf("daemon running (pid %d), logging to %s\n", pid, daemon.LogPath())
		return nil
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the daemon",
	RunE: func(c *cobra.Command, args []string) error {
		pid, err := daemon.Stop()
		if err != nil {
			return err
		}
		if pid == 0 {
			fmt.Println("daemon not running")
		} else {
			fmt.Printf("daemon stopped (pid %d)\n", pid)
		}
		return nil
	},
}
```

For `daemonStatusCmd`, replace `readPidfile()`/`pidAlive()` with
`daemon.Running()`. `runDaemon` still uses `lockPath()` — change it to
`daemon.LockPath()`. Remove now-unused imports (`os/exec`, `strconv`,
`strings`, `path/filepath` if nothing else needs them — let the compiler tell
you).

- [ ] **Step 5: Run the full test + build**

Run: `go build ./... && go test ./internal/daemon/ ./cmd/... -v`
Expected: PASS. The existing `daemon_test.go` must still pass unchanged.

- [ ] **Step 6: Manual smoke**

```bash
go run . daemon start && go run . daemon status && go run . daemon stop
```
Expected: starts, reports `running (pid N)`, stops cleanly.

- [ ] **Step 7: Commit**

```bash
git add internal/daemon/ cmd/daemon.go
git commit -m "refactor(daemon): move process control into internal/daemon"
```

---

## Task 4: `pomo desktop` command

**Files:**
- Create: `cmd/desktop.go`

**Interfaces:**
- Consumes: `pomoexec.Find`.
- Produces: `pomo desktop` — locates `pomo-desktop`, execs it with
  `syscall.Exec` (replace the process so the terminal is not held), passing
  through any extra args. On not-found, returns the `pomoexec` error plus a
  hint: `"build it with: make desktop-build"`.

- [ ] **Step 1: Write the command**

```go
//go:build darwin

package cmd

import (
	"fmt"
	"os"
	"syscall"

	"github.com/spf13/cobra"

	"pomo/internal/pomoexec"
)

var desktopCmd = &cobra.Command{
	Use:   "desktop",
	Short: "Open the pomo desktop app",
	Args:  cobra.ArbitraryArgs,
	RunE: func(c *cobra.Command, args []string) error {
		bin, err := pomoexec.Find("pomo-desktop")
		if err != nil {
			return fmt.Errorf("%w\nbuild it with: make desktop-build", err)
		}
		argv := append([]string{bin}, args...)
		return syscall.Exec(bin, argv, os.Environ())
	},
}

func init() {
	rootCmd.AddCommand(desktopCmd)
}
```

Add `cmd/desktop_other.go` (`//go:build !darwin`) registering a `desktop`
command whose `RunE` returns `errors.New("pomo desktop is macOS-only")`, so
the command still lists on Linux.

- [ ] **Step 2: Build**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 3: Manual check**

```bash
go run . desktop
```
Expected: error `pomo-desktop not found ... build it with: make desktop-build`
(the app does not exist yet — that is correct).

- [ ] **Step 4: Commit**

```bash
git add cmd/desktop.go cmd/desktop_other.go
git commit -m "feat(cmd): pomo desktop launches the desktop app"
```

---

## Task 5: Wails + Svelte scaffold, Makefile, gitignore, single-instance lock

**Files:**
- Create: `desktop/` (Wails scaffold), `desktop/events.go`, `desktop/clock.go`,
  `desktop/README.md`
- Modify: `Makefile`, `.gitignore`, `README.md`, `go.mod`, `go.sum`

**Interfaces:**
- Produces:
  - `desktop/main.go` with `wails.Run` binding an `*App` (Task 6 fills the
    services in).
  - `desktop/events.go`: `const (EventTick = "timer:tick"; EventPhase =
    "timer:phase"; EventError = "timer:error"; EventToday = "today:changed")`
  - `desktop/clock.go`: `type Clock interface { Now() time.Time }`,
    `type realClock struct{}`, `func (realClock) Now() time.Time { return time.Now() }`
  - a `make desktop-build` that produces `build/bin/pomo-desktop.app` and a
    convenience copy of the inner binary to `build/bin/pomo-desktop`.

- [ ] **Step 1: Install the Wails CLI (one-time, document it)**

Run: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
Then: `wails doctor`
Expected: reports a working environment (Node 20+, Xcode CLT). Record any
missing deps in `desktop/README.md`.

- [ ] **Step 2: Scaffold into `desktop/`**

Run from the repo root:

```bash
wails init -n pomo-desktop -t svelte-ts -d desktop
```

This creates `desktop/` with `main.go`, `app.go`, `wails.json`,
`frontend/` (Svelte + Vite + TS), and `frontend/wailsjs/` bindings. It also
adds `github.com/wailsapp/wails/v2` to `go.mod`.

- [ ] **Step 3: Make the module path consistent**

`wails init` may write `desktop/go.mod`. Delete it — this repo is a single
module. Move the Wails require into the root `go.mod`:

```bash
rm -f desktop/go.mod desktop/go.sum
go get github.com/wailsapp/wails/v2@v2.10.1
go get fyne.io/systray@latest
go mod tidy
```

Fix `desktop/main.go` / `desktop/app.go` import paths to `pomo/desktop/...`
if the scaffold used a bare module name.

- [ ] **Step 4: Verify the scaffold builds and runs**

Run: `cd desktop && wails dev`
Expected: a window opens with the default Wails+Svelte template. Close it.
Then: `cd desktop && wails build` → `desktop/build/bin/pomo-desktop.app`.

- [ ] **Step 5: Add `events.go` and `clock.go`**

Write the two files exactly as in Interfaces above.

- [ ] **Step 6: Single-instance lock in `main.go`**

In `desktop/main.go`, before `wails.Run`:

```go
lock, err := os.OpenFile(
	filepath.Join(db.Dir(), "desktop.lock"),
	os.O_CREATE|os.O_RDWR, 0o644)
if err == nil {
	if ferr := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); ferr != nil {
		fmt.Fprintln(os.Stderr, "pomo-desktop is already running")
		os.Exit(0)
	}
}
```

(Focusing the existing window from a second launch needs IPC; for v1, exiting
quietly is acceptable — note it in `desktop/README.md`.)

- [ ] **Step 7: Makefile targets**

Add:

```makefile
desktop-dev:
	cd desktop && wails dev

desktop-build:
	cd desktop && wails build
	cp desktop/build/bin/pomo-desktop.app/Contents/MacOS/pomo-desktop desktop/build/bin/pomo-desktop
```

- [ ] **Step 8: gitignore + README**

Append to `.gitignore`:

```
desktop/frontend/node_modules/
desktop/frontend/dist/
desktop/build/bin/
```

`desktop/README.md`: dev = `make desktop-dev`, build = `make desktop-build`,
prerequisites (wails CLI, Node 20+), the "already running → exits" caveat.
Add one line to the root `README.md` pointing at it.

- [ ] **Step 9: Build the whole repo**

Run: `go build ./... && go vet ./... && go test ./...`
Expected: PASS (frontend not involved).

- [ ] **Step 10: Commit**

```bash
git add desktop/ Makefile .gitignore README.md go.mod go.sum
git commit -m "feat(desktop): Wails v2 + Svelte scaffold and build targets"
```

---

## Task 6: TimerService — state machine, ticker, DB writes, rehydrate

**Files:**
- Create: `desktop/timer.go`
- Create: `desktop/timer_test.go`
- Modify: `desktop/app.go` (hold the service, wire startup/shutdown)

**Interfaces:**
- Consumes: `*db.DB`, `Clock` (Task 5), `pomoconfig.Load`, `internal/model`,
  `internal/sound`, `internal/notify`, Wails `runtime.EventsEmit`.
- Produces:

```go
type Phase string
const (
	PhaseIdle        Phase = "idle"
	PhaseRunning     Phase = "running"
	PhasePaused      Phase = "paused"
	PhaseBreakPrompt Phase = "break_prompt"
	PhaseBreak       Phase = "break"
)

type State struct {
	Phase     Phase  `json:"phase"`
	Task      string `json:"task"`
	Remaining int    `json:"remaining"` // seconds
	Duration  int    `json:"duration"`  // seconds
	SessionID int64  `json:"sessionId"`
}

type TimerService struct { /* unexported fields */ }

func NewTimerService(d *db.DB, clk Clock) *TimerService

// Bound, called from the frontend:
func (t *TimerService) GetState() State
func (t *TimerService) Start(task string, minutes int) (State, error) // err if a session already runs
func (t *TimerService) Pause() State
func (t *TimerService) Resume() State
func (t *TimerService) Cancel() State
func (t *TimerService) StartBreak(minutes int) State
func (t *TimerService) SkipBreak() State

// Called from main once the Wails ctx exists:
func (t *TimerService) Attach(ctx context.Context) // stores ctx for EventsEmit, starts the ticker, calls rehydrate
func (t *TimerService) Shutdown()                  // stops the ticker goroutine
```

**Design notes for the implementer:**
- All mutable state sits behind one `sync.Mutex`. The ticker goroutine and the
  bound methods both take it.
- `remaining` is always recomputed, never decremented:
  `remaining = duration - elapsedFocus`, where
  `elapsedFocus = int(now.Sub(startedAt)/time.Second) - pauseAccum - curPause`
  and `curPause = int(now.Sub(pausedAt)/time.Second)` when paused, else 0.
- On `Start`: `LastRunningSession()` guard → error if non-nil. Then
  `FindOrCreateTask` + `CreateSession(status running, PlannedDuration =
  minutes*60, StartedAt = clk.Now())`. `repo_path`/`repo_branch` left `""`.
- On `Pause`: `db.SetPaused(id, clk.Now())`, phase → paused.
- On `Resume`: `db.ClearPaused(id, secondsSincePausedAt)`, phase → running.
- On natural completion (`remaining <= 0` while running): call
  `db.FinishSession(id, StatusCompleted, actual, "")` where
  `actual = duration - <nothing>` → actually `actual = elapsedFocus capped at
  duration`; then `sound.PlayFinish()`, `notify.New().Send("Pomodoro done",
  task)`, phase → break_prompt, emit `EventPhase`.
- On `Cancel`: `db.FinishSession(id, StatusCancelled, elapsedFocus, "")`,
  phase → idle.
- On `StartBreak(minutes)`: no DB row (breaks are not sessions in this schema);
  phase → break with its own countdown; when it hits 0 → idle. `SkipBreak` →
  idle immediately.
- The ticker fires every 1s: recompute, emit `EventTick` with `GetState()`;
  when a running or break countdown crosses 0, perform the transition above.
- `Attach` → `rehydrate`: `LastRunningSession()`. If nil → idle. Else compute
  `elapsedFocus`:
  - if `>= PlannedDuration`: `FinishSession(StatusCompleted, PlannedDuration)`,
    phase → idle (the break window is long gone).
  - else if `session.PausedAt != nil`: phase → paused, restore `pausedAt`.
  - else: phase → running.
  All from the stored `StartedAt` + `PauseAccumSecs` + `PausedAt`.

- [ ] **Step 1: Write the failing tests**

`desktop/timer_test.go` — use a `fakeClock` you add here:

```go
package main

import (
	"path/filepath"
	"testing"
	"time"

	"pomo/internal/db"
	"pomo/internal/model"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) Now() time.Time     { return f.t }
func (f *fakeClock) add(d time.Duration) { f.t = f.t.Add(d) }

func newTestTimer(t *testing.T) (*TimerService, *fakeClock, *db.DB) {
	t.Helper()
	d, err := db.OpenAt(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	clk := &fakeClock{t: time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC)}
	return NewTimerService(d, clk), clk, d
}

func TestStartThenComplete(t *testing.T) {
	ts, clk, d := newTestTimer(t)
	st, err := ts.Start("write plan", 25)
	if err != nil {
		t.Fatal(err)
	}
	if st.Phase != PhaseRunning || st.Remaining != 1500 {
		t.Fatalf("bad start state: %+v", st)
	}

	clk.add(25 * time.Minute)
	ts.tick() // unexported: one ticker iteration
	if got := ts.GetState().Phase; got != PhaseBreakPrompt {
		t.Fatalf("phase = %s, want break_prompt", got)
	}
	s, _ := d.LastRunningSession()
	if s != nil {
		t.Fatal("session should be finished")
	}
	all, _ := d.ListSessions(db.SessionFilter{})
	if all[0].Status != model.StatusCompleted || all[0].ActualDuration != 1500 {
		t.Fatalf("finished row wrong: %+v", all[0])
	}
}

func TestPauseExcludedFromElapsed(t *testing.T) {
	ts, clk, _ := newTestTimer(t)
	ts.Start("x", 25)
	clk.add(5 * time.Minute)
	ts.Pause()
	clk.add(10 * time.Minute) // paused — should not count
	ts.Resume()
	clk.add(5 * time.Minute)
	ts.tick()
	// 10 min of real focus elapsed → 15 min remaining.
	if got := ts.GetState().Remaining; got != 900 {
		t.Fatalf("remaining = %d, want 900", got)
	}
}

func TestRehydrateRunning(t *testing.T) {
	ts, clk, d := newTestTimer(t)
	ts.Start("resume me", 25)
	clk.add(10 * time.Minute)

	// New service, same db + a clock 10 min after start.
	ts2 := NewTimerService(d, clk)
	ts2.rehydrate()
	st := ts2.GetState()
	if st.Phase != PhaseRunning || st.Remaining != 900 {
		t.Fatalf("rehydrate state = %+v", st)
	}
}

func TestRehydrateExpiredCompletes(t *testing.T) {
	ts, clk, d := newTestTimer(t)
	ts.Start("done while away", 25)
	clk.add(40 * time.Minute)

	ts2 := NewTimerService(d, clk)
	ts2.rehydrate()
	if ts2.GetState().Phase != PhaseIdle {
		t.Fatal("expired session should rehydrate to idle")
	}
	all, _ := d.ListSessions(db.SessionFilter{})
	if all[0].Status != model.StatusCompleted {
		t.Fatalf("status = %s, want completed", all[0].Status)
	}
}

func TestStartRefusesSecondSession(t *testing.T) {
	ts, _, _ := newTestTimer(t)
	ts.Start("first", 25)
	if _, err := ts.Start("second", 25); err == nil {
		t.Fatal("expected error starting a second session")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./desktop/ -run Test -v`
Expected: FAIL — `NewTimerService` undefined.

- [ ] **Step 3: Implement `timer.go`**

Write `TimerService` per the Interfaces + Design notes. Structure:
- fields: `mu sync.Mutex`, `db *db.DB`, `clk Clock`, `ctx context.Context`,
  `st State`, plus rehydrate-support fields `startedAt time.Time`,
  `pausedAt time.Time`, `pauseAccum int`, `breakEndsAt time.Time`,
  `stop chan struct{}`.
- `tick()` (unexported): lock, recompute, handle a 0-crossing, emit. `Attach`
  launches `go t.loop()` which is `for { select { case <-time.After(time.Second): t.tick() ... case <-t.stop: return } }`.
- `emit(event string, payload any)`: `if t.ctx != nil { runtime.EventsEmit(t.ctx, event, payload) }` — guarded so tests (no ctx) do not panic.
- helper `elapsedFocus(now time.Time) int` implementing the formula in Design
  notes; used by both `recompute` and `rehydrate`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./desktop/ -run Test -v`
Expected: PASS (all six).

- [ ] **Step 5: Wire into `app.go` / `main.go`**

In `desktop/app.go`, give `App` a `timer *TimerService` field, construct it in
`NewApp` with `db.Open()` + `realClock{}`. In the Wails `OnStartup` hook call
`a.timer.Attach(ctx)`; in `OnShutdown` call `a.timer.Shutdown()` then
`a.db.Close()`. In `main.go`, add `a.timer` to the `Bind` slice.

- [ ] **Step 6: Build + manual**

Run: `go build ./... && cd desktop && wails dev`
In the dev window, open the browser devtools console and run
`window.go.main.TimerService.Start("test", 1)` — expect the returned state and
`timer:tick` events in the console after wiring the frontend (next tasks). For
now just confirm no crash and the method is present.

- [ ] **Step 7: Commit**

```bash
git add desktop/timer.go desktop/timer_test.go desktop/app.go desktop/main.go
git commit -m "feat(desktop): TimerService state machine with pause + rehydrate"
```

---

## Task 7: SessionService — today's sessions

**Files:**
- Create: `desktop/session.go`
- Create: `desktop/session_test.go`
- Modify: `desktop/main.go` (bind), `desktop/timer.go` (emit `EventToday` after finish/start)

**Interfaces:**
- Consumes: `*db.DB`, `internal/report` day helpers if convenient (else compute
  inline).
- Produces:

```go
type SessionRow struct {
	Task     string `json:"task"`
	Minutes  int    `json:"minutes"`  // actual_duration / 60, rounded
	Status   string `json:"status"`
	StartedAt string `json:"startedAt"` // RFC3339, for ordering/display
}

type TodayView struct {
	Date         string       `json:"date"`        // "Mon 6 Sep"
	FocusMinutes int          `json:"focusMinutes"`// sum of completed actual_duration
	Count        int          `json:"count"`       // completed sessions today
	Sessions     []SessionRow `json:"sessions"`
}

type SessionService struct{ db *db.DB }
func NewSessionService(d *db.DB) *SessionService
func (s *SessionService) Today() (TodayView, error)
```

- [ ] **Step 1: Write the failing test**

```go
func TestTodayView(t *testing.T) {
	d, _ := db.OpenAt(filepath.Join(t.TempDir(), "p.db"))
	now := time.Now()
	id, _ := d.CreateSession(model.Session{
		TaskName: "ship", PlannedDuration: 1500,
		Status: model.StatusRunning, StartedAt: now,
	})
	_ = d.FinishSession(id, model.StatusCompleted, 1500, "")

	id2, _ := d.CreateSession(model.Session{
		TaskName: "email", PlannedDuration: 1500,
		Status: model.StatusRunning, StartedAt: now,
	})
	_ = d.FinishSession(id2, model.StatusCancelled, 300, "")

	v, err := NewSessionService(d).Today()
	if err != nil {
		t.Fatal(err)
	}
	if v.Count != 1 || v.FocusMinutes != 25 {
		t.Fatalf("view = %+v", v)
	}
	if len(v.Sessions) != 2 {
		t.Fatalf("want 2 rows, got %d", len(v.Sessions))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./desktop/ -run TestTodayView -v`
Expected: FAIL — `NewSessionService` undefined.

- [ ] **Step 3: Implement `session.go`**

`Today()` builds a `db.SessionFilter{From: &startOfDay, To: &startOfTomorrow}`,
calls `ListSessions`, maps rows, sums `ActualDuration` where
`Status == StatusCompleted` for `FocusMinutes`/`Count`. Format `Date` with
`time.Now().Format("Mon 2 Jan")`.

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./desktop/ -run TestTodayView -v`
Expected: PASS.

- [ ] **Step 5: Emit on change**

In `timer.go`, after `CreateSession` in `Start` and after `FinishSession` in
complete/cancel, call `t.emit(EventToday, nil)`. The frontend re-fetches on
that event.

- [ ] **Step 6: Bind + build**

Add `NewSessionService(a.db)` to `App` and the `Bind` slice.
Run: `go build ./... && go test ./desktop/ -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add desktop/session.go desktop/session_test.go desktop/timer.go desktop/app.go desktop/main.go
git commit -m "feat(desktop): SessionService today view"
```

---

## Task 8: DaemonService — tracking toggle

**Files:**
- Create: `desktop/daemonsvc.go`
- Create: `desktop/daemonsvc_test.go`
- Modify: `desktop/main.go` (bind)

**Interfaces:**
- Consumes: `internal/daemon` `Spawn` / `Stop` / `Running`.
- Produces:

```go
type DaemonStatus struct {
	Running bool `json:"running"`
	PID     int  `json:"pid"`
}

// controller is the seam for tests.
type controller interface {
	spawn() (int, error)
	stop() (int, error)
	running() (int, bool)
}

type DaemonService struct{ ctl controller }
func NewDaemonService() *DaemonService            // wires the real internal/daemon
func (s *DaemonService) Status() DaemonStatus
func (s *DaemonService) SetTracking(on bool) (DaemonStatus, error)
```

- [ ] **Step 1: Write the failing test**

```go
type fakeCtl struct {
	pid     int
	up      bool
	spawnErr error
}

func (f *fakeCtl) spawn() (int, error) {
	if f.spawnErr != nil {
		return 0, f.spawnErr
	}
	f.up, f.pid = true, 111
	return 111, nil
}
func (f *fakeCtl) stop() (int, error) { p := f.pid; f.up, f.pid = false, 0; return p, nil }
func (f *fakeCtl) running() (int, bool) { return f.pid, f.up }

func TestSetTracking(t *testing.T) {
	s := &DaemonService{ctl: &fakeCtl{}}
	if s.Status().Running {
		t.Fatal("should start down")
	}
	st, err := s.SetTracking(true)
	if err != nil || !st.Running || st.PID != 111 {
		t.Fatalf("on: %+v %v", st, err)
	}
	st, _ = s.SetTracking(false)
	if st.Running {
		t.Fatal("should be down after off")
	}
}

func TestSetTrackingSpawnError(t *testing.T) {
	s := &DaemonService{ctl: &fakeCtl{spawnErr: errors.New("no binary")}}
	if _, err := s.SetTracking(true); err == nil {
		t.Fatal("expected spawn error to propagate")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./desktop/ -run TestSetTracking -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `daemonsvc.go`**

```go
type realCtl struct{}

func (realCtl) spawn() (int, error)   { return daemon.Spawn() }
func (realCtl) stop() (int, error)    { return daemon.Stop() }
func (realCtl) running() (int, bool)  { return daemon.Running() }

func NewDaemonService() *DaemonService { return &DaemonService{ctl: realCtl{}} }

func (s *DaemonService) Status() DaemonStatus {
	pid, ok := s.ctl.running()
	return DaemonStatus{Running: ok, PID: pid}
}

func (s *DaemonService) SetTracking(on bool) (DaemonStatus, error) {
	if on {
		if _, err := s.ctl.spawn(); err != nil {
			return s.Status(), err
		}
	} else {
		if _, err := s.ctl.stop(); err != nil {
			return s.Status(), err
		}
	}
	return s.Status(), nil
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `go test ./desktop/ -run TestSetTracking -v`
Expected: PASS.

- [ ] **Step 5: Bind + build + full test**

Add to `App` + `Bind`. Run: `go build ./... && go test ./... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add desktop/daemonsvc.go desktop/daemonsvc_test.go desktop/app.go desktop/main.go
git commit -m "feat(desktop): DaemonService tracking toggle"
```

---

## Task 9: Menubar tray + window lifecycle

**Files:**
- Create: `desktop/tray.go`
- Modify: `desktop/main.go` (start tray, window-hide options), `desktop/timer.go`
  (notify tray on phase/tick), `desktop/app.go` (quit-guard)

**Interfaces:**
- Consumes: `fyne.io/systray`, `TimerService.GetState()`, Wails `runtime`
  window functions (`runtime.WindowShow`, `runtime.Quit`).
- Produces:
  - `func StartTray(onShow, onStart, onPauseResume, onCancel func(), daemon *DaemonService, quit func())` — runs `systray` on its own goroutine via `systray.RunWithExternalLoop` (returns start/stop funcs).
  - `func TrayUpdate(st TimerService's State, tracking bool)` — sets the tray
    title (`🍅 24:13` / `☕ 04:30` / `🍅` idle) and toggles menu item check
    states. `TimerService` calls this from `tick()` and on every phase change.

**SPIKE FIRST (15 min, throwaway):** confirm `fyne.io/systray` coexists with
Wails' macOS event loop. In a scratch branch, start `systray.RunWithExternalLoop`
before `wails.Run` and check the menubar icon appears and the window still
opens. If they fight for the main thread and there is no clean fix in 15
minutes: **descope the tray to v1.1**, ship window-only (Wails
`options.App{ ... }` without hide-on-close), and skip to Task 10. Record the
outcome in `desktop/README.md`.

- [ ] **Step 1: (if spike passed) Write `tray.go`**

Menu items: *Show Window*, *Start…*, *Pause/Resume* (title swaps with phase),
*Cancel session*, separator, *Tracking* (checkbox, calls
`DaemonService.SetTracking`), separator, *Quit*. Each `systray.AddMenuItem`
handler runs its callback in a goroutine reading `<-item.ClickedCh` in a loop.

- [ ] **Step 2: Window hide-on-close**

In `main.go` Wails options, set:

```go
Mac: &mac.Options{ /* ... */ },
OnBeforeClose: func(ctx context.Context) (prevent bool) {
	runtime.WindowHide(ctx)
	return true // prevent the quit; only the tray Quit / Cmd-Q really exits
},
```

For a true quit path, the tray *Quit* item calls a `quitFn` that sets a
`reallyQuit` bool then `runtime.Quit(ctx)`; `OnBeforeClose` returns `false`
when `reallyQuit`.

- [ ] **Step 3: Quit-with-running-session guard**

In the `quitFn`, if `timer.GetState().Phase` is `running`/`paused`, use
`runtime.MessageDialog` (Question, buttons "Leave running", "Cancel session",
"Stay"). "Leave running" → quit (rehydrates next launch). "Cancel session" →
`timer.Cancel()` then quit. "Stay" → abort.

- [ ] **Step 4: Tray reflects timer**

In `timer.go` `tick()` and phase transitions, call `TrayUpdate(t.st, tracking)`.
Poll `DaemonService.Status()` inside `tick()` (once per second is fine) to keep
the `tracking` value fresh, so an external `pomo daemon stop` is picked up.

- [ ] **Step 5: Build + manual checklist**

Run: `make desktop-build && open desktop/build/bin/pomo-desktop.app`
Verify:
- menubar shows `🍅` idle
- start a 1-minute session → menubar counts down
- close the window → timer keeps running, menubar still counts
- *Show Window* from the tray → window returns, state matches
- let it hit 0 → chime + notification + break prompt
- Cmd-Q with a running session → dialog appears
- toggle *Tracking* → `pomo daemon status` in a terminal confirms

- [ ] **Step 6: Commit**

```bash
git add desktop/tray.go desktop/main.go desktop/timer.go desktop/app.go desktop/README.md
git commit -m "feat(desktop): menubar tray and window lifecycle"
```

---

## Task 10: Frontend — stores + event wiring

**Files:**
- Create: `desktop/frontend/src/lib/stores.ts`, `desktop/frontend/src/lib/format.ts`
- Modify: `desktop/frontend/src/App.svelte`, `desktop/frontend/src/app.css`

**Interfaces:**
- Consumes generated bindings in `desktop/frontend/wailsjs/go/main/` —
  `TimerService`, `SessionService`, `DaemonService` — and
  `wailsjs/runtime` (`EventsOn`).
- Produces:
  - `timer` — `writable<State>` (`{phase, task, remaining, duration, sessionId}`)
  - `today` — `writable<TodayView>`
  - `daemon` — `writable<{running:boolean, pid:number}>`
  - `initStores()` — called once from `App.svelte onMount`: fetches
    `TimerService.GetState()`, `SessionService.Today()`, `DaemonService.Status()`,
    then `EventsOn("timer:tick", s => timer.set(s))`,
    `EventsOn("timer:phase", s => timer.set(s))`,
    `EventsOn("today:changed", () => SessionService.Today().then(today.set))`.
  - `format.ts`: `export const mmss = (s:number) => `${Math.floor(s/60)}:${String(Math.max(0,s%60)).padStart(2,'0')}``

- [ ] **Step 1: Write `format.ts` + a Vitest test**

`desktop/frontend/src/lib/format.test.ts`:

```ts
import { expect, test } from "vitest";
import { mmss } from "./format";

test("mmss pads seconds", () => {
	expect(mmss(1500)).toBe("25:00");
	expect(mmss(65)).toBe("1:05");
	expect(mmss(-3)).toBe("0:00");
});
```

Add `vitest` to `frontend/package.json` devDeps and a `"test": "vitest run"`
script.

- [ ] **Step 2: Run to verify it fails**

Run: `cd desktop/frontend && npm i && npm test`
Expected: FAIL — `./format` has no `mmss`.

- [ ] **Step 3: Implement `format.ts` and `stores.ts`**

Write both per Interfaces.

- [ ] **Step 4: Run to verify it passes**

Run: `cd desktop/frontend && npm test`
Expected: PASS.

- [ ] **Step 5: App.svelte shell + theme**

`App.svelte`: `onMount(initStores)`. Derive the visible screen:

```svelte
<script lang="ts">
  import { timer } from "./lib/stores";
  import QuickStart from "./screens/QuickStart.svelte";
  import Timer from "./screens/Timer.svelte";
  import Today from "./screens/Today.svelte";
  let tab: "start" | "today" = "start";
  $: phase = $timer.phase;
  $: showTimer = phase === "running" || phase === "paused" || phase === "break_prompt" || phase === "break";
</script>

{#if showTimer}
  <Timer />
{:else}
  <nav>
    <button class:active={tab==='start'} on:click={() => tab='start'}>Start</button>
    <button class:active={tab==='today'} on:click={() => tab='today'}>Today</button>
  </nav>
  {#if tab === 'start'}<QuickStart />{:else}<Today />{/if}
{/if}
```

`app.css`: dark-first tokens — `--bg:#1a1a1a; --fg:#e8e8e8; --accent:#e14434;
--muted:#8a8a8a;`, a `@media (prefers-color-scheme: light)` override, and
`font-variant-numeric: tabular-nums` on the countdown class.

- [ ] **Step 6: Build the frontend**

Run: `cd desktop && wails build`
Expected: builds; app launches (screens are stubs until Task 11 — that's fine).

- [ ] **Step 7: Commit**

```bash
git add desktop/frontend/src/lib desktop/frontend/src/App.svelte desktop/frontend/src/app.css desktop/frontend/package.json
git commit -m "feat(desktop): frontend stores and event wiring"
```

---

## Task 11: Frontend — QuickStart, Timer, Today screens

**Files:**
- Create: `desktop/frontend/src/screens/QuickStart.svelte`,
  `Timer.svelte`, `Today.svelte`

**Interfaces:**
- Consumes: `timer` / `today` / `daemon` stores, generated bindings, `mmss`.
- Produces: three rendered screens. No new exported symbols.

- [ ] **Step 1: QuickStart.svelte**

```svelte
<script lang="ts">
  import { TimerService } from "../../wailsjs/go/main/TimerService";
  let task = "";
  let minutes = 25;
  let error = "";
  const chips = [15, 25, 50];
  async function start() {
    if (!task.trim()) return;
    try { await TimerService.Start(task.trim(), minutes); error = ""; }
    catch (e) { error = String(e); }
  }
</script>

<section>
  <input placeholder="What are you working on?" bind:value={task}
         on:keydown={(e) => e.key === 'Enter' && start()} autofocus />
  <div class="chips">
    {#each chips as c}
      <button class:active={minutes===c} on:click={() => minutes=c}>{c}m</button>
    {/each}
  </div>
  {#if error}<p class="err">{error}</p>{/if}
  <button class="primary" on:click={start}>Start</button>
</section>
```

- [ ] **Step 2: Timer.svelte**

Shows `$timer.task`, big `mmss($timer.remaining)` in a `.countdown` element, an
SVG ring with `stroke-dashoffset` bound to `1 - remaining/duration`. Buttons:
- running → *Pause* (`TimerService.Pause()`), *Cancel* (`TimerService.Cancel()`)
- paused → *Resume* (`TimerService.Resume()`), *Cancel*
- `phase === 'break_prompt'` → *Start break (5m)* (`TimerService.StartBreak(5)`),
  *Skip* (`TimerService.SkipBreak()`)
- `phase === 'break'` → countdown + *Skip*

- [ ] **Step 3: Today.svelte**

```svelte
<script lang="ts">
  import { today, daemon } from "../lib/stores";
  import { DaemonService } from "../../wailsjs/go/main/DaemonService";
  async function toggle() {
    const s = await DaemonService.SetTracking(!$daemon.running);
    daemon.set(s);
  }
</script>

<header>
  <h2>{$today.date}</h2>
  <p>{$today.focusMinutes} min focus · {$today.count} sessions</p>
  <button class:on={$daemon.running} on:click={toggle}>
    Tracking {$daemon.running ? "●" : "○"}
  </button>
</header>
<ul>
  {#each $today.sessions as s}
    <li><span>{s.task}</span><span>{s.minutes}m</span><span class="pill {s.status}">{s.status}</span></li>
  {/each}
</ul>
```

- [ ] **Step 4: Poll daemon status while window is open**

In `Today.svelte` `onMount`, `setInterval(() => DaemonService.Status().then(daemon.set), 5000)`; clear on `onDestroy`.

- [ ] **Step 5: Full manual run-through**

Run: `make desktop-build && open desktop/build/bin/pomo-desktop.app`
Walk the whole flow: type task → chip → Start → Timer screen counts down →
Pause/Resume → let it complete → break prompt → Start break → break counts →
back to QuickStart → Today shows the completed session and focus total →
toggle Tracking → confirm with `pomo daemon status`.

- [ ] **Step 6: Cross-check with the CLI**

```bash
pomo start "cli session"   # in a terminal
# now launch the desktop app → QuickStart should show the "already running" error
```

- [ ] **Step 7: Commit**

```bash
git add desktop/frontend/src/screens
git commit -m "feat(desktop): QuickStart, Timer, and Today screens"
```

---

## Task 12: Docs + final verification

**Files:**
- Modify: `desktop/README.md`, `CLAUDE.md` (one line under Commands),
  `specs/2026-09-06-desktop-app.md` (mark shipped), `plans/` — nothing

- [ ] **Step 1: CLAUDE.md**

Under `## Commands`, add:

```
- `pomo desktop` — open the desktop app (`desktop/`, Wails v2 + Svelte).
  Build it with `make desktop-build`.
```

And a one-line `desktop/` entry under `## Architecture`.

- [ ] **Step 2: Full suite**

Run: `go build ./... && go vet ./... && go test ./... && (cd desktop/frontend && npm test)`
Expected: all PASS.

- [ ] **Step 3: Manual checklist (copy into `desktop/README.md`)**

- [ ] launch, run a 1-min session, quit mid-run, relaunch → timer rehydrates
- [ ] pause, quit, relaunch → still paused, correct remaining
- [ ] complete a session → chime + notification + break prompt
- [ ] Today totals update after a completed session
- [ ] tracking toggle on/off ↔ `pomo daemon status`
- [ ] external `pomo daemon stop` reflected in the UI within ~5s
- [ ] second `open pomo-desktop.app` → exits quietly, first instance intact
- [ ] `pomo start` in a terminal → desktop QuickStart shows the running-session error

- [ ] **Step 4: Commit + tag the spec**

```bash
git add CLAUDE.md desktop/README.md specs/2026-09-06-desktop-app.md
git commit -m "docs: desktop app shipped"
```

---

## Self-Review

**Spec coverage:**
- §1 v1 scope (quick-start, timer, today, tray, daemon toggle) → Tasks 6, 11, 7, 9, 8. ✓
- §2 architecture / third DB client / no IPC → Tasks 5–8. ✓
- §2 Wails v2 not v3 → Global Constraints + Task 5 (`@v2.10.1`). ✓
- §3 code layout, `desktop/` dir, Makefile, cmd/desktop.go → Tasks 4, 5. ✓
- §3 extract `internal/daemon` Spawn/Stop/Status → Task 3. ✓
- §4 TimerService state machine, wall-clock recompute, rehydrate → Task 6. ✓
- §4 pause schema columns → Task 1. ✓
- §4 report pause accounting → resolved: reports read `actual_duration`, which
  Task 6 computes pause-excluded at finish time; no `internal/report` change
  needed. Task 1 Step 7 re-runs report tests to confirm. ✓
- §5 tray title, window hide-on-close, quit guard, single-instance → Tasks 5, 9. ✓
- §6 DaemonService, 5s poll, spawn-failure toast → Tasks 8, 11 Step 4; toast =
  the `error` binding surfaced in QuickStart/Today. ✓
- §7 Svelte stores + 3 screens + theme → Tasks 10, 11. ✓
- §8 build/dist, `wails build`, unsigned `.app` → Task 5. ✓
- §9 testing table → Tasks 1, 3, 6, 7, 8 (Go) + 10 (Vitest). ✓
- §11 build order → matches Task order 1→12. ✓

**Placeholder scan:** tray coexistence is the one genuine unknown — handled
with an explicit time-boxed spike and a named fallback (window-only v1.1), not
a "TODO". No other placeholders.

**Type consistency:** `State`/`Phase` constants defined in Task 6 and reused
verbatim in Tasks 9–11. `TodayView`/`SessionRow` defined in Task 7, consumed in
Task 11. `DaemonStatus` defined in Task 8, consumed in Tasks 10–11.
`pomoexec.Find` (Task 2) consumed in Tasks 3, 4. `daemon.Spawn/Stop/Running`
(Task 3) consumed in Task 8.
