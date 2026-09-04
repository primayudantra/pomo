# Report Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the drift-events schema, the `internal/report` aggregation package, and a `pomo review` CLI command — the risk-free foundation for the ADHD focus layer, with zero change to existing runtime behaviour.

**Architecture:** Additive SQLite schema changes via `CREATE TABLE IF NOT EXISTS` plus a new `ensureColumn` helper (SQLite `ALTER TABLE ADD COLUMN` is not idempotent). A new pure-logic `internal/report` package owns all windowed aggregation and rendering (text / markdown / JSON), shared by the future daemon, the future `/review` slash command, and the new `pomo review` CLI. Streak/day-bucket helpers currently inline in `internal/tui/stats.go` move into `internal/report` so there is one implementation.

**Tech Stack:** Go 1.25, `modernc.org/sqlite` (pure Go, no cgo), `github.com/spf13/cobra`, `github.com/charmbracelet/lipgloss`. Standard `testing` package (repo currently has no tests — this plan adds the first).

**Spec:** `specs/2026-09-04-schema-config-migration.md` and `specs/2026-09-04-review-and-digest.md` (parent: `specs/2026-09-04-adhd-focus-overview.md`)

## Global Constraints

- Go version floor: `go 1.25` (per `go.mod`) — do not raise it.
- No new third-party dependencies. Everything here uses the stdlib plus libraries already in `go.mod`.
- Existing runtime behaviour must not change: no command output, timer flow, or TUI screen changes except the one contained `stats.go` internal refactor in Task 6, which must render byte-identical output.
- SQLite driver name is `"sqlite"` (modernc), not `"sqlite3"`.
- DB file location is `~/.pomo/pomo.db` via `db.Dir()` — never hardcode a path.
- Durations in the `sessions` / `drift_events` tables are integer **seconds**.
- Weeks are ISO weeks, Monday start (matches existing `stats.go` convention).
- Commit after every task with a `feat:` / `refactor:` / `test:` prefixed message. This is not a git repo yet — Task 1 Step 1 runs `git init`.

---

### Task 1: Repo init + `make test` target

**Files:**
- Create: `.gitignore`
- Modify: `Makefile`

**Interfaces:**
- Consumes: nothing.
- Produces: `make test` runs `go test ./...`; a git repo exists so later tasks can commit.

- [ ] **Step 1: Initialise git and ignore the build artefact**

Run:
```bash
cd /Users/primayudantra/Documents/projects/pomo
git init
```

Create `.gitignore`:
```
/pomo
*.db
*.db-wal
*.db-shm
.DS_Store
```

- [ ] **Step 2: Add the test target to the Makefile**

Modify `Makefile` — change the `.PHONY` line and append a `test` target:
```makefile
BIN := $(HOME)/.local/bin/pomo

.PHONY: build install test

build:
	go build -o pomo .

install: build
	mkdir -p $(HOME)/.local/bin
	cp pomo $(BIN)
	chmod +x $(BIN)
	@echo "installed to $(BIN)"

test:
	go test ./...
```

- [ ] **Step 3: Verify**

Run: `make test`
Expected: PASS with `no test files` for every package (nothing is tested yet). No compile errors.

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "chore: init repo, add make test target"
```

---

### Task 2: `dbtest` helper for temp databases

**Files:**
- Create: `internal/db/dbtest/dbtest.go`

**Interfaces:**
- Consumes: `db.Open` cannot be used (it hardcodes `~/.pomo`). This helper opens a `*db.DB` on a temp path directly.
- Produces: `dbtest.NewTemp(t *testing.T) *db.DB` — a migrated database on a `t.TempDir()` file, closed automatically via `t.Cleanup`.

- [ ] **Step 1: Add an exported constructor to `internal/db` that takes a path**

Modify `internal/db/db.go` — extract the open-and-migrate logic so tests can point it at any file. Add below `Open`:
```go
// OpenAt opens (creating if needed) a pomo database at an explicit path and
// runs migrations. Open() is the normal entrypoint; OpenAt exists for tests
// and tools that need a non-default location.
func OpenAt(path string) (*DB, error) {
	sqlDB, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if _, err := sqlDB.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &DB{sqlDB}, nil
}
```
Then change `Open` to build the path and delegate:
```go
func Open() (*DB, error) {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return OpenAt(filepath.Join(dir, "pomo.db"))
}
```

- [ ] **Step 2: Verify existing build still works**

Run: `go build ./...`
Expected: PASS.

- [ ] **Step 3: Write the dbtest helper**

Create `internal/db/dbtest/dbtest.go`:
```go
// Package dbtest provides a throwaway pomo database for unit tests.
package dbtest

import (
	"path/filepath"
	"testing"

	"pomo/internal/db"
)

