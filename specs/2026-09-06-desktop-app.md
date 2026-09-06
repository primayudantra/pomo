# Desktop App — Design Spec

Status: approved for planning (2026-09-06).

A native macOS desktop app for pomo, built on the existing Go core. First
desktop surface alongside the Cobra CLI and the Bubble Tea TUI.

---

## 1. Goal & scope

Give the user a windowed + menubar Pomodoro experience without a terminal,
reusing `internal/*` directly.

**v1 scope — "Timer + quick-start + today":**

- Quick-start: type/pick a task, pick a duration, run it.
- Running timer: visible countdown, pause/resume, cancel, break prompt.
- Today: list of today's sessions, focus total, streak, tracking toggle.
- Menubar (tray) countdown + quick controls; window is optional.
- One-click start/stop of the drift daemon ("tracking on/off").

**Explicitly out of v1** (backlog): history/stats screens, drift overlay UI,
`/review` `/insights` `/chat` AI surface, settings screen, Linux/Windows,
code-signing + notarization + DMG, repo/branch stamping for desktop sessions.

---

## 2. Architecture

```
┌─────────────────────────────────────────┐
│  pomo-desktop (Wails v2 app, one binary) │
│                                         │
│  Frontend: Svelte + Vite, native webview │
│      │  Wails bindings (JSON-RPC)       │
│  Go backend (desktop/)                  │
│   ├─ TimerService   → owns countdown    │
│   ├─ SessionService → internal/db       │
│   ├─ DaemonService  → start/stop/status │
│   └─ tray           → menubar title+menu│
└──────────────┬──────────────────────────┘
               │ direct, WAL mode, 5s busy timeout
        ~/.pomo/pomo.db  ← also CLI, also daemon
```

- Desktop app is a **third independent client** of `~/.pomo/pomo.db`. No IPC
  dependency in v1. Reuses `internal/db`, `internal/model`,
  `internal/pomoconfig`, `internal/sound`, `internal/notify`.
- The drift daemon stays a separate detached process. The desktop app only
  spawns / stops / polls it — a UI switch over the same code path as
  `pomo daemon start|stop|status`.
- The "only one `running` session at a time" invariant is preserved: every
  start path calls `database.LastRunningSession()` first and refuses if one
  exists (started by CLI, TUI, or a previous desktop run).

### Toolkit decision

**Wails v2 (stable)**, not v3 (alpha). v3 has nicer tray/multiwindow but its
API is still churning. Tray via the `systray` integration. Revisit v3 post-v1.

Pure Go, no cgo (consistent with `modernc.org/sqlite`). macOS only in v1
(consistent with the darwin focus of `internal/watch` and `internal/sound`).

---

## 3. Code layout

New top-level `desktop/` directory. `cmd/` stays Cobra-only; `main.go` at the
repo root is unchanged. The desktop app is a separate build target / binary.

```
desktop/
  main.go        — Wails bootstrap, tray setup, single-instance lock
  app.go         — App struct, lifecycle hooks (startup / shutdown)
  timer.go       — TimerService: state machine + ticker goroutine
  session.go     — SessionService: thin wrapper over internal/db
  daemon.go      — DaemonService: start / stop / status via internal/daemon
  tray.go        — menubar title updater + menu items
  events.go      — event-name constants (backend → frontend emits)
  frontend/
    index.html
    src/
      App.svelte
      lib/
        api.ts       — Wails bindings re-export + typed helpers
        stores.ts    — Svelte stores: timer, today, daemon
      screens/
        QuickStart.svelte
        Timer.svelte
        Today.svelte
    package.json  vite.config.ts  svelte.config.js
  wails.json
```

- `Makefile`: add `make desktop-dev` (`wails dev`) and `make desktop-build`
  (`wails build`).
- `cmd/desktop.go` — new Cobra command `pomo desktop`. Locates the
  `pomo-desktop` binary (next to the running `pomo` executable first, then
  `$PATH`), execs it, and returns. If not found: an error telling the user to
  run `make desktop-build` / install the app. Same binary-discovery helper
  `DaemonService` uses to find `pomo`.
- `.gitignore`: `desktop/frontend/node_modules`, `desktop/frontend/dist`,
  `desktop/build/bin`.

### Extract `internal/daemon` spawn helpers

