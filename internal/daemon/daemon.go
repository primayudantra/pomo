// Package daemon holds the drift-detection loop. The Loop.Tick method is a
// single pure-ish iteration — no sleeping, all time from an injected clock —
// so it is fully unit-tested with fakes. cmd/daemon.go is the thin
// sleep-loop + pidfile/flock wrapper around it.
package daemon

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"math/rand"
	"os"
	"time"

	"pomo/internal/ai"
	"pomo/internal/db"
	"pomo/internal/drift"
	"pomo/internal/ipc"
	"pomo/internal/model"
	"pomo/internal/notify"
	"pomo/internal/nudge"
	"pomo/internal/pomoconfig"
	"pomo/internal/watch"
)

// Broadcaster is the subset of *ipc.Server the loop needs.
type Broadcaster interface{ Broadcast(ipc.Event) }

type Deps struct {
	DB     *db.DB
	Watch  watch.Watcher
	Notify notify.Notifier
	IPC    Broadcaster
	AI     ai.Nudger
	Cfg    pomoconfig.Config
	Now    func() time.Time
}

type Loop struct {
	d         Deps
	active    *sessionState
	startedAt time.Time
	lastTick  time.Time
}

type sessionState struct {
	sessionID int64
	repoPath  string
	lastWrite time.Time

	drift drift.State
	nudge nudge.State

	episodeID      int64
	episodeSeconds int
	recoveredFor   time.Duration

	checkpointAt      time.Time
	checkpointAsked   bool
	checkpointDeadln  time.Time
	checkpointNoUntil time.Time
}

func NewLoop(d Deps) *Loop {
	l := &Loop{d: d, startedAt: nowOr(d.Now)}
	return l
}

func nowOr(f func() time.Time) time.Time {
	if f != nil {
		return f()
	}
	return time.Now()
}

func (l *Loop) now() time.Time { return nowOr(l.d.Now) }

func (l *Loop) tickSeconds() int {
	s := int(l.d.Cfg.Daemon.Tick / time.Second)
	if s <= 0 {
		s = 15
	}
	return s
}

// Tick runs one loop iteration.
func (l *Loop) Tick() {
	now := l.now()
	l.lastTick = now

	sess, err := l.d.DB.LastRunningSession()
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		log.Println("daemon: LastRunningSession:", err)
		return
	}
	if sess == nil {
		if l.active != nil && l.active.episodeID != 0 {
			l.closeEpisode(now)
		}
		l.active = nil
		l.d.IPC.Broadcast(ipc.Event{Type: "idle"})
		return
	}

	if l.active == nil || l.active.sessionID != sess.ID {
		l.active = l.newSession(sess, now)
		l.d.IPC.Broadcast(ipc.Event{Type: "watching", SessionID: sess.ID, RepoPath: sess.RepoPath})
	}
	a := l.active

	// --- foreground signal ---
	class := watch.Neutral
	appName, title, detail := "", "", ""
	if app, ferr := l.d.Watch.Foreground(); ferr == nil {
		class = watch.Classify(app, l.d.Cfg.Drift)
		appName, title = app.Name, app.Title
		detail = app.Name
		if app.Title != "" {
			detail = app.Name + " — " + app.Title
		}
	}

	// --- fs signal ---
	fsStale := false
	if a.repoPath != "" {
		if watch.RepoActive(a.repoPath, a.lastWrite) {
			a.lastWrite = now
		}
		fsStale = now.Sub(a.lastWrite) >= l.d.Cfg.Drift.FsStale
	}

	// --- checkpoint ---
	if l.d.Cfg.CheckpointEnabled && !a.checkpointAt.IsZero() && !a.checkpointAsked && !now.Before(a.checkpointAt) {
		a.checkpointAsked = true
		a.checkpointDeadln = now.Add(l.d.Cfg.Drift.CheckpointTimeout)
		l.d.IPC.Broadcast(ipc.Event{Type: "checkpoint", Text: "on task?"})
	}
	if a.checkpointAsked && !a.checkpointDeadln.IsZero() && now.After(a.checkpointDeadln) {
		// timeout == "no"
		a.checkpointNoUntil = now.Add(l.d.Cfg.Drift.CheckpointPenalty)
		a.checkpointDeadln = time.Time{}
	}

	// --- score ---
	res := a.drift.Step(now, drift.Signals{
		Class:        class,
		Detail:       detail,
		FsStale:      fsStale,
		CheckpointNo: now.Before(a.checkpointNoUntil),
	}, l.d.Cfg.Drift)

	// --- episode bookkeeping ---
	switch {
	case res.Drifting && a.episodeID == 0:
		id, e := l.d.DB.OpenDriftEpisode(sess.ID, now, res.Trigger, res.Detail)
		if e != nil {
			log.Println("daemon: OpenDriftEpisode:", e)
		} else {
			a.episodeID = id
			a.episodeSeconds = 0
			a.recoveredFor = 0
		}
	case res.Drifting && a.episodeID != 0:
		a.episodeSeconds += l.tickSeconds()
		a.recoveredFor = 0
		if e := l.d.DB.UpdateDriftEpisode(a.episodeID, a.episodeSeconds, res.Detail); e != nil {
			log.Println("daemon: UpdateDriftEpisode:", e)
		}
	case !res.Drifting && a.episodeID != 0:
		a.recoveredFor += l.d.Cfg.Daemon.Tick
		if a.recoveredFor >= l.d.Cfg.Drift.RecoverGrace {
			l.closeEpisode(now)
		}
	}

	// --- nudge ---
	d := a.nudge.Evaluate(now, a.episodeID != 0, nudge.Context{
		Task:           sess.TaskName,
		DistractApp:    appName,
		DistractDetail: title,
		DriftMinutes:   a.episodeSeconds / 60,
		SessionMinutes: int(now.Sub(sess.StartedAt).Minutes()),
		Hour:           now.Hour(),
	}, l.d.Cfg.Nudge, l.aiLine(sess))
	if d.Fire {
		_ = l.d.Notify.Send("pomo", d.Text)
		l.d.IPC.Broadcast(ipc.Event{Type: "nudge", Text: d.Text, Level: d.Level, Actions: d.Actions})
	}
}

