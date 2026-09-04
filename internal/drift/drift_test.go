package drift_test

import (
	"testing"
	"time"

	"pomo/internal/drift"
	"pomo/internal/pomoconfig"
	"pomo/internal/watch"
)

func cfg() pomoconfig.DriftConfig {
	return pomoconfig.DriftConfig{DistractGrace: 3 * time.Minute, FsStale: 10 * time.Minute}
}

func TestDistractNeedsGrace(t *testing.T) {
	var s drift.State
	c := cfg()
	t0 := time.Now()

	r := s.Step(t0, drift.Signals{Class: watch.Distract, Detail: "Slack"}, c)
	if r.Drifting {
		t.Fatal("first distract tick should not be drifting yet")
	}
	r = s.Step(t0.Add(2*time.Minute), drift.Signals{Class: watch.Distract, Detail: "Slack"}, c)
	if r.Drifting {
		t.Fatal("2min < grace, not drifting")
	}
	r = s.Step(t0.Add(3*time.Minute+time.Second), drift.Signals{Class: watch.Distract, Detail: "Slack"}, c)
	if !r.Drifting || r.Trigger != "foreground" || r.Detail != "Slack" {
		t.Fatalf("after grace should be drifting: %+v", r)
	}
}

func TestFocusResetsDistractClock(t *testing.T) {
	var s drift.State
	c := cfg()
	t0 := time.Now()
	s.Step(t0, drift.Signals{Class: watch.Distract, Detail: "Slack"}, c)
	s.Step(t0.Add(1*time.Minute), drift.Signals{Class: watch.Focus, Detail: "iTerm2"}, c)
	r := s.Step(t0.Add(2*time.Minute), drift.Signals{Class: watch.Distract, Detail: "Slack"}, c)
	if r.Drifting {
		t.Fatal("distract clock should have reset after a Focus tick")
	}
}

func TestFsStaleWhileNotFocused(t *testing.T) {
	var s drift.State
	c := cfg()
	now := time.Now()
	if r := s.Step(now, drift.Signals{Class: watch.Neutral, FsStale: true, Detail: "Preview"}, c); !r.Drifting || r.Trigger != "fs_stale" {
		t.Fatalf("fs stale + neutral => drifting: %+v", r)
	}
	if r := s.Step(now, drift.Signals{Class: watch.Focus, FsStale: true, Detail: "iTerm2"}, c); r.Drifting {
		t.Fatal("fs stale but Focus => not drifting")
	}
}

func TestCheckpointNo(t *testing.T) {
	var s drift.State
	if r := s.Step(time.Now(), drift.Signals{Class: watch.Focus, CheckpointNo: true, Detail: "iTerm2"}, cfg()); !r.Drifting || r.Trigger != "checkpoint_no" {
		t.Fatalf("checkpoint no => drifting: %+v", r)
	}
}
