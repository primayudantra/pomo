package tui

import (
	"strings"
	"testing"
	"time"

	"pomo/internal/db/dbtest"
	"pomo/internal/model"
)

func TestSlashReviewFillsResult(t *testing.T) {
	d := dbtest.NewTemp(t)
	id, _ := d.CreateSession(model.Session{
		TaskName: "t", Tag: "backend", PlannedDuration: 1500,
		Status: model.StatusRunning, StartedAt: time.Now(),
	})
	_ = d.FinishSession(id, model.StatusCompleted, 1500, "")

	a := NewApp(d)
	a.enterPrompt(screenDashboard)
	a.promptInput.SetValue("/review today")
	a.updatePrompt(keyMsg("enter"))

	if a.screen != screenResult {
		t.Fatalf("screen = %v, want screenResult", a.screen)
	}
	if !strings.Contains(a.result.View(), "1 done") {
		t.Fatalf("result missing review content:\n%s", a.result.View())
	}
}

func TestSlashReviewBadWindow(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.enterPrompt(screenDashboard)
	a.promptInput.SetValue("/review last-tuesday")
	a.updatePrompt(keyMsg("enter"))
	if a.screen != screenPrompt || a.promptErr == "" {
		t.Fatalf("bad window should stay on prompt with an error")
	}
}

func TestSlashDriftIdle(t *testing.T) {
	d := dbtest.NewTemp(t)
	a := NewApp(d)
	a.enterPrompt(screenDashboard)
	a.promptInput.SetValue("/drift")
	a.updatePrompt(keyMsg("enter"))
	if a.screen != screenResult {
		t.Fatalf("/drift should open screenResult even when idle, got %v", a.screen)
	}
}
