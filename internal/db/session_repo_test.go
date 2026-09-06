package db_test

import (
	"testing"
	"time"

	"pomo/internal/db"
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

func TestFinishSessionStatusGuard(t *testing.T) {
	d := dbtest.NewTemp(t)
	id, err := d.CreateSession(model.Session{
		TaskName:        "guarded",
		PlannedDuration: 1500,
		Status:          model.StatusRunning,
		StartedAt:       time.Now(),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := d.FinishSession(id, model.StatusCompleted, 1500, ""); err != nil {
		t.Fatalf("first FinishSession should succeed: %v", err)
	}
	// A second finish must not overwrite the row: status is no longer running.
	if err := d.FinishSession(id, model.StatusCancelled, 1, ""); err == nil {
		t.Fatal("second FinishSession should error (session not running)")
	}
	all, err := d.ListSessions(db.SessionFilter{})
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(all) != 1 || all[0].Status != model.StatusCompleted || all[0].ActualDuration != 1500 {
		t.Fatalf("row was overwritten by the second finish: %+v", all)
	}
}
