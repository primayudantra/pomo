# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

- `make build` — `go build -o pomo .`
- `make install` — build + copy to `~/.local/bin/pomo`
- `go run .` — run without installing
- `make test` / `go test ./...` — test suite (add `-run TestName ./pkg/` for one test)
- `go vet ./...` — static checks

Requires Go 1.25.

## What it is

Local-first terminal Pomodoro tracker. Cobra CLI + Bubble Tea TUI. SQLite via
`modernc.org/sqlite` (pure Go, no cgo). See `prod_specs.md` for product intent.

## Architecture

- **`main.go` → `cmd/`** — Cobra command tree. `cmd/root.go` `Execute()` opens the
  DB once, stashes it in the package-level `database` var, and every command reads
  that. Bare `pomo` with no args opens the interactive dashboard; `pomo <task>` is
  a shortcut for `pomo start <task>`.
- **`internal/db`** — `DB` wraps `*sql.DB`. Schema is an inline `CREATE TABLE IF
  NOT EXISTS` string run on `Open()` (no migration framework). Tables: `tasks`,
  `sessions`, `config` (key/value). DB file lives at `~/.pomo/pomo.db`.
- **`internal/model`** — plain structs (`Task`, `Session`) + `SessionStatus`
  constants (`running`/`completed`/`cancelled`/`skipped`/`interrupted`). Durations
  stored as int seconds.
- **`internal/pomoconfig`** — typed `Config` loaded from the `config` table with
  hardcoded `defaults()`; `pomo config set <key> <value>` writes string values back.
- **`internal/tui`** — one persistent Bubble Tea `App` (`app.go`) drives ALL
  interactive screens (dashboard, task select, timer, note, settings, stats) via a
  `screen` enum. Design rule: screen transitions stay inside this one program so
  skip/cancel/complete never drop the user back to the shell — only explicit quit
  from the dashboard exits. `theme.go` = lipgloss styles.
- **`internal/sound`** — 4 mp3 clips `//go:embed`-ed; playback shells out to a
  platform audio player (`afplay` on macOS) against a temp file.
- **`internal/report`** — all windowed aggregation (sessions + `drift_events`) and
  rendering (text / markdown / JSON). Shared by `pomo review`, the TUI stats
  screen, and (later) the daemon's weekly digest. Day-bucket and streak helpers
  live here, not in `internal/tui`.
- **`internal/ipc`** — newline-delimited-JSON Unix-domain-socket channel
  (`~/.pomo/daemon.sock`) between the planned daemon (server, fan-out
  `Broadcast`) and the TUI (client). Not yet wired to anything.
- **`internal/ai`** — BYOK provider layer (`anthropic` | `openrouter`, raw
  `net/http`) behind `Nudger` / `Recapper` / `Chatter`. Empty provider →
  no-op returning `ErrNoProvider`. Not yet wired to anything.

## Conventions

- Only one session may be `running` at a time — commands check
  `database.LastRunningSession()` before starting.
- Cross-terminal control is limited: `pause`/`resume`/`stop` from another terminal
  mostly just tell you to use the in-timer keybinding (`[p]`/`[r]`).
- New commands: add a file in `cmd/`, register in its `init()` with
  `rootCmd.AddCommand(...)`.
- Two processes will open `~/.pomo/pomo.db` (CLI + planned daemon); it runs in WAL
  mode with a 5s busy timeout. Keep writes small. `db.OpenAt(path)` is the
  test/tools entrypoint; `db.Open()` is the normal one.
- Additive schema changes: new tables via the `schema` const
  (`CREATE TABLE IF NOT EXISTS`); new columns via `ensureColumn` in `OpenAt`
  (SQLite `ALTER TABLE ADD COLUMN` is not idempotent).
- Config keys are grouped (`drift.*`, `nudge.*`, `ai.*`, `daemon.*`, `digest.*`);
  `pomoconfig.Load` parses them in one pass, bad values fall back to the default.
  `pomo config set` validates against `configKeys` in `cmd/config.go`. `ai.key` is
  masked (`pomoconfig.MaskKey`) wherever shown.