The process-spawn / pidfile / flock wrapper currently in `cmd/daemon.go` moves
into `internal/daemon` as `Spawn()`, `Stop()`, `Status()`. `cmd/daemon.go`
becomes a thin caller — no behavior change; existing daemon tests still cover
it. `DaemonService` in the desktop app calls the same functions.

---

## 4. TimerService (backend)

The running timer lives in Go, in one goroutine. Closing or minimizing the
window keeps it alive; the tray reflects state.

### State machine

```
idle ──start(task,dur)──▶ running ──tick(1s)──▶ running
running ──pause──▶ paused ──resume──▶ running
running ──(remaining==0)──▶ completed ──▶ break_prompt
running ──cancel──▶ idle
break_prompt ──startBreak(dur)──▶ break_running ──▶ idle
break_prompt ──skipBreak──▶ idle
```

### Behavior

- **start**: check `LastRunningSession()`; if present, return an error to the
  UI and stay idle. Else insert a `sessions` row with status `running`.
  `repo_path` / `repo_branch` left null in v1 (app cwd is the home dir).
- **tick**: `time.Ticker` at 1s. `remaining` is recomputed from the wall
  clock each tick (`endsAt - now`), never decremented — correct across
  sleep / suspend. Emits `timer:tick` `{remaining, phase}`.
- **complete**: update the row → `completed`; play the completion clip via
  `internal/sound`; fire a desktop notification via `internal/notify`; emit
  `timer:completed`.
- **pause / resume**: the row stays `running` (status only changes on terminal
  states — matches CLI semantics). Pause time is tracked in the DB (below).
- **cancel**: update the row → `cancelled`; emit `timer:cancelled`.
- **startup rehydrate**: on launch, `LastRunningSession()`. If a `running`
  session exists, reconstruct timer state from `started_at`, `duration`, and
  the pause columns — including "was paused when the app quit". If the
  computed end is already in the past, close the row `completed`
  retroactively and go to `break_prompt` (or idle if the break window has
  also passed).
- **crash safety**: no separate timer table. The `sessions` row plus the
  pause columns are the only source of truth.

### Pause persistence — schema change

Additive, via `ensureColumn` in `db.OpenAt` (SQLite `ALTER TABLE ADD COLUMN`
is not idempotent, so `ensureColumn` guards it):

```
sessions.paused_at         TEXT    NULL             -- RFC3339, set on pause, cleared on resume
sessions.pause_accum_secs  INTEGER NOT NULL DEFAULT 0 -- cumulative paused seconds
```

- **pause**: `paused_at = now`.
- **resume**: `pause_accum_secs += now - paused_at`; clear `paused_at`.
- **remaining** =
  `duration - (now - started_at - pause_accum_secs - pausedNow)`
  where `pausedNow = (now - paused_at)` if currently paused, else 0.
- The CLI ignores the new columns (no behavior change). If cross-terminal
  pause is wanted later, the columns already exist.

### `internal/report` pause accounting

`internal/report` aggregation must subtract `pause_accum_secs` when computing
actual focus time, otherwise paused sessions overcount. Audit the current
calc and adjust; add a test. Small, included in this work.

---

## 5. Tray + window lifecycle

- **Single instance**: flock on `~/.pomo/desktop.lock` at startup. A second
  launch focuses the existing window and exits.
- **Tray title**: `🍅 24:13` while running, `☕ 4:30` on break, icon-only
  when idle.
- **Tray menu**: *Show Window* · *Start…* (opens the window on QuickStart) ·
  *Pause* / *Resume* (context-sensitive) · *Cancel session* · ─── ·
  *Tracking: ● / ○* (daemon toggle) · ─── · *Quit*.
- **Window close** = hide, not quit. The backend timer keeps running. Quit
  only via the tray menu or Cmd-Q.
- **Quit with a running session**: confirm dialog — *leave it running* /
  *cancel it* / *stay*. Leaving it running is fine; it rehydrates on next
  launch.
- **Dock icon**: visible in v1 (accessory-only mode is a later option).
- The frontend receives `timer:*` events whether or not the window is shown.
  On window show it calls `GetState()` for a full snapshot to resync.

---

## 6. DaemonService (tracking toggle)

- `Status() -> {running bool, pid int}` — from `~/.pomo/daemon.pid` plus a
  flock probe.
- `SetTracking(on bool)` — spawns the same detached `pomo daemon run`
  process the CLI does, or stops it. The desktop app quitting does **not**
  stop the daemon (independent lifecycle — the user wants explicit control).
