package nudge_test

import (
	"errors"
	"testing"
	"time"

	"pomo/internal/nudge"
	"pomo/internal/pomoconfig"
)

func cfg() pomoconfig.NudgeConfig {
	return pomoconfig.NudgeConfig{Enabled: true, MaxPerSession: 4, MinGap: 5 * time.Minute}
}

func TestNoFireWithoutEpisode(t *testing.T) {
	var s nudge.State
	d := s.Evaluate(time.Now(), false, nudge.Context{Task: "x"}, cfg(), nil)
	if d.Fire {
		t.Fatal("no episode => no fire")
	}
}

func TestFiresThenRespectsMinGap(t *testing.T) {
	var s nudge.State
	c := cfg()
	t0 := time.Now()
	d := s.Evaluate(t0, true, nudge.Context{Task: "ship"}, c, nil)
	if !d.Fire || d.Level != 1 || d.Text != "still on ship?" {
		t.Fatalf("first fire: %+v", d)
	}
	d = s.Evaluate(t0.Add(2*time.Minute), true, nudge.Context{Task: "ship"}, c, nil)
	if d.Fire {
		t.Fatal("within min gap => no fire")
	}
	d = s.Evaluate(t0.Add(6*time.Minute), true, nudge.Context{Task: "ship", DistractApp: "Chrome", DriftMinutes: 12}, c, nil)
	if !d.Fire || d.Level != 2 {
		t.Fatalf("second fire should be level 2: %+v", d)
	}
	if d.Text != "12 min on Chrome — ship still the plan?" {
		t.Fatalf("L2 text = %q", d.Text)
	}
}

func TestEscalationCapsAtThreeAndMaxPerSession(t *testing.T) {
	var s nudge.State
	c := cfg()
	c.MinGap = 0
	now := time.Now()
	levels := []int{}
	for i := 0; i < 6; i++ {
		d := s.Evaluate(now.Add(time.Duration(i)*time.Minute), true, nudge.Context{Task: "t"}, c, nil)
		if d.Fire {
			levels = append(levels, d.Level)
		}
	}
	if len(levels) != 4 {
		t.Fatalf("fired %d times, want 4: %v", len(levels), levels)
	}
	if levels[0] != 1 || levels[1] != 2 || levels[2] != 3 || levels[3] != 3 {
		t.Fatalf("levels = %v, want [1 2 3 3]", levels)
	}
}

func TestResetDropsLevel(t *testing.T) {
	var s nudge.State
	c := cfg()
	c.MinGap = 0
	now := time.Now()
	s.Evaluate(now, true, nudge.Context{Task: "t"}, c, nil)
	s.Evaluate(now.Add(time.Minute), true, nudge.Context{Task: "t"}, c, nil)
	s.Reset()
	d := s.Evaluate(now.Add(2*time.Minute), true, nudge.Context{Task: "t"}, c, nil)
	if d.Level != 1 {
		t.Fatalf("after reset, next fire level = %d, want 1", d.Level)
	}
}

func TestAILineUsedThenDedup(t *testing.T) {
	var s nudge.State
	c := cfg()
	c.MinGap = 0
	now := time.Now()
	fixed := func(nudge.Context) (string, error) { return "close the tab", nil }
	d := s.Evaluate(now, true, nudge.Context{Task: "t"}, c, fixed)
	if d.Text != "close the tab" {
		t.Fatalf("AI line not used: %q", d.Text)
	}
	d = s.Evaluate(now.Add(time.Minute), true, nudge.Context{Task: "t"}, c, fixed)
	if d.Text == "close the tab" {
		t.Fatal("duplicate AI line should have been rejected")
	}
}

func TestAIErrorFallsBackToCanned(t *testing.T) {
	var s nudge.State
	c := cfg()
	now := time.Now()
	boom := func(nudge.Context) (string, error) { return "", errors.New("timeout") }
	d := s.Evaluate(now, true, nudge.Context{Task: "t"}, c, boom)
	if !d.Fire || d.Text != "still on t?" {
		t.Fatalf("AI error should fall back to canned: %+v", d)
	}
}
