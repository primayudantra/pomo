package daemon

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"pomo/internal/db"
)

type Status struct {
	PID            int       `json:"pid"`
	StartedAt      time.Time `json:"started_at"`
	LastTick       time.Time `json:"last_tick"`
	WatchBackend   string    `json:"watch_backend"`
	WatchSupported bool      `json:"watch_supported"`
	WatchingID     int64     `json:"watching_id"`
}

func statusPath() string { return filepath.Join(db.Dir(), "daemon.status") }

func WriteStatus(s Status) error {
	if err := os.MkdirAll(db.Dir(), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statusPath(), b, 0o644)
}

func ReadStatus() (Status, bool) {
	b, err := os.ReadFile(statusPath())
	if err != nil {
		return Status{}, false
	}
	var s Status
	if json.Unmarshal(b, &s) != nil {
		return Status{}, false
	}
	return s, true
}

func RemoveStatus() { _ = os.Remove(statusPath()) }
