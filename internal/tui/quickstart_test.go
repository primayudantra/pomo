package tui

import (
	"testing"
	"time"

	"pomo/internal/db/dbtest"
	"pomo/internal/model"
)

func TestQuickStartLandsOnDurationConfirm(t *testing.T) {
	d := dbtest.NewTemp(t)
	a, err := quickStartApp(d, "daemons tart", "")
	if err != nil {
		t.Fatal(err)
	}
	if a.screen != screenDuration {
		t.Fatalf("screen = %v, want screenDuration (confirm, not auto-run)", a.screen)
	}
	if a.pendingTask != "daemons tart" {
		t.Fatalf("pendingTask = %q", a.pendingTask)
	}
	// nothing started yet
	if s, _ := d.LastRunningSession(); s != nil {
		t.Fatal("quick start must not create a running session before the user confirms")
	}
	// esc from the duration screen goes back to the dashboard, not the picker
	a.updateDuration(keyMsg("esc"))
	if a.screen != screenDashboard {
		t.Fatalf("esc should return to dashboard, got %v", a.screen)
	}
}

func TestQuickStartWithRunningSessionOpensDashboard(t *testing.T) {
	d := dbtest.NewTemp(t)
	_, _ = d.CreateSession(model.Session{
		TaskName: "busy", PlannedDuration: 1500, Status: model.StatusRunning, StartedAt: time.Now(),
	})
	a, err := quickStartApp(d, "something else", "")
	if err != nil {
		t.Fatal(err)
	}
	if a.screen != screenDashboard {
		t.Fatalf("with a running session, quick start should open the dashboard, got %v", a.screen)
	}
}
