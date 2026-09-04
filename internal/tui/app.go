package tui

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"pomo/internal/ai"
	"pomo/internal/db"
	"pomo/internal/gitinfo"
	"pomo/internal/ipc"
	"pomo/internal/model"
	"pomo/internal/pomoconfig"
	"pomo/internal/report"
	"pomo/internal/sound"
)

type screen int

const (
	screenDashboard screen = iota
	screenTaskSelect
	screenNewTask
	screenDuration
	screenTimer
	screenNote
	screenSettings
	screenStats
	screenQuitConfirm
	screenPrompt
	screenResult
	screenChat
)

const (
	statsPeriodDay = iota
	statsPeriodWeek
	statsPeriodMonth
	statsPeriodCount
)

// App is the single, persistent Bubble Tea program that backs the
// interactive dashboard. Every screen transition happens inside this one
// program so actions like skip/cancel/complete never drop the user back to
// the shell — only an explicit quit from the dashboard does.
type App struct {
	db  *db.DB
	cfg pomoconfig.Config

	screen     screen
	quit       bool
	err        error
	flash      string
	flashColor lipgloss.Color

	width  int
	height int

	dash DashboardModel

	list          list.Model
	newTaskInput  textinput.Model
	durationInput textinput.Model
	noteInput     textinput.Model

	timer          TimerModel
	onBreak        bool
	muted          bool
	pendingTaskID  int64
	pendingTask    string
	pendingTag     string
	pendingSession int64

	settingsCursor int
	statsPeriod    int

	quitInput textinput.Model
	quitErr   bool

	startCwd string

	promptInput  textinput.Model
	promptOrigin screen
	promptErr    string

	result      viewport.Model
	resultTitle string
	resultCmd   slashCommand
	recapBody   string

	chat          viewport.Model
	chatInput     textinput.Model
	chatHistory   []ai.Msg
	chatStreaming bool
	chatErr       string
	chatCancel    func()

	daemonUp         bool
	ipcClient        *ipc.Client
	daemonEvents     chan ipc.Event
	nudgeOverlay     *nudgeOverlay
	checkpointActive bool
}

const (
	settingRowSound = iota
	settingRowSoundChoice
	settingRowDriftEnabled
	settingRowCheckpoint
	settingRowNudges
	settingRowAIProvider
	settingRowCount
)

type taskItem struct {
	id   int64
	name string
}

func (t taskItem) Title() string       { return t.name }
func (t taskItem) Description() string { return "" }
func (t taskItem) FilterValue() string { return t.name }

func NewApp(d *db.DB) *App {
	cfg := pomoconfig.Load(d)

	delegate := list.NewDefaultDelegate()
	delegate.Styles.SelectedTitle = delegate.Styles.SelectedTitle.
		Foreground(bright).Background(rowBg).BorderForeground(accent).Bold(true)
	delegate.Styles.SelectedDesc = delegate.Styles.SelectedDesc.Background(rowBg).BorderForeground(accent)
	delegate.Styles.NormalTitle = delegate.Styles.NormalTitle.Foreground(muted)
	delegate.ShowDescription = false
	delegate.SetHeight(1)
	delegate.SetSpacing(0)

	l := list.New(nil, delegate, 44, 12)
	l.Title = "What are you working on?"
	l.Styles.Title = lipgloss.NewStyle().Bold(true).Foreground(bright)
	l.SetShowStatusBar(false)
	l.SetShowHelp(false)
	l.SetFilteringEnabled(false)

	ni := textinput.New()
	ni.Placeholder = "Task name"
	ni.CharLimit = 120
	ni.Width = 40
	ni.PromptStyle = styleAccent
	ni.TextStyle = styleBright

	di := textinput.New()
	di.Placeholder = strconv.Itoa(int(cfg.Focus.Minutes()))
	di.CharLimit = 3
	di.Width = 10
	di.PromptStyle = styleAccent
	di.TextStyle = styleBright

	note := textinput.New()
	note.Placeholder = "optional note..."
	note.CharLimit = 300
	note.Width = 40
	note.PromptStyle = styleAccent
	note.TextStyle = styleBright

	qi := textinput.New()
	qi.Placeholder = "confirm"
	qi.CharLimit = 20
	qi.Width = 20
	qi.PromptStyle = styleAccent
	qi.TextStyle = styleBright

	pi := textinput.New()
	pi.CharLimit = 120
	pi.Width = 40
	pi.PromptStyle = styleAccent
	pi.TextStyle = styleBright

	ci := textinput.New()
	ci.Placeholder = "type a message..."
	ci.CharLimit = maxChatInput
	ci.Width = 60
	ci.PromptStyle = styleAccent
	ci.TextStyle = styleBright

	cwd, _ := os.Getwd()

	a := &App{
		db:            d,
		cfg:           cfg,
		screen:        screenDashboard,
		dash:          NewDashboard(d),
		list:          l,
		newTaskInput:  ni,
		durationInput: di,
		noteInput:     note,
		quitInput:     qi,
		startCwd:      cwd,
		promptInput:   pi,
		chatInput:     ci,
		result:        viewport.New(80, 20),
		chat:          viewport.New(80, 20),
		daemonEvents:  make(chan ipc.Event, 16),
	}
	return a
}

