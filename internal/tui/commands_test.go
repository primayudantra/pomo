package tui

import (
	"strings"
	"testing"
)

func TestResolveCommand(t *testing.T) {
	if _, ok := resolveCommand("/review"); !ok {
		t.Error("/review should resolve")
	}
	if _, ok := resolveCommand("recap"); !ok {
		t.Error("alias 'recap' (no slash) should resolve")
	}
	if _, ok := resolveCommand("/nope"); ok {
		t.Error("/nope should not resolve")
	}
}

func TestFilterCommands(t *testing.T) {
	all := filterCommands("")
	if len(all) == 0 || len(all) > 6 {
		t.Fatalf("empty query returned %d", len(all))
	}
	got := filterCommands("rev")
	found := false
	for _, c := range got {
		if c.Name == "/review" {
			found = true
		}
	}
	if !found {
		t.Errorf("filter 'rev' missing /review: %+v", got)
	}
}

func TestGuardChatInput(t *testing.T) {
	if _, err := guardChatInput("   \n  "); err == nil {
		t.Error("blank input should be rejected")
	}
	if _, err := guardChatInput(strings.Repeat("a", maxChatInput+1)); err == nil {
		t.Error("over-long input should be rejected")
	}
	if _, err := guardChatInput("why do I keep drifting?"); err != nil {
		t.Errorf("normal input rejected: %v", err)
	}
	junk := "hi\x00\x01\x02\x03\x04\x05\x06\x07\x08 there \x0e\x0f\x10\x11\x12"
	if _, err := guardChatInput(junk); err == nil {
		t.Error("control-char junk should be rejected")
	}
	clean, err := guardChatInput("  hello\n\n\n\nworld  ")
	if err != nil || clean != "hello\n\nworld" {
		t.Errorf("normalise = %q, %v", clean, err)
	}
}
