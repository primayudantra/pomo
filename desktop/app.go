package main

import (
	"context"

	"pomo/internal/db"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx       context.Context
	db        *db.DB
	timer     *TimerService
	session   *SessionService
	daemonSvc *DaemonService

	// reallyQuit is set by Quit() to let OnBeforeClose fall through to a
	// real exit instead of hiding the window.
	reallyQuit bool
}

// Quit is the app's real exit path. If a session is running or paused it
// asks the user what to do first. Bound, so JS can call it too.
func (a *App) Quit() {
	st := a.timer.GetState()
	if st.Phase == PhaseRunning || st.Phase == PhasePaused {
		sel, _ := wr.MessageDialog(a.ctx, wr.MessageDialogOptions{
			Type:          wr.QuestionDialog,
			Title:         "Session still running",
			Message:       "A Pomodoro is still running. What do you want to do?",
			Buttons:       []string{"Leave it running", "Cancel session", "Stay"},
			DefaultButton: "Leave it running",
			CancelButton:  "Stay",
		})
		switch sel {
		case "Stay":
			return
		case "Cancel session":
			a.timer.Cancel()
		}
		// "Leave it running" falls through — the row stays `running` and
		// rehydrates on next launch.
	}
	a.reallyQuit = true
	wr.Quit(a.ctx)
}

// NewApp creates a new App application struct
func NewApp() *App {
	d, err := db.Open()
	if err != nil {
		panic(err)
	}
	return &App{
		db:        d,
		timer:     NewTimerService(d, realClock{}),
		session:   NewSessionService(d),
		daemonSvc: NewDaemonService(),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.timer.Attach(ctx)
}

// shutdown is called on app exit: stop the ticker, close the db.
func (a *App) shutdown(ctx context.Context) {
	a.timer.Shutdown()
	if a.db != nil {
		_ = a.db.Close()
	}
}
