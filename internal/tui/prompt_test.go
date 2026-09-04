package tui

import (
	"testing"

	"pomo/internal/db/dbtest"
)

func TestEnterAndLeavePrompt(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.screen = screenDashboard
	a.enterPrompt(screenDashboard)
	if a.screen != screenPrompt {
		t.Fatalf("screen = %v, want screenPrompt", a.screen)
	}
	a.updatePrompt(keyMsg("esc"))
	if a.screen != screenDashboard {
		t.Fatalf("esc should return to dashboard, got %v", a.screen)
	}
}

func TestPromptUnknownCommand(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.enterPrompt(screenDashboard)
	a.promptInput.SetValue("/bogus")
	a.updatePrompt(keyMsg("enter"))
	if a.promptErr == "" || a.screen != screenPrompt {
		t.Fatalf("unknown command should stay on prompt with an error; err=%q screen=%v", a.promptErr, a.screen)
	}
}

func TestPromptOpensSettings(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.enterPrompt(screenDashboard)
	a.promptInput.SetValue("/settings")
	a.updatePrompt(keyMsg("enter"))
	if a.screen != screenSettings {
		t.Fatalf("/settings should open settings, got %v", a.screen)
	}
}