// RunApp starts the persistent interactive dashboard.
func RunApp(d *db.DB) error {
	a := NewApp(d)
	a.connectDaemon()
	p := tea.NewProgram(a, tea.WithAltScreen())
	_, err := p.Run()
	a.closeDaemon()
	return err
}

// RunAppTaskSelect starts the same persistent app but jumps straight to the
// arrow-key task picker, for `pomo start` invoked with no task argument.
func RunAppTaskSelect(d *db.DB) error {
	a := NewApp(d)
	a.connectDaemon()
	a.loadTaskList()
	a.screen = screenTaskSelect
	p := tea.NewProgram(a, tea.WithAltScreen())
	_, err := p.Run()
	a.closeDaemon()
	return err
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(dashTick(), a.waitDaemonEvent())
}

func (a *App) loadTaskList() {
	tasks, _ := a.db.ListOpenTasks()
	items := make([]list.Item, 0, len(tasks)+1)
	for _, t := range tasks {
		items = append(items, taskItem{id: t.ID, name: t.Name})
	}
	items = append(items, taskItem{id: 0, name: "+ New task"})
	a.list.SetItems(items)
	a.list.Select(0)
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyMsg); ok && kmsg.String() == "ctrl+c" {
		a.quit = true
		sound.Stop()
		return a, tea.Quit
	}
	if wmsg, ok := msg.(tea.WindowSizeMsg); ok {
		a.width, a.height = wmsg.Width, wmsg.Height
		a.result.Width, a.result.Height = wmsg.Width, max(4, wmsg.Height-6)
		a.chat.Width, a.chat.Height = wmsg.Width, max(4, wmsg.Height-6)
	}

	if rm, ok := msg.(recapMsg); ok {
		a.applyRecap(rm)
		return a, nil
	}

	if cd, ok := msg.(chatDeltaMsg); ok {
		return a, a.handleChatDelta(cd)
	}

	if dm, ok := msg.(daemonEventMsg); ok {
		return a, a.handleDaemonEvent(dm.e)
	}

	switch a.screen {
	case screenDashboard:
		return a.updateDashboard(msg)
	case screenTaskSelect:
		return a.updateTaskSelect(msg)
	case screenNewTask:
		return a.updateNewTask(msg)
	case screenDuration:
		return a.updateDuration(msg)
	case screenTimer:
		return a.updateTimer(msg)
	case screenNote:
		return a.updateNote(msg)
	case screenSettings:
		return a.updateSettings(msg)
	case screenStats:
		return a.updateStats(msg)
	case screenQuitConfirm:
		return a.updateQuitConfirm(msg)
	case screenPrompt:
		return a.updatePrompt(msg)
	case screenResult:
		return a.updateResult(msg)
	case screenChat:
		return a.updateChat(msg)
	}
	return a, nil
}

