package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"pomo/internal/ipc"
)

type daemonEventMsg struct{ e ipc.Event }

type nudgeOverlay struct {
	text    string
	level   int
	actions []string
}

func (a *App) connectDaemon() {
	if a.daemonEvents == nil {
		a.daemonEvents = make(chan ipc.Event, 16)
	}
	if a.ipcClient != nil {
		return
	}
	c, err := ipc.Dial(ipc.SocketPath(), func(e ipc.Event) {
		select {
		case a.daemonEvents <- e:
		default:
		}
	})
	if err != nil {
		a.daemonUp = false
		return
	}
	a.ipcClient = c
	a.daemonUp = true
}

func (a *App) waitDaemonEvent() tea.Cmd {
	return func() tea.Msg {
		e, ok := <-a.daemonEvents
		if !ok {
			return nil
		}
		return daemonEventMsg{e}
	}
}

func (a *App) sendDaemon(e ipc.Event) {
	if a.ipcClient != nil {
		_ = a.ipcClient.Send(e)
	}
}

func (a *App) closeDaemon() {
	if a.ipcClient != nil {
		_ = a.ipcClient.Close()
		a.ipcClient = nil
	}
}

// handleDaemonEvent applies an inbound event and returns the cmd that keeps
// the event stream alive.
func (a *App) handleDaemonEvent(e ipc.Event) tea.Cmd {
	switch e.Type {
	case "watching", "idle":
		a.daemonUp = true
	case "checkpoint":
		a.checkpointActive = true
	case "nudge":
		a.nudgeOverlay = &nudgeOverlay{text: e.Text, level: e.Level, actions: e.Actions}
	}
	return a.waitDaemonEvent()
}

func (a *App) daemonIndicator() string {
	if a.daemonUp {
		return styleMuted.Render("● daemon up")
	}
	return styleDim.Render("○ daemon off")
}
