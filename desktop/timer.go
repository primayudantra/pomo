package main

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"time"

	"pomo/internal/model"
	"pomo/internal/notify"
	"pomo/internal/sound"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"

	"pomo/internal/db"
)

// playFinish plays the completion chime; a package var so tests can stub it.
var playFinish = sound.PlayFinish

// sendNotify fires the desktop notification; a package var so tests can stub it.
var sendNotify = func(title, body string) { _ = notify.New().Send(title, body) }

// Phase is the timer's high-level state.
type Phase string

const (
	PhaseIdle        Phase = "idle"
	PhaseRunning     Phase = "running"
	PhasePaused      Phase = "paused"
	PhaseBreakPrompt Phase = "break_prompt"
	PhaseBreak       Phase = "break"
)

// State is the snapshot handed to the frontend.
type State struct {
	Phase     Phase  `json:"phase"`
	Task      string `json:"task"`
	Remaining int    `json:"remaining"` // seconds
	Duration  int    `json:"duration"`  // seconds
	SessionID int64  `json:"sessionId"`
}

// TimerService owns the running timer: a state machine plus a 1s ticker.
type TimerService struct {
	mu  sync.Mutex
	db  *db.DB
	clk Clock
	ctx context.Context

	st State

	startedAt   time.Time
	pausedAt    time.Time
	pauseAccum  int
	breakEndsAt time.Time

	stop chan struct{}
}

// NewTimerService builds a service around a db handle and a clock.
func NewTimerService(d *db.DB, clk Clock) *TimerService {
	return &TimerService{
		db:   d,
		clk:  clk,
		st:   State{Phase: PhaseIdle},
		stop: make(chan struct{}),
	}
}

// GetState returns a copy of the current state.
func (t *TimerService) GetState() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.st
}

// Start begins a focus session. It errors if one is already running.
func (t *TimerService) Start(task string, minutes int) (State, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if minutes <= 0 {
		return t.st, errors.New("duration must be positive")
	}

	running, err := t.db.LastRunningSession()
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return t.st, err
	}
	if running != nil {
		return t.st, errors.New("a session is already running")
	}

	taskID, err := t.db.FindOrCreateTask(task, "")
	if err != nil {
		return t.st, err
	}
	now := t.clk.Now()
	dur := minutes * 60
	id, err := t.db.CreateSession(model.Session{
		TaskID:          taskID,
		TaskName:        task,
		PlannedDuration: dur,
		Status:          model.StatusRunning,
		StartedAt:       now,
	})
	if err != nil {
		t.emit(EventError, map[string]string{"message": "could not start session: " + err.Error()})
		return t.st, err
	}

	t.startedAt = now
	t.pausedAt = time.Time{}
	t.pauseAccum = 0
	t.st = State{
		Phase:     PhaseRunning,
		Task:      task,
		Duration:  dur,
		Remaining: dur,
		SessionID: id,
	}
	t.emit(EventToday, nil)
	t.emit(EventPhase, t.st)
	return t.st, nil
}

// Pause suspends the running session.
func (t *TimerService) Pause() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.st.Phase != PhaseRunning {
		return t.st
	}
	now := t.clk.Now()
	_ = t.db.SetPaused(t.st.SessionID, now) // best-effort: a lost pause is not data loss
	t.pausedAt = now
	t.st.Phase = PhasePaused
	t.recompute(now)
	t.emit(EventPhase, t.st)
	return t.st
}

// Resume un-pauses the session.
func (t *TimerService) Resume() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.st.Phase != PhasePaused {
		return t.st
	}
	now := t.clk.Now()
	pause := int(now.Sub(t.pausedAt) / time.Second)
	_ = t.db.ClearPaused(t.st.SessionID, pause) // best-effort: a lost pause is not data loss
	t.pauseAccum += pause
	t.pausedAt = time.Time{}
	t.st.Phase = PhaseRunning
	t.recompute(now)
	t.emit(EventPhase, t.st)
	return t.st
}

// Cancel abandons the running/paused session.
func (t *TimerService) Cancel() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.st.Phase != PhaseRunning && t.st.Phase != PhasePaused {
		return t.st
	}
	now := t.clk.Now()
	elapsed := t.elapsedFocus(now)
	if err := t.db.FinishSession(t.st.SessionID, model.StatusCancelled, elapsed, ""); err != nil {
		t.emit(EventError, map[string]string{"message": "could not cancel session: " + err.Error()})
	}
	t.st = State{Phase: PhaseIdle}
	t.emit(EventToday, nil)
	t.emit(EventPhase, t.st)
	return t.st
}

// StartBreak begins a break countdown. Breaks are not persisted.
func (t *TimerService) StartBreak(minutes int) State {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.st.Phase != PhaseBreakPrompt || minutes <= 0 {
		return t.st
	}
	now := t.clk.Now()
	dur := minutes * 60
	t.breakEndsAt = now.Add(time.Duration(dur) * time.Second)
	t.st = State{
		Phase:     PhaseBreak,
		Duration:  dur,
		Remaining: dur,
	}
	t.emit(EventPhase, t.st)
	return t.st
}

