package db_test

import (
	"testing"
	"time"

	"pomo/internal/db/dbtest"
	"pomo/internal/model"
)

func TestPauseTracking(t *testing.T) {
	d := dbtest.NewTemp(t)
	id, err := d.CreateSession(model.Session{
		TaskName: "x", PlannedDuration: 1500,
		Status: model.StatusRunning, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	pausedAt := time.Now()
	if err := d.SetPaused(id, pausedAt); err != nil {
		t.Fatal(err)
	}
	got, err := d.LastRunningSession()
	if err != nil {
		t.Fatal(err)
	}
	if got.PausedAt == nil {
		t.Fatal("PausedAt should be set")
	}

	if err := d.ClearPaused(id, 42); err != nil {
		t.Fatal(err)
	}
	got, _ = d.LastRunningSession()
	if got.PausedAt != nil {
		t.Errorf("PausedAt should be cleared, got %v", got.PausedAt)
	}
	if got.PauseAccumSecs != 42 {
		t.Errorf("PauseAccumSecs = %d, want 42", got.PauseAccumSecs)
	}

	// ClearPaused accumulates.
	_ = d.SetPaused(id, time.Now())
	_ = d.ClearPaused(id, 8)
	got, _ = d.LastRunningSession()
	if got.PauseAccumSecs != 50 {
		t.Errorf("PauseAccumSecs = %d, want 50", got.PauseAccumSecs)
	}
}
