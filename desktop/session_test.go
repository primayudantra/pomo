package main

import (
	"path/filepath"
	"testing"
	"time"

	"pomo/internal/db"
	"pomo/internal/model"
)

func TestTodayView(t *testing.T) {
	d, _ := db.OpenAt(filepath.Join(t.TempDir(), "p.db"))
	now := time.Now()
	id, _ := d.CreateSession(model.Session{
		TaskName: "ship", PlannedDuration: 1500,
		Status: model.StatusRunning, StartedAt: now,
	})
	_ = d.FinishSession(id, model.StatusCompleted, 1500, "")

	id2, _ := d.CreateSession(model.Session{
		TaskName: "email", PlannedDuration: 1500,
		Status: model.StatusRunning, StartedAt: now,
	})
	_ = d.FinishSession(id2, model.StatusCancelled, 300, "")

	v, err := NewSessionService(d).Today()
	if err != nil {
		t.Fatal(err)
	}
	if v.Count != 1 || v.FocusMinutes != 25 {
		t.Fatalf("view = %+v", v)
	}
	if len(v.Sessions) != 2 {
		t.Fatalf("want 2 rows, got %d", len(v.Sessions))
	}
}
