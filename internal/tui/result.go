package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"pomo/internal/model"
	"pomo/internal/report"
)

func argOrEmpty(args []string) string {
	if len(args) > 0 {
		return strings.Join(args, " ")
	}
	return ""
}

// runReportCommand handles /review, /recap, /insights, /drift.
func (a *App) runReportCommand(c slashCommand, args []string) (tea.Model, tea.Cmd) {
	a.resultCmd = c
	switch c.Name {
	case "/drift":
		return a.showDrift()
	default: // /review, /recap, /insights
		win, err := report.ParseWindow(argOrEmpty(args))
		if err != nil {
			a.promptErr = err.Error()
			return a, nil
		}
		sum, err := report.Build(a.db, win)
		if err != nil {
			a.promptErr = err.Error()
			return a, nil
		}
		body := report.RenderText(sum)
		a.resultTitle = c.Name + " " + win.Label
		a.result.SetContent(body)
		a.result.GotoTop()
		a.screen = screenResult
		if c.Name == "/insights" {
			return a, a.startRecap(sum)
		}
		return a, nil
	}
}

func (a *App) showDrift() (tea.Model, tea.Cmd) {
	var events []model.DriftEvent
	var title string
	if s, err := a.db.LastRunningSession(); err == nil && s != nil {
		events, _ = a.db.DriftEventsForSession(s.ID)
		title = "/drift · this session"
	} else {
		w := report.Today()
		events, _ = a.db.ListDriftEvents(w.From, w.To)
		title = "/drift · today"
	}
	a.resultTitle = title
	a.result.SetContent(renderDriftView(events))
	a.result.GotoTop()
	a.screen = screenResult
	return a, nil
}

func renderDriftView(events []model.DriftEvent) string {
	if len(events) == 0 {
		return "no drift recorded."
	}
	var b strings.Builder
	total := 0
	for _, e := range events {
		total += e.Seconds
		app := e.Detail
		if app == "" {
			app = "idle"
		}
		fmt.Fprintf(&b, "%s  %-6s  %-24s  %s\n",
			e.StartedAt.Format("15:04"), report.FmtDur(e.Seconds), app, e.Trigger)
	}
	fmt.Fprintf(&b, "\ntotal drift: %s across %d episodes\n", report.FmtDur(total), len(events))
	return b.String()
}

func (a *App) updateResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			a.screen = a.promptOrigin
			return a, nil
		case "r":
			if a.resultCmd.Name != "" && a.resultCmd.Name != "/help" {
				return a.runReportCommand(a.resultCmd, nil)
			}
		}
	}
	var cmd tea.Cmd
	a.result, cmd = a.result.Update(msg)
	return a, cmd
}

func (a *App) viewResult() string {
	return "\n" + styleBright.Bold(true).Render(a.resultTitle) + "\n\n" + a.result.View() +
		"\n" + dimHelp("↑/↓ scroll · r refresh · esc back")
}
