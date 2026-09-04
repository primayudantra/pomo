package daemon_test

import (
	"testing"
	"time"

	"pomo/internal/ai"
	"pomo/internal/daemon"
	"pomo/internal/db/dbtest"
	"pomo/internal/ipc"
	"pomo/internal/model"
	"pomo/internal/pomoconfig"
	"pomo/internal/watch"
)

type fakeWatch struct{ app watch.ForegroundApp }

func (f *fakeWatch) Foreground() (watch.ForegroundApp, error) { return f.app, nil }
func (f *fakeWatch) Supported() bool                          { return true }
func (f *fakeWatch) Name() string                             { return "fake" }

type fakeNotify struct{ n int }

func (f *fakeNotify) Send(string, string) error { f.n++; return nil }

type fakeBus struct{ events []ipc.Event }

func (f *fakeBus) Broadcast(e ipc.Event) { f.events = append(f.events, e) }

func baseCfg() pomoconfig.Config {
	c := pomoconfig.Config{}
	c.Daemon.Tick = time.Minute
	c.Drift = pomoconfig.DriftConfig{
		Enabled: true, DistractGrace: 3 * time.Minute, FsStale: 10 * time.Minute,
		CheckpointTimeout: 20 * time.Second, CheckpointPenalty: 2 * time.Minute,
		RecoverGrace: time.Minute,
		FocusApps:    pomoconfig.DefaultFocusApps(), DistractApps: pomoconfig.DefaultDistractApps(),
	}
	c.Nudge = pomoconfig.NudgeConfig{Enabled: true, MaxPerSession: 4, MinGap: 5 * time.Minute}
	c.CheckpointEnabled = false
	return c
}

func TestDriftEpisodeRecordedAndNudged(t *testing.T) {
	d := dbtest.NewTemp(t)
	id, err := d.CreateSession(model.Session{
		TaskName: "ship", PlannedDuration: 1500, Status: model.StatusRunning, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}

	fw := &fakeWatch{app: watch.ForegroundApp{Name: "Slack"}}
	fn := &fakeNotify{}
	fb := &fakeBus{}
	nAI, _, _ := ai.New(ai.Config{})

	clock := time.Now()
	loop := daemon.NewLoop(daemon.Deps{
		DB: d, Watch: fw, Notify: fn, IPC: fb, AI: nAI, Cfg: baseCfg(),
		Now: func() time.Time { return clock },
	})

	for i := 0; i < 6; i++ {
		loop.Tick()
		clock = clock.Add(time.Minute)
	}

	events, err := d.DriftEventsForSession(id)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected a drift episode to be recorded")
	}
	if events[0].Trigger != "foreground" || events[0].Seconds < 60 {
		t.Fatalf("episode looks wrong: %+v", events[0])
	}
	if fn.n == 0 {
		t.Fatal("expected at least one nudge notification")
	}
	sawNudge := false
	for _, e := range fb.events {
		if e.Type == "nudge" {
			sawNudge = true
		}
	}
	if !sawNudge {
		t.Fatal("expected a nudge IPC event")
	}
}

func TestNoSessionClosesEpisodeAndGoesIdle(t *testing.T) {
	d := dbtest.NewTemp(t)
	id, _ := d.CreateSession(model.Session{
		TaskName: "x", PlannedDuration: 1500, Status: model.StatusRunning, StartedAt: time.Now(),
	})
	fw := &fakeWatch{app: watch.ForegroundApp{Name: "Discord"}}
	fb := &fakeBus{}
	nAI, _, _ := ai.New(ai.Config{})
	clock := time.Now()
	loop := daemon.NewLoop(daemon.Deps{
		DB: d, Watch: fw, Notify: &fakeNotify{}, IPC: fb, AI: nAI, Cfg: baseCfg(),
		Now: func() time.Time { return clock },
	})
	for i := 0; i < 5; i++ {
		loop.Tick()
		clock = clock.Add(time.Minute)
	}
	_ = d.FinishSession(id, model.StatusCompleted, 300, "")
	loop.Tick()

	events, _ := d.DriftEventsForSession(id)
	for _, e := range events {
		if e.EndedAt == nil {
			t.Fatalf("episode %d left open after session ended", e.ID)
		}
	}
	last := fb.events[len(fb.events)-1]
	if last.Type != "idle" {
		t.Fatalf("last event = %q, want idle", last.Type)
	}
}

func TestCheckpointNoTriggersDrift(t *testing.T) {
	d := dbtest.NewTemp(t)
	id, _ := d.CreateSession(model.Session{
		TaskName: "x", PlannedDuration: 1500, Status: model.StatusRunning, StartedAt: time.Now(),
	})
	fw := &fakeWatch{app: watch.ForegroundApp{Name: "iTerm2"}}
	nAI, _, _ := ai.New(ai.Config{})
	clock := time.Now()
	loop := daemon.NewLoop(daemon.Deps{
		DB: d, Watch: fw, Notify: &fakeNotify{}, IPC: &fakeBus{}, AI: nAI, Cfg: baseCfg(),
		Now: func() time.Time { return clock },
	})
	loop.Tick()
	loop.HandleEvent(ipc.Event{Type: "checkpoint-answer", Answer: "n"})
	loop.Tick()
	clock = clock.Add(time.Minute)
	loop.Tick()

	events, _ := d.DriftEventsForSession(id)
	if len(events) == 0 || events[0].Trigger != "checkpoint_no" {
		t.Fatalf("checkpoint 'no' should have opened a checkpoint_no episode: %+v", events)
	}
}