func (a *App) updateQuitConfirm(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			a.screen = screenDashboard
			return a, nil
		case "enter":
			if strings.EqualFold(strings.TrimSpace(a.quitInput.Value()), "confirm") {
				a.quit = true
				sound.Stop()
				return a, tea.Quit
			}
			a.quitErr = true
			a.quitInput.SetValue("")
			return a, nil
		}
	}
	var cmd tea.Cmd
	a.quitInput, cmd = a.quitInput.Update(msg)
	return a, cmd
}

func (a *App) updateDashboard(msg tea.Msg) (tea.Model, tea.Cmd) {
	m, cmd := a.dash.Update(msg)
	a.dash = m.(DashboardModel)

	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "q":
			a.quitInput.SetValue("")
			a.quitInput.Focus()
			a.quitErr = false
			a.screen = screenQuitConfirm
			return a, textinput.Blink
		case "enter":
			a.flash = ""
			a.loadTaskList()
			a.screen = screenTaskSelect
			return a, nil
		case "s":
			a.settingsCursor = 0
			a.screen = screenSettings
			return a, nil
		case "a":
			a.screen = screenStats
			return a, nil
		case "/":
			return a.enterPrompt(screenDashboard)
		}
	}
	return a, cmd
}

func (a *App) updateStats(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	switch km.String() {
	case "esc", "q":
		a.screen = screenDashboard
		return a, nil
	case "tab", "right", "l":
		a.statsPeriod = (a.statsPeriod + 1) % statsPeriodCount
	case "shift+tab", "left", "h":
		a.statsPeriod = (a.statsPeriod - 1 + statsPeriodCount) % statsPeriodCount
	}
	return a, nil
}

func (a *App) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return a, nil
	}
	switch km.String() {
	case "esc", "q":
		sound.Stop()
		a.screen = screenDashboard
		return a, nil
	case "up", "k":
		if a.settingsCursor > 0 {
			a.settingsCursor--
		}
	case "down", "j":
		if a.settingsCursor < settingRowCount-1 {
			a.settingsCursor++
		}
	case "enter", " ", "left", "right":
		switch a.settingsCursor {
		case settingRowSound:
			a.cfg.Sound = !a.cfg.Sound
			_ = a.db.SetConfig("sound", strconv.FormatBool(a.cfg.Sound))
		case settingRowSoundChoice:
			opts := sound.StartOptions()
			cur := 0
			for i, o := range opts {
				if o.ID == a.cfg.SoundChoice {
					cur = i
					break
				}
			}
			if km.String() == "left" {
				cur = (cur - 1 + len(opts)) % len(opts)
			} else {
				cur = (cur + 1) % len(opts)
			}
			a.cfg.SoundChoice = opts[cur].ID
			_ = a.db.SetConfig("sound_choice", string(a.cfg.SoundChoice))
			if a.cfg.Sound {
				sound.Play(a.cfg.SoundChoice)
			}
		case settingRowDriftEnabled:
			a.cfg.Drift.Enabled = !a.cfg.Drift.Enabled
			_ = a.db.SetConfig("drift.enabled", strconv.FormatBool(a.cfg.Drift.Enabled))
		case settingRowCheckpoint:
			a.cfg.CheckpointEnabled = !a.cfg.CheckpointEnabled
			_ = a.db.SetConfig("checkpoint.enabled", strconv.FormatBool(a.cfg.CheckpointEnabled))
		case settingRowNudges:
			a.cfg.Nudge.Enabled = !a.cfg.Nudge.Enabled
			_ = a.db.SetConfig("nudge.enabled", strconv.FormatBool(a.cfg.Nudge.Enabled))
		case settingRowAIProvider:
			next := map[string]string{"": "anthropic", "anthropic": "openrouter", "openrouter": ""}
			a.cfg.AI.Provider = next[a.cfg.AI.Provider]
			_ = a.db.SetConfig("ai.provider", a.cfg.AI.Provider)
		}
	}
	return a, nil
}

