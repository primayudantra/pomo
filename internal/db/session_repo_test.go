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