// NewTemp returns a migrated *db.DB backed by a file under t.TempDir().
// It is closed automatically when the test finishes.
func NewTemp(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pomo.db")
	d, err := db.OpenAt(path)
	if err != nil {
		t.Fatalf("dbtest.NewTemp: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}
```

- [ ] **Step 4: Write a smoke test for the helper**

Create `internal/db/dbtest/dbtest_test.go`:
```go
package dbtest_test

import (
	"testing"

	"pomo/internal/db/dbtest"
)

func TestNewTempIsUsable(t *testing.T) {
	d := dbtest.NewTemp(t)
	if _, err := d.AddTask("hello", ""); err != nil {
		t.Fatalf("AddTask on temp db: %v", err)
	}
	tasks, err := d.ListOpenTasks()
	if err != nil {
		t.Fatalf("ListOpenTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Name != "hello" {
		t.Fatalf("got %+v, want one task named hello", tasks)
	}
}
```

- [ ] **Step 5: Run the test**

Run: `go test ./internal/db/...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "test: add dbtest.NewTemp helper and db.OpenAt"
```

---

### Task 3: `sessions.repo_path` / `repo_branch` columns

**Files:**
- Modify: `internal/db/db.go`
- Modify: `internal/model/model.go`
- Test: `internal/db/session_repo_test.go` (create)

**Interfaces:**
- Consumes: `dbtest.NewTemp` (Task 2).
- Produces:
  - `model.Session` gains `RepoPath string` and `RepoBranch string`.
  - `db.CreateSession` persists those two fields from the passed `model.Session`.
  - `db.LastRunningSession` and `db.ListSessions` populate them on read.
  - `db.ensureColumn(sqlDB *sql.DB, table, col, ddl string) error` — idempotent column add.

- [ ] **Step 1: Write the failing test**

Create `internal/db/session_repo_test.go`:
```go
package db_test

import (
	"testing"
	"time"

	"pomo/internal/db/dbtest"
	"pomo/internal/model"
)

func TestSessionRepoFieldsRoundTrip(t *testing.T) {
	d := dbtest.NewTemp(t)
	id, err := d.CreateSession(model.Session{
		TaskName:        "fix bug",
		Tag:             "backend",
		PlannedDuration: 1500,
		Status:          model.StatusRunning,
		StartedAt:       time.Now(),
		RepoPath:        "/home/me/proj",
		RepoBranch:      "feature/x",
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	got, err := d.LastRunningSession()
	if err != nil {
		t.Fatalf("LastRunningSession: %v", err)
	}
	if got.ID != id || got.RepoPath != "/home/me/proj" || got.RepoBranch != "feature/x" {
		t.Fatalf("got %+v, want repo_path/branch persisted", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/db/ -run TestSessionRepoFieldsRoundTrip -v`
Expected: FAIL — compile error, `RepoPath`/`RepoBranch` are not fields of `model.Session`.

- [ ] **Step 3: Add the model fields**

Modify `internal/model/model.go` — add to the `Session` struct after `CreatedAt`:
```go
	RepoPath   string
	RepoBranch string
```

- [ ] **Step 4: Add `ensureColumn` and call it from `OpenAt`**

Modify `internal/db/db.go`. Add the helper:
```go
// ensureColumn adds a column if the table does not already have it. SQLite's
// ALTER TABLE ADD COLUMN errors when the column exists, so this is how we do
// additive migrations on top of the CREATE TABLE IF NOT EXISTS schema.
func ensureColumn(sqlDB *sql.DB, table, col, ddl string) error {
	rows, err := sqlDB.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid, notnull, pk int
			name, ctype      string
			dflt             sql.NullString
		)
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == col {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = sqlDB.Exec("ALTER TABLE " + table + " ADD COLUMN " + ddl)
	return err
}
```
In `OpenAt`, after the `sqlDB.Exec(schema)` block and before `return &DB{sqlDB}, nil`:
```go
	for _, c := range []struct{ table, col, ddl string }{
		{"sessions", "repo_path", "repo_path TEXT DEFAULT ''"},
		{"sessions", "repo_branch", "repo_branch TEXT DEFAULT ''"},
	} {
		if err := ensureColumn(sqlDB, c.table, c.col, c.ddl); err != nil {
			return nil, fmt.Errorf("migrate %s.%s: %w", c.table, c.col, err)
		}
	}
```

- [ ] **Step 5: Persist and read the new columns**

Modify `internal/db/db.go`:

`CreateSession` — extend the INSERT:
```go
func (d *DB) CreateSession(s model.Session) (int64, error) {
	res, err := d.Exec(`INSERT INTO sessions
		(task_id, task_name, tag, planned_duration, actual_duration, status, note, started_at, created_at, repo_path, repo_branch)
		VALUES (?, ?, ?, ?, 0, ?, '', ?, ?, ?, ?)`,
		s.TaskID, s.TaskName, s.Tag, s.PlannedDuration, s.Status, s.StartedAt, time.Now(), s.RepoPath, s.RepoBranch)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}
```

`scanSession` (used by `LastRunningSession`) — add the columns to SELECT and Scan:
```go
func (d *DB) LastRunningSession() (*model.Session, error) {
	row := d.QueryRow(`SELECT id, task_id, task_name, tag, planned_duration, actual_duration, status, note, started_at, completed_at, created_at, repo_path, repo_branch
		FROM sessions WHERE status = 'running' ORDER BY id DESC LIMIT 1`)
	return scanSession(row)
}

func scanSession(row *sql.Row) (*model.Session, error) {
	var s model.Session
	var completedAt sql.NullTime
	if err := row.Scan(&s.ID, &s.TaskID, &s.TaskName, &s.Tag, &s.PlannedDuration, &s.ActualDuration,
		&s.Status, &s.Note, &s.StartedAt, &completedAt, &s.CreatedAt, &s.RepoPath, &s.RepoBranch); err != nil {
		return nil, err
	}
	if completedAt.Valid {
		s.CompletedAt = &completedAt.Time
	}
	return &s, nil
}
```

`ListSessions` — add the columns to its SELECT and its inline `rows.Scan`:
```go
	q := `SELECT id, task_id, task_name, tag, planned_duration, actual_duration, status, note, started_at, completed_at, created_at, repo_path, repo_branch
		FROM sessions WHERE 1=1`
```
```go
		if err := rows.Scan(&s.ID, &s.TaskID, &s.TaskName, &s.Tag, &s.PlannedDuration, &s.ActualDuration,
			&s.Status, &s.Note, &s.StartedAt, &completedAt, &s.CreatedAt, &s.RepoPath, &s.RepoBranch); err != nil {
```

- [ ] **Step 6: Run the test**

Run: `go test ./internal/db/ -run TestSessionRepoFieldsRoundTrip -v`
Expected: PASS.

- [ ] **Step 7: Run the full suite and build**

Run: `make test && go build ./...`
Expected: PASS. (`internal/tui` calls `ListSessions` but only reads existing fields — unaffected.)

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat(db): add sessions.repo_path/repo_branch with idempotent migration"
```

---

### Task 4: `drift_events` table + model + CRUD

**Files:**
- Modify: `internal/db/db.go`
- Modify: `internal/model/model.go`
- Create: `internal/db/drift.go`
- Test: `internal/db/drift_test.go` (create)

**Interfaces:**
- Consumes: `dbtest.NewTemp` (Task 2), `ensureColumn` pattern (Task 3, not needed here — new table via schema const).
- Produces:
  - `model.DriftEvent{ID int64; SessionID int64; StartedAt time.Time; EndedAt *time.Time; Seconds int; Trigger string; Detail string}`
  - `db.OpenDriftEpisode(sessionID int64, at time.Time, trigger, detail string) (int64, error)`
  - `db.UpdateDriftEpisode(id int64, seconds int, detail string) error`
  - `db.CloseDriftEpisode(id int64, endedAt time.Time, seconds int) error`
  - `db.ListDriftEvents(from, to time.Time) ([]model.DriftEvent, error)` — filters by `started_at` in `[from, to)`, ordered ascending.
  - `db.DriftEventsForSession(sessionID int64) ([]model.DriftEvent, error)` — ordered ascending.

- [ ] **Step 1: Write the failing test**

Create `internal/db/drift_test.go`:
```go
package db_test

import (
	"testing"
	"time"

	"pomo/internal/db/dbtest"
)

func TestDriftEpisodeLifecycle(t *testing.T) {
	d := dbtest.NewTemp(t)
	start := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)

	id, err := d.OpenDriftEpisode(1, start, "foreground", "Google Chrome")
	if err != nil {
		t.Fatalf("OpenDriftEpisode: %v", err)
	}
	if err := d.UpdateDriftEpisode(id, 45, "Google Chrome — reddit.com"); err != nil {
		t.Fatalf("UpdateDriftEpisode: %v", err)
	}
	if err := d.CloseDriftEpisode(id, start.Add(90*time.Second), 90); err != nil {
		t.Fatalf("CloseDriftEpisode: %v", err)
	}

	got, err := d.ListDriftEvents(start.Add(-time.Hour), start.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListDriftEvents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	e := got[0]
	if e.SessionID != 1 || e.Seconds != 90 || e.Trigger != "foreground" ||
		e.Detail != "Google Chrome — reddit.com" || e.EndedAt == nil {
		t.Fatalf("unexpected event: %+v", e)
	}
}

func TestListDriftEventsWindowExcludesOutside(t *testing.T) {
	d := dbtest.NewTemp(t)
	base := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	if _, err := d.OpenDriftEpisode(1, base.Add(-2*time.Hour), "fs_stale", ""); err != nil {
		t.Fatal(err)
	}
	inWindow, err := d.OpenDriftEpisode(1, base.Add(10*time.Minute), "fs_stale", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.ListDriftEvents(base, base.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != inWindow {
		t.Fatalf("window filter wrong: got %+v", got)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/db/ -run TestDrift -v`
Expected: FAIL — `OpenDriftEpisode` undefined.

- [ ] **Step 3: Add the table to the schema const**

Modify `internal/db/db.go` — append to the `schema` const string, before the closing backtick:
```sql

CREATE TABLE IF NOT EXISTS drift_events (
	id         INTEGER PRIMARY KEY AUTOINCREMENT,
	session_id INTEGER NOT NULL,
	started_at TIMESTAMP NOT NULL,
	ended_at   TIMESTAMP,
	seconds    INTEGER DEFAULT 0,
	trigger    TEXT NOT NULL,
	detail     TEXT DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_drift_session ON drift_events(session_id);
CREATE INDEX IF NOT EXISTS idx_drift_started ON drift_events(started_at);
```

- [ ] **Step 4: Add the model type**

Modify `internal/model/model.go` — add at the end:
```go
type DriftEvent struct {
	ID        int64
	SessionID int64
	StartedAt time.Time
	EndedAt   *time.Time
	Seconds   int
	Trigger   string // "foreground" | "fs_stale" | "checkpoint_no"
	Detail    string
}
```

- [ ] **Step 5: Implement the CRUD in a new file**

Create `internal/db/drift.go`:
```go
package db

import (
	"database/sql"
	"time"

	"pomo/internal/model"
)

// OpenDriftEpisode inserts a new open (ended_at NULL) drift episode and
// returns its id.
func (d *DB) OpenDriftEpisode(sessionID int64, at time.Time, trigger, detail string) (int64, error) {
	res, err := d.Exec(`INSERT INTO drift_events (session_id, started_at, seconds, trigger, detail)
		VALUES (?, ?, 0, ?, ?)`, sessionID, at, trigger, detail)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateDriftEpisode bumps the running seconds counter and the dominant-app
// detail on an open episode.
func (d *DB) UpdateDriftEpisode(id int64, seconds int, detail string) error {
	_, err := d.Exec(`UPDATE drift_events SET seconds = ?, detail = ? WHERE id = ?`,
		seconds, detail, id)
	return err
}

// CloseDriftEpisode stamps ended_at and the final seconds on an episode.
func (d *DB) CloseDriftEpisode(id int64, endedAt time.Time, seconds int) error {
	_, err := d.Exec(`UPDATE drift_events SET ended_at = ?, seconds = ? WHERE id = ?`,
		endedAt, seconds, id)
	return err
}

// ListDriftEvents returns episodes whose started_at is in [from, to), ascending.
func (d *DB) ListDriftEvents(from, to time.Time) ([]model.DriftEvent, error) {
	rows, err := d.Query(`SELECT id, session_id, started_at, ended_at, seconds, trigger, detail
		FROM drift_events WHERE started_at >= ? AND started_at < ? ORDER BY started_at ASC`, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDriftEvents(rows)
}

// DriftEventsForSession returns every episode for one session, ascending.
func (d *DB) DriftEventsForSession(sessionID int64) ([]model.DriftEvent, error) {
	rows, err := d.Query(`SELECT id, session_id, started_at, ended_at, seconds, trigger, detail
		FROM drift_events WHERE session_id = ? ORDER BY started_at ASC`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanDriftEvents(rows)
}

func scanDriftEvents(rows *sql.Rows) ([]model.DriftEvent, error) {
	var out []model.DriftEvent
	for rows.Next() {
		var e model.DriftEvent
		var ended sql.NullTime
		if err := rows.Scan(&e.ID, &e.SessionID, &e.StartedAt, &ended, &e.Seconds, &e.Trigger, &e.Detail); err != nil {
			return nil, err
		}
		if ended.Valid {
			e.EndedAt = &ended.Time
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
```

- [ ] **Step 6: Run the tests**

Run: `go test ./internal/db/ -run TestDrift -v`
Expected: PASS (both).

- [ ] **Step 7: Full suite + build**

Run: `make test && go build ./...`
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add -A
git commit -m "feat(db): add drift_events table and episode CRUD"
```

---

### Task 5: `internal/report` — Window + ParseWindow

**Files:**
- Create: `internal/report/window.go`
- Test: `internal/report/window_test.go` (create)

**Interfaces:**
- Consumes: nothing (pure time logic).
- Produces:
  - `report.Window{From, To time.Time; Label string}`
  - `report.Today() Window`, `report.ThisWeek() Window`, `report.ThisMonth() Window` — computed in `time.Local`.
  - `report.ParseWindow(s string) (Window, error)` — accepts `"today"`, `"week"`, `"month"`, `"YYYY-Www"` (ISO week, e.g. `2026-W36`), `"YYYY-MM"`. Empty string → `Today()`. Anything else → error.
  - `Window.From` is inclusive, `Window.To` exclusive.

- [ ] **Step 1: Write the failing test**

Create `internal/report/window_test.go`:
```go
package report_test

import (
	"testing"
	"time"

	"pomo/internal/report"
)

func TestParseWindowKeywords(t *testing.T) {
	for _, s := range []string{"", "today", "week", "month"} {
		w, err := report.ParseWindow(s)
		if err != nil {
			t.Fatalf("ParseWindow(%q): %v", s, err)
		}
		if !w.To.After(w.From) {
			t.Fatalf("ParseWindow(%q): To %v not after From %v", s, w.To, w.From)
		}
	}
}

func TestParseWindowISOWeek(t *testing.T) {
	w, err := report.ParseWindow("2026-W36")
	if err != nil {
		t.Fatalf("ParseWindow: %v", err)
	}
	// ISO week 36 of 2026 starts Monday 2026-08-31.
	wantFrom := time.Date(2026, 8, 31, 0, 0, 0, 0, time.Local)
	if !w.From.Equal(wantFrom) {
		t.Fatalf("From = %v, want %v", w.From, wantFrom)
	}
	if w.To.Sub(w.From) != 7*24*time.Hour {
		t.Fatalf("week span = %v, want 168h", w.To.Sub(w.From))
	}
	if w.Label != "2026-W36" {
		t.Fatalf("Label = %q", w.Label)
	}
}

func TestParseWindowMonth(t *testing.T) {
	w, err := report.ParseWindow("2026-02")
	if err != nil {
		t.Fatalf("ParseWindow: %v", err)
	}
	if w.From != time.Date(2026, 2, 1, 0, 0, 0, 0, time.Local) ||
		w.To != time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local) {
		t.Fatalf("Feb 2026 window wrong: %+v", w)
	}
}

func TestParseWindowGarbage(t *testing.T) {
	if _, err := report.ParseWindow("last-tuesday"); err == nil {
		t.Fatal("expected error for garbage input")
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/report/ -v`
Expected: FAIL — package `report` does not exist.

- [ ] **Step 3: Implement**

Create `internal/report/window.go`:
```go
// Package report aggregates pomo session and drift-event history into
// windowed summaries and renders them as text, markdown, or JSON. It is the
// single source of aggregation logic shared by the CLI, the TUI, and the
// daemon's weekly digest.
package report

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Window is a half-open time range [From, To) with a display label.
type Window struct {
	From  time.Time
	To    time.Time
	Label string
}

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// Today is the current local calendar day.
func Today() Window {
	from := midnight(time.Now())
	return Window{From: from, To: from.AddDate(0, 0, 1), Label: "today"}
}

// ThisWeek is the current ISO week (Monday start), local time.
func ThisWeek() Window {
	now := time.Now()
	wd := int(now.Weekday())
	if wd == 0 {
		wd = 7
	}
	from := midnight(now).AddDate(0, 0, -(wd - 1))
	y, w := from.ISOWeek()
	return Window{From: from, To: from.AddDate(0, 0, 7), Label: fmt.Sprintf("%d-W%02d", y, w)}
}

// ThisMonth is the current local calendar month.
func ThisMonth() Window {
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return Window{From: from, To: from.AddDate(0, 1, 0), Label: from.Format("2006-01")}
}

// ParseWindow resolves a window spec. Accepted: "" / "today", "week",
// "month", "YYYY-Www" (ISO week), "YYYY-MM".
func ParseWindow(s string) (Window, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "today":
		return Today(), nil
	case "week":
		return ThisWeek(), nil
	case "month":
		return ThisMonth(), nil
	}
	if y, w, ok := parseISOWeek(s); ok {
		from := isoWeekStart(y, w)
		return Window{From: from, To: from.AddDate(0, 0, 7), Label: fmt.Sprintf("%d-W%02d", y, w)}, nil
	}
	if t, err := time.ParseInLocation("2006-01", s, time.Local); err == nil {
		return Window{From: t, To: t.AddDate(0, 1, 0), Label: t.Format("2006-01")}, nil
	}
	return Window{}, fmt.Errorf("unrecognised window %q (want today|week|month|YYYY-Www|YYYY-MM)", s)
}

func parseISOWeek(s string) (year, week int, ok bool) {
	parts := strings.Split(strings.ToUpper(s), "-W")
	if len(parts) != 2 {
		return 0, 0, false
	}
	y, err1 := strconv.Atoi(parts[0])
	w, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || w < 1 || w > 53 {
		return 0, 0, false
	}
	return y, w, true
}

// isoWeekStart returns the local midnight of the Monday of ISO week (year, week).
func isoWeekStart(year, week int) time.Time {
	// Jan 4th is always in ISO week 1.
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.Local)
	wd := int(jan4.Weekday())
	if wd == 0 {
		wd = 7
	}
	week1Monday := jan4.AddDate(0, 0, -(wd - 1))
	return week1Monday.AddDate(0, 0, 7*(week-1))
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/report/ -v`
Expected: PASS (all four).

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "feat(report): add Window and ParseWindow"
```

---

### Task 6: Move streak/day-bucket helpers into `internal/report`

**Files:**
- Create: `internal/report/days.go`
- Modify: `internal/tui/stats.go:15-101` (remove the moved helpers, call the new ones)
- Modify: `internal/tui/app.go:667,693-694` (callers of `loadDayStats`/`computeStreaks`/`mostActiveDay` — rename to `report.*`)
- Test: `internal/report/days_test.go` (create)

**Interfaces:**
- Consumes: `db.ListSessions` (existing), `model.Session` / `model.StatusRunning` (existing).
- Produces:
  - `report.DayStat{Secs int; Sessions int}`
  - `report.DayKey(t time.Time) string` → `"2006-01-02"`
  - `report.LoadDayStats(d *db.DB) map[string]report.DayStat` — every non-running session bucketed by local calendar day.
  - `report.Streaks(stats map[string]report.DayStat) (current, longest int)`
  - `report.MostActiveDay(stats map[string]report.DayStat) (label string, secs int)`
- `internal/tui/stats.go` keeps its rendering helpers (`heatCell`, `renderHeatmap`, `breakdownDays`, …) but the four data helpers now live in `report` and are called via `report.` — output must be byte-identical.

- [ ] **Step 1: Characterise current behaviour with a test on the new location**

Create `internal/report/days_test.go`:
```go
package report_test

import (
	"testing"
	"time"

	"pomo/internal/db/dbtest"
	"pomo/internal/model"
	"pomo/internal/report"
)

func seedSession(t *testing.T, d interface {
	CreateSession(model.Session) (int64, error)
	FinishSession(int64, model.SessionStatus, int, string) error
}, start time.Time, secs int) {
	t.Helper()
	id, err := d.CreateSession(model.Session{
		TaskName: "x", PlannedDuration: 1500, Status: model.StatusRunning, StartedAt: start,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.FinishSession(id, model.StatusCompleted, secs, ""); err != nil {
		t.Fatal(err)
	}
}

func TestLoadDayStatsAndStreaks(t *testing.T) {
	d := dbtest.NewTemp(t)
	now := time.Now()
	// three consecutive days ending today
	seedSession(t, d, midnightLocal(now.AddDate(0, 0, -2)).Add(9*time.Hour), 1500)
	seedSession(t, d, midnightLocal(now.AddDate(0, 0, -1)).Add(9*time.Hour), 1500)
	seedSession(t, d, midnightLocal(now).Add(9*time.Hour), 1500)

	stats := report.LoadDayStats(d)
	if len(stats) != 3 {
		t.Fatalf("got %d day buckets, want 3", len(stats))
	}
	cur, longest := report.Streaks(stats)
	if cur != 3 || longest != 3 {
		t.Fatalf("streaks: current=%d longest=%d, want 3/3", cur, longest)
	}
}

func midnightLocal(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/report/ -run TestLoadDayStats -v`
Expected: FAIL — `report.LoadDayStats` undefined.

- [ ] **Step 3: Create `internal/report/days.go` with the moved code**

Move `dayStat`, `dayKey`, `loadDayStats`, `computeStreaks`, `mostActiveDay` out of `internal/tui/stats.go` and into `internal/report/days.go`, exporting them:
```go
package report

import (
	"sort"
	"time"

	"pomo/internal/db"
	"pomo/internal/model"
)

// DayStat aggregates one calendar day's finished-session activity.
type DayStat struct {
	Secs     int
	Sessions int
}

// DayKey is the local-date bucket key for a timestamp.
func DayKey(t time.Time) string { return t.Format("2006-01-02") }

// LoadDayStats buckets every non-running session into its calendar day. The
// dataset is a single user's history, small enough to scan in full.
func LoadDayStats(d *db.DB) map[string]DayStat {
	sessions, _ := d.ListSessions(db.SessionFilter{})
	stats := map[string]DayStat{}
	for _, s := range sessions {
		if s.Status == model.StatusRunning {
			continue
		}
		key := DayKey(s.StartedAt)
		ds := stats[key]
		ds.Secs += s.ActualDuration
		ds.Sessions++
		stats[key] = ds
	}
	return stats
}

// Streaks returns the current consecutive-active-day streak and the longest
// streak ever recorded.
func Streaks(stats map[string]DayStat) (current, longest int) {
	dates := make([]time.Time, 0, len(stats))
	for k, v := range stats {
		if v.Secs <= 0 {
			continue
		}
		if t, err := time.ParseInLocation("2006-01-02", k, time.Local); err == nil {
			dates = append(dates, t)
		}
	}
	if len(dates) == 0 {
		return 0, 0
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	longest, run := 1, 1
	for i := 1; i < len(dates); i++ {
		if dates[i].Sub(dates[i-1]).Hours() == 24 {
			run++
		} else {
			run = 1
		}
		if run > longest {
			longest = run
		}
	}

	now := time.Now()
	cursor := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if stats[DayKey(cursor)].Secs <= 0 {
		cursor = cursor.AddDate(0, 0, -1)
		if stats[DayKey(cursor)].Secs <= 0 {
			return 0, longest
		}
	}
	for stats[DayKey(cursor)].Secs > 0 {
		current++
		cursor = cursor.AddDate(0, 0, -1)
	}
	return current, longest
}

// MostActiveDay returns the label ("Jan 02") and seconds of the best day on record.
func MostActiveDay(stats map[string]DayStat) (string, int) {
	best, bestSecs := "-", 0
	for k, v := range stats {
		if v.Secs > bestSecs {
			t, err := time.ParseInLocation("2006-01-02", k, time.Local)
			if err != nil {
				continue
			}
			best, bestSecs = t.Format("Jan 02"), v.Secs
		}
	}
	return best, bestSecs
}
```

- [ ] **Step 4: Update `internal/tui/stats.go` to use the moved helpers**

In `internal/tui/stats.go`:
- Delete the `dayStat` type, `dayKey`, `loadDayStats`, `computeStreaks`, `mostActiveDay` (lines ~15-101).
- Add `"pomo/internal/report"` to imports; drop `"sort"` if now unused (keep `"pomo/internal/model"` only if still referenced — after removal it is not, so drop it too; keep `"time"`, `"fmt"`, `"strings"`, lipgloss).
- Replace remaining references:
  - `dayStat` → `report.DayStat`
  - `dayKey(` → `report.DayKey(`
  - `loadDayStats(` → `report.LoadDayStats(`
  - `computeStreaks(` → `report.Streaks(`
  - `mostActiveDay(` → `report.MostActiveDay(`
- Then find every caller of these in the `tui` package (grep below) and apply the same rename.

Run to find callers:
```bash
grep -rn "loadDayStats\|computeStreaks\|mostActiveDay\|dayStat\|dayKey" internal/tui/
```
Confirmed callers are in `internal/tui/app.go` (lines ~667, 693-694): `loadDayStats(a.db)` → `report.LoadDayStats(a.db)`, `computeStreaks(stats)` → `report.Streaks(stats)`, `mostActiveDay(stats)` → `report.MostActiveDay(stats)`. Add the `"pomo/internal/report"` import to `app.go` too. `dashboard.go` has its own `fmtDur` — leave it alone.

- [ ] **Step 5: Build and run the full suite**

Run: `go build ./... && make test`
Expected: PASS. No behaviour change — `tui` renders identically.

- [ ] **Step 6: Manual check the TUI stats screen still renders**

Run: `go run . ` then open the stats screen (press the stats key from the dashboard).
Expected: heatmap, streaks, most-active-day all display as before.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "refactor(report): move day-bucket and streak helpers out of tui/stats"
```

---

### Task 7: `internal/report` — Summary + Build

**Files:**
- Create: `internal/report/summary.go`
- Test: `internal/report/summary_test.go` (create)

**Interfaces:**
- Consumes: `db.ListSessions(db.SessionFilter{From,To})`, `db.ListDriftEvents(from,to)` (Task 4), `report.LoadDayStats` / `report.Streaks` (Task 6), `model.Session` / `model.SessionStatus` constants, `model.DriftEvent`.
- Produces:
```go
type AppDrift struct { App string; Seconds int }
type TagStat  struct { Tag string; FocusSeconds int }
type HourStat struct { Hour int; FocusSeconds int; DriftSeconds int }

type Summary struct {
	Window         Window
	Planned        int
	Completed      int
	Cancelled      int
	Skipped        int
	FocusSeconds   int
	PlannedSeconds int
	DriftSeconds   int
	DriftEpisodes  int
	DriftByApp     []AppDrift  // desc by Seconds
	ByTag          []TagStat   // desc by FocusSeconds
	Streak         int
	BestHour       *HourStat   // nil when no completed focus in window
}

func Build(d *db.DB, w Window) (Summary, error)
```
  - "Planned" = count of sessions whose `StartedAt` is in the window and status is not `running`.
  - "Completed" counts `StatusCompleted`; "Cancelled" `StatusCancelled`; "Skipped" `StatusSkipped`. `StatusInterrupted` counts toward neither completed nor cancelled but its `ActualDuration` still adds to `FocusSeconds`.
  - `FocusSeconds` = sum of `ActualDuration` for `completed` + `interrupted` sessions in the window.
  - `PlannedSeconds` = sum of `PlannedDuration` for all non-running sessions in the window.
  - `DriftByApp` groups `DriftEvent.Detail` by its app prefix: text before `" — "` if present, else the whole string; empty → `"idle"`.
  - `BestHour` = the clock hour (0–23, from `StartedAt.Hour()`) with the greatest `FocusSeconds`; ties break toward lower `DriftSeconds`, then lower hour.
  - `Streak` = `report.Streaks(report.LoadDayStats(d))` current value (whole-history streak, not window-scoped — matches the dashboard).

- [ ] **Step 1: Write the failing test**

Create `internal/report/summary_test.go`:
```go
package report_test

import (
	"testing"
	"time"

	"pomo/internal/db"
	"pomo/internal/db/dbtest"
	"pomo/internal/model"
	"pomo/internal/report"
)

func TestBuildAggregates(t *testing.T) {
	d := dbtest.NewTemp(t)
	day := time.Date(2026, 9, 4, 0, 0, 0, 0, time.Local)
	w := report.Window{From: day, To: day.AddDate(0, 0, 1), Label: "test"}

	mk := func(status model.SessionStatus, tag string, hour, planned, actual int) int64 {
		id, err := d.CreateSession(model.Session{
			TaskName: "t", Tag: tag, PlannedDuration: planned,
			Status: model.StatusRunning, StartedAt: day.Add(time.Duration(hour) * time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := d.FinishSession(id, status, actual, ""); err != nil {
			t.Fatal(err)
		}
		return id
	}
	s1 := mk(model.StatusCompleted, "backend", 10, 1500, 1500)
	mk(model.StatusCompleted, "backend", 11, 1500, 1200)
	mk(model.StatusCancelled, "docs", 14, 1500, 300)

	// drift on session 1: 28 min Chrome, 9 min Slack
	id, _ := d.OpenDriftEpisode(s1, day.Add(10*time.Hour+5*time.Minute), "foreground", "Google Chrome — reddit.com")
	d.CloseDriftEpisode(id, day.Add(10*time.Hour+33*time.Minute), 28*60)
	id2, _ := d.OpenDriftEpisode(s1, day.Add(10*time.Hour+40*time.Minute), "foreground", "Slack")
	d.CloseDriftEpisode(id2, day.Add(10*time.Hour+49*time.Minute), 9*60)

	sum, err := report.Build(d, w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if sum.Planned != 3 || sum.Completed != 2 || sum.Cancelled != 1 {
		t.Fatalf("counts: %+v", sum)
	}
	if sum.FocusSeconds != 2700 { // 1500 + 1200
		t.Fatalf("FocusSeconds = %d, want 2700", sum.FocusSeconds)
	}
	if sum.PlannedSeconds != 4500 {
		t.Fatalf("PlannedSeconds = %d, want 4500", sum.PlannedSeconds)
	}
	if sum.DriftSeconds != 37*60 || sum.DriftEpisodes != 2 {
		t.Fatalf("drift: %ds / %d episodes", sum.DriftSeconds, sum.DriftEpisodes)
	}
	if len(sum.DriftByApp) != 2 || sum.DriftByApp[0].App != "Google Chrome" || sum.DriftByApp[0].Seconds != 28*60 {
		t.Fatalf("DriftByApp = %+v", sum.DriftByApp)
	}
	if len(sum.ByTag) == 0 || sum.ByTag[0].Tag != "backend" || sum.ByTag[0].FocusSeconds != 2700 {
		t.Fatalf("ByTag = %+v", sum.ByTag)
	}
	if sum.BestHour == nil || sum.BestHour.Hour != 10 {
		t.Fatalf("BestHour = %+v, want hour 10", sum.BestHour)
	}
}

func TestBuildEmptyWindow(t *testing.T) {
	d := dbtest.NewTemp(t)
	sum, err := report.Build(d, report.Window{
		From: time.Now().AddDate(0, 0, -1), To: time.Now(), Label: "empty",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if sum.Planned != 0 || sum.BestHour != nil || len(sum.DriftByApp) != 0 {
		t.Fatalf("empty window not empty: %+v", sum)
	}
	_ = db.SessionFilter{}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/report/ -run TestBuild -v`
Expected: FAIL — `report.Build` undefined.

- [ ] **Step 3: Implement**

Create `internal/report/summary.go`:
```go
package report

import (
	"sort"
	"strings"

	"pomo/internal/db"
	"pomo/internal/model"
)

type AppDrift struct {
	App     string
	Seconds int
}

type TagStat struct {
	Tag          string
	FocusSeconds int
}

type HourStat struct {
	Hour         int
	FocusSeconds int
	DriftSeconds int
}

type Summary struct {
	Window         Window
	Planned        int
	Completed      int
	Cancelled      int
	Skipped        int
	FocusSeconds   int
	PlannedSeconds int
	DriftSeconds   int
	DriftEpisodes  int
	DriftByApp     []AppDrift
	ByTag          []TagStat
	Streak         int
	BestHour       *HourStat
}

func isFocusStatus(s model.SessionStatus) bool {
	return s == model.StatusCompleted || s == model.StatusInterrupted
}

// appPrefix extracts the application name from a drift detail string.
func appPrefix(detail string) string {
	if detail == "" {
		return "idle"
	}
	if i := strings.Index(detail, " — "); i >= 0 {
		return detail[:i]
	}
	return detail
}

// Build aggregates sessions and drift events in the window into a Summary.
func Build(d *db.DB, w Window) (Summary, error) {
	sum := Summary{Window: w}

	sessions, err := d.ListSessions(db.SessionFilter{From: &w.From, To: &w.To})
	if err != nil {
		return sum, err
	}

	tagFocus := map[string]int{}
	hourFocus := map[int]int{}
	for _, s := range sessions {
		if s.Status == model.StatusRunning {
			continue
		}
		sum.Planned++
		sum.PlannedSeconds += s.PlannedDuration
		switch s.Status {
		case model.StatusCompleted:
			sum.Completed++
		case model.StatusCancelled:
			sum.Cancelled++
		case model.StatusSkipped:
			sum.Skipped++
		}
		if isFocusStatus(s.Status) {
			sum.FocusSeconds += s.ActualDuration
			tagFocus[s.Tag] += s.ActualDuration
			hourFocus[s.StartedAt.Hour()] += s.ActualDuration
		}
	}

	events, err := d.ListDriftEvents(w.From, w.To)
	if err != nil {
		return sum, err
	}
	appDrift := map[string]int{}
	hourDrift := map[int]int{}
	for _, e := range events {
		sum.DriftSeconds += e.Seconds
		sum.DriftEpisodes++
		appDrift[appPrefix(e.Detail)] += e.Seconds
		hourDrift[e.StartedAt.Hour()] += e.Seconds
	}

	for app, secs := range appDrift {
		sum.DriftByApp = append(sum.DriftByApp, AppDrift{App: app, Seconds: secs})
	}
	sort.Slice(sum.DriftByApp, func(i, j int) bool {
		if sum.DriftByApp[i].Seconds != sum.DriftByApp[j].Seconds {
			return sum.DriftByApp[i].Seconds > sum.DriftByApp[j].Seconds
		}
		return sum.DriftByApp[i].App < sum.DriftByApp[j].App
	})

	for tag, secs := range tagFocus {
		sum.ByTag = append(sum.ByTag, TagStat{Tag: tag, FocusSeconds: secs})
	}
	sort.Slice(sum.ByTag, func(i, j int) bool {
		if sum.ByTag[i].FocusSeconds != sum.ByTag[j].FocusSeconds {
			return sum.ByTag[i].FocusSeconds > sum.ByTag[j].FocusSeconds
		}
		return sum.ByTag[i].Tag < sum.ByTag[j].Tag
	})

	for h, secs := range hourFocus {
		cand := HourStat{Hour: h, FocusSeconds: secs, DriftSeconds: hourDrift[h]}
		if sum.BestHour == nil ||
			cand.FocusSeconds > sum.BestHour.FocusSeconds ||
			(cand.FocusSeconds == sum.BestHour.FocusSeconds && cand.DriftSeconds < sum.BestHour.DriftSeconds) ||
			(cand.FocusSeconds == sum.BestHour.FocusSeconds && cand.DriftSeconds == sum.BestHour.DriftSeconds && cand.Hour < sum.BestHour.Hour) {
			c := cand
			sum.BestHour = &c
		}
	}

	sum.Streak, _ = Streaks(LoadDayStats(d))
	return sum, nil
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/report/ -run TestBuild -v`
Expected: PASS (both).

- [ ] **Step 5: Full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(report): add Summary and Build aggregation"
```

---

### Task 8: `internal/report` — RenderText, RenderMarkdown, JSON

**Files:**
- Create: `internal/report/render.go`
- Test: `internal/report/render_test.go` (create)
- Test fixtures: `internal/report/testdata/summary.md` (create in Step 5)

**Interfaces:**
- Consumes: `Summary` (Task 7).
- Produces:
  - `report.RenderText(s Summary) string` — plain-text block (lipgloss styling optional; keep it ASCII-safe so tests are stable — no color codes: use `lipgloss` only if `lipgloss.SetColorProfile` is forced off in tests; simpler to emit plain strings here and let the TUI wrap).
  - `report.RenderMarkdown(s Summary, recap string) string` — the digest markdown from `specs/2026-09-04-review-and-digest.md` §5. `recap == ""` omits the `## Recap` section.
  - `report.(Summary).JSON() ([]byte, error)` — `json.MarshalIndent`, 2-space. Stable key set.
  - Helper `report.FmtDur(secs int) string` → `"1h02m"` / `"12m"` / `"0m"` (mirrors the existing unexported `fmtDur` in `internal/tui/dashboard.go:105` — keep formats identical; do NOT import tui, and do NOT touch the tui copy).

- [ ] **Step 1: Write the failing tests**

Create `internal/report/render_test.go`:
```go
package report_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pomo/internal/report"
)

func sampleSummary() report.Summary {
	day := time.Date(2026, 9, 4, 0, 0, 0, 0, time.Local)
	return report.Summary{
		Window:         report.Window{From: day, To: day.AddDate(0, 0, 1), Label: "today"},
		Planned:        4,
		Completed:      3,
		Cancelled:      1,
		FocusSeconds:   62 * 60,
		PlannedSeconds: 100 * 60,
		DriftSeconds:   41 * 60,
		DriftEpisodes:  5,
		DriftByApp:     []report.AppDrift{{"Google Chrome", 28 * 60}, {"Slack", 9 * 60}, {"idle", 4 * 60}},
		ByTag:          []report.TagStat{{"backend", 38 * 60}, {"docs", 15 * 60}},
		Streak:         4,
		BestHour:       &report.HourStat{Hour: 10, FocusSeconds: 18 * 60, DriftSeconds: 0},
	}
}

func TestFmtDur(t *testing.T) {
	cases := map[int]string{0: "0m", 12 * 60: "12m", 62 * 60: "1h02m", 3600: "1h00m"}
	for secs, want := range cases {
		if got := report.FmtDur(secs); got != want {
			t.Errorf("FmtDur(%d) = %q, want %q", secs, got, want)
		}
	}
}

func TestRenderTextContainsKeyNumbers(t *testing.T) {
	out := report.RenderText(sampleSummary())
	for _, want := range []string{"today", "3 done", "1h02m", "41m", "Google Chrome", "backend", "10:00", "streak 4"} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderText missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderMarkdownOmitsRecapWhenEmpty(t *testing.T) {
	with := report.RenderMarkdown(sampleSummary(), "some recap")
	without := report.RenderMarkdown(sampleSummary(), "")
	if !strings.Contains(with, "## Recap") || !strings.Contains(with, "some recap") {
		t.Error("recap section missing when recap provided")
	}
	if strings.Contains(without, "## Recap") {
		t.Error("recap section present when recap empty")
	}
}

func TestJSONStableKeys(t *testing.T) {
	b, err := sampleSummary().JSON()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"Window", "Planned", "Completed", "FocusSeconds", "DriftByApp", "ByTag", "Streak", "BestHour"} {
		if _, ok := m[k]; !ok {
			t.Errorf("JSON missing key %q", k)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/report/ -run 'TestFmtDur|TestRender|TestJSON' -v`
Expected: FAIL — undefined `report.RenderText` etc.

- [ ] **Step 3: Implement**

Create `internal/report/render.go`:
```go
package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// FmtDur formats a second count as "1h02m" or "12m".
func FmtDur(secs int) string {
	d := time.Duration(secs) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// JSON renders the summary as indented JSON.
func (s Summary) JSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// RenderText renders a plain-text summary block for terminal display.
func RenderText(s Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "FOCUS  %s\n", s.Window.Label)
	fmt.Fprintf(&b, "  %d planned · %d done · %d cancelled · %d skipped\n",
		s.Planned, s.Completed, s.Cancelled, s.Skipped)
	fmt.Fprintf(&b, "  %s focus / %s planned\n", FmtDur(s.FocusSeconds), FmtDur(s.PlannedSeconds))
	fmt.Fprintf(&b, "\nDRIFT  %s · %d episodes\n", FmtDur(s.DriftSeconds), s.DriftEpisodes)
	for _, a := range s.DriftByApp {
		fmt.Fprintf(&b, "  %-16s %s\n", a.App, FmtDur(a.Seconds))
	}
	if len(s.ByTag) > 0 {
		b.WriteString("\nBY TAG\n")
		for _, tg := range s.ByTag {
			name := tg.Tag
			if name == "" {
				name = "(untagged)"
			}
			fmt.Fprintf(&b, "  %-16s %s\n", name, FmtDur(tg.FocusSeconds))
		}
	}
	fmt.Fprintf(&b, "\nstreak %d", s.Streak)
	if s.BestHour != nil {
		fmt.Fprintf(&b, "  ·  best hour %02d:00 (%s focus, %s drift)",
			s.BestHour.Hour, FmtDur(s.BestHour.FocusSeconds), FmtDur(s.BestHour.DriftSeconds))
	}
	b.WriteString("\n")
	return b.String()
}

// RenderMarkdown renders the weekly-digest markdown. An empty recap omits the
// Recap section.
func RenderMarkdown(s Summary, recap string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Pomo — %s\n\n", s.Window.Label)

	b.WriteString("## Focus\n\n")
	fmt.Fprintf(&b, "**Totals:** %d sessions · %d completed · %s focus / %s planned · %s drift\n\n",
		s.Planned, s.Completed, FmtDur(s.FocusSeconds), FmtDur(s.PlannedSeconds), FmtDur(s.DriftSeconds))

	if len(s.DriftByApp) > 0 {
		b.WriteString("## Drift\n\n")
		for _, a := range s.DriftByApp {
			fmt.Fprintf(&b, "- %s — %s\n", a.App, FmtDur(a.Seconds))
		}
		b.WriteString("\n")
	}

	if len(s.ByTag) > 0 {
		b.WriteString("## By tag\n\n")
		for _, tg := range s.ByTag {
			name := tg.Tag
			if name == "" {
				name = "(untagged)"
			}
			fmt.Fprintf(&b, "- %s — %s\n", name, FmtDur(tg.FocusSeconds))
		}
		b.WriteString("\n")
	}

	if strings.TrimSpace(recap) != "" {
		fmt.Fprintf(&b, "## Recap\n\n%s\n", strings.TrimSpace(recap))
	}
	return b.String()
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/report/ -run 'TestFmtDur|TestRender|TestJSON' -v`
Expected: PASS (all four).

- [ ] **Step 5: Full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(report): add RenderText, RenderMarkdown, JSON"
```

---

### Task 9: `pomo review` CLI command

**Files:**
- Create: `cmd/review.go`
- Test: `cmd/review_test.go` (create)

**Interfaces:**
- Consumes: `report.ParseWindow`, `report.Build`, `report.RenderText`, `Summary.JSON` (Tasks 5, 7, 8); the package-level `database *db.DB` in `cmd/root.go`.
- Produces: a cobra command `review` registered on `rootCmd`.
  - `pomo review [today|week|month|YYYY-Www|YYYY-MM]` → `RenderText` to stdout.
  - `--json` → `Summary.JSON()` to stdout.
  - `--include-notes` flag is accepted but currently a no-op (reserved for the `--insights` path in a later chunk). Document that in the flag help: `"reserved; no effect yet"`.
  - No AI in this chunk — no `--insights` flag yet.
  - Exposes `runReview(args []string, jsonOut bool, w io.Writer) error` for testing without cobra plumbing.

- [ ] **Step 1: Write the failing test**

Create `cmd/review_test.go`:
```go
package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"pomo/internal/db/dbtest"
	"pomo/internal/model"
)

func TestRunReviewText(t *testing.T) {
	d := dbtest.NewTemp(t)
	database = d // package-level handle used by runReview
	t.Cleanup(func() { database = nil })

	now := time.Now()
	id, err := d.CreateSession(model.Session{
		TaskName: "t", Tag: "backend", PlannedDuration: 1500,
		Status: model.StatusRunning, StartedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.FinishSession(id, model.StatusCompleted, 1500, ""); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runReview([]string{"today"}, false, &buf); err != nil {
		t.Fatalf("runReview: %v", err)
	}
	if !strings.Contains(buf.String(), "1 done") {
		t.Fatalf("output missing '1 done':\n%s", buf.String())
	}
}

func TestRunReviewJSON(t *testing.T) {
	d := dbtest.NewTemp(t)
	database = d
	t.Cleanup(func() { database = nil })

	var buf bytes.Buffer
	if err := runReview([]string{"today"}, true, &buf); err != nil {
		t.Fatalf("runReview: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(buf.String()), "{") {
		t.Fatalf("expected JSON object, got:\n%s", buf.String())
	}
}

func TestRunReviewBadWindow(t *testing.T) {
	d := dbtest.NewTemp(t)
	database = d
	t.Cleanup(func() { database = nil })
	if err := runReview([]string{"not-a-window"}, false, new(bytes.Buffer)); err == nil {
		t.Fatal("expected error for bad window")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/ -run TestRunReview -v`
Expected: FAIL — `runReview` undefined.

- [ ] **Step 3: Implement**

Create `cmd/review.go`:
```go
package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"pomo/internal/report"
)

var (
	reviewJSON         bool
	reviewIncludeNotes bool
)

var reviewCmd = &cobra.Command{
	Use:   "review [today|week|month|YYYY-Www|YYYY-MM]",
	Short: "Show focus vs plan and drift for a period",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		return runReview(args, reviewJSON, os.Stdout)
	},
}

func init() {
	reviewCmd.Flags().BoolVar(&reviewJSON, "json", false, "output machine-readable JSON")
	reviewCmd.Flags().BoolVar(&reviewIncludeNotes, "include-notes", false, "reserved; no effect yet")
	rootCmd.AddCommand(reviewCmd)
}

func runReview(args []string, jsonOut bool, w io.Writer) error {
	spec := ""
	if len(args) == 1 {
		spec = args[0]
	}
	win, err := report.ParseWindow(spec)
	if err != nil {
		return err
	}
	sum, err := report.Build(database, win)
	if err != nil {
		return err
	}
	if jsonOut {
		b, err := sum.JSON()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(b))
		return err
	}
	_, err = fmt.Fprint(w, report.RenderText(sum))
	return err
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./cmd/ -run TestRunReview -v`
Expected: PASS (all three).

- [ ] **Step 5: Full suite + build + smoke**

Run:
```bash
make test && go build -o pomo . && ./pomo review today && ./pomo review today --json
```
Expected: suite PASS; `./pomo review today` prints the text block; `--json` prints an indented JSON object. (A fresh `~/.pomo/pomo.db` will show all zeros — that is correct.)

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(cmd): add pomo review command"
```

---

### Task 10: Update CLAUDE.md

**Files:**
- Modify: `CLAUDE.md`

**Interfaces:**
- Consumes: nothing.
- Produces: docs reflect the new package + command + test target.

- [ ] **Step 1: Edit CLAUDE.md**

Under **Commands**, change the test line to:
```
- `make test` / `go test ./...` — test suite (add `-run TestName ./pkg/` for one test)
```

Under **Architecture**, add a bullet:
```
- **`internal/report`** — all windowed aggregation (sessions + `drift_events`) and
  rendering (text / markdown / JSON). Shared by `pomo review`, the TUI stats
  screen, and (later) the daemon's weekly digest. Day-bucket and streak helpers
  live here, not in `internal/tui`.
```

Under **Conventions**, add:
```
- Two processes will open `~/.pomo/pomo.db` (CLI + daemon); it runs in WAL mode
  with a 5s busy timeout. Keep writes small. `db.OpenAt(path)` is the test/tools
  entrypoint; `db.Open()` is the normal one.
- Additive schema changes: new tables via the `schema` const
  (`CREATE TABLE IF NOT EXISTS`); new columns via `ensureColumn` in `OpenAt`
  (SQLite `ALTER TABLE ADD COLUMN` is not idempotent).
```

- [ ] **Step 2: Verify + commit**

Run: `make test`
Expected: PASS.
```bash
git add -A
git commit -m "docs: update CLAUDE.md for internal/report and pomo review"
```

---

## Self-Review

**1. Spec coverage**

`specs/2026-09-04-schema-config-migration.md`:
- `drift_events` table + indexes — Task 4. ✓
- `ensureColumn` + `repo_path`/`repo_branch` — Task 3. ✓
- `model.DriftEvent`, Session fields — Tasks 3, 4. ✓
- `OpenDriftEpisode`/`UpdateDriftEpisode`/`CloseDriftEpisode`/`ListDriftEvents`/`DriftEventsForSession` — Task 4. ✓
- WAL DSN + busy timeout — Task 2 (`OpenAt`). ✓
- Config keys, `/settings` rows, `pomo config` display — **deferred to the 4/5 chunk** (this plan is schema + report + review CLI only, per the user's chunk decision). Noted, not a gap.
- Backward compat (existing db gets columns, empty drift table) — Task 3 Step 4 + Task 4; `ensureColumn` handles the pre-existing-db case.

`specs/2026-09-04-review-and-digest.md`:
- `internal/report` package: `Window`, `Today/ThisWeek/ThisMonth`, `ParseWindow` — Task 5. ✓
- `Summary` + all fields, `Build` — Task 7. ✓
- `AppDrift` grouping on detail prefix, `ByTag`, `Streak`, `BestHour` — Task 7. ✓
- `RenderText`, `RenderMarkdown`, `JSON` — Task 8. ✓
- `stats.go` refactor to share aggregation — Task 6. ✓
- `pomo review [window] [--json] [--include-notes]` — Task 9. ✓
- `--insights` / recap / `RecapContext` — **deferred** (needs `internal/ai`, a later chunk). `--include-notes` flag stubbed. Noted, not a gap.
- `pomo digest` + daemon weekly auto-write — **deferred** (needs the daemon). `RenderMarkdown` is built now so the digest task is small later. Noted, not a gap.

**2. Placeholder scan** — no "TBD"/"handle edge cases"/"similar to Task N". Every code step has full code. `--include-notes` is an intentional documented stub, not a placeholder.

**3. Type consistency**
- `report.DayStat` / `report.DayKey` / `report.LoadDayStats` / `report.Streaks` / `report.MostActiveDay` — defined Task 6, consumed Task 7 (`Streaks`, `LoadDayStats`). Names match.
- `report.Window{From,To,Label}` — Task 5, consumed Tasks 7/8/9. Match.
- `report.Summary` field names — defined Task 7, used Task 8 render + Task 9. Cross-checked: `FocusSeconds`, `PlannedSeconds`, `DriftByApp`, `ByTag`, `BestHour`, `Streak`, `DriftEpisodes`. Match.
- `db.OpenDriftEpisode(sessionID int64, at time.Time, trigger, detail string)` — Task 4 def, Task 7 test usage. Match.
- `db.ListDriftEvents(from, to time.Time)` — Task 4 def, Task 7 `Build` usage. Match.
- `db.OpenAt(path string)` — Task 2 def, used by `dbtest.NewTemp` (Task 2) and everywhere via `dbtest`. Match.
- `runReview([]string, bool, io.Writer) error` — Task 9 def + test. Match.
- `report.FmtDur` — Task 8 def + test; not confused with `internal/tui/theme.go`'s unexported `fmtDur` (kept separate deliberately, per Task 8 interface note).

No issues found.