func (a *App) updateTaskSelect(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			a.screen = screenDashboard
			return a, nil
		case "enter":
			if it, ok := a.list.SelectedItem().(taskItem); ok {
				if it.id == 0 {
					a.newTaskInput.SetValue("")
					a.newTaskInput.Focus()
					a.screen = screenNewTask
					return a, textinput.Blink
				}
				return a.goToDuration(it.id, it.name, "")
			}
		case "d", "x":
			if it, ok := a.list.SelectedItem().(taskItem); ok && it.id != 0 {
				_ = a.db.DeleteTask(it.id)
				a.loadTaskList()
			}
			return a, nil
		}
	}
	var cmd tea.Cmd
	a.list, cmd = a.list.Update(msg)
	return a, cmd
}

func (a *App) updateNewTask(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			a.screen = screenTaskSelect
			return a, nil
		case "enter":
			name := strings.TrimSpace(a.newTaskInput.Value())
			if name == "" {
				return a, nil
			}
			taskID, err := a.db.FindOrCreateTask(name, "")
			if err != nil {
				a.err = err
				return a, nil
			}
			return a.goToDuration(taskID, name, "")
		}
	}
	var cmd tea.Cmd
	a.newTaskInput, cmd = a.newTaskInput.Update(msg)
	return a, cmd
}

// goToDuration remembers which task was picked and asks how long this
// Pomodoro should run (defaults to the configured focus length).
func (a *App) goToDuration(taskID int64, name, tag string) (tea.Model, tea.Cmd) {
	a.pendingTaskID = taskID
	a.pendingTask = name
	a.pendingTag = tag
	a.durationInput.SetValue("")
	a.durationInput.Focus()
	a.screen = screenDuration
	return a, textinput.Blink
}

func (a *App) updateDuration(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "esc":
			a.screen = screenTaskSelect
			return a, nil
		case "enter":
			minutes := int(a.cfg.Focus.Minutes())
			if v := strings.TrimSpace(a.durationInput.Value()); v != "" {
				if n, err := strconv.Atoi(v); err == nil && n > 0 {
					minutes = n
				}
			}
			return a.beginSession(a.pendingTaskID, a.pendingTask, a.pendingTag, minutes)
		}
	}
	var cmd tea.Cmd
	a.durationInput, cmd = a.durationInput.Update(msg)
	return a, cmd
}

func (a *App) beginSession(taskID int64, name, tag string, minutes int) (tea.Model, tea.Cmd) {
	duration := time.Duration(minutes) * time.Minute
	repoPath, repoBranch := gitinfo.Describe(a.startCwd)
	sessionID, err := a.db.CreateSession(model.Session{
		TaskID:          taskID,
		TaskName:        name,
		Tag:             tag,
		PlannedDuration: int(duration.Seconds()),
		Status:          model.StatusRunning,
		StartedAt:       time.Now(),
		RepoPath:        repoPath,
		RepoBranch:      repoBranch,
	})
	if err != nil {
		a.err = err
		a.screen = screenDashboard
		return a, nil
	}
	a.pendingTaskID = taskID
	a.pendingTask = name
	a.pendingTag = tag
	a.pendingSession = sessionID
	a.onBreak = false
	a.timer = NewTimer(name, int(duration.Seconds()))
	a.timer.Embedded = true
	a.timer.OnBreak = false
	a.screen = screenTimer
	return a, tea.Batch(a.timer.Init(), a.playLoopSoundCmd())
}

// startBreak runs a short (or, every SessionsBeforeLong pomodoros, long)
// break timer without touching session history — breaks aren't Pomodoros.
func (a *App) startBreak() (tea.Model, tea.Cmd) {
	length := a.cfg.ShortBreak
	label := "☕ Short Break"
	if a.cfg.SessionsBeforeLong > 0 && a.dash.completed > 0 && a.dash.completed%a.cfg.SessionsBeforeLong == 0 {
		length = a.cfg.LongBreak
		label = "🌴 Long Break"
	}
	a.onBreak = true
	a.timer = NewTimer(label, int(length.Seconds()))
	a.timer.Embedded = true
	a.timer.OnBreak = true
	a.screen = screenTimer
	return a, tea.Batch(a.timer.Init(), a.playLoopSoundCmd())
}

