// Package nudge is the pure escalation state machine for drift nudges. It
// decides when to nudge, at what level, and with what text — using an AI line
// when one is supplied and non-duplicate, else a canned template.
package nudge

import (
	"fmt"
	"time"

	"pomo/internal/pomoconfig"
)

type Context struct {
	Task           string
	DistractApp    string
	DistractDetail string
	DriftMinutes   int
	SessionMinutes int
	Hour           int
}

type Decision struct {
	Fire    bool
	Level   int
	Text    string
	Actions []string
}

type State struct {
	Level  int
	Count  int
	LastAt time.Time
	recent []string
}

// LineFn generates an AI nudge line. nil, an error, or a duplicate of a
// recent line all fall back to a canned template.
type LineFn func(Context) (string, error)

// Reset drops the escalation back to level 0 (checkpoint answered "yes"). It
// does not clear LastAt — the min-gap still applies.
func (s *State) Reset() {
	s.Level = 0
	s.Count = 0
}

// Snooze pushes the nudge clock forward so the next opportunity is MinGap out.
func (s *State) Snooze(now time.Time) {
	s.LastAt = now
}

func (s *State) Evaluate(now time.Time, episodeOpen bool, ctx Context, cfg pomoconfig.NudgeConfig, line LineFn) Decision {
	if !cfg.Enabled || !episodeOpen {
		return Decision{}
	}
	if s.Count >= cfg.MaxPerSession {
		return Decision{}
	}
	if !s.LastAt.IsZero() && now.Sub(s.LastAt) < cfg.MinGap {
		return Decision{}
	}

	level := s.Count + 1
	if level > 3 {
		level = 3
	}

	text := ""
	if line != nil {
		if got, err := line(ctx); err == nil && got != "" && !s.isRecent(got) {
			text = got
		}
	}
	if text == "" {
		text = canned(level, ctx)
	}

	s.remember(text)
	s.Level = level
	s.Count++
	s.LastAt = now

	return Decision{Fire: true, Level: level, Text: text, Actions: actionsFor(level)}
}

func (s *State) isRecent(line string) bool {
	for _, r := range s.recent {
		if r == line {
			return true
		}
	}
	return false
}

func (s *State) remember(line string) {
	s.recent = append(s.recent, line)
	if len(s.recent) > 3 {
		s.recent = s.recent[len(s.recent)-3:]
	}
}

func canned(level int, ctx Context) string {
	switch level {
	case 1:
		return fmt.Sprintf("still on %s?", ctx.Task)
	case 2:
		if ctx.DistractApp != "" {
			return fmt.Sprintf("%d min on %s — %s still the plan?", ctx.DriftMinutes, ctx.DistractApp, ctx.Task)
		}
		return fmt.Sprintf("%d min drifting — %s still the plan?", ctx.DriftMinutes, ctx.Task)
	default:
		return "rough stretch. want a real break or a reset?"
	}
}

func actionsFor(level int) []string {
	switch level {
	case 1:
		return []string{"snooze", "drifted"}
	case 2:
		return []string{"refocus", "drifted", "break"}
	default:
		return []string{"break", "refocus"}
	}
}
