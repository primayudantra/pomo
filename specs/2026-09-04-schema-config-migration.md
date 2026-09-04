# Schema, Config, Migration + Wiring

Date: 2026-09-04
Parent: `2026-09-04-adhd-focus-overview.md`

## 1. Schema changes (`internal/db/db.go`)

The current `schema` const is run on every `Open()` with `CREATE TABLE IF NOT
EXISTS`. There is no migration framework. Keep that style; add idempotent
statements.

### New table

```sql
CREATE TABLE IF NOT EXISTS drift_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id INTEGER NOT NULL,
    started_at TIMESTAMP NOT NULL,
    ended_at   TIMESTAMP,
    seconds    INTEGER DEFAULT 0,
    trigger    TEXT NOT NULL,          -- 'foreground' | 'fs_stale' | 'checkpoint_no'
    detail     TEXT DEFAULT ''         -- "Google Chrome — reddit.com"
);
CREATE INDEX IF NOT EXISTS idx_drift_session ON drift_events(session_id);
CREATE INDEX IF NOT EXISTS idx_drift_started ON drift_events(started_at);
```

### New columns on `sessions`

`ALTER TABLE ADD COLUMN` is not idempotent in SQLite (errors if the column
exists). Add a helper run after the `schema` exec:

```go
func ensureColumn(db *sql.DB, table, col, ddl string) error {
    rows, _ := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
    // scan; if col present, return nil
    // else: db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", table, ddl))
}
```

Calls:
```go
ensureColumn(sqlDB, "sessions", "repo_path",   "repo_path TEXT DEFAULT ''")
ensureColumn(sqlDB, "sessions", "repo_branch", "repo_branch TEXT DEFAULT ''")
```

### model changes (`internal/model/model.go`)

```go
type Session struct {
    ... existing ...
    RepoPath   string
    RepoBranch string
}

type DriftEvent struct {
    ID        int64
    SessionID int64
    StartedAt time.Time
    EndedAt   *time.Time
    Seconds   int
    Trigger   string
    Detail    string
}
```

`scanSession` / `ListSessions` / `CreateSession` / `LastRunningSession` updated to
read/write the two new columns. `CreateSession` signature gains the repo fields
via the `model.Session` it already takes — no new parameter.

## 2. New `internal/db` methods

```go
func (d *DB) OpenDriftEpisode(sessionID int64, at time.Time, trigger, detail string) (int64, error)
func (d *DB) UpdateDriftEpisode(id int64, seconds int, detail string) error
func (d *DB) CloseDriftEpisode(id int64, endedAt time.Time, seconds int) error
func (d *DB) ListDriftEvents(from, to time.Time) ([]model.DriftEvent, error)
func (d *DB) DriftEventsForSession(sessionID int64) ([]model.DriftEvent, error)
```

## 3. Config keys

All stored as strings in the `config` table (existing `GetConfig`/`SetConfig`).
`internal/pomoconfig` gains typed fields + defaults + parsing. `pomo config set`
already writes arbitrary keys; add validation for the new ones.

| Key | Type | Default | Notes |
|---|---|---|---|
| `daemon.tick` | duration | `15s` | loop interval |
| `drift.enabled` | bool | `true` | master switch for drift detection |
| `drift.distract_grace` | duration | `3m` | continuous distract before drifting |
| `drift.fs_stale` | duration | `10m` | no repo write before "stale" |
| `drift.checkpoint_timeout` | duration | `20s` | unanswered checkpoint = "no" |
| `drift.checkpoint_penalty` | duration | `2m` | drift window after a "no" |
| `drift.recover_grace` | duration | `1m` | focus time needed to close an episode |
| `drift.focus_apps` | csv | (built-in list) | extra focus match rules |
| `drift.distract_apps` | csv | (built-in list) | extra distract match rules |
| `drift.neutral_apps` | csv | `""` | extra neutral match rules |
| `drift.focus_title_hints` | csv | `localhost,github.com,stackoverflow,pomo,docs.` | reclassify a browser reading to neutral |
| `nudge.enabled` | bool | `true` | |
| `nudge.max_per_session` | int | `4` | |
| `nudge.min_gap` | duration | `5m` | |
| `ai.provider` | enum | `""` | `""` \| `anthropic` \| `openrouter` |
| `ai.key` | string | `""` | plaintext; masked on display |
| `ai.model` | string | `""` | empty → provider default (`claude-haiku-4-5`) |
| `digest.notify` | bool | `false` | notify on weekly auto-digest |
| `checkpoint.enabled` | bool | `true` | the C1 mid-session prompt |

`pomoconfig.Config` gains a nested `Drift`, `Nudge`, `AI`, `Daemon`, `Digest`
grouping (or flat fields — implementer's choice; keep `Load` a single pass over
`AllConfig()`).

CSV values: `SetConfig` stores the raw string; `pomoconfig` splits on `,`,
trims, lowercases, and appends to the built-in list (does not replace it — to
remove a default, a future `drift.<list>_remove` key; out of scope for v1).

### `pomo config` display

- `ai.key` shown as `sk-…<last4>` when set, `(unset)` otherwise.
- New keys listed under a `focus / drift` and `ai` section.
- A footer note when `ai.provider` is set:
  `ai.key is stored in plaintext at ~/.pomo/pomo.db`.

## 4. `/settings` screen additions

`internal/tui/app.go` `screenSettings` currently drives the pomodoro timing +
sound options. Add rows for: `drift.enabled`, `checkpoint.enabled`,
`nudge.enabled`, `ai.provider` (cycle `off / anthropic / openrouter`), `ai.key`
(masked text input), `ai.model` (text input). Writes via `db.SetConfig` +
`pomoconfig` reload, same as existing settings rows.

## 5. Wiring / ownership

| Process | Owns | Reads |
|---|---|---|
| `pomo` (CLI/TUI) | task + session CRUD, slash palette, chat, `/review` render | `drift_events` (read), config |
| `pomo daemon` | drift scoring, `drift_events` writes, nudges, weekly digest | `sessions` (read), config |
| both | `internal/db` (separate `*sql.DB` handles, WAL mode) | |

### SQLite concurrency

Two processes now open `pomo.db`. Enable WAL + a busy timeout in `db.Open`:

```go
sql.Open("sqlite", path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
```

(modernc.org/sqlite DSN pragma syntax.) Writes from both sides are small and
infrequent; WAL + 5s busy timeout is sufficient. No shared in-process state.

## 6. Backward compatibility

- Fresh install: `schema` + `ensureColumn` create everything. No difference.
- Existing `~/.pomo/pomo.db`: `ensureColumn` adds the two columns (default `''`),
  `drift_events` created empty. Old sessions have empty `repo_path` → daemon
  simply skips the fs signal for any still-running one. No data migration, no
  version bump needed.
- Daemon never installed / not running: zero behaviour change anywhere.