// playLoopSoundCmd starts the configured clip looping for the length of the
// running Pomodoro/break, unless sound is disabled in settings or muted for
// this run.
func (a *App) playLoopSoundCmd() tea.Cmd {
	if !a.cfg.Sound || a.muted {
		return nil
	}
	choice := a.cfg.SoundChoice
	return func() tea.Msg {
		sound.Loop(choice)
		return nil
	}
}

// playFinishSound rings the fixed completion chime, unless sound is
// disabled in settings or muted for this run.
func (a *App) playFinishSound() {
	if a.cfg.Sound && !a.muted {
		sound.PlayFinish()
	}
}

func (a *App) updateTimer(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		if a.checkpointActive {
			switch km.String() {
			case "y", "n":
				a.sendDaemon(ipc.Event{Type: "checkpoint-answer", Answer: km.String()})
				a.checkpointActive = false
				return a, nil
			}
		}
		if a.nudgeOverlay != nil {
			switch km.String() {
			case "d", "r", "s", "esc":
				action := km.String()
				if action == "s" || action == "esc" {
					action = "snooze"
				}
				a.sendDaemon(ipc.Event{Type: "nudge-action", Action: action})
				a.nudgeOverlay = nil
				return a, nil
			case "b":
				a.sendDaemon(ipc.Event{Type: "nudge-action", Action: "b"})
				a.nudgeOverlay = nil
				// fall through to the timer's own break/skip handling
			}
		}
	}
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "/" {
		return a.enterPrompt(screenTimer)
	}
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "m" {
		a.muted = !a.muted
		if a.muted {
			sound.Stop()
			return a, nil
		}
		return a, a.playLoopSoundCmd()
	}

	m, cmd := a.timer.Update(msg)
	a.timer = m.(TimerModel)

	if a.timer.Quit {
		if a.onBreak {
			a.onBreak = false
			switch a.timer.Result.Status {
			case "completed":
				a.playFinishSound()
				a.flash = "☕ Break's over. Back to it!"
				a.flashColor = accent2
			default:
				sound.Stop()
				a.flash = "Break ended."
				a.flashColor = muted
			}
			a.screen = screenDashboard
			return a, nil
		}

		status := model.SessionStatus(a.timer.Result.Status)
		_ = a.db.FinishSession(a.pendingSession, status, a.timer.Result.ActualSeconds, "")
		a.dash.refresh()

		switch status {
		case model.StatusCompleted:
			a.playFinishSound()
			a.flash = fmt.Sprintf("🍅 %q completed — %dm saved to history.", a.pendingTask, a.timer.Result.ActualSeconds/60)
			a.flashColor = accent2
			a.noteInput.SetValue("")
			a.noteInput.Focus()
			a.screen = screenNote
			return a, textinput.Blink
		case model.StatusSkipped:
			sound.Stop()
			a.flash = fmt.Sprintf("⏭ %q skipped.", a.pendingTask)
			a.flashColor = muted
			a.screen = screenDashboard
		case model.StatusCancelled:
			sound.Stop()
			a.flash = fmt.Sprintf("✗ %q cancelled.", a.pendingTask)
			a.flashColor = muted
			a.screen = screenDashboard
		}
		return a, nil
	}
	return a, cmd
}

func (a *App) updateNote(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "enter", "esc":
			note := strings.TrimSpace(a.noteInput.Value())
			if note != "" {
				_ = a.db.FinishSession(a.pendingSession, model.StatusCompleted, a.timer.Result.ActualSeconds, note)
			}
			a.dash.refresh()
			if a.cfg.AutoStartBreak {
				return a.startBreak()
			}
			a.screen = screenDashboard
			return a, nil
		}
	}
	var cmd tea.Cmd
	a.noteInput, cmd = a.noteInput.Update(msg)
	return a, cmd
}

