package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// runReportCommand handles /review, /recap, /insights, /drift.
// Filled in Tasks 3 & 4.
func (a *App) runReportCommand(c slashCommand, args []string) (tea.Model, tea.Cmd) {
	a.promptErr = c.Name + " not wired yet"
	return a, nil
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
