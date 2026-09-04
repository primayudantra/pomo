package tui

import (
	"fmt"
	"os"
	"os/user"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"pomo/internal/db"
	"pomo/internal/model"
)

// currentUsername resolves the local machine's username for the dashboard
// greeting, falling back through env vars if os/user can't resolve one
// (e.g. no cgo name service support in some minimal environments).
func currentUsername() string {
	if u, err := user.Current(); err == nil && u.Username != "" {
		if i := strings.LastIndexAny(u.Username, `\/`); i >= 0 {
			return u.Username[i+1:]
		}
		return u.Username
	}
	if v := os.Getenv("USER"); v != "" {
		return v
	}
	if v := os.Getenv("USERNAME"); v != "" {
		return v
	}
	return "there"
}

const dashWidth = 46
const dashInner = dashWidth - 4 // minus "│ " and " │"

var vbar = styleBorder.Render("│")

type DashboardModel struct {
	DB *db.DB

	today     []model.Session
	current   *model.Session
	completed int
	focusSecs int
	pomodoros int
}

func NewDashboard(d *db.DB) DashboardModel {
	m := DashboardModel{DB: d}
	m.refresh()
	return m
}

func (m *DashboardModel) refresh() {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	sessions, _ := m.DB.ListSessions(db.SessionFilter{From: &start})
	m.today = sessions

	m.focusSecs = 0
	m.pomodoros = 0
	m.completed = 0
	m.current = nil
	for i := range sessions {
		s := sessions[i]
		if s.Status == model.StatusRunning {
			cp := s
			m.current = &cp
			continue
		}
		if s.PlannedDuration >= 60 {
			m.pomodoros++
		}
		m.focusSecs += s.ActualDuration
		if s.Status == model.StatusCompleted {
			m.completed++
		}
	}
}

type dashTickMsg time.Time

func dashTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return dashTickMsg(t) })
}

func (m DashboardModel) Init() tea.Cmd {
	return dashTick()
}

// Update only reacts to its own tick; screen navigation (q/enter) is
// handled by the parent App so a single Bubble Tea program stays in
// control of the whole session.
func (m DashboardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := msg.(dashTickMsg); ok {
		m.refresh()
		return m, dashTick()
	}
	return m, nil
}

func fmtDur(secs int) string {
	h := secs / 3600
	mnt := (secs % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, mnt)
	}
	return fmt.Sprintf("%dm", mnt)
}

// row pads content to the box's inner width and closes it with the right
// border, so every line — not just the header — stays boxed.
func row(content string) string {
	pad := dashInner - lipgloss.Width(content)
	if pad < 0 {
		pad = 0
	}
	return vbar + " " + content + strings.Repeat(" ", pad) + " " + vbar
}

func (m DashboardModel) View() string {
	title := lipgloss.NewStyle().Bold(true).Foreground(bright).Align(lipgloss.Center).Width(dashInner)
	subtitle := lipgloss.NewStyle().Foreground(muted).Align(lipgloss.Center).Width(dashInner)
	header := styleAccent.Bold(true)
	label := styleMuted.Width(22)
	value := styleBright.Bold(true)

	var b strings.Builder
	line := styleBorder.Render("├" + strings.Repeat("─", dashWidth-2) + "┤")
	top := styleBorder.Render("╭" + strings.Repeat("─", dashWidth-2) + "╮")
	bottom := styleBorder.Render("╰" + strings.Repeat("─", dashWidth-2) + "╯")

	b.WriteString(top + "\n")
	name := currentUsername()
	if len(name) > 20 {
		name = name[:19] + "…"
	}
	b.WriteString(row(title.Render(fmt.Sprintf("POMO - Welcome (%s)", name))) + "\n")
	b.WriteString(row(subtitle.Render(time.Now().Format("Jan 02, 2006"))) + "\n")
	b.WriteString(line + "\n")
	b.WriteString(row("") + "\n")
	b.WriteString(row(header.Render("◆ TODAY")) + "\n")
	b.WriteString(row("") + "\n")
	b.WriteString(row(fmt.Sprintf("%s%s", label.Render("Focus Time"), value.Render(fmtDur(m.focusSecs)))) + "\n")
	b.WriteString(row(fmt.Sprintf("%s%s", label.Render("Pomodoros"), value.Render(fmt.Sprint(m.pomodoros)))) + "\n")
	b.WriteString(row(fmt.Sprintf("%s%s", label.Render("Completed Tasks"), value.Render(fmt.Sprint(m.completed)))) + "\n")
	b.WriteString(row("") + "\n")
	b.WriteString(line + "\n")
	b.WriteString(row("") + "\n")
	b.WriteString(row(header.Render("◆ CURRENT")) + "\n")
	b.WriteString(row("") + "\n")
	if m.current != nil {
		elapsed := int(time.Since(m.current.StartedAt).Seconds())
		remain := m.current.PlannedDuration - elapsed
		if remain < 0 {
			remain = 0
		}
		pct := float64(elapsed) / float64(m.current.PlannedDuration)
		b.WriteString(row(fmt.Sprintf("🍅 %s", styleBright.Render(m.current.TaskName))) + "\n")
		b.WriteString(row("") + "\n")
		b.WriteString(row(value.Render(fmt.Sprintf("%02d:%02d", remain/60, remain%60))) + "\n")
		b.WriteString(row(gradientBar(pct, 22)) + "\n")
	} else {
		b.WriteString(row(styleMuted.Render("No active session.")) + "\n")
		b.WriteString(row("") + "\n")
		b.WriteString(row(styleMuted.Render("[Enter] Start Pomodoro")) + "\n")
	}
	b.WriteString(row("") + "\n")
	b.WriteString(bottom + "\n")
	b.WriteString(dimHelp("[Enter] Start Pomodoro   [a] Analytics   [s] Settings   [q] Quit (confirm)"))
	return b.String()
}
