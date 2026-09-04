package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"

	"pomo/internal/model"
)

const schema = `
CREATE TABLE IF NOT EXISTS tasks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	tag TEXT DEFAULT '',
	done INTEGER DEFAULT 0,
	created_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_id INTEGER,
	task_name TEXT NOT NULL,
	tag TEXT DEFAULT '',
	planned_duration INTEGER NOT NULL,
	actual_duration INTEGER DEFAULT 0,
	status TEXT NOT NULL,
	note TEXT DEFAULT '',
	started_at TIMESTAMP NOT NULL,
	completed_at TIMESTAMP,
	created_at TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS config (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

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
`

type DB struct {
	*sql.DB
}

func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".pomo")
}

func Open() (*DB, error) {
	dir := Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return OpenAt(filepath.Join(dir, "pomo.db"))
}

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
	for _, c := range []struct{ table, col, ddl string }{
		{"sessions", "repo_path", "repo_path TEXT DEFAULT ''"},
		{"sessions", "repo_branch", "repo_branch TEXT DEFAULT ''"},
	} {
		if err := ensureColumn(sqlDB, c.table, c.col, c.ddl); err != nil {
			return nil, fmt.Errorf("migrate %s.%s: %w", c.table, c.col, err)
		}
	}
	return &DB{sqlDB}, nil
}

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

// --- Tasks ---

func (d *DB) AddTask(name, tag string) (int64, error) {
	res, err := d.Exec(`INSERT INTO tasks (name, tag, done, created_at) VALUES (?, ?, 0, ?)`,
		name, tag, time.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (d *DB) ListOpenTasks() ([]model.Task, error) {
	rows, err := d.Query(`SELECT id, name, tag, done, created_at FROM tasks WHERE done = 0 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func (d *DB) ListAllTasks() ([]model.Task, error) {
	rows, err := d.Query(`SELECT id, name, tag, done, created_at FROM tasks ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTasks(rows)
}

func scanTasks(rows *sql.Rows) ([]model.Task, error) {
	var tasks []model.Task
	for rows.Next() {
		var t model.Task
		var done int
		if err := rows.Scan(&t.ID, &t.Name, &t.Tag, &done, &t.CreatedAt); err != nil {
			return nil, err
		}
		t.Done = done == 1
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (d *DB) GetTask(id int64) (*model.Task, error) {
	row := d.QueryRow(`SELECT id, name, tag, done, created_at FROM tasks WHERE id = ?`, id)
	var t model.Task
	var done int
	if err := row.Scan(&t.ID, &t.Name, &t.Tag, &done, &t.CreatedAt); err != nil {
		return nil, err
	}
	t.Done = done == 1
	return &t, nil
}

func (d *DB) CompleteTask(id int64) error {
	_, err := d.Exec(`UPDATE tasks SET done = 1 WHERE id = ?`, id)
	return err
}

func (d *DB) DeleteTask(id int64) error {
	_, err := d.Exec(`DELETE FROM tasks WHERE id = ?`, id)
	return err
}

func (d *DB) FindOrCreateTask(name, tag string) (int64, error) {
	row := d.QueryRow(`SELECT id FROM tasks WHERE name = ? AND done = 0 LIMIT 1`, name)
	var id int64
	err := row.Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	return d.AddTask(name, tag)
}

// --- Sessions ---

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

func (d *DB) DeleteSession(id int64) error {
	res, err := d.Exec(`DELETE FROM sessions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("no session with id %d", id)
	}
	return nil
}

func (d *DB) FinishSession(id int64, status model.SessionStatus, actualDuration int, note string) error {
	now := time.Now()
	_, err := d.Exec(`UPDATE sessions SET status = ?, actual_duration = ?, note = ?, completed_at = ? WHERE id = ?`,
		status, actualDuration, note, now, id)
	return err
}

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

type SessionFilter struct {
	From *time.Time
	To   *time.Time
	Task string
	Tag  string
}

func (d *DB) ListSessions(f SessionFilter) ([]model.Session, error) {
	q := `SELECT id, task_id, task_name, tag, planned_duration, actual_duration, status, note, started_at, completed_at, created_at, repo_path, repo_branch
		FROM sessions WHERE 1=1`
	var args []interface{}
	if f.From != nil {
		q += ` AND started_at >= ?`
		args = append(args, *f.From)
	}
	if f.To != nil {
		q += ` AND started_at < ?`
		args = append(args, *f.To)
	}
	if f.Task != "" {
		q += ` AND task_name LIKE ?`
		args = append(args, "%"+f.Task+"%")
	}
	if f.Tag != "" {
		q += ` AND tag = ?`
		args = append(args, f.Tag)
	}
	q += ` ORDER BY started_at ASC`
	rows, err := d.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []model.Session
	for rows.Next() {
		var s model.Session
		var completedAt sql.NullTime
		if err := rows.Scan(&s.ID, &s.TaskID, &s.TaskName, &s.Tag, &s.PlannedDuration, &s.ActualDuration,
			&s.Status, &s.Note, &s.StartedAt, &completedAt, &s.CreatedAt, &s.RepoPath, &s.RepoBranch); err != nil {
			return nil, err
		}
		if completedAt.Valid {
			s.CompletedAt = &completedAt.Time
		}
		sessions = append(sessions, s)
	}
	return sessions, rows.Err()
}

// --- Config ---

func (d *DB) GetConfig(key, def string) string {
	row := d.QueryRow(`SELECT value FROM config WHERE key = ?`, key)
	var v string
	if err := row.Scan(&v); err != nil {
		return def
	}
	return v
}

func (d *DB) SetConfig(key, value string) error {
	_, err := d.Exec(`INSERT INTO config (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

func (d *DB) AllConfig() (map[string]string, error) {
	rows, err := d.Query(`SELECT key, value FROM config`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		m[k] = v
	}
	return m, nil
}
