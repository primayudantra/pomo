package ai_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pomo/internal/ai"
)

func TestStreamAnthropic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		lines := []string{
			`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hel"}}`,
			`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"lo"}}`,
			`data: {"type":"message_stop"}`,
		}
		for _, l := range lines {
			io.WriteString(w, l+"\n\n")
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer srv.Close()

	_, _, c := ai.New(ai.Config{Provider: "anthropic", Key: "k", BaseURL: srv.URL})
	var got strings.Builder
	err := c.Stream(context.Background(), "sys", []ai.Msg{{Role: "user", Content: "hi"}}, func(d string) {
		got.WriteString(d)
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got.String() != "hello" {
		t.Fatalf("streamed %q, want 'hello'", got.String())
	}
}

func TestStreamOpenRouter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fl, _ := w.(http.Flusher)
		for _, l := range []string{
			`data: {"choices":[{"delta":{"content":"wor"}}]}`,
			`data: {"choices":[{"delta":{"content":"ld"}}]}`,
			`data: [DONE]`,
		} {
			io.WriteString(w, l+"\n\n")
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer srv.Close()

	_, _, c := ai.New(ai.Config{Provider: "openrouter", Key: "k", BaseURL: srv.URL})
	var got strings.Builder
	if err := c.Stream(context.Background(), "sys", []ai.Msg{{Role: "user", Content: "hi"}}, func(d string) { got.WriteString(d) }); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got.String() != "world" {
		t.Fatalf("streamed %q", got.String())
	}
}

func TestStreamHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()
	_, _, c := ai.New(ai.Config{Provider: "anthropic", Key: "k", BaseURL: srv.URL})
	called := false
	err := c.Stream(context.Background(), "s", []ai.Msg{{Role: "user", Content: "x"}}, func(string) { called = true })
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if called {
		t.Fatal("onDelta called despite error")
	}
}