func (l *Loop) newSession(sess *model.Session, now time.Time) *sessionState {
	a := &sessionState{
		sessionID: sess.ID,
		repoPath:  sess.RepoPath,
		lastWrite: sess.StartedAt,
	}
	if l.d.Cfg.CheckpointEnabled && sess.PlannedDuration > 0 {
		third := sess.PlannedDuration / 3
		offset := third + rand.Intn(third+1)
		a.checkpointAt = sess.StartedAt.Add(time.Duration(offset) * time.Second)
	}
	return a
}

func (l *Loop) closeEpisode(now time.Time) {
	a := l.active
	if a == nil || a.episodeID == 0 {
		return
	}
	if e := l.d.DB.CloseDriftEpisode(a.episodeID, now, a.episodeSeconds); e != nil {
		log.Println("daemon: CloseDriftEpisode:", e)
	}
	a.episodeID = 0
	a.episodeSeconds = 0
	a.recoveredFor = 0
}

// aiLine returns a nudge.LineFn that calls the AI provider with a 3s budget.
func (l *Loop) aiLine(sess *model.Session) nudge.LineFn {
	return func(nc nudge.Context) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return l.d.AI.Line(ctx, ai.NudgeContext{
			Task:           nc.Task,
			Tag:            sess.Tag,
			DistractApp:    nc.DistractApp,
			DistractDetail: nc.DistractDetail,
			DriftMinutes:   nc.DriftMinutes,
			SessionMinutes: nc.SessionMinutes,
			Level:          l.active.nudge.Count + 1,
			Hour:           nc.Hour,
		})
	}
}

// HandleEvent processes an inbound IPC event from the TUI.
func (l *Loop) HandleEvent(e ipc.Event) {
	if l.active == nil {
		return
	}
	a := l.active
	now := l.now()
	switch e.Type {
	case "checkpoint-answer":
		if e.Answer == "y" {
			a.nudge.Reset()
			a.checkpointDeadln = time.Time{}
		} else {
			a.checkpointNoUntil = now.Add(l.d.Cfg.Drift.CheckpointPenalty)
			a.checkpointDeadln = time.Time{}
		}
	case "nudge-action":
		switch e.Action {
		case "d", "r":
			if a.episodeID != 0 {
				_ = l.d.DB.UpdateDriftEpisode(a.episodeID, a.episodeSeconds, "(self-reported)")
				l.closeEpisode(now)
			}
			a.drift = drift.State{}
			if e.Action == "r" {
				a.nudge.Reset()
			}
		case "snooze":
			a.nudge.Snooze(now)
		}
	}
}

// StatusSnapshot is written to ~/.pomo/daemon.status by the run command.
func (l *Loop) StatusSnapshot() Status {
	s := Status{
		PID:            os.Getpid(),
		StartedAt:      l.startedAt,
		LastTick:       l.lastTick,
		WatchBackend:   l.d.Watch.Name(),
		WatchSupported: l.d.Watch.Supported(),
	}
	if l.active != nil {
		s.WatchingID = l.active.sessionID
	}
	return s
}