func (a *App) View() string {
	if a.quit {
		return ""
	}

	var content string
	switch a.screen {
	case screenTaskSelect:
		content = "\n" + a.list.View() + "\n" + dimHelp("↑/↓ move   enter select   d delete   esc back")
	case screenNewTask:
		content = "\n" + styleBright.Bold(true).Render("New task") +
			"\n\n" + a.newTaskInput.View() + "\n\n" + dimHelp("enter confirm   esc back")
	case screenDuration:
		content = "\n" + styleAccent.Bold(true).Render("🍅 "+a.pendingTask) +
			"\n\n" + styleMuted.Render("How many minutes? (default "+strconv.Itoa(int(a.cfg.Focus.Minutes()))+")") +
			"\n\n" + a.durationInput.View() + "\n\n" + dimHelp("enter start   esc back")
	case screenTimer:
		content = a.timer.View()
		if a.muted {
			content += "\n" + styleMuted.Render("🔇 muted")
		}
		if a.checkpointActive {
			content += "\n\n" + styleAccent.Render("on task?  [y]  [n]")
		}
		if a.nudgeOverlay != nil {
			content += "\n\n" + styleAccent.Render(a.nudgeOverlay.text) +
				"\n" + dimHelp("[b] break  [r] refocus  [d] drifted  [s] snooze")
		}
	case screenSettings:
		content = a.settingsView()
	case screenStats:
		content = a.statsView()
	case screenQuitConfirm:
		content = "\n" + styleErr.Bold(true).Render("Quit pomo?") +
			"\n\n" + styleMuted.Render("Type \"confirm\" and press enter to quit.") +
			"\n\n" + a.quitInput.View()
		if a.quitErr {
			content += "\n\n" + styleErr.Render("Type \"confirm\" exactly, or esc to cancel.")
		}
		content += "\n\n" + dimHelp("enter confirm   esc cancel")
	case screenNote:
		content = "\n" + lipgloss.NewStyle().Foreground(a.flashColor).Render(a.flash) + "\n\n" +
			styleBright.Bold(true).Render("Add a note?") + styleMuted.Render(" (optional)") +
			"\n\n" + a.noteInput.View() + "\n\n" + dimHelp("enter/esc continue")
	case screenPrompt:
		content = a.viewPrompt()
	case screenResult:
		content = a.viewResult()
	case screenChat:
		content = a.viewChat()
	default:
		view := a.dash.View()
		view += "\n" + a.daemonIndicator()
		if a.flash != "" {
			view += "\n\n" + lipgloss.NewStyle().Foreground(a.flashColor).Render(a.flash)
		}
		if a.err != nil {
			view += "\n\n" + styleErr.Render(a.err.Error())
		}
		content = view
	}

	if a.width == 0 || a.height == 0 {
		return content
	}
	return lipgloss.Place(a.width, a.height, lipgloss.Center, lipgloss.Center, content)
}

