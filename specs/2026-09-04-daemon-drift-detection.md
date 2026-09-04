# Daemon + Drift Detection

Date: 2026-09-04
Parent: `2026-09-04-adhd-focus-overview.md`

## 1. `pomo daemon`

New file `cmd/daemon.go`. Subcommands:

- `pomo daemon` (or `pomo daemon run`) — run the loop in the foreground. This is
  what the OS unit invokes. Logs to stderr.
- `pomo daemon start` — write the OS unit (launchd plist on darwin,
  systemd user unit on linux), load it, then report status. If units are
  unsupported on the platform, spawn a detached background process and record its
  PID in `~/.pomo/daemon.pid`.
- `pomo daemon stop` — unload the unit / kill the PID; remove the pidfile.
- `pomo daemon status` — up/down, PID, last tick time, current watched session,
  watch backend + `Supported()` result.

Single-instance guard: on `run`, acquire an exclusive flock on
`~/.pomo/daemon.lock`. Second instance exits with a clear message.

## 2. Loop

```
for {
    sleep(cfg.Daemon.Tick)          // default 15s
    sess, _ := db.LastRunningSession()
    if sess == nil {
        maybeWeeklyDigest(db)       // see review-and-digest spec
        activeState = nil
        continue
    }
    if activeState == nil || activeState.sessionID != sess.ID {
        activeState = newActiveState(sess)   // resets nudge machine, schedules checkpoint
        ipc.Broadcast(Event{Type: "watching", SessionID: sess.ID})
    }
    tick(activeState, sess)
}
```

`activeState` holds: session id, repo path, fsnotify watcher, `lastWriteAt`,
nudge escalation level, last-nudge time, nudge count, checkpoint schedule +
answer, current open drift episode (if any).

### `tick(state, sess)`

1. **Foreground signal** — `watch.Foreground()` → `ForegroundApp{Name, BundleID,
   Title}`. Classify via `classify(app, cfg)` → `focus | distract | neutral`.
2. **fs signal** — if `state.repoPath != ""`, drain fsnotify events, update
   `lastWriteAt` on any write/create/rename under the repo (ignore `.git/`,
   editor swap files, `node_modules`). `fsStale := time.Since(lastWriteAt) >
   cfg.Drift.FsStale` (default 10m).
3. **Checkpoint** — if now ≥ scheduled checkpoint time and not yet asked:
   `ipc.Broadcast(Event{Type:"checkpoint"})`, mark asked, start a
   `cfg.Drift.CheckpointTimeout` (20s) timer. Answer arrives via IPC
   (`checkpoint-answer y|n`); timeout counts as `n`.
4. **Score** — the tick is *drifting* when any of:
   - foreground class == `distract` for a continuous run ≥ `cfg.Drift.DistractGrace`
     (default 3m) — tracked by `distractSince`;
   - `fsStale && foregroundClass != focus`;
   - checkpoint answered `n` (drifting for this tick and the next
     `cfg.Drift.CheckpointPenalty` = 2m).
5. **Episode bookkeeping**
   - drifting and no open episode → open one: `{started_at: now, trigger:
     <dominant reason>, detail: <app name + title>}`.
   - drifting and open episode → extend: `seconds += tick`, update `detail` to the
     dominant app.
   - not drifting and open episode: increment a `recoveredTicks` counter; when
     `recoveredTicks * tick >= cfg.Drift.RecoverGrace` (default 1m), close the
     episode (`ended_at`, final `seconds`), write the row, reset counter.
6. **Nudge** — pass the episode/score state to `nudge.Evaluate(state)` (see
   nudge spec). It may return an action → `notify.Send` + `ipc.Broadcast`.

All DB writes go through new `internal/db` helpers (see schema spec):
`OpenDriftEpisode`, `UpdateDriftEpisode`, `CloseDriftEpisode`.

## 3. `internal/watch`

```go
package watch

type ForegroundApp struct {
    Name     string // "Google Chrome"
    BundleID string // "com.google.Chrome" (darwin); "" elsewhere
    Title    string // active window / tab title; "" if unavailable
}

type Watcher interface {
    Foreground() (ForegroundApp, error)
    Supported() bool   // false → daemon runs checkpoint + fs signals only
    Name() string      // "darwin/lsappinfo", "linux/xdotool", "unsupported"
}

func New() Watcher   // build-tag dispatch by GOOS
```

