// Package drift is the pure per-tick drift scorer for the daemon. It holds
// only the minimal cross-tick memory (how long the user has been on a
// distract app) and returns a verdict; episode bookkeeping lives in the
// daemon.
package drift

import (
	"time"

	"pomo/internal/pomoconfig"
	"pomo/internal/watch"
)

type Signals struct {
	Class        watch.Class
	Detail       string
	FsStale      bool
	CheckpointNo bool
}

type State struct {
	distractSince time.Time
}

type Result struct {
	Drifting bool
	Trigger  string
	Detail   string
}

// Step folds one tick's signals into the state and returns the verdict.
func (s *State) Step(now time.Time, sig Signals, cfg pomoconfig.DriftConfig) Result {
	if sig.Class == watch.Distract {
		if s.distractSince.IsZero() {
			s.distractSince = now
		}
	} else {
		s.distractSince = time.Time{}
	}

	switch {
	case sig.CheckpointNo:
		return Result{Drifting: true, Trigger: "checkpoint_no", Detail: sig.Detail}
	case !s.distractSince.IsZero() && now.Sub(s.distractSince) >= cfg.DistractGrace:
		return Result{Drifting: true, Trigger: "foreground", Detail: sig.Detail}
	case sig.FsStale && sig.Class != watch.Focus:
		return Result{Drifting: true, Trigger: "fs_stale", Detail: sig.Detail}
	default:
		return Result{}
	}
}