- The UI polls `Status()` every ~5s while the window is open, so an external
  `pomo daemon stop` is reflected.
- Spawn-failure surface (e.g. `pomo` not on `PATH`): toast in the UI. The
  desktop build looks for the `pomo` binary next to its own executable
  first, then falls back to `$PATH`.

---

## 7. Frontend (Svelte)

**Stores** (`stores.ts`):

- `timer` — `{phase, remaining, task, duration}`, fed by `timer:*` events.
- `today` — `{sessions[], totals}`.
- `daemon` — `{running}`.

**QuickStart.svelte** — autofocused task text input + duration chips
(15 / 25 / 50 / custom) → `TimerService.Start(task, dur)`. Enter submits.
Error banner when a session is already running elsewhere.

**Timer.svelte** — large `MM:SS` in monospace numerals, task name, phase
label, SVG circular progress ring driven by `remaining / duration`. Buttons:
Pause/Resume, Cancel. On `completed`, an inline break prompt: *Start break
(5m)* / *Skip*. Break end → QuickStart.

**Today.svelte** — header: date, total focus time, streak, `Tracking ● / ○`
toggle. List of today's sessions: task · duration · status pill
(completed / cancelled / skipped). Source: `SessionService.Today()`.

**Nav** — no router. A single `view` derived from `timer.phase`
(idle → QuickStart unless the user opened Today; running/paused → Timer;
break → Timer). A small tab switch between QuickStart and Today while idle.

**Theme** — dark-first, single tomato-red accent, monospace numerals for the
countdown. Matches the terminal aesthetic.

---

## 8. Build & distribution

- `wails dev` — Vite HMR + Go rebuild. Node 20+ needed at dev time only.
- `wails build` — `pomo-desktop.app`; Go core compiled in, frontend assets
  embedded via `embed.FS`. End users need nothing installed.
- No cgo. macOS only.
- v1 distribution: unsigned `.app` in a zip via `make desktop-build`.
  Notarization + DMG deferred.

---

## 9. Testing

- **TimerService** — injected clock (same pattern as the `internal/daemon`
  tests). Table tests: start → tick → complete; pause/resume offset math;
  rehydrate-from-db for running / paused / already-expired sessions.
- **SessionService** — `db.OpenAt(tmpfile)`: insert + read today, the
  running-session guard.
- **DaemonService** — fake spawn func; assert start / stop / status
  transitions.
- **Schema** — `ensureColumn` idempotency test for the two new columns.
- **report** — focus-time calc subtracts `pause_accum_secs`.
- **Frontend** — Vitest for the store reducers (event → state). No E2E in v1.

---

## 10. Risks

| Risk | Mitigation |
|---|---|
| Wails v3 alpha API churn | pin Wails v2 stable |
| Three processes writing the DB | already WAL + 5s busy timeout; desktop writes are small and infrequent |
| Timer drift on sleep / suspend | recompute `remaining` from the wall clock, never decrement |
| daemon spawn can't find `pomo` | look next to the app bundle, then `$PATH`; toast on failure |
| repo stamping (app cwd is home) | leave `repo_path` null for desktop sessions in v1 |
| Node toolchain in the repo | dev-time only; gitignore `node_modules` / `dist`; users get a self-contained `.app` |

---

## 11. Build order (for the plan)

1. Schema: `paused_at` + `pause_accum_secs` via `ensureColumn` + tests;
   `internal/report` pause accounting + test.
2. Extract `internal/daemon` `Spawn` / `Stop` / `Status`; repoint
   `cmd/daemon.go`; confirm existing tests pass.
3. `desktop/` skeleton: Wails v2 + Svelte scaffold, `App` struct, build
   targets in the `Makefile`, single-instance lock. Add `cmd/desktop.go`
   (`pomo desktop` → locate + exec `pomo-desktop`).
4. `TimerService` — state machine, ticker, DB writes, rehydrate; tests.
5. `SessionService` + `DaemonService`; tests.
6. Tray: title updater + menu, window hide-on-close, quit confirm.
7. Frontend: stores + `timer:*` wiring, then QuickStart, Timer, Today.
8. Manual checklist: run/pause/quit/relaunch, break flow, tracking toggle,
   external `pomo daemon stop`, second-launch focus.
