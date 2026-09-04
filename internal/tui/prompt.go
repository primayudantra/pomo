package tui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"pomo/internal/model"
)

func (a *App) enterPrompt(origin screen) (tea.Model, tea.Cmd) {
	a.promptOrigin = origin
	a.promptErr = ""
	a.promptInput.SetValue("/")
	a.promptInput.CursorEnd()
	a.promptInput.Focus()
	a.screen = screenPrompt
	return a, textinput.Blink
}

func (a *App) updatePrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			a.screen = a.promptOrigin
			return a, nil
		case "tab":
			if ms := filterCommands(a.promptQuery()); len(ms) > 0 {
				a.promptInput.SetValue(ms[0].Name + " ")
				a.promptInput.CursorEnd()
			}
			return a, nil
		case "enter":
			return a.dispatchPrompt()
		}
	}
	var cmd tea.Cmd
	a.promptInput, cmd = a.promptInput.Update(msg)
	a.promptErr = ""
	return a, cmd
}

func (a *App) promptQuery() string {
	v := strings.TrimPrefix(strings.TrimSpace(a.promptInput.Value()), "/")
	if i := strings.IndexByte(v, ' '); i >= 0 {
		v = v[:i]
	}
	return v
}

func (a *App) dispatchPrompt() (tea.Model, tea.Cmd) {
	fields := strings.Fields(a.promptInput.Value())
	if len(fields) == 0 {
		a.screen = a.promptOrigin
		return a, nil
	}
	c, ok := resolveCommand(fields[0])
	if !ok {
		a.promptErr = "unknown command: " + fields[0] + " — try /help"
		return a, nil
	}
	if c.NeedsSession && !a.hasRunningSession() {
		a.promptErr = fields[0] + " needs a running session"
		return a, nil
	}
	return a.runSlashCommand(c, fields[1:])
}

func (a *App) hasRunningSession() bool {
	s, err := a.db.LastRunningSession()
	return err == nil && s != nil
}

func (a *App) skipRunningSession() (tea.Model, tea.Cmd) {
	s, err := a.db.LastRunningSession()
	if err != nil || s == nil {
		a.screen = a.promptOrigin
		return a, nil
	}
	actual := int(time.Since(s.StartedAt).Seconds())
	_ = a.db.FinishSession(s.ID, model.StatusSkipped, actual, "")
	a.dash.refresh()
	a.flash = "⏭ session skipped."
	a.flashColor = muted
	a.screen = screenDashboard
	return a, nil
}

func (a *App) runSlashCommand(c slashCommand, args []string) (tea.Model, tea.Cmd) {
	switch c.Name {
	case "/settings":
		a.settingsCursor = 0
		a.screen = screenSettings
		return a, nil
	case "/quit":
		a.quitInput.SetValue("")
		a.quitInput.Focus()
		a.quitErr = false
		a.screen = screenQuitConfirm
		return a, textinput.Blink
	case "/start":
		a.loadTaskList()
		a.screen = screenTaskSelect
		return a, nil
	case "/skip":
		return a.skipRunningSession()
	case "/help":
		a.resultTitle = "commands"
		a.resultCmd = c
		a.result.SetContent(renderHelp())
		a.result.GotoTop()
		a.screen = screenResult
		return a, nil
	case "/review", "/insights", "/drift":
		return a.runReportCommand(c, args)
	case "/chat":
		return a.runChatCommand(args)
	default:
		a.promptErr = c.Name + " not wired yet"
		return a, nil
	}
}

func renderHelp() string {
	var b strings.Builder
	for _, c := range slashCommands {
		name := c.Name
		if len(c.Aliases) > 0 {
			name += " (" + strings.Join(c.Aliases, ", ") + ")"
		}
		b.WriteString(name)
		b.WriteString("\n    ")
		b.WriteString(c.Help)
		b.WriteString("\n")
	}
	return b.String()
}

func (a *App) viewPrompt() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(a.promptInput.View())
	b.WriteString("\n\n")
	for _, c := range filterCommands(a.promptQuery()) {
		if c.NeedsSession && !a.hasRunningSession() {
			b.WriteString(styleDim.Render("  "+c.Name+"  "+c.Help+"  (needs a session)") + "\n")
			continue
		}
		b.WriteString("  " + c.Name + "  " + styleMuted.Render(c.Help) + "\n")
	}
	if a.promptErr != "" {
		b.WriteString("\n" + styleErr.Render(a.promptErr) + "\n")
	}
	b.WriteString("\n" + dimHelp("tab complete · enter run · esc back"))
	return b.String()
}
