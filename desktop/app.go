package main

import (
	"context"

	"pomo/internal/db"
)

// App struct
type App struct {
	ctx       context.Context
	db        *db.DB
	timer     *TimerService
	session   *SessionService
	daemonSvc *DaemonService
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
