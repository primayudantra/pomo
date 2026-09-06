# pomo

A terminal Pomodoro productivity tracker. Local-first — no account, no backend,
no cloud sync. One SQLite file at `~/.pomo/pomo.db`.

It answers three questions: **What am I working on? How long did I spend on it?
What did I actually get done?**

```
pomo                       # interactive dashboard
pomo "Fix reconciliation"  # start a 25-minute focus session on that task
pomo review week           # focus-vs-plan + drift for the week
```

## Install

```sh
make install    # builds and copies to ~/.local/bin/pomo
# or
go build -o pomo .
```

Requires Go 1.25.

## Features

### Interactive dashboard (`pomo`)

A single Bubble Tea app. Everything happens inside it — task picker → duration →
running timer → note prompt → back to the dashboard. Skip / cancel / complete
never drop you back to the shell; only an explicit quit does.

- Arrow-key task list with per-task session dots.
- Live countdown timer with pause `[p]` / resume `[r]` / skip `[s]`.
- Settings screen: focus / short-break / long-break lengths, sessions-before-long,
  auto-start break / focus, sound on/off, start-sound choice, notifications.
- Stats screen: GitHub-style contribution heatmap, current & longest streak,
  most-active day, day / week / month breakdown bars.

### Sessions

Every session is recorded with its planned vs actual duration and an outcome:
`completed`, `cancelled`, `skipped`, or `interrupted`. Optional end-of-session
note. Optional tag. Only one session runs at a time.

### Tasks

Lightweight task list — add, list open, mark done, delete. Starting a session by
name reuses an open task or creates one.

### Reporting

| Command | Shows |
|---|---|
| `pomo today` | today's summary |
| `pomo week` | last 7 days |
| `pomo month` | this month |
| `pomo stats` | full stats (also in the TUI) |
| `pomo history` | session log, filterable by `--today` / `--yesterday` / `--week` / `--date` / `--task`, `--details` for notes |
| `pomo review [today\|week\|month\|YYYY-Www\|YYYY-MM]` | focus vs plan, drift totals, per-tag breakdown, best hour, streak — `--json` for scripts |

### Sound & notifications

Four bundled clips (`//go:embed`), played via the platform audio player
(`afplay` on macOS). Desktop notifications on session events. Both toggleable.

### Configuration (`pomo config`)

`pomo config` prints every setting; `pomo config set <key> <value>` changes one
(validated — unknown keys and bad values are rejected).

Pomodoro keys: `focus`, `short_break`, `long_break`, `sessions_before_long`,
`auto_start_break`, `auto_start_focus`, `sound`, `sound_choice`, `notifications`.

Focus / drift & AI keys (used by the in-progress focus layer, see Roadmap):
`daemon.tick`, `drift.*`, `nudge.*`, `checkpoint.enabled`, `ai.provider`
(`anthropic` | `openrouter`), `ai.key`, `ai.model`, `digest.notify`.
`ai.key` is stored locally in plaintext and masked wherever displayed.

## Command reference

```
pomo [task]                 dashboard, or quick-start a session
pomo start [task]           start a session   --duration N  --tag NAME
pomo pause | resume | stop | skip
pomo task add|list|done|delete
pomo today | week | month | stats
pomo history [filters]
pomo review [window] [--json]
pomo config [set KEY VALUE]
pomo daemon start | stop | status
pomo digest [--week YYYY-Www] [--notify]
```

### In the dashboard

Press `/` for the command palette: `/review`, `/insights`, `/drift`, `/chat`,
`/settings`, `/start`, `/skip`, `/help`, `/quit`.

## Data

Everything lives in `~/.pomo/`:

- `pomo.db` — SQLite (WAL mode). Tables: `tasks`, `sessions`, `drift_events`, `config`.
- `reviews/` — weekly digest markdown files (once the focus layer lands).

## Roadmap — ADHD focus layer

In progress. Design specs in [`specs/`](specs/), plans in [`plans/`](plans/).

Shipped in full:

- **Drift detection daemon** (`pomo daemon start`) — a background process watches
  the foreground app (macOS) and repo file activity during a session and records
  drift episodes when you slip onto Chrome / Slack / YouTube. `pomo daemon status`
  shows what it's doing. On non-macOS it runs the checkpoint + file-activity
  signals only.
- **Gentle escalating nudges** — a desktop notification when you drift, escalating
  L1 → L3, phrased by your own AI key if configured (`ai.provider` = `anthropic`
  or `openrouter`, BYOK), plain templates otherwise. Inside a running timer the
  nudge shows as an overlay with `[b] break  [r] refocus  [d] drifted  [s] snooze`.
- **Slash-command surface** — press `/` from the dashboard or a running timer:
  `/review`, `/insights` (AI recap), `/drift`, `/chat` (a streamed focus coach —
  input is validated, the system prompt is fixed), `/settings`, `/help`.
- **Weekly digest** — `~/.pomo/reviews/YYYY-Www.md`, via `pomo digest` or written
  automatically by the daemon on the first Monday tick.

Backlog (deferred, not blocking): a launchd/systemd unit (the daemon currently
runs as a detached process with a pidfile), Linux foreground-app watching, and
browser-tab attribution when macOS automation permission is denied.

## Development

```sh
make build      # go build -o pomo .
make test       # go test ./...
go vet ./...
```

Architecture notes for contributors (and for Claude Code) are in
[`CLAUDE.md`](CLAUDE.md).

A macOS desktop shell (Wails v2 + Svelte) lives in [`desktop/`](desktop/) —
`make desktop-dev` / `make desktop-build`.
