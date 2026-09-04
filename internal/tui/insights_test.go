package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"pomo/internal/ai"
	"pomo/internal/db/dbtest"
	"pomo/internal/model"
)

type fakeRecapper struct{ text string }

func (f fakeRecapper) Recap(context.Context, ai.RecapContext) (string, error) { return f.text, nil }

func TestInsightsAppendsRecap(t *testing.T) {
	d := dbtest.NewTemp(t)
	id, _ := d.CreateSession(model.Session{
		TaskName: "t", PlannedDuration: 1500, Status: model.StatusRunning, StartedAt: time.Now(),
	})
	_ = d.FinishSession(id, model.StatusCompleted, 1500, "")

	orig := newRecapper
	newRecapper = func(ai.Config) ai.Recapper { return fakeRecapper{text: "you did fine.\n→ Try: mornings"} }
	defer func() { newRecapper = orig }()

	a := NewApp(d)
	a.cfg.AI.Provider = "anthropic"
	a.enterPrompt(screenDashboard)
	a.promptInput.SetValue("/insights today")
	m, cmd := a.updatePrompt(keyMsg("enter"))
	a = m.(*App)
	if cmd == nil {
		t.Fatal("expected an async recap command")
	}
	msg := cmd()
	m, _ = a.Update(msg)
	a = m.(*App)
	if !strings.Contains(a.result.View(), "→ Try: mornings") {
		t.Fatalf("recap not shown:\n%s", a.result.View())
	}
}

func TestInsightsNoProvider(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.cfg.AI.Provider = ""
	a.enterPrompt(screenDashboard)
	a.promptInput.SetValue("/insights today")
	a.updatePrompt(keyMsg("enter"))
	if !strings.Contains(a.result.View(), "ai.provider") {
		t.Fatalf("expected a 'set ai.provider' hint:\n%s", a.result.View())
	}
}