// SkipBreak ends any break immediately.
func (t *TimerService) SkipBreak() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.st.Phase != PhaseBreakPrompt && t.st.Phase != PhaseBreak {
		return t.st
	}
	t.st = State{Phase: PhaseIdle}
	t.emit(EventPhase, t.st)
	return t.st
}

// Attach stores the Wails context, starts the ticker and rehydrates.
func (t *TimerService) Attach(ctx context.Context) {
	t.mu.Lock()
	t.ctx = ctx
	t.mu.Unlock()
	go t.loop()
	t.rehydrate()
}

// Shutdown stops the ticker goroutine.
func (t *TimerService) Shutdown() {
	select {
	case <-t.stop:
	default:
		close(t.stop)
	}
}

func (t *TimerService) loop() {
	for {
		select {
		case <-time.After(time.Second):
			t.tick()
		case <-t.stop:
			return
		}
	}
}

// tick performs one ticker iteration: recompute, handle a 0-crossing, emit.
func (t *TimerService) tick() {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.clk.Now()
	switch t.st.Phase {
	case PhaseRunning:
		t.recompute(now)
		if t.st.Remaining <= 0 {
			t.complete(now)
		}
	case PhasePaused:
		t.recompute(now)
	case PhaseBreak:
		t.st.Remaining = int(t.breakEndsAt.Sub(now) / time.Second)
		if now.After(t.breakEndsAt) || now.Equal(t.breakEndsAt) {
			t.st = State{Phase: PhaseIdle}
			t.emit(EventPhase, t.st)
		}
	}
	t.emit(EventTick, t.st)
}

// complete finishes a running session naturally. Caller holds the lock.
func (t *TimerService) complete(now time.Time) {
	elapsed := t.elapsedFocus(now)
	if elapsed > t.st.Duration {
		elapsed = t.st.Duration
	}
	task := t.st.Task
	if err := t.db.FinishSession(t.st.SessionID, model.StatusCompleted, elapsed, ""); err != nil {
		t.emit(EventError, map[string]string{"message": "could not finish session: " + err.Error()})
		t.st = State{Phase: PhaseIdle}
		t.emit(EventToday, nil)
		t.emit(EventPhase, t.st)
		return
	}
	playFinish()
	go sendNotify("Pomodoro done", task)
	t.st = State{Phase: PhaseBreakPrompt, Task: task}
	t.emit(EventToday, nil)
	t.emit(EventPhase, t.st)
}

// recompute refreshes Remaining from the wall clock. Caller holds the lock.
func (t *TimerService) recompute(now time.Time) {
	t.st.Remaining = t.st.Duration - t.elapsedFocus(now)
}

// elapsedFocus is focus seconds elapsed, excluding paused time.
func (t *TimerService) elapsedFocus(now time.Time) int {
	curPause := 0
	if !t.pausedAt.IsZero() {
		curPause = int(now.Sub(t.pausedAt) / time.Second)
	}
	return int(now.Sub(t.startedAt)/time.Second) - t.pauseAccum - curPause
}

// rehydrate restores state from the last running session in the db.
func (t *TimerService) rehydrate() {
	t.mu.Lock()
	defer t.mu.Unlock()

	s, err := t.db.LastRunningSession()
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		t.st = State{Phase: PhaseIdle}
		return
	}
	if s == nil {
		t.st = State{Phase: PhaseIdle}
		return
	}

	t.startedAt = s.StartedAt
	t.pauseAccum = s.PauseAccumSecs
	t.pausedAt = time.Time{}
	if s.PausedAt != nil {
		t.pausedAt = *s.PausedAt
	}

	now := t.clk.Now()
	elapsed := t.elapsedFocus(now)
	if elapsed >= s.PlannedDuration {
		if err := t.db.FinishSession(s.ID, model.StatusCompleted, s.PlannedDuration, ""); err != nil {
			t.emit(EventError, map[string]string{"message": "could not finish session: " + err.Error()})
		}
		t.st = State{Phase: PhaseIdle}
		t.emit(EventToday, nil)
		return
	}

	phase := PhaseRunning
	if s.PausedAt != nil {
		phase = PhasePaused
	}
	t.st = State{
		Phase:     phase,
		Task:      s.TaskName,
		Duration:  s.PlannedDuration,
		Remaining: s.PlannedDuration - elapsed,
		SessionID: s.ID,
	}
	t.emit(EventPhase, t.st)
}

// emit sends an event to the frontend; a no-op before Attach.
func (t *TimerService) emit(event string, payload any) {
	if t.ctx == nil {
		return
	}
	wr.EventsEmit(t.ctx, event, payload)
}
