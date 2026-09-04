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
