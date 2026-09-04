// Package ai is the BYOK provider layer for pomo's optional AI features:
// nudge lines, review recaps, and the /chat stream. Providers are "anthropic"
// and "openrouter"; an empty provider yields no-op implementations.
package ai

import (
	"context"
	"errors"
	"net/http"
	"time"
)

var ErrNoProvider = errors.New("ai: no provider configured")

type Config struct {
	Provider string
	Key      string
	Model    string
	BaseURL  string
}

type NudgeContext struct {
	Task           string
	Tag            string
	DistractApp    string
	DistractDetail string
	DriftMinutes   int
	SessionMinutes int
	Level          int
	Hour           int
}

type RecapContext struct {
	Label          string
	FocusMinutes   int
	PlannedMinutes int
	Completed      int
	Planned        int
	DriftMinutes   int
	TopDrift       []string
	ByTag          []string
	BestHour       string
	Notes          []string
}

type Msg struct {
	Role    string
	Content string
}

type Nudger interface {
	Line(ctx context.Context, nc NudgeContext) (string, error)
}
type Recapper interface {
	Recap(ctx context.Context, rc RecapContext) (string, error)
}
type Chatter interface {
	Stream(ctx context.Context, system string, msgs []Msg, onDelta func(string)) error
}

func DefaultModel(provider string) string {
	switch provider {
	case "anthropic":
		return "claude-haiku-4-5"
	case "openrouter":
		return "anthropic/claude-3.5-haiku"
	default:
		return ""
	}
}

// New returns the three interfaces for cfg.
func New(cfg Config) (Nudger, Recapper, Chatter) {
	if cfg.Provider != "anthropic" && cfg.Provider != "openrouter" {
		n := noop{}
		return n, n, n
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel(cfg.Provider)
	}
	c := &httpProvider{cfg: cfg, hc: &http.Client{}}
	return c, c, c
}

type noop struct{}

func (noop) Line(context.Context, NudgeContext) (string, error)  { return "", ErrNoProvider }
func (noop) Recap(context.Context, RecapContext) (string, error) { return "", ErrNoProvider }
func (noop) Stream(context.Context, string, []Msg, func(string)) error {
	return ErrNoProvider
}

// httpProvider implements all three interfaces against a real HTTP endpoint.
// Transport details are in anthropic.go / openrouter.go / stream.go.
type httpProvider struct {
	cfg Config
	hc  *http.Client
}

const (
	systemNudge = "You are a terse, non-judgmental focus buddy for a developer. " +
		"Reply with ONE sentence, 15 words max, no emoji, no exclamation marks."
	systemRecap = "You summarise a developer's focus session data. 3 short sentences, " +
		"then one line starting with '→ Try:' with a concrete suggestion. No praise, no fluff."
	systemChat = "You are a terse focus coach for a developer. Ground every answer in the " +
		"supplied session and drift data. 4 sentences max. Be concrete."
)

var (
	nudgeTimeout = 3 * time.Second
	recapTimeout = 8 * time.Second
	chatTimeout  = 30 * time.Second
)
