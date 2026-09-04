# Drift Daemon Implementation Plan (Chunk 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `pomo daemon` — a background process that watches the foreground app and repo file activity during a running session, scores drift, records drift episodes to `drift_events`, and fires gentle escalating nudges (text from the user's AI key when set, canned otherwise). No TUI changes; the timer works identically whether or not the daemon runs.

**Architecture:** Six new leaf packages plus one command. `internal/gitinfo` resolves a session's repo. `internal/watch` reads the foreground app (darwin: `lsappinfo` + `osascript`; other OS: stub) and classifies it, and checks repo file staleness. `internal/drift` is a pure per-tick scorer. `internal/notify` sends desktop notifications. `internal/nudge` is a pure escalation state machine with canned fallback text. `internal/daemon` owns the `Loop` that wires all of the above with `internal/db`, `internal/ipc`, and `internal/ai` behind an injectable clock — its `Tick` method is fully unit-tested with fakes and no real sleeping. `cmd/daemon.go` provides `run` (the real sleep-loop) and `start` / `stop` / `status` (detached process + pidfile).

**Tech Stack:** Go 1.25, stdlib only (`os/exec`, `bufio`, `context`, `syscall` for the flock/pidfile, `net/http/httptest` already used). No new third-party dependencies — the fs-activity check is an early-exit `filepath.WalkDir`, not `fsnotify`.

**Spec:** `specs/2026-09-04-daemon-drift-detection.md`, `specs/2026-09-04-nudge-and-ai.md` §1–2 & §4 (parent: `specs/2026-09-04-adhd-focus-overview.md`)

## Global Constraints

- Go version floor: `go 1.25` — do not raise it.
- **No new third-party dependencies.** fs activity is a bounded `filepath.WalkDir` with early exit; no `fsnotify`.
- No change to existing runtime behaviour. `pomo start`, the TUI, and every existing command behave identically. The daemon is opt-in (`pomo daemon start`); when it is not running, nothing new happens and nothing errors.
- Foreground-app watching is implemented for **darwin only** this chunk. Other GOOS gets a stub whose `Supported()` returns false; the daemon then runs checkpoint + fs signals only. Linux is a later chunk.
- The daemon must never block its loop on the network. AI nudge-line calls use a 3s context; any failure falls back to a canned line.
- Never trigger a blocking GUI dialog. Notifications are fire-and-forget (`terminal-notifier` / `osascript -e 'display notification'` / `notify-send`), never `display dialog`.
- `os/exec` calls for `lsappinfo` / `osascript` / notifiers use `exec.CommandContext` with a 2s timeout.
- Socket path: `ipc.SocketPath()`. Daemon files live under `db.Dir()` (`~/.pomo/`): `daemon.pid`, `daemon.lock`, `daemon.status`.
- Drift episode `trigger` values are exactly `"foreground"`, `"fs_stale"`, `"checkpoint_no"`. `detail` is `"<App>"` or `"<App> — <title>"`.
- Commit after every task (`feat:` / `test:` / `docs:`). Work continues on `master`.

---

### Task 1: `internal/gitinfo` + capture repo path on session start

**Files:**
- Create: `internal/gitinfo/gitinfo.go`
- Test: `internal/gitinfo/gitinfo_test.go` (create)
- Modify: `cmd/start.go:61-68` (set `RepoPath` / `RepoBranch` on `CreateSession`)
- Modify: `internal/tui/app.go:427-434` (same, from a cwd captured at program start)

**Interfaces:**
- Consumes: `os/exec`.
- Produces:
  - `gitinfo.Toplevel(dir string) string` — `git -C dir rev-parse --show-toplevel` trimmed, or `""` on any error / not a repo.
  - `gitinfo.Branch(dir string) string` — `git -C dir rev-parse --abbrev-ref HEAD` trimmed, or `""`.
  - `gitinfo.Describe(dir string) (repoPath, branch string)` — both in one call.
- After this task, new sessions created via `pomo start <task>` and via the TUI carry `RepoPath` / `RepoBranch`. Sessions started with no cwd context (should not happen) carry `""` — harmless.

- [ ] **Step 1: Write the failing test**

Create `internal/gitinfo/gitinfo_test.go`:
```go
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
	// macOS /tmp is a symlink to /private/tmp; compare by base name + suffix.
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/gitinfo/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement `internal/gitinfo/gitinfo.go`**

```go
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
```

- [ ] **Step 4: Run the test**

Run: `go test ./internal/gitinfo/ -v`
Expected: PASS.

- [ ] **Step 5: Capture on `pomo start <task>`**

In `cmd/start.go` `runStart`, after `taskID, err := database.FindOrCreateTask(...)` and before `CreateSession`:
```go
	cwd, _ := os.Getwd()
	repoPath, repoBranch := gitinfo.Describe(cwd)
```
Add `"pomo/internal/gitinfo"` to the imports. Then in the `CreateSession(model.Session{...})` literal add:
```go
		RepoPath:        repoPath,
		RepoBranch:      repoBranch,
```

- [ ] **Step 6: Capture in the TUI**

In `internal/tui/app.go`:
- Add a field to `App`: `startCwd string`.
- In `NewApp` (line ~96), set `a.startCwd, _ = os.Getwd()` (add `"os"` to imports if absent).
- In `beginSession` (line ~427), compute `repoPath, repoBranch := gitinfo.Describe(a.startCwd)` and add the two fields to the `CreateSession` literal.
- Add `"pomo/internal/gitinfo"` to imports.

- [ ] **Step 7: Build + full suite**

Run: `go build ./... && make test`
Expected: PASS. Manually confirm nothing else references those `CreateSession` call sites.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat(gitinfo): resolve repo path/branch and capture on session start"
```

---

### Task 2: `internal/watch` — foreground app + classification + fs staleness

**Files:**
- Create: `internal/watch/watch.go` (interface, `Classify`, `RepoActive`, `New`)
- Create: `internal/watch/watch_darwin.go` (build tag `darwin`)
- Create: `internal/watch/watch_other.go` (build tag `!darwin`)
- Test: `internal/watch/watch_test.go` (create — `Classify` + `RepoActive`, both pure/filesystem, run on every OS)

**Interfaces:**
- Consumes: `pomoconfig.DriftConfig` (chunk 2), `os/exec`, `filepath`.
- Produces:
```go
package watch

type ForegroundApp struct {
	Name     string // "Google Chrome"
	BundleID string // "com.google.Chrome" on darwin, "" elsewhere
	Title    string // active window/tab title, "" if unavailable
}

type Watcher interface {
	Foreground() (ForegroundApp, error)
	Supported() bool
	Name() string // "darwin/lsappinfo" | "unsupported"
}

func New() Watcher // build-tag dispatch

// Class is the drift classification of a foreground reading.
type Class string
const (
	Focus    Class = "focus"
	Neutral  Class = "neutral"
	Distract Class = "distract"
)

// Classify buckets an app against the config lists. Browsers default to
// Distract, but a title containing a focus hint downgrades that reading to
// Neutral (not Focus — reading docs is not writing code).
func Classify(app ForegroundApp, cfg pomoconfig.DriftConfig) Class

// RepoActive reports whether any file under repoPath was modified at or after
// `since`, skipping .git and common vendor/build dirs. Returns false for an
// empty path or a walk error. Early-exits on the first newer file.
func RepoActive(repoPath string, since time.Time) bool

var ErrUnsupported = errors.New("watch: foreground detection not supported on this platform")
```

