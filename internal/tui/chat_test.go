package tui

import (
	"context"
	"strings"
	"testing"

	"pomo/internal/ai"
	"pomo/internal/db/dbtest"
)

type fakeChatter struct{ chunks []string }

func (f fakeChatter) Stream(ctx context.Context, system string, msgs []ai.Msg, onDelta func(string)) error {
	for _, c := range f.chunks {
		onDelta(c)
	}
	return nil
}

func mustCmd(name string) slashCommand {
	c, _ := resolveCommand(name)
	return c
}

func TestChatRejectsJunk(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.cfg.AI.Provider = "anthropic"
	a.runChatCommand(nil)
	a.chatInput.SetValue("   ")
	a.updateChat(keyMsg("enter"))
	if a.chatErr == "" {
		t.Fatal("blank message should set chatErr")
	}
	if a.chatStreaming {
		t.Fatal("must not start a stream for rejected input")
	}
}

func TestChatStreamsDeltas(t *testing.T) {
	orig := newChatter
	newChatter = func(ai.Config) ai.Chatter { return fakeChatter{chunks: []string{"hel", "lo"}} }
	defer func() { newChatter = orig }()

	a := NewApp(dbtest.NewTemp(t))
	a.cfg.AI.Provider = "anthropic"
	a.runChatCommand(nil)
	a.chatInput.SetValue("hi")
	m, cmd := a.updateChat(keyMsg("enter"))
	a = m.(*App)
	if cmd == nil {
		t.Fatal("expected a stream command")
	}
	for i := 0; cmd != nil && i < 20; i++ {
		msg := cmd()
		m, cmd = a.Update(msg)
		a = m.(*App)
	}
	if !strings.Contains(a.chat.View(), "hello") {
		t.Fatalf("streamed text missing:\n%s", a.chat.View())
	}
}

func TestChatNoProvider(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.cfg.AI.Provider = ""
	a.runChatCommand(nil)
	if a.screen != screenChat || !strings.Contains(a.viewChat(), "ai.provider") {
		t.Fatalf("no-provider chat should show the hint")
	}
}
