package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type TimerResult struct {
	Status        string // completed, skipped, cancelled
	ActualSeconds int
}

type tickMsg time.Time

type TimerModel struct {
	Task       string
	Total      int // seconds
	Remain     int
	Paused     bool
	Quit       bool
	Result     TimerResult
	StartedAt  time.Time
	pausedFor  time.Duration
	pauseStart time.Time

	// OnBreak selects which ASCII figure View renders — typing at a
	// laptop, or leaning back with coffee. Tick drives that figure's
	// animation and advances every second regardless of Paused, so the
	// blink/steam keeps moving even while the countdown is frozen.
	OnBreak bool
	Tick    int

	// Embedded is true when this model runs as a sub-screen inside a
	// larger Bubble Tea program (see App). In that mode it must not
	// return tea.Quit itself — that would tear down the whole program —
	// it just flips Quit and lets the parent decide what happens next.
	Embedded bool
}

func NewTimer(task string, seconds int) TimerModel {
	return TimerModel{
		Task:      task,
		Total:     seconds,
		Remain:    seconds,
		StartedAt: time.Now(),
	}
}

func (m TimerModel) Init() tea.Cmd {
	return tick()
}

func tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m TimerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "p":
			if !m.Paused {
				m.Paused = true
				m.pauseStart = time.Now()
			}
		case "r":
			if m.Paused {
				m.pausedFor += time.Since(m.pauseStart)
				m.Paused = false
			}
		case "s":
			m.Quit = true
			m.Result = TimerResult{Status: "skipped", ActualSeconds: m.Total - m.Remain}
			return m, m.finishCmd()
		case "q", "ctrl+c":
			m.Quit = true
			m.Result = TimerResult{Status: "cancelled", ActualSeconds: m.Total - m.Remain}
			return m, m.finishCmd()
		}
		return m, nil
	case tickMsg:
		m.Tick++
		if m.Paused {
			return m, tick()
		}
		m.Remain--
		if m.Remain <= 0 {
			m.Quit = true
			m.Result = TimerResult{Status: "completed", ActualSeconds: m.Total}
			return m, m.finishCmd()
		}
		return m, tick()
	}
	return m, nil
}

func (m TimerModel) finishCmd() tea.Cmd {
	if m.Embedded {
		return nil
	}
	return tea.Quit
}

func (m TimerModel) View() string {
	if m.Quit && !m.Embedded {
		return ""
	}
	mins := m.Remain / 60
	secs := m.Remain % 60
	timeStr := fmt.Sprintf("%02d:%02d", mins, secs)

	pct := 1 - float64(m.Remain)/float64(m.Total)

	status := ""
	if m.Paused {
		status = styleMuted.Render(" (paused)")
	}

	art := focusFrame(m.Tick)
	if m.OnBreak {
		art = breakFrame(m.Tick)
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(styleBright.Render(art))
	b.WriteString("\n\n")
	b.WriteString(styleAccent.Bold(true).Render(fmt.Sprintf("🍅 %s%s", timeStr, status)))
	b.WriteString("\n\n")
	b.WriteString(styleMuted.Render(m.Task))
	b.WriteString("\n\n")
	b.WriteString(gradientBar(pct, 30))
	b.WriteString("\n\n")
	b.WriteString(dimHelp("[p] pause  [r] resume  [s] skip  [m] mute  [q] quit"))
	b.WriteString("\n")
	return b.String()
}