### darwin (`watch_darwin.go`)

- Front app + bundle id: `lsappinfo front` → asn, then
  `lsappinfo info -only name -only bundleID <asn>`. Parse the quoted values.
  No extra permission required.
- Window / tab title, best effort:
  - Chrome/Brave/Edge/Arc: `osascript -e 'tell application "<app>" to get title
    of active tab of front window'`.
  - Safari: `... name of current tab of front window`.
  - Generic fallback: System Events `... name of front window of (first process
    whose frontmost is true)`.
  - First call may raise the macOS automation (TCC) prompt. On any error
    (`-1743` not authorised, app not scriptable, timeout): set `Title = ""`,
    log once at info, and set an internal `titleDegraded` flag surfaced in
    `pomo daemon status`. **Never block; never re-prompt in a loop.**
- Wrap every `osascript` call in a 2s `exec.CommandContext` timeout.

### linux (`watch_linux.go`)

Best effort, in order of availability:
- Hyprland: `hyprctl activewindow -j` → `.class`, `.title`.
- X11: `xdotool getactivewindow getwindowname` + `xprop -id <id> WM_CLASS`.
- Else `Supported() == false`.

### other (`watch_other.go`)

`Supported() == false`, `Foreground()` returns `ErrUnsupported`.

## 4. Classification

`classify(app ForegroundApp, cfg) string`:

- Config holds three lists of match rules (`cfg.Drift.FocusApps`,
  `DistractApps`, `NeutralApps`), each rule a lowercase substring matched
  against `Name`, `BundleID`, or `"<Name>: <Title>"`.
- Defaults:
  - focus: `terminal`, `iterm`, `alacritty`, `kitty`, `wezterm`, `ghostty`,
    `code`, `vscode`, `zed`, `nvim`, `vim`, `emacs`, `jetbrains`, `xcode`,
    `preview`, `dash`.
  - distract: `chrome`, `safari`, `firefox`, `arc`, `brave`, `edge`, `slack`,
    `discord`, `telegram`, `whatsapp`, `messages`, `mail`, `youtube`,
    `twitter`, `x.com`, `reddit`, `netflix`, `steam`.
  - Browsers appear in `distract` by default, but a title containing any
    `cfg.Drift.FocusTitleHints` (default: `localhost`, `github.com`,
    `stackoverflow`, `pomo`, `docs.`) reclassifies that reading to `neutral`
    (not `focus` — browsing docs is not the same as writing code).
- Precedence: explicit user rule > title hint > default list > `neutral`.

## 5. Repo path capture

- `cmd/start.go` `runStart`: before creating the session, capture
  `cwd, _ := os.Getwd()` and `repoRoot := gitToplevel(cwd)` (`git rev-parse
  --show-toplevel`, empty on failure), `branch := gitBranch(cwd)`.
- TUI-started sessions: capture the `pomo` process cwd at program start, store on
  `App`, pass through when the session is created.
- Persist to the new `sessions.repo_path` / `sessions.repo_branch` columns via an
  extended `CreateSession`.
- The daemon reads these columns for its fs watch and (later) git-aware features.

## 6. Config keys (defined fully in schema spec)

`daemon.tick`, `drift.distract_grace`, `drift.fs_stale`,
`drift.checkpoint_timeout`, `drift.checkpoint_penalty`, `drift.recover_grace`,
`drift.focus_apps`, `drift.distract_apps`, `drift.neutral_apps`,
`drift.focus_title_hints`, `drift.enabled`.

## 7. Failure handling

- `watch.Foreground()` error on a tick → treat foreground class as `neutral` for
  that tick, do not open an episode from the foreground rule alone, log at debug.
- fsnotify watcher setup failure → disable the fs signal for the session, log
  once, continue.
- DB write failure → log at error, keep looping (next tick retries the episode
  close).
- IPC broadcast with no client connected → silently dropped.
