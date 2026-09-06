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
}

// onBeforeClose runs when the user closes the window. In v1 closing the
// window quits the app (hide-on-close returns with the tray in v1.1). If a
// session is running or paused we prompt first. Returning true prevents the
// close.
func (a *App) onBeforeClose(ctx context.Context) (prevent bool) {
	st := a.timer.GetState()
	if st.Phase != PhaseRunning && st.Phase != PhasePaused {
		return false
	}
	sel, _ := wr.MessageDialog(ctx, wr.MessageDialogOptions{
		Type:          wr.QuestionDialog,
		Title:         "Session still running",
		Message:       "A Pomodoro is still running. What do you want to do?",
		Buttons:       []string{"Leave it running", "Cancel session", "Stay"},
		DefaultButton: "Leave it running",
		CancelButton:  "Stay",
	})
	switch sel {
	case "Stay":
		return true
	case "Cancel session":
		a.timer.Cancel()
		return false
	default:
		// "Leave it running" — the row stays `running` and rehydrates
		// on next launch.
		return false
	}
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