- [ ] **Step 1: Write the failing test**

Create `internal/watch/watch_test.go`:
```go
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

	// a fresh write under .git must not count
	os.MkdirAll(filepath.Join(dir, ".git"), 0o755)
	mustWrite(".git/COMMIT_EDITMSG")
	if watch.RepoActive(dir, future) {
		t.Error("changes under .git must be ignored")
	}
	if watch.RepoActive("", past) {
		t.Error("empty path must be false")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/watch/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement `internal/watch/watch.go`**

```go
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

// Classify buckets a foreground reading. Precedence: explicit user focus rule
// > explicit user/built-in distract rule (with title-hint downgrade) >
// built-in focus rule > neutral.
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
```

- [ ] **Step 4: Implement `internal/watch/watch_other.go`**

```go
//go:build !darwin

package watch

func New() Watcher { return unsupported{} }

type unsupported struct{}

func (unsupported) Foreground() (ForegroundApp, error) { return ForegroundApp{}, ErrUnsupported }
func (unsupported) Supported() bool                    { return false }
func (unsupported) Name() string                       { return "unsupported" }
```

- [ ] **Step 5: Implement `internal/watch/watch_darwin.go`**

```go
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

	// Front app name via AppleScript System Events (reliable, no extra perms).
	name, err := run(ctx, "osascript", "-e",
		`tell application "System Events" to get name of first application process whose frontmost is true`)
	if err != nil || name == "" {
		return ForegroundApp{}, ErrUnsupported
	}
	app := ForegroundApp{Name: name}

	// Bundle id (best effort).
	if bid, e := run(ctx, "osascript", "-e",
		`tell application "System Events" to get bundle identifier of first application process whose frontmost is true`); e == nil {
		app.BundleID = bid
	}

	// Window / tab title (best effort — may raise the automation prompt once).
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
```

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/watch/ -v`
Expected: PASS (`TestClassify`, `TestClassifyUserOverrideWins`, `TestRepoActive`). The darwin `Foreground()` is not unit-tested (it drives the real OS) — it is exercised in the Task 7 manual checklist.

- [ ] **Step 7: Build for both tag sets + full suite**

Run:
```bash
go build ./... && GOOS=linux go build ./internal/watch/ && make test
```
Expected: PASS (the linux cross-build proves `watch_other.go` compiles).

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat(watch): foreground app (darwin) + Classify + RepoActive"
```

---

### Task 3: `internal/drift` — pure per-tick scorer

**Files:**
- Create: `internal/drift/drift.go`
- Test: `internal/drift/drift_test.go` (create)

**Interfaces:**
- Consumes: `pomoconfig.DriftConfig`, `watch.Class`.
- Produces:
```go
package drift

type Signals struct {
	Class         watch.Class // foreground classification this tick
	Detail        string      // "App" or "App — title"
	FsStale       bool        // repo known & no write within cfg.FsStale
	CheckpointNo  bool        // checkpoint answered "no" (or timed out) recently
}

// State carries the cross-tick memory the scorer needs.
type State struct {
	distractSince time.Time // zero when not currently on a distract app
}

// Result is the verdict for one tick.
type Result struct {
	Drifting bool
	Trigger  string // "foreground" | "fs_stale" | "checkpoint_no" | "" when not drifting
	Detail   string // dominant app/title for the episode row
}

// Step folds one tick's signals into the state and returns the verdict.
// Rules (any true => drifting):
//   - Class == Distract continuously for >= cfg.DistractGrace
//   - FsStale && Class != Focus
//   - CheckpointNo
func (s *State) Step(now time.Time, sig Signals, cfg pomoconfig.DriftConfig) Result
```

- [ ] **Step 1: Write the failing test**

Create `internal/drift/drift_test.go`:
```go
package drift_test

import (
	"testing"
	"time"

	"pomo/internal/drift"
	"pomo/internal/pomoconfig"
	"pomo/internal/watch"
)

func cfg() pomoconfig.DriftConfig {
	return pomoconfig.DriftConfig{DistractGrace: 3 * time.Minute, FsStale: 10 * time.Minute}
}

func TestDistractNeedsGrace(t *testing.T) {
	var s drift.State
	c := cfg()
	t0 := time.Now()

	r := s.Step(t0, drift.Signals{Class: watch.Distract, Detail: "Slack"}, c)
	if r.Drifting {
		t.Fatal("first distract tick should not be drifting yet")
	}
	r = s.Step(t0.Add(2*time.Minute), drift.Signals{Class: watch.Distract, Detail: "Slack"}, c)
	if r.Drifting {
		t.Fatal("2min < grace, not drifting")
	}
	r = s.Step(t0.Add(3*time.Minute+time.Second), drift.Signals{Class: watch.Distract, Detail: "Slack"}, c)
	if !r.Drifting || r.Trigger != "foreground" || r.Detail != "Slack" {
		t.Fatalf("after grace should be drifting: %+v", r)
	}
}

func TestFocusResetsDistractClock(t *testing.T) {
	var s drift.State
	c := cfg()
	t0 := time.Now()
	s.Step(t0, drift.Signals{Class: watch.Distract, Detail: "Slack"}, c)
	s.Step(t0.Add(1*time.Minute), drift.Signals{Class: watch.Focus, Detail: "iTerm2"}, c)
	r := s.Step(t0.Add(2*time.Minute), drift.Signals{Class: watch.Distract, Detail: "Slack"}, c)
	if r.Drifting {
		t.Fatal("distract clock should have reset after a Focus tick")
	}
}

func TestFsStaleWhileNotFocused(t *testing.T) {
	var s drift.State
	c := cfg()
	now := time.Now()
	if r := s.Step(now, drift.Signals{Class: watch.Neutral, FsStale: true, Detail: "Preview"}, c); !r.Drifting || r.Trigger != "fs_stale" {
		t.Fatalf("fs stale + neutral => drifting: %+v", r)
	}
	if r := s.Step(now, drift.Signals{Class: watch.Focus, FsStale: true, Detail: "iTerm2"}, c); r.Drifting {
		t.Fatal("fs stale but Focus => not drifting")
	}
}

