package main

import (
	"path/filepath"
	"testing"
	"time"

	"pomo/internal/db"
	"pomo/internal/model"
)

type fakeClock struct{ t time.Time }

func (f *fakeClock) Now() time.Time      { return f.t }
func (f *fakeClock) add(d time.Duration) { f.t = f.t.Add(d) }

func newTestTimer(t *testing.T) (*TimerService, *fakeClock, *db.DB) {
	t.Helper()
	d, err := db.OpenAt(filepath.Join(t.TempDir(), "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	clk := &fakeClock{t: time.Date(2026, 9, 6, 10, 0, 0, 0, time.UTC)}
	return NewTimerService(d, clk), clk, d
}

func TestStartThenComplete(t *testing.T) {
	ts, clk, d := newTestTimer(t)
	st, err := ts.Start("write plan", 25)
	if err != nil {
		t.Fatal(err)
	}
	if st.Phase != PhaseRunning || st.Remaining != 1500 {
		t.Fatalf("bad start state: %+v", st)
	}

	clk.add(25 * time.Minute)
	ts.tick() // unexported: one ticker iteration
	if got := ts.GetState().Phase; got != PhaseBreakPrompt {
		t.Fatalf("phase = %s, want break_prompt", got)
	}
	s, _ := d.LastRunningSession()
	if s != nil {
		t.Fatal("session should be finished")
	}
	all, _ := d.ListSessions(db.SessionFilter{})
	if all[0].Status != model.StatusCompleted || all[0].ActualDuration != 1500 {
		t.Fatalf("finished row wrong: %+v", all[0])
	}
}

func TestPauseExcludedFromElapsed(t *testing.T) {
	ts, clk, _ := newTestTimer(t)
	ts.Start("x", 25)
	clk.add(5 * time.Minute)
	ts.Pause()
	clk.add(10 * time.Minute) // paused — should not count
	ts.Resume()
	clk.add(5 * time.Minute)
	ts.tick()
	// 10 min of real focus elapsed → 15 min remaining.
	if got := ts.GetState().Remaining; got != 900 {
		t.Fatalf("remaining = %d, want 900", got)
	}
}

func TestRehydrateRunning(t *testing.T) {
	ts, clk, d := newTestTimer(t)
	ts.Start("resume me", 25)
	clk.add(10 * time.Minute)

	// New service, same db + a clock 10 min after start.
	ts2 := NewTimerService(d, clk)
	ts2.rehydrate()
	st := ts2.GetState()
	if st.Phase != PhaseRunning || st.Remaining != 900 {
		t.Fatalf("rehydrate state = %+v", st)
	}
}

func TestRehydrateExpiredCompletes(t *testing.T) {
	ts, clk, d := newTestTimer(t)
	ts.Start("done while away", 25)
	clk.add(40 * time.Minute)

	ts2 := NewTimerService(d, clk)
	ts2.rehydrate()
	if ts2.GetState().Phase != PhaseIdle {
		t.Fatal("expired session should rehydrate to idle")
	}
	all, _ := d.ListSessions(db.SessionFilter{})
	if all[0].Status != model.StatusCompleted {
		t.Fatalf("status = %s, want completed", all[0].Status)
	}
}

func TestStartRefusesSecondSession(t *testing.T) {
	ts, _, _ := newTestTimer(t)
	ts.Start("first", 25)
	if _, err := ts.Start("second", 25); err == nil {
		t.Fatal("expected error starting a second session")
	}
}
