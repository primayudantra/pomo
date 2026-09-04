# Slash Palette + `/chat`

Date: 2026-09-04
Parent: `2026-09-04-adhd-focus-overview.md`

## 1. Principle

All interaction stays inside the one persistent Bubble Tea program
(`internal/tui/app.go`) — matches the existing rule that screen transitions never
drop the user to the shell. Slash commands are the human surface; the
`pomo <verb>` CLI subcommands are kept thin, for scripting / cron / editor
plugins / the daemon.

## 2. New screens

Add to the `screen` enum in `app.go`:

```go
screenPrompt   // the "/" palette
screenResult   // scrollable output pane (viewport) for /review, /drift, /help
screenChat     // streaming chat
```

Entry: pressing `/` from `screenDashboard` or `screenTimer` pushes `screenPrompt`
and records the origin screen so `esc` returns there.

## 3. Command registry

New file `internal/tui/commands.go`:

```go
type Command struct {
    Name    string
    Aliases []string
    Help    string
    Run     func(args []string, a *App) tea.Cmd   // returns a msg the App handles
}

var Commands = []Command{ ... }
```

| Name | Aliases | Args | Effect |
|---|---|---|---|
| `/review` | `/recap` | `[today\|week\|month]` (default today) | render `internal/report` summary in `screenResult` |
| `/insights` | — | `[today\|week\|month]` | `/review` + forced AI recap block |
| `/drift` | — | — | current session drift so far, or today's episodes if idle |
| `/chat` | `/ask` | `[initial question…]` | open `screenChat`, optionally send the initial question |
| `/start` | — | `<task…>` | leave palette, run existing start flow with that task |
| `/note` | — | `[text…]` | attach a note to the running session (existing `FinishSession` note path) |
| `/skip` | — | — | skip current session (existing behaviour) |
| `/settings` | `/config` | — | push existing `screenSettings` |
| `/help` | `/?` | — | list commands in `screenResult` |
| `/quit` | `/q` | — | existing quit-confirm flow |

Commands unavailable in the current context (e.g. `/skip` when no session runs)
are shown greyed with a reason and refuse to run.

## 4. Palette UX (`screenPrompt`)

- `bubbles/textinput` for the line (pre-filled with `/`).
- Below it, a filtered list (reuse `github.com/sahilm/fuzzy`, already a
  dependency) of matching commands: `name` + `help`, max 6 rows.
- Keys: `↑/↓` move selection, `tab` completes to the selected name, `enter` runs
  (selected command if the line is just a prefix, else the typed command +
  args), `esc` back to origin.
- Unknown command → inline red `unknown command: /foo — try /help`.

## 5. `screenResult`

- `bubbles/viewport` with the rendered string, lipgloss-styled to match
  `internal/tui/stats.go`.
- `esc` → origin screen. `r` → re-run the same command (refresh). `↑/↓/pgup/pgdn`
  scroll.
- For `/insights`, the AI recap is fetched by a `tea.Cmd` that returns a
  `recapMsg`; while pending, show `RECAP  …thinking` then swap in the text (or
  `RECAP  (unavailable: <reason>)`).

## 6. `screenChat`

- Layout: scrollback viewport on top, `textinput` on the bottom, `esc` to origin.
- `App` holds `chatHistory []ai.Msg` for the lifetime of the program (not
  persisted).
- On send:
  1. Append user msg to history + viewport.
  2. Build a system-context preamble (not shown to the user) from:
     current/last session (task, tag, planned vs actual), today's `drift_events`
     aggregates (total, top apps), open task list, current hour.
  3. `tea.Cmd` calls `chatter.Stream(ctx, preamble+history, onDelta)`. Deltas are
     delivered to the App as `chatDeltaMsg` and appended to the in-progress
     assistant bubble; a final `chatDoneMsg` (or `chatErrMsg`) closes it.
  4. Streaming uses a 30s context. On `chatErrMsg`, append `[error: <reason>]`
     after whatever text already streamed.
- No provider configured: `screenChat` renders only
  `set  ai.provider  and  ai.key  in /settings to enable chat` + `esc`.

### Bubble Tea streaming pattern

The `tea.Cmd` cannot emit multiple messages itself; use the standard pattern:
the command writes deltas to a channel, and a secondary `listenForDelta(ch)`
`tea.Cmd` reads one value and returns it as a msg, re-issued from `Update` until
a sentinel closes it. Document this in `commands.go` / `app.go` where it lives.

## 7. IPC client in the TUI

New: `internal/ipc` client used by `app.go`.

- On program start, `ipc.Dial("~/.pomo/daemon.sock")`. Failure → `daemonUp =
  false`, no retry loop (a single retry when a session starts).
- When a session starts, send `{type:"session-start", id, repo_path}` and begin a
  read goroutine feeding `daemonEventMsg` into the App.
- Events handled:
  - `checkpoint` → overlay `on task? [y] [n]` on `screenTimer`; the keypress
    sends `{type:"checkpoint-answer", answer:"y|n"}`.
  - `nudge` → overlay the nudge banner (text + action keys from the event);
    action keypress sends `{type:"nudge-action", action:"b|r|d|snooze"}`.
  - `watching` / `idle` → update the dashboard daemon indicator.
- The daemon indicator on the dashboard: `● daemon up` / `○ daemon off`
  (`daemon off` links to `/help` line "run `pomo daemon start`").

## 8. IPC protocol (`internal/ipc`)

- Unix domain socket, path `filepath.Join(db.Dir(), "daemon.sock")`.
- Newline-delimited JSON. One struct:

```go
type Event struct {
    Type      string          `json:"type"`
    SessionID int64           `json:"id,omitempty"`
    RepoPath  string          `json:"repo_path,omitempty"`
    Text      string          `json:"text,omitempty"`     // nudge / checkpoint copy
    Level     int             `json:"level,omitempty"`
    Actions   []string        `json:"actions,omitempty"`
    Answer    string          `json:"answer,omitempty"`
    Action    string          `json:"action,omitempty"`
}
```

- Server (`ipc.Serve`) accepts multiple clients; `Broadcast` fans out. Client
  disconnect is non-fatal.
- Daemon is the server; TUI is a client. If the daemon starts after the TUI, the
  TUI's on-session-start retry picks it up; otherwise the user re-opens pomo.
- Socket file removed on daemon shutdown and stale-cleaned on startup
  (`net.Dial` probe → if refused, `os.Remove`).
