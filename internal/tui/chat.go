package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

// runChatCommand opens /chat. Filled in Task 5.
func (a *App) runChatCommand(args []string) (tea.Model, tea.Cmd) {
	a.promptErr = "/chat not wired yet"
	return a, nil
}

func (a *App) updateChat(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
		if a.chatCancel != nil {
			a.chatCancel()
		}
		a.screen = a.promptOrigin
		return a, nil
	}
	var cmd tea.Cmd
	a.chat, cmd = a.chat.Update(msg)
	return a, cmd
}

func (a *App) viewChat() string {
	return "\n" + a.chat.View()
}
