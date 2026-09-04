package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"pomo/internal/ai"
	"pomo/internal/report"
)

// newChatter is swappable in tests.
var newChatter = func(c ai.Config) ai.Chatter {
	_, _, ch := ai.New(c)
	return ch
}

const chatHistoryCap = 12

type chatDelta struct {
	text string
	err  error
	done bool
}

type chatDeltaMsg struct {
	text    string
	err     error
	done    bool
	chanRef chan chatDelta
}

func (a *App) runChatCommand(args []string) (tea.Model, tea.Cmd) {
	a.chatErr = ""
	a.chatStreaming = false
	a.screen = screenChat
	if a.cfg.AI.Provider == "" {
		a.chatHistory = nil
		a.chat.SetContent("set  ai.provider  and  ai.key  in /settings to enable chat")
		return a, nil
	}
	a.chatHistory = nil
	a.chatInput.SetValue("")
	a.chatInput.Focus()
	a.renderChat()
	if len(args) > 0 {
		a.chatInput.SetValue(strings.Join(args, " "))
		return a.submitChat()
	}
	return a, nil
}

func (a *App) submitChat() (tea.Model, tea.Cmd) {
	clean, err := guardChatInput(a.chatInput.Value())
	if err != nil {
		a.chatErr = err.Error()
		return a, nil
	}
	a.chatErr = ""
	a.chatInput.SetValue("")
	a.chatHistory = append(a.chatHistory, ai.Msg{Role: "user", Content: clean})
	a.chatHistory = append(a.chatHistory, ai.Msg{Role: "assistant", Content: ""})
	a.chatStreaming = true
	a.renderChat()
	return a, a.startChatStream()
}

func capHistory(h []ai.Msg) []ai.Msg {
	if len(h) <= chatHistoryCap {
		return h
	}
	return h[len(h)-chatHistoryCap:]
}

func (a *App) chatPreamble() ai.Msg {
	var b strings.Builder
	b.WriteString("[context — do not quote back verbatim]\n")
	if s, err := a.db.LastRunningSession(); err == nil && s != nil {
		fmt.Fprintf(&b, "running session: %s (planned %dm)\n", s.TaskName, s.PlannedDuration/60)
	}
	w := report.Today()
	if sum, err := report.Build(a.db, w); err == nil {
		fmt.Fprintf(&b, "today: %s focus / %s planned, %s drift\n",
			report.FmtDur(sum.FocusSeconds), report.FmtDur(sum.PlannedSeconds), report.FmtDur(sum.DriftSeconds))
		for _, ad := range sum.DriftByApp {
			fmt.Fprintf(&b, "  drift: %s %s\n", ad.App, report.FmtDur(ad.Seconds))
		}
	}
	if tasks, err := a.db.ListOpenTasks(); err == nil && len(tasks) > 0 {
		names := make([]string, 0, len(tasks))
		for _, t := range tasks {
			names = append(names, t.Name)
		}
		fmt.Fprintf(&b, "open tasks: %s\n", strings.Join(names, ", "))
	}
	return ai.Msg{Role: "user", Content: b.String()}
}

func (a *App) startChatStream() tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	a.chatCancel = cancel

	history := append([]ai.Msg{a.chatPreamble()}, capHistory(a.chatHistory)...)
	// drop the trailing empty assistant placeholder before sending
	if n := len(history); n > 0 && history[n-1].Role == "assistant" && history[n-1].Content == "" {
		history = history[:n-1]
	}
	chatter := newChatter(a.aiConfig())
	ch := make(chan chatDelta, 32)

	go func() {
		defer close(ch)
		err := chatter.Stream(ctx, ai.SystemChat, history, func(d string) {
			ch <- chatDelta{text: d}
		})
		if err != nil {
			ch <- chatDelta{err: err}
			return
		}
		ch <- chatDelta{done: true}
	}()

	return waitChatDelta(ch)
}

func waitChatDelta(ch chan chatDelta) tea.Cmd {
	return func() tea.Msg {
		d, ok := <-ch
		if !ok {
			return chatDeltaMsg{done: true}
		}
		return chatDeltaMsg{text: d.text, err: d.err, done: d.done, chanRef: ch}
	}
}

func (a *App) handleChatDelta(m chatDeltaMsg) tea.Cmd {
	switch {
	case m.err != nil:
		a.appendAssistant(fmt.Sprintf("\n[error: %v]", m.err))
		a.chatStreaming = false
		a.chatCancel = nil
		a.renderChat()
		return nil
	case m.done:
		a.chatStreaming = false
		a.chatCancel = nil
		a.renderChat()
		return nil
	default:
		a.appendAssistant(m.text)
		a.renderChat()
		if m.chanRef != nil {
			return waitChatDelta(m.chanRef)
		}
		return nil
	}
}

func (a *App) appendAssistant(s string) {
	if n := len(a.chatHistory); n > 0 && a.chatHistory[n-1].Role == "assistant" {
		a.chatHistory[n-1].Content += s
	}
}

func (a *App) renderChat() {
	var b strings.Builder
	for _, m := range a.chatHistory {
		who := "you"
		if m.Role == "assistant" {
			who = "pomo"
		}
		b.WriteString(styleAccent.Render(who) + "  " + m.Content + "\n\n")
	}
	a.chat.SetContent(b.String())
	a.chat.GotoBottom()
}

func (a *App) updateChat(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			if a.chatCancel != nil {
				a.chatCancel()
				a.chatCancel = nil
			}
			a.screen = a.promptOrigin
			return a, nil
		case "enter":
			if a.cfg.AI.Provider == "" || a.chatStreaming {
				return a, nil
			}
			return a.submitChat()
		}
	}
	var cmd tea.Cmd
	a.chatInput, cmd = a.chatInput.Update(msg)
	return a, cmd
}

func (a *App) viewChat() string {
	if a.cfg.AI.Provider == "" {
		return "\n" + a.chat.View() + "\n" + dimHelp("esc back")
	}
	foot := "enter send · esc back"
	if a.chatStreaming {
		foot = "…streaming · esc cancel"
	}
	b := "\n" + a.chat.View() + "\n\n" + a.chatInput.View()
	if a.chatErr != "" {
		b += "\n" + styleErr.Render(a.chatErr)
	}
	return b + "\n" + dimHelp(foot)
}