func TestCheckpointNo(t *testing.T) {
	var s drift.State
	if r := s.Step(time.Now(), drift.Signals{Class: watch.Focus, CheckpointNo: true, Detail: "iTerm2"}, cfg()); !r.Drifting || r.Trigger != "checkpoint_no" {
		t.Fatalf("checkpoint no => drifting: %+v", r)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/drift/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement `internal/drift/drift.go`**

```go
// Package drift is the pure per-tick drift scorer for the daemon. It holds
// only the minimal cross-tick memory (how long the user has been on a
// distract app) and returns a verdict; episode bookkeeping lives in the
// daemon.
package drift

import (
	"time"

	"pomo/internal/pomoconfig"
	"pomo/internal/watch"
)

type Signals struct {
	Class        watch.Class
	Detail       string
	FsStale      bool
	CheckpointNo bool
}

type State struct {
	distractSince time.Time
}

type Result struct {
	Drifting bool
	Trigger  string
	Detail   string
}

func (s *State) Step(now time.Time, sig Signals, cfg pomoconfig.DriftConfig) Result {
	// Track the continuous distract run.
	if sig.Class == watch.Distract {
		if s.distractSince.IsZero() {
			s.distractSince = now
		}
	} else {
		s.distractSince = time.Time{}
	}

	switch {
	case sig.CheckpointNo:
		return Result{Drifting: true, Trigger: "checkpoint_no", Detail: sig.Detail}
	case !s.distractSince.IsZero() && now.Sub(s.distractSince) >= cfg.DistractGrace:
		return Result{Drifting: true, Trigger: "foreground", Detail: sig.Detail}
	case sig.FsStale && sig.Class != watch.Focus:
		return Result{Drifting: true, Trigger: "fs_stale", Detail: sig.Detail}
	default:
		return Result{}
	}
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/drift/ -v`
Expected: PASS (all four).

- [ ] **Step 5: Full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(drift): pure per-tick drift scorer"
```

---

### Task 4: `internal/notify` — desktop notifications

**Files:**
- Create: `internal/notify/notify.go`
- Create: `internal/notify/notify_darwin.go` (`//go:build darwin`)
- Create: `internal/notify/notify_other.go` (`//go:build !darwin`)
- Test: `internal/notify/notify_test.go` (create)

**Interfaces:**
- Consumes: `os/exec`.
- Produces:
```go
package notify

type Notifier interface {
	Send(title, body string) error // best effort, never blocks, never a GUI dialog
}

func New() Notifier // real platform notifier

// command builds the exec argv for the platform notifier, or nil when no
// notifier binary is available. Exposed for testing.
func command(title, body string) []string
```
  - darwin: `terminal-notifier -title <t> -message <b> -group pomo` if on PATH, else `osascript -e 'display notification "<b>" with title "<t>"'`.
  - other (linux): `notify-send -a pomo <t> <b>` if on PATH.
  - No binary found → `command` returns nil, `Send` returns nil (the daemon still delivers the inline IPC banner).
  - Exec runs under a 2s `exec.CommandContext` timeout.

- [ ] **Step 1: Write the failing test**

Create `internal/notify/notify_test.go`:
```go
package notify

import (
	"strings"
	"testing"
)

func TestCommandIncludesTitleAndBody(t *testing.T) {
	argv := command("Focus", "back to it")
	if argv == nil {
		t.Skip("no notifier binary on this machine")
	}
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "Focus") || !strings.Contains(joined, "back to it") {
		t.Fatalf("argv missing title/body: %v", argv)
	}
	// must never be a blocking dialog
	if strings.Contains(joined, "display dialog") {
		t.Fatalf("notifier must not use a blocking dialog: %v", argv)
	}
}

func TestSendNeverErrorsHard(t *testing.T) {
	// Whatever the platform, Send must not return an error for ordinary input.
	if err := New().Send("t", "b"); err != nil {
		t.Fatalf("Send returned %v, want nil", err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/notify/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement `internal/notify/notify.go`**

```go
// Package notify sends fire-and-forget desktop notifications for the daemon.
// It never opens a blocking GUI dialog and never returns an error for
// ordinary input — a missing notifier binary is a silent no-op.
package notify

import (
	"context"
	"os/exec"
	"time"
)

type Notifier interface {
	Send(title, body string) error
}

type execNotifier struct{}

func New() Notifier { return execNotifier{} }

func (execNotifier) Send(title, body string) error {
	argv := command(title, body)
	if argv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, argv[0], argv[1:]...).Run()
	return nil
}

func onPath(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}
```

- [ ] **Step 4: Implement `internal/notify/notify_darwin.go`**

```go
//go:build darwin

package notify

import "fmt"

func command(title, body string) []string {
	if onPath("terminal-notifier") {
		return []string{"terminal-notifier", "-title", title, "-message", body, "-group", "pomo"}
	}
	script := fmt.Sprintf("display notification %q with title %q", body, title)
	return []string{"osascript", "-e", script}
}
```

- [ ] **Step 5: Implement `internal/notify/notify_other.go`**

```go
//go:build !darwin

package notify

func command(title, body string) []string {
	if onPath("notify-send") {
		return []string{"notify-send", "-a", "pomo", title, body}
	}
	return nil
}
```

- [ ] **Step 6: Run the tests + cross-build**

Run: `go test ./internal/notify/ -v && GOOS=linux go build ./internal/notify/`
Expected: PASS; linux build clean.

- [ ] **Step 7: Full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat(notify): fire-and-forget desktop notifications"
```

---

### Task 5: `internal/nudge` — escalation state machine

**Files:**
- Create: `internal/nudge/nudge.go`
- Test: `internal/nudge/nudge_test.go` (create)

**Interfaces:**
- Consumes: `pomoconfig.NudgeConfig`.
- Produces:
```go
package nudge

type Context struct {
	Task           string
	DistractApp    string
	DistractDetail string
	DriftMinutes   int
	SessionMinutes int
	Hour           int
}

type Decision struct {
	Fire    bool
	Level   int      // 1..3
	Text    string
	Actions []string // key hints, e.g. ["snooze","drifted"]
}

type State struct {
	Level  int
	Count  int
	LastAt time.Time
	recent []string // last 3 lines emitted, for de-dup
}

// LineFn generates an AI nudge line. nil, an error, or a duplicate of a
// recent line all fall back to a canned template.
type LineFn func(Context) (string, error)

// Evaluate decides whether to nudge this tick. It fires only when an episode
// is open, the min-gap has elapsed, and Count < MaxPerSession.
func (s *State) Evaluate(now time.Time, episodeOpen bool, ctx Context, cfg pomoconfig.NudgeConfig, line LineFn) Decision

// Reset drops the escalation back to level 0 (called when a checkpoint is
// answered "yes"). It does not clear LastAt — the min-gap still applies.
func (s *State) Reset()
```
  Level selection at fire time: `Level = min(3, Count+1)`. Actions:
  - 1 → `["snooze","drifted"]`
  - 2 → `["refocus","drifted","break"]`
  - 3 → `["break","refocus"]`
  Canned templates:
  - L1: `"still on <task>?"`
  - L2 with app: `"<n> min on <app> — <task> still the plan?"`; without app: `"<n> min drifting — <task> still the plan?"`
  - L3: `"rough stretch. want a real break or a reset?"`

- [ ] **Step 1: Write the failing test**

Create `internal/nudge/nudge_test.go`:
```go
package nudge_test

import (
	"errors"
	"testing"
	"time"

	"pomo/internal/nudge"
	"pomo/internal/pomoconfig"
)

func cfg() pomoconfig.NudgeConfig {
	return pomoconfig.NudgeConfig{Enabled: true, MaxPerSession: 4, MinGap: 5 * time.Minute}
}

func TestNoFireWithoutEpisode(t *testing.T) {
	var s nudge.State
	d := s.Evaluate(time.Now(), false, nudge.Context{Task: "x"}, cfg(), nil)
	if d.Fire {
		t.Fatal("no episode => no fire")
	}
}

func TestFiresThenRespectsMinGap(t *testing.T) {
	var s nudge.State
	c := cfg()
	t0 := time.Now()
	d := s.Evaluate(t0, true, nudge.Context{Task: "ship"}, c, nil)
	if !d.Fire || d.Level != 1 || d.Text != "still on ship?" {
		t.Fatalf("first fire: %+v", d)
	}
	d = s.Evaluate(t0.Add(2*time.Minute), true, nudge.Context{Task: "ship"}, c, nil)
	if d.Fire {
		t.Fatal("within min gap => no fire")
	}
	d = s.Evaluate(t0.Add(6*time.Minute), true, nudge.Context{Task: "ship", DistractApp: "Chrome", DriftMinutes: 12}, c, nil)
	if !d.Fire || d.Level != 2 {
		t.Fatalf("second fire should be level 2: %+v", d)
	}
	if d.Text != "12 min on Chrome — ship still the plan?" {
		t.Fatalf("L2 text = %q", d.Text)
	}
}

func TestEscalationCapsAtThreeAndMaxPerSession(t *testing.T) {
	var s nudge.State
	c := cfg()
	c.MinGap = 0
	now := time.Now()
	levels := []int{}
	for i := 0; i < 6; i++ {
		d := s.Evaluate(now.Add(time.Duration(i)*time.Minute), true, nudge.Context{Task: "t"}, c, nil)
		if d.Fire {
			levels = append(levels, d.Level)
		}
	}
	// max 4 fires, levels 1,2,3,3
	if len(levels) != 4 {
		t.Fatalf("fired %d times, want 4: %v", len(levels), levels)
	}
	if levels[0] != 1 || levels[1] != 2 || levels[2] != 3 || levels[3] != 3 {
		t.Fatalf("levels = %v, want [1 2 3 3]", levels)
	}
}

func TestResetDropsLevel(t *testing.T) {
	var s nudge.State
	c := cfg()
	c.MinGap = 0
	now := time.Now()
	s.Evaluate(now, true, nudge.Context{Task: "t"}, c, nil)          // L1
	s.Evaluate(now.Add(time.Minute), true, nudge.Context{Task: "t"}, c, nil) // L2
	s.Reset()
	d := s.Evaluate(now.Add(2*time.Minute), true, nudge.Context{Task: "t"}, c, nil)
	if d.Level != 1 {
		t.Fatalf("after reset, next fire level = %d, want 1", d.Level)
	}
}

func TestAILineUsedThenDedup(t *testing.T) {
	var s nudge.State
	c := cfg()
	c.MinGap = 0
	now := time.Now()
	fixed := func(nudge.Context) (string, error) { return "close the tab", nil }
	d := s.Evaluate(now, true, nudge.Context{Task: "t"}, c, fixed)
	if d.Text != "close the tab" {
		t.Fatalf("AI line not used: %q", d.Text)
	}
	// same line again => must fall back to canned
	d = s.Evaluate(now.Add(time.Minute), true, nudge.Context{Task: "t"}, c, fixed)
	if d.Text == "close the tab" {
		t.Fatal("duplicate AI line should have been rejected")
	}
}

func TestAIErrorFallsBackToCanned(t *testing.T) {
	var s nudge.State
	c := cfg()
	now := time.Now()
	boom := func(nudge.Context) (string, error) { return "", errors.New("timeout") }
	d := s.Evaluate(now, true, nudge.Context{Task: "t"}, c, boom)
	if !d.Fire || d.Text != "still on t?" {
		t.Fatalf("AI error should fall back to canned: %+v", d)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/nudge/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement `internal/nudge/nudge.go`**

```go
// Package nudge is the pure escalation state machine for drift nudges. It
// decides when to nudge, at what level, and with what text — using an AI line
// when one is supplied and non-duplicate, else a canned template.
package nudge

import (
	"fmt"
	"time"

	"pomo/internal/pomoconfig"
)

type Context struct {
	Task           string
	DistractApp    string
	DistractDetail string
	DriftMinutes   int
	SessionMinutes int
	Hour           int
}

type Decision struct {
	Fire    bool
	Level   int
	Text    string
	Actions []string
}

type State struct {
	Level  int
	Count  int
	LastAt time.Time
	recent []string
}

type LineFn func(Context) (string, error)

func (s *State) Reset() {
	s.Level = 0
	s.Count = 0
}

func (s *State) Evaluate(now time.Time, episodeOpen bool, ctx Context, cfg pomoconfig.NudgeConfig, line LineFn) Decision {
	if !cfg.Enabled || !episodeOpen {
		return Decision{}
	}
	if s.Count >= cfg.MaxPerSession {
		return Decision{}
	}
	if !s.LastAt.IsZero() && now.Sub(s.LastAt) < cfg.MinGap {
		return Decision{}
	}

	level := s.Count + 1
	if level > 3 {
		level = 3
	}

	text := ""
	if line != nil {
		if got, err := line(ctx); err == nil && got != "" && !s.isRecent(got) {
			text = got
		}
	}
	if text == "" {
		text = canned(level, ctx)
	}

	s.remember(text)
	s.Level = level
	s.Count++
	s.LastAt = now

	return Decision{Fire: true, Level: level, Text: text, Actions: actionsFor(level)}
}

func (s *State) isRecent(line string) bool {
	for _, r := range s.recent {
		if r == line {
			return true
		}
	}
	return false
}

func (s *State) remember(line string) {
	s.recent = append(s.recent, line)
	if len(s.recent) > 3 {
		s.recent = s.recent[len(s.recent)-3:]
	}
}

func canned(level int, ctx Context) string {
	switch level {
	case 1:
		return fmt.Sprintf("still on %s?", ctx.Task)
	case 2:
		if ctx.DistractApp != "" {
			return fmt.Sprintf("%d min on %s — %s still the plan?", ctx.DriftMinutes, ctx.DistractApp, ctx.Task)
		}
		return fmt.Sprintf("%d min drifting — %s still the plan?", ctx.DriftMinutes, ctx.Task)
	default:
		return "rough stretch. want a real break or a reset?"
	}
}

func actionsFor(level int) []string {
	switch level {
	case 1:
		return []string{"snooze", "drifted"}
	case 2:
		return []string{"refocus", "drifted", "break"}
	default:
		return []string{"break", "refocus"}
	}
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/nudge/ -v`
Expected: PASS (all six).

- [ ] **Step 5: Full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(nudge): escalation state machine with canned fallback"
```

---

### Task 6: `internal/daemon` — the loop

**Files:**
- Create: `internal/daemon/daemon.go`
- Create: `internal/daemon/status.go` (status file read/write)
- Test: `internal/daemon/daemon_test.go` (create)

**Interfaces:**
- Consumes: `db.DB`, `watch.Watcher`, `notify.Notifier`, `ipc.Server` (or an interface it satisfies), `ai.Nudger`, `pomoconfig.Config`, `drift.State`, `nudge.State`, `gitinfo` (not needed — repo path is read off the session row).
- Produces:
```go
package daemon

// Broadcaster is the subset of *ipc.Server the loop needs (so tests can fake it).
type Broadcaster interface{ Broadcast(ipc.Event) }

type Deps struct {
	DB     *db.DB
	Watch  watch.Watcher
	Notify notify.Notifier
	IPC    Broadcaster
	AI     ai.Nudger        // may be the no-op; loop wraps it with a 3s ctx
	Cfg    pomoconfig.Config
	Now    func() time.Time // injectable clock; nil => time.Now
}

type Loop struct { /* unexported: deps + active *sessionState */ }

func NewLoop(d Deps) *Loop

// Tick runs one iteration: reads the running session, gathers signals, scores
// drift, writes/updates/closes the drift_events episode, and fires a nudge.
// It never sleeps and never blocks longer than the 2s watch + 3s AI budgets.
func (l *Loop) Tick()

// HandleEvent processes an inbound IPC event from the TUI (checkpoint answers
// and nudge-action keypresses).
func (l *Loop) HandleEvent(e ipc.Event)

// Status is the snapshot written to ~/.pomo/daemon.status and read by
// `pomo daemon status`.
type Status struct {
	PID            int       `json:"pid"`
	StartedAt      time.Time `json:"started_at"`
	LastTick       time.Time `json:"last_tick"`
	WatchBackend   string    `json:"watch_backend"`
	WatchSupported bool      `json:"watch_supported"`
	WatchingID     int64     `json:"watching_id"` // 0 = idle
}

func WriteStatus(s Status) error       // to ~/.pomo/daemon.status
func ReadStatus() (Status, bool)       // false if absent/unreadable
```

  **`sessionState`** (per running session, rebuilt when the session id changes):
  `sessionID int64`, `repoPath string`, `lastWrite time.Time`, `drift.State`,
  `nudge.State`, `episodeID int64` (0 = none open), `episodeStart time.Time`,
  `episodeSeconds int`, `recoveredFor time.Duration`,
  `checkpointAt time.Time` (scheduled), `checkpointAsked bool`,
  `checkpointNoUntil time.Time`.

  **Tick algorithm:**
  1. `sess, _ := l.deps.DB.LastRunningSession()`. Nil → close any open episode with the last known seconds, clear `active`, broadcast `{Type:"idle"}`, return.
  2. New/changed session id → `active = newSessionState(sess, l.now())`; broadcast `{Type:"watching", SessionID: sess.ID, RepoPath: sess.RepoPath}`; schedule checkpoint at a random point in the middle third of `PlannedDuration` (only if `cfg.CheckpointEnabled`).
  3. Foreground: `app, err := l.deps.Watch.Foreground()`. On error → `class = Neutral`, `detail = ""`. Else `class = watch.Classify(app, cfg.Drift)`, `detail = app.Name` (+ `" — " + app.Title` when title set).
  4. fs: if `active.repoPath != ""`, `fresh := watch.RepoActive(repoPath, active.lastWrite)`; if `fresh` set `active.lastWrite = now`. `fsStale := now.Sub(active.lastWrite) >= cfg.Drift.FsStale`. (On the first tick `lastWrite` is the session's `StartedAt`, so no immediate stale.)
  5. checkpoint: if scheduled, `now >= checkpointAt`, not asked → broadcast `{Type:"checkpoint", Text:"on task?"}`, `checkpointAsked = true`, start a wall-clock deadline `now + cfg.Drift.CheckpointTimeout`. A timeout with no answer is treated as "no" (handled in `HandleEvent` + a deadline check here).
  6. `res := active.drift.Step(now, drift.Signals{Class: class, Detail: detail, FsStale: fsStale, CheckpointNo: now.Before(active.checkpointNoUntil)}, cfg.Drift)`.
  7. Episode bookkeeping:
     - `res.Drifting` && `episodeID == 0` → `OpenDriftEpisode(sess.ID, now, res.Trigger, res.Detail)`, record start, `recoveredFor = 0`.
     - `res.Drifting` && `episodeID != 0` → `episodeSeconds += tick`, `UpdateDriftEpisode(episodeID, episodeSeconds, res.Detail)`, `recoveredFor = 0`.
     - `!res.Drifting` && `episodeID != 0` → `recoveredFor += tick`; when `recoveredFor >= cfg.Drift.RecoverGrace` → `CloseDriftEpisode(episodeID, now, episodeSeconds)`, reset episode fields.
     - `tick` = `cfg.Daemon.Tick`.
  8. Nudge: `d := active.nudge.Evaluate(now, episodeID != 0, nudge.Context{Task: sess.TaskName, DistractApp: appName, DistractDetail: title, DriftMinutes: episodeSeconds/60, SessionMinutes: int(now.Sub(sess.StartedAt).Minutes()), Hour: now.Hour()}, cfg.Nudge, l.aiLine)`. If `d.Fire` → `l.deps.Notify.Send("pomo", d.Text)` and `l.deps.IPC.Broadcast(ipc.Event{Type:"nudge", Text:d.Text, Level:d.Level, Actions:d.Actions})`.
  9. `active.lastTick = now`; periodically (every tick is fine) `WriteStatus(...)`.

  **`l.aiLine`** is `func(nudge.Context) (string, error)` — builds `ai.NudgeContext` from the `nudge.Context` + level, calls `l.deps.AI.Line` with `context.WithTimeout(ctx, 3s)`.

  **`HandleEvent`:**
  - `Type == "checkpoint-answer"`: `"y"` → `active.nudge.Reset()`, clear checkpoint deadline; `"n"` → `active.checkpointNoUntil = now + cfg.Drift.CheckpointPenalty`.
  - `Type == "nudge-action"`: `"d"` (drifted) / `"r"` (refocus) → force-close the open episode (`detail += " (self-reported)"`), reset `drift.State`; `"r"` additionally `nudge.Reset()`. `"snooze"` → `active.nudge.LastAt = now` (push the gap; expose a `nudge.State.Snooze(now, gap)` helper). `"b"` (break) → no-op here; the daemon sees the session row change on the next tick.

- [ ] **Step 1: Write the failing test**

Create `internal/daemon/daemon_test.go`:
```go
package daemon_test

import (
	"testing"
	"time"

	"pomo/internal/ai"
	"pomo/internal/daemon"
	"pomo/internal/db/dbtest"
	"pomo/internal/ipc"
	"pomo/internal/model"
	"pomo/internal/pomoconfig"
	"pomo/internal/watch"
)

type fakeWatch struct{ app watch.ForegroundApp }

func (f *fakeWatch) Foreground() (watch.ForegroundApp, error) { return f.app, nil }
func (f *fakeWatch) Supported() bool                          { return true }
func (f *fakeWatch) Name() string                             { return "fake" }

type fakeNotify struct{ n int }

func (f *fakeNotify) Send(string, string) error { f.n++; return nil }

type fakeBus struct{ events []ipc.Event }

func (f *fakeBus) Broadcast(e ipc.Event) { f.events = append(f.events, e) }

func baseCfg() pomoconfig.Config {
	c := pomoconfig.Config{}
	c.Daemon.Tick = time.Minute
	c.Drift = pomoconfig.DriftConfig{
		Enabled: true, DistractGrace: 3 * time.Minute, FsStale: 10 * time.Minute,
		CheckpointTimeout: 20 * time.Second, CheckpointPenalty: 2 * time.Minute,
		RecoverGrace: time.Minute,
		FocusApps:    pomoconfig.DefaultFocusApps(), DistractApps: pomoconfig.DefaultDistractApps(),
	}
	c.Nudge = pomoconfig.NudgeConfig{Enabled: true, MaxPerSession: 4, MinGap: 5 * time.Minute}
	c.CheckpointEnabled = false
	return c
}

func TestDriftEpisodeRecordedAndNudged(t *testing.T) {
	d := dbtest.NewTemp(t)
	id, err := d.CreateSession(model.Session{
		TaskName: "ship", PlannedDuration: 1500, Status: model.StatusRunning, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	fw := &fakeWatch{app: watch.ForegroundApp{Name: "Slack"}}
	fn := &fakeNotify{}
	fb := &fakeBus{}
	nAI, _, _ := ai.New(ai.Config{}) // no-op

	clock := time.Now()
	loop := daemon.NewLoop(daemon.Deps{
		DB: d, Watch: fw, Notify: fn, IPC: fb, AI: nAI, Cfg: baseCfg(),
		Now: func() time.Time { return clock },
	})

	// 5 ticks a minute apart, all on Slack → after grace an episode opens,
	// and once it has run a few minutes a nudge fires.
	for i := 0; i < 6; i++ {
		loop.Tick()
		clock = clock.Add(time.Minute)
	}

	events, err := d.DriftEventsForSession(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected a drift episode to be recorded")
	}
	if events[0].Trigger != "foreground" || events[0].Seconds < 60 {
		t.Fatalf("episode looks wrong: %+v", events[0])
	}
	if fn.n == 0 {
		t.Fatal("expected at least one nudge notification")
	}
	sawNudge := false
	for _, e := range fb.events {
		if e.Type == "nudge" {
			sawNudge = true
		}
	}
	if !sawNudge {
		t.Fatal("expected a nudge IPC event")
	}
}

func TestNoSessionClosesEpisodeAndGoesIdle(t *testing.T) {
	d := dbtest.NewTemp(t)
	id, _ := d.CreateSession(model.Session{
		TaskName: "x", PlannedDuration: 1500, Status: model.StatusRunning, StartedAt: time.Now(),
	})
	fw := &fakeWatch{app: watch.ForegroundApp{Name: "Discord"}}
	fb := &fakeBus{}
	nAI, _, _ := ai.New(ai.Config{})
	clock := time.Now()
	loop := daemon.NewLoop(daemon.Deps{
		DB: d, Watch: fw, Notify: &fakeNotify{}, IPC: fb, AI: nAI, Cfg: baseCfg(),
		Now: func() time.Time { return clock },
	})
	for i := 0; i < 5; i++ {
		loop.Tick()
		clock = clock.Add(time.Minute)
	}
	// finish the session; next tick must close the open episode and go idle
	_ = d.FinishSession(id, model.StatusCompleted, 300, "")
	loop.Tick()

	events, _ := d.DriftEventsForSession(id)
	for _, e := range events {
		if e.EndedAt == nil {
			t.Fatalf("episode %d left open after session ended", e.ID)
		}
	}
	last := fb.events[len(fb.events)-1]
	if last.Type != "idle" {
		t.Fatalf("last event = %q, want idle", last.Type)
	}
}

func TestCheckpointNoTriggersDrift(t *testing.T) {
	d := dbtest.NewTemp(t)
	id, _ := d.CreateSession(model.Session{
		TaskName: "x", PlannedDuration: 1500, Status: model.StatusRunning, StartedAt: time.Now(),
	})
	fw := &fakeWatch{app: watch.ForegroundApp{Name: "iTerm2"}} // focused — only checkpoint can cause drift
	nAI, _, _ := ai.New(ai.Config{})
	clock := time.Now()
	loop := daemon.NewLoop(daemon.Deps{
		DB: d, Watch: fw, Notify: &fakeNotify{}, IPC: &fakeBus{}, AI: nAI, Cfg: baseCfg(),
		Now: func() time.Time { return clock },
	})
	loop.Tick()
	loop.HandleEvent(ipc.Event{Type: "checkpoint-answer", Answer: "n"})
	loop.Tick()
	clock = clock.Add(time.Minute)
	loop.Tick()

	events, _ := d.DriftEventsForSession(id)
	if len(events) == 0 || events[0].Trigger != "checkpoint_no" {
		t.Fatalf("checkpoint 'no' should have opened a checkpoint_no episode: %+v", events)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/daemon/ -v`
Expected: FAIL — package does not exist.

- [ ] **Step 3: Implement `internal/daemon/status.go`**

```go
package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"pomo/internal/db"
)

type Status struct {
	PID            int       `json:"pid"`
	StartedAt      time.Time `json:"started_at"`
	LastTick       time.Time `json:"last_tick"`
	WatchBackend   string    `json:"watch_backend"`
	WatchSupported bool      `json:"watch_supported"`
	WatchingID     int64     `json:"watching_id"`
}

func statusPath() string { return filepath.Join(db.Dir(), "daemon.status") }

func WriteStatus(s Status) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statusPath(), b, 0o644)
}

func ReadStatus() (Status, bool) {
	b, err := os.ReadFile(statusPath())
	if err != nil {
		return Status{}, false
	}
	var s Status
	if json.Unmarshal(b, &s) != nil {
		return Status{}, false
	}
	return s, true
}
```

- [ ] **Step 4: Implement `internal/daemon/daemon.go`**

Write the `Loop`, `Deps`, `Broadcaster`, `sessionState`, `NewLoop`, `Tick`,
`HandleEvent`, and `aiLine` exactly per the algorithm in the Interfaces block
above. Key details to get right:
- `now()` helper: `if l.deps.Now != nil { return l.deps.Now() }; return time.Now()`.
- `tickSeconds := int(l.deps.Cfg.Daemon.Tick / time.Second)` for episode second accumulation.
- checkpoint scheduling uses `math/rand` seeded once; skip entirely when `!cfg.CheckpointEnabled`.
- `Broadcaster` lets the test pass `*fakeBus`; the real command passes `*ipc.Server`.
- Guard every `l.deps.DB.*` call; on error, log to stderr (`log.Println`) and continue — never panic in the loop.
- Add `nudge.State.Snooze(now time.Time, gap time.Duration)` to `internal/nudge/nudge.go` in this task (one-liner: `s.LastAt = now` — the gap is already enforced by `MinGap`; document that Snooze just resets the clock so the next opportunity is `MinGap` out).

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/daemon/ -v`
Expected: PASS (all three).

- [ ] **Step 6: Full suite + vet + race**

Run: `make test && go vet ./... && go test -race ./internal/daemon/ ./internal/ipc/`
Expected: PASS, no races.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(daemon): drift loop wiring watch, scoring, nudge, ipc"
```

---

### Task 7: `cmd/daemon.go` — run / start / stop / status

**Files:**
- Create: `cmd/daemon.go`
- Test: `cmd/daemon_test.go` (create — pidfile helpers only)

**Interfaces:**
- Consumes: `internal/daemon`, `internal/watch`, `internal/notify`, `internal/ipc`, `internal/ai`, `pomoconfig`, package-level `database`.
- Produces the `daemon` cobra command with four subcommands:
  - `pomo daemon run` — acquire an exclusive flock on `~/.pomo/daemon.lock` (fail fast if held); start `ipc.Serve(ipc.SocketPath(), loop.HandleEvent)`; build the real `daemon.Loop`; then `for { time.Sleep(cfg.Daemon.Tick); loop.Tick() }`. `SIGINT`/`SIGTERM` → close the ipc server, remove the lock, exit 0. Logs to stderr.
  - `pomo daemon start` — if `daemon status` shows a live PID, report and exit. Else spawn `exec.Command(os.Args[0], "daemon", "run")` with `Setsid` (detached), `Stdout`/`Stderr` to `~/.pomo/daemon.log`, write child PID to `~/.pomo/daemon.pid`, print "started (pid N)".
  - `pomo daemon stop` — read `daemon.pid`, `syscall.Kill(pid, SIGTERM)`, remove the pidfile, print "stopped". No pidfile → "not running".
  - `pomo daemon status` — read `daemon.pid` + `daemon.ReadStatus()`; print running/stopped, PID, last tick age, watch backend + supported, currently-watched session id.
- Helpers (tested): `readPidfile() (int, bool)`, `pidAlive(pid int) bool` (`syscall.Kill(pid, 0) == nil`), `writePidfile(pid int) error`, `removePidfile()`.

- [ ] **Step 1: Write the failing test**

Create `cmd/daemon_test.go`:
```go
package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPidfileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	_ = os.MkdirAll(filepath.Join(dir, ".pomo"), 0o755)

	if _, ok := readPidfile(); ok {
		t.Fatal("no pidfile yet, want ok=false")
	}
	if err := writePidfile(4242); err != nil {
		t.Fatal(err)
	}
	pid, ok := readPidfile()
	if !ok || pid != 4242 {
		t.Fatalf("readPidfile = %d, %v", pid, ok)
	}
	removePidfile()
	if _, ok := readPidfile(); ok {
		t.Fatal("pidfile should be gone after removePidfile")
	}
}

func TestPidAliveForSelf(t *testing.T) {
	if !pidAlive(os.Getpid()) {
		t.Fatal("current process should be alive")
	}
	if pidAlive(1 << 30) {
		t.Fatal("absurd pid should not be alive")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/ -run 'TestPidfile|TestPidAlive' -v`
Expected: FAIL — helpers undefined.

- [ ] **Step 3: Implement `cmd/daemon.go`**

Implement the four subcommands and the helpers per the Interfaces block.
Details:
- pidfile path: `filepath.Join(db.Dir(), "daemon.pid")` — import `pomo/internal/db`.
- `readPidfile`: read file, `strconv.Atoi(strings.TrimSpace(...))`.
- `pidAlive`: `syscall.Kill(pid, 0) == nil`.
- `run`: use `golang.org/x/sys`? No — `syscall.Flock` is in stdlib `syscall` on unix. Acquire with `syscall.Flock(fd, LOCK_EX|LOCK_NB)`; on `EWOULDBLOCK` print "daemon already running" and exit 1. Keep the `*os.File` open for the process lifetime.
- `start`: `cmd := exec.Command(exe, "daemon", "run")`; `cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}`; redirect to the log file; `cmd.Start()`; `writePidfile(cmd.Process.Pid)`; do **not** `Wait`.
- signal handling in `run`: `signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)`, on receipt close ipc + `syscall.Flock(fd, LOCK_UN)` + remove lock file + `os.Exit(0)`.
- `status`: compute `time.Since(status.LastTick).Round(time.Second)`.

- [ ] **Step 4: Run the helper tests**

Run: `go test ./cmd/ -run 'TestPidfile|TestPidAlive' -v`
Expected: PASS.

- [ ] **Step 5: Full suite + build + live smoke**

Run:
```bash
make test && go build -o pomo . && D=$(mktemp -d) && HOME=$D sh -c '
  ./pomo daemon status;
  ./pomo daemon start;
  sleep 2;
  ./pomo daemon status;
  ./pomo daemon stop;
  ./pomo daemon status'
```
Expected: suite PASS. `status` first prints "not running"; after `start`, prints running + a PID + watch backend `darwin/lsappinfo`; after `stop`, "not running" again. Check `$D/.pomo/daemon.log` has no panic.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(cmd): pomo daemon run/start/stop/status"
```

---

### Task 8: Docs

**Files:**
- Modify: `CLAUDE.md`
- Modify: `README.md`
- Modify: `specs/README.md`

**Interfaces:**
- Consumes: nothing.
- Produces: docs describe the daemon and its packages.

- [ ] **Step 1: CLAUDE.md**

Under **Commands**, add:
```
- `pomo daemon start|stop|status` — background drift-detection daemon
  (darwin foreground-app watching; other OS runs checkpoint + fs signals only).
  `pomo daemon run` is the foreground loop the detached process executes.
```

Under **Architecture**, add:
```
- **`internal/watch`** — foreground-app reader (darwin: `lsappinfo`/`osascript`;
  `!darwin`: stub), `Classify` (focus/neutral/distract vs config lists), and
  `RepoActive` (bounded early-exit `WalkDir` for the fs-staleness signal).
- **`internal/drift`** — pure per-tick scorer: `State.Step(now, Signals, cfg)
  -> Result{Drifting, Trigger, Detail}`. No I/O.
- **`internal/nudge`** — pure escalation state machine (L1–L3, min-gap,
  max-per-session), AI line with canned fallback and de-dup.
- **`internal/notify`** — fire-and-forget desktop notifications
  (`terminal-notifier`/`osascript`/`notify-send`), never a blocking dialog.
- **`internal/daemon`** — the `Loop`: one `Tick()` gathers signals, scores
  drift, writes `drift_events` episodes, fires nudges over `notify` + `ipc`.
  Fully unit-tested with fakes and an injected clock; `cmd/daemon.go` is the
  thin sleep-loop + pidfile/flock wrapper around it.
- **`internal/gitinfo`** — `git rev-parse` wrapper; `pomo start` and the TUI
  stamp `sessions.repo_path` / `repo_branch` from the cwd at launch.
```

- [ ] **Step 2: README.md**

In the "Roadmap — ADHD focus layer" section, move the drift-daemon and nudge
bullets from future to shipped, and update the "Shipped so far" line:
```
Shipped so far: the drift-detection daemon (`pomo daemon`), gentle escalating
nudges (AI-authored with a key, canned otherwise), the `drift_events` schema and
the `pomo review` aggregation, plus the config surface. Still to come: the TUI
slash-command surface (`/review`, `/insights`, `/drift`, `/chat`) and the weekly
digest file.
```

- [ ] **Step 3: specs/README.md**

Update the Progress list: mark chunk 3 done, leave chunk 4 not started.

- [ ] **Step 4: Verify + commit**

Run: `make test`
Expected: PASS.
```bash
git add -A
git commit -m "docs: document the drift daemon and its packages"
```

---

## Self-Review

**1. Spec coverage**

`specs/2026-09-04-daemon-drift-detection.md`:
- §1 `pomo daemon` run/start/stop/status, single-instance flock — Task 7. Launchd/systemd unit generation is **deliberately deferred**: this chunk uses a detached process + pidfile (`Setsid`), which is simpler and reversible. The spec's §1 offered "OS unit **or** detached process with pidfile" — the pidfile path is taken. Noted, not a gap.
- §2 loop, `activeState`, tick algorithm (foreground/fs/checkpoint/score/episode/nudge) — Task 6. ✓
- §3 `internal/watch` interface, darwin impl, `!darwin` stub, 2s exec timeout, title degradation — Task 2. ✓
- §4 classification lists + title-hint downgrade + precedence — Task 2 `Classify` + tests. ✓
- §5 repo-path capture on `pomo start` and TUI — Task 1. ✓
- §6 config keys — already shipped in chunk 2; consumed here. ✓
- §7 failure handling (watch error → neutral, fs setup fail → skip, DB error → log+continue, IPC no client → dropped) — Task 6 Step 4 details + `ipc` already drops. ✓
- `drift_events` schema, `Open/Update/CloseDriftEpisode`, `DriftEventsForSession` — shipped chunk 1; consumed here. ✓
- Browser tab-title via AppleScript with TCC-denial fallback to `""` — Task 2 `watch_darwin.go` `title()`. ✓

`specs/2026-09-04-nudge-and-ai.md`:
- §1 `internal/nudge` state machine, escalation table, min-gap, max-per-session, checkpoint-reset, canned templates, action lists — Task 5. ✓
- §1 actions handled by daemon via IPC (`d`/`r`/`b`/`snooze`) — Task 6 `HandleEvent`. `b` (break) is a documented no-op (session row change drives it). ✓
- §2 `internal/notify` darwin/linux/no-op, no blocking dialog, 2s timeout — Task 4. ✓
- §3 `internal/ai` — shipped chunk 2; the daemon's `aiLine` wraps `ai.Nudger.Line` with a 3s context and falls back to canned via `nudge` — Task 6. ✓
- §4 config keys — chunk 2. ✓

Deferred to chunk 4 (not gaps — no daemon-side work): TUI `screenChat`/slash palette, `pomo digest` + weekly auto-write, `/settings` rows, the TUI-side `ipc` client and checkpoint/nudge overlays. The daemon already broadcasts the `checkpoint` / `nudge` / `watching` / `idle` events those will consume.

**2. Placeholder scan** — Task 6 Step 4 is described as "write per the Interfaces block" rather than a full code listing, because the algorithm is fully specified step-by-step in the Interfaces block (9 numbered steps + `HandleEvent` rules + `aiLine` definition) and every type it uses is defined. This is a large but completely determined function; the numbered algorithm is the spec an implementer follows. Tasks 1–5, 7 have full code. No "TODO"/"handle errors"/"similar to".

**3. Type consistency**
- `watch.ForegroundApp{Name,BundleID,Title}` — Task 2 def; used by `Classify` (Task 2), the daemon Tick (Task 6), and `fakeWatch` (Task 6 test). Fields match.
- `watch.Class` + `Focus`/`Neutral`/`Distract` consts — Task 2 def; consumed by `drift.Signals.Class` and `drift.State.Step` (Task 3) and the daemon (Task 6). ✓
- `watch.Classify(ForegroundApp, pomoconfig.DriftConfig) Class` — Task 2 def, Task 6 use. ✓
- `watch.RepoActive(string, time.Time) bool` — Task 2 def, Task 6 use (step 4). ✓
- `watch.Watcher` interface (`Foreground`/`Supported`/`Name`) — Task 2 def; `fakeWatch` (Task 6 test) implements exactly these three; `daemon.Deps.Watch` is `watch.Watcher`. ✓
- `drift.Signals{Class,Detail,FsStale,CheckpointNo}`, `drift.State`, `drift.Result{Drifting,Trigger,Detail}`, `State.Step(time.Time, Signals, pomoconfig.DriftConfig) Result` — Task 3 def; Task 6 step 6 constructs `drift.Signals{...}` with exactly these fields and reads `res.Drifting`/`res.Trigger`/`res.Detail`. ✓
- `nudge.Context`, `nudge.Decision{Fire,Level,Text,Actions}`, `nudge.State`, `State.Evaluate(now, episodeOpen, Context, pomoconfig.NudgeConfig, LineFn) Decision`, `State.Reset()`, `State.Snooze(now, gap)` — Task 5 def (Snooze added in Task 6 Step 4, noted); Task 6 step 8 calls `Evaluate` with a `nudge.Context{Task,DistractApp,DistractDetail,DriftMinutes,SessionMinutes,Hour}` literal — matches the struct. ✓
- `nudge.LineFn = func(Context) (string, error)` — Task 5 def; `daemon.aiLine` has this signature (Task 6). ✓
- `notify.Notifier.Send(title, body string) error` — Task 4 def; `fakeNotify.Send` (Task 6 test) matches; `daemon.Deps.Notify` is `notify.Notifier`. ✓
- `daemon.Deps{DB,Watch,Notify,IPC,AI,Cfg,Now}`, `daemon.Broadcaster.Broadcast(ipc.Event)`, `daemon.NewLoop(Deps) *Loop`, `(*Loop).Tick()`, `(*Loop).HandleEvent(ipc.Event)` — Task 6 def; Task 6 tests use exactly this shape; `*ipc.Server.Broadcast(Event)` (chunk 2) satisfies `Broadcaster`. ✓
- `ipc.Event` fields `Type`/`SessionID`/`RepoPath`/`Text`/`Level`/`Actions`/`Answer`/`Action` — chunk 2 def; daemon broadcasts `{Type:"watching",SessionID,RepoPath}`, `{Type:"nudge",Text,Level,Actions}`, `{Type:"checkpoint",Text}`, `{Type:"idle"}` and reads `e.Answer`/`e.Action` in `HandleEvent`. All fields exist. ✓
- `ai.Nudger.Line(context.Context, ai.NudgeContext) (string, error)` + `ai.NudgeContext` fields — chunk 2 def; `daemon.aiLine` builds `ai.NudgeContext{Task,Tag,DistractApp,DistractDetail,DriftMinutes,SessionMinutes,Level,Hour}` — matches chunk-2 struct. ✓
- `gitinfo.Describe(string) (string, string)` — Task 1 def; Task 1 Steps 5–6 use it. ✓
- `db.DriftEventsForSession`, `db.OpenDriftEpisode`, `db.UpdateDriftEpisode`, `db.CloseDriftEpisode`, `db.LastRunningSession` — all chunk 1, signatures unchanged. ✓
- `pomoconfig.Config.Daemon.Tick`, `.Drift.*`, `.Nudge.*`, `.CheckpointEnabled`, `pomoconfig.DefaultFocusApps()`/`DefaultDistractApps()` — chunk 2; used across Tasks 2/3/5/6 tests and impl. ✓

No inconsistencies found.
