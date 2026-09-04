# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

- `make build` — `go build -o pomo .`
- `make install` — build + copy to `~/.local/bin/pomo`
- `go run .` — run without installing
- `go vet ./...` — static checks (no test suite exists yet)

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

## Conventions

- Only one session may be `running` at a time — commands check
  `database.LastRunningSession()` before starting.
- Cross-terminal control is limited: `pause`/`resume`/`stop` from another terminal
  mostly just tell you to use the in-timer keybinding (`[p]`/`[r]`).
- New commands: add a file in `cmd/`, register in its `init()` with
  `rootCmd.AddCommand(...)`.