func (a *App) settingsView() string {
	label := styleMuted.Width(16)
	rowStyle := func(active bool) lipgloss.Style {
		if active {
			return styleAccent.Bold(true)
		}
		return styleBright
	}

	soundVal := "Off"
	if a.cfg.Sound {
		soundVal = "On"
	}
	choiceVal := string(a.cfg.SoundChoice)
	for _, o := range sound.StartOptions() {
		if o.ID == a.cfg.SoundChoice {
			choiceVal = o.Label
			break
		}
	}

	cursor := func(row int) string {
		if a.settingsCursor == row {
			return "▸ "
		}
		return "  "
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(styleBright.Bold(true).Render("⚙ Settings"))
	b.WriteString("\n\n")
	b.WriteString(cursor(settingRowSound) + label.Render("Sound") +
		rowStyle(a.settingsCursor == settingRowSound).Render(soundVal))
	b.WriteString("\n")
	b.WriteString(cursor(settingRowSoundChoice) + label.Render("Start Sound") +
		rowStyle(a.settingsCursor == settingRowSoundChoice).Render(choiceVal))
	b.WriteString("\n\n")

	onOff := func(v bool) string {
		if v {
			return "On"
		}
		return "Off"
	}
	b.WriteString(cursor(settingRowDriftEnabled) + label.Render("Drift Detect") +
		rowStyle(a.settingsCursor == settingRowDriftEnabled).Render(onOff(a.cfg.Drift.Enabled)))
	b.WriteString("\n")
	b.WriteString(cursor(settingRowCheckpoint) + label.Render("Checkpoint") +
		rowStyle(a.settingsCursor == settingRowCheckpoint).Render(onOff(a.cfg.CheckpointEnabled)))
	b.WriteString("\n")
	b.WriteString(cursor(settingRowNudges) + label.Render("Nudges") +
		rowStyle(a.settingsCursor == settingRowNudges).Render(onOff(a.cfg.Nudge.Enabled)))
	b.WriteString("\n")
	provVal := a.cfg.AI.Provider
	if provVal == "" {
		provVal = "off"
	}
	b.WriteString(cursor(settingRowAIProvider) + label.Render("AI Provider") +
		rowStyle(a.settingsCursor == settingRowAIProvider).Render(provVal))
	b.WriteString("\n\n")
	b.WriteString(styleDim.Render("AI key: " + pomoconfig.MaskKey(a.cfg.AI.Key) +
		"   (set with: pomo config set ai.key …)"))
	b.WriteString("\n\n")
	b.WriteString(dimHelp("↑/↓ move   enter/←/→ change   esc back"))
	return b.String()
}

func (a *App) statsView() string {
	stats := report.LoadDayStats(a.db)

	width := a.width
	if width == 0 {
		width = 80
	}
	weeks := (width - 12) / 2
	if weeks > 24 {
		weeks = 24
	}
	if weeks < 8 {
		weeks = 8
	}
	barWidth := width - 26
	if barWidth > 30 {
		barWidth = 30
	}
	if barWidth < 10 {
		barWidth = 10
	}

	totalFocus, totalSessions := 0, 0
	for _, v := range stats {
		totalFocus += v.Secs
		totalSessions += v.Sessions
	}
	current, longest := report.Streaks(stats)
	bestDay, bestDaySecs := report.MostActiveDay(stats)

	label := styleMuted.Width(18)
	value := styleBright.Bold(true)

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(styleAccent.Bold(true).Render("◆ ANALYTICS"))
	b.WriteString("\n\n")
	b.WriteString(renderHeatmap(stats, weeks))
	b.WriteString("\n")
	b.WriteString(heatLegend())
	b.WriteString("\n\n")

	b.WriteString(label.Render("Total Focus") + value.Render(fmtDur(totalFocus)))
	b.WriteString("    ")
	b.WriteString(label.Render("Sessions") + value.Render(fmt.Sprint(totalSessions)))
	b.WriteString("\n")
	b.WriteString(label.Render("Current Streak") + value.Render(fmt.Sprintf("%d days", current)))
	b.WriteString("    ")
	b.WriteString(label.Render("Longest Streak") + value.Render(fmt.Sprintf("%d days", longest)))
	b.WriteString("\n")
	b.WriteString(label.Render("Most Active Day") + value.Render(fmt.Sprintf("%s (%s)", bestDay, fmtDur(bestDaySecs))))
	b.WriteString("\n\n")

	tabs := []string{"Day", "Week", "Month"}
	var tabRow strings.Builder
	for i, t := range tabs {
		if i == a.statsPeriod {
			tabRow.WriteString(styleAccent.Bold(true).Render("[" + t + "]"))
		} else {
			tabRow.WriteString(styleMuted.Render(" " + t + " "))
		}
		tabRow.WriteString("  ")
	}
	b.WriteString(tabRow.String())
	b.WriteString("\n\n")

	switch a.statsPeriod {
	case statsPeriodDay:
		b.WriteString(breakdownDays(stats, 7, barWidth))
	case statsPeriodWeek:
		b.WriteString(breakdownWeeks(stats, 8, barWidth))
	case statsPeriodMonth:
		b.WriteString(breakdownMonths(stats, 6, barWidth))
	}

	b.WriteString("\n")
	b.WriteString(dimHelp("tab cycle range   esc back"))
	return b.String()
}

func dimHelp(s string) string {
	return styleMuted.Render(s)
}
