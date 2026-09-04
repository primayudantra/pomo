package ai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pomo/internal/ai"
)

func TestAnthropicLine(t *testing.T) {
	var gotAuth, gotVersion, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"content":[{"type":"text","text":"take a lap and come back"}]}`)
	}))
	defer srv.Close()

	n, _, _ := ai.New(ai.Config{Provider: "anthropic", Key: "sk-ant-test", BaseURL: srv.URL})
	line, err := n.Line(context.Background(), ai.NudgeContext{
		Task: "fix bug", DistractApp: "Google Chrome", DriftMinutes: 20, Level: 2, Hour: 15,
	})
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	if line != "take a lap and come back" {
		t.Fatalf("line = %q", line)
	}
	if gotAuth != "sk-ant-test" || gotVersion != "2023-06-01" {
		t.Fatalf("headers: auth=%q version=%q", gotAuth, gotVersion)
	}
	var body struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		System    string `json:"system"`
	}
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatalf("body not json: %v", err)
	}
	if body.Model != "claude-haiku-4-5" || body.MaxTokens != 120 {
		t.Fatalf("body model/max = %q/%d", body.Model, body.MaxTokens)
	}
	if !strings.Contains(body.System, "focus buddy") {
		t.Fatalf("system prompt missing: %q", body.System)
	}
	if !strings.Contains(gotBody, "fix bug") || !strings.Contains(gotBody, "Google Chrome") {
		t.Fatalf("user payload missing context: %s", gotBody)
	}
}

func TestAnthropicRecap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":[{"type":"text","text":"you drifted a lot.\n→ Try: mornings"}]}`)
	}))
	defer srv.Close()

	_, r, _ := ai.New(ai.Config{Provider: "anthropic", Key: "k", BaseURL: srv.URL})
	out, err := r.Recap(context.Background(), ai.RecapContext{Label: "today", FocusMinutes: 62, DriftMinutes: 41})
	if err != nil {
		t.Fatalf("Recap: %v", err)
	}
	if !strings.Contains(out, "→ Try:") {
		t.Fatalf("recap = %q", out)
	}
}

func TestAnthropicHTTPErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		io.WriteString(w, `{"error":{"message":"rate limited"}}`)
	}))
	defer srv.Close()

	n, _, _ := ai.New(ai.Config{Provider: "anthropic", Key: "k", BaseURL: srv.URL})
	if _, err := n.Line(context.Background(), ai.NudgeContext{Task: "x"}); err == nil {
		t.Fatal("expected error on 429")
	}
}
