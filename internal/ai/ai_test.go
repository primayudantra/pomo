package ai_test

import (
	"context"
	"errors"
	"testing"

	"pomo/internal/ai"
)

func TestNoProviderIsNoOp(t *testing.T) {
	n, r, c := ai.New(ai.Config{})

	if _, err := n.Line(context.Background(), ai.NudgeContext{Task: "x"}); !errors.Is(err, ai.ErrNoProvider) {
		t.Errorf("Line err = %v, want ErrNoProvider", err)
	}
	if _, err := r.Recap(context.Background(), ai.RecapContext{Label: "today"}); !errors.Is(err, ai.ErrNoProvider) {
		t.Errorf("Recap err = %v, want ErrNoProvider", err)
	}
	called := false
	err := c.Stream(context.Background(), "sys", []ai.Msg{{Role: "user", Content: "hi"}}, func(string) { called = true })
	if !errors.Is(err, ai.ErrNoProvider) {
		t.Errorf("Stream err = %v, want ErrNoProvider", err)
	}
	if called {
		t.Error("Stream should not call onDelta with no provider")
	}
}

func TestDefaultModel(t *testing.T) {
	if ai.DefaultModel("anthropic") != "claude-haiku-4-5" {
		t.Errorf("anthropic default = %q", ai.DefaultModel("anthropic"))
	}
	if ai.DefaultModel("openrouter") != "anthropic/claude-3.5-haiku" {
		t.Errorf("openrouter default = %q", ai.DefaultModel("openrouter"))
	}
}
