package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"pomo/internal/ai"
	"pomo/internal/report"
)

// newRecapper is swappable in tests.
var newRecapper = func(c ai.Config) ai.Recapper {
	_, r, _ := ai.New(c)
	return r
}

type recapMsg struct {
	text string
	err  error
}

func (a *App) aiConfig() ai.Config {
	return ai.Config{Provider: a.cfg.AI.Provider, Key: a.cfg.AI.Key, Model: a.cfg.AI.Model}
}

func recapContextFrom(sum report.Summary) ai.RecapContext {
	rc := ai.RecapContext{
		Label:          sum.Window.Label,
		FocusMinutes:   sum.FocusSeconds / 60,
		PlannedMinutes: sum.PlannedSeconds / 60,
		Completed:      sum.Completed,
		Planned:        sum.Planned,
		DriftMinutes:   sum.DriftSeconds / 60,
	}
	for _, ad := range sum.DriftByApp {
		rc.TopDrift = append(rc.TopDrift, fmt.Sprintf("%s %s", ad.App, report.FmtDur(ad.Seconds)))
		if len(rc.TopDrift) == 3 {
			break
		}
	}
	for _, ts := range sum.ByTag {
		rc.ByTag = append(rc.ByTag, fmt.Sprintf("%s %s", ts.Tag, report.FmtDur(ts.FocusSeconds)))
	}
	if sum.BestHour != nil {
		rc.BestHour = fmt.Sprintf("%02d:00", sum.BestHour.Hour)
	}
	return rc
}

func (a *App) startRecap(sum report.Summary) tea.Cmd {
	rc := recapContextFrom(sum)
	cfg := a.aiConfig()
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		text, err := newRecapper(cfg).Recap(ctx, rc)
		return recapMsg{text: text, err: err}
	}
}

// applyRecap swaps the "…thinking" line for the recap text (or an error).
func (a *App) applyRecap(m recapMsg) {
	if a.screen != screenResult {
		return
	}
	line := "RECAP\n\n" + m.text + "\n"
	if m.err != nil {
		line = fmt.Sprintf("RECAP  (unavailable: %v)\n", m.err)
	}
	a.result.SetContent(a.recapBody + "\n" + line)
	a.result.GotoTop()
}
