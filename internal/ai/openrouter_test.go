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

func TestOpenRouterLine(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"close the tab"}}]}`)
	}))
	defer srv.Close()

	n, _, _ := ai.New(ai.Config{Provider: "openrouter", Key: "or-key", BaseURL: srv.URL})
	line, err := n.Line(context.Background(), ai.NudgeContext{Task: "ship it", DriftMinutes: 12})
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	if line != "close the tab" {
		t.Fatalf("line = %q", line)
	}
	if gotAuth != "Bearer or-key" {
		t.Fatalf("auth = %q", gotAuth)
	}
	var body struct {
		Model     string           `json:"model"`
		MaxTokens int              `json:"max_tokens"`
		Messages  []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatal(err)
	}
	if body.Model != "anthropic/claude-3.5-haiku" || body.MaxTokens != 120 {
		t.Fatalf("model/max = %q/%d", body.Model, body.MaxTokens)
	}
	if len(body.Messages) != 2 || body.Messages[0]["role"] != "system" {
		t.Fatalf("messages shape wrong: %v", body.Messages)
	}
	if !strings.Contains(gotBody, "ship it") {
		t.Fatalf("user content missing: %s", gotBody)
	}
}

func TestOpenRouterHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	_, r, _ := ai.New(ai.Config{Provider: "openrouter", Key: "k", BaseURL: srv.URL})
	if _, err := r.Recap(context.Background(), ai.RecapContext{Label: "today"}); err == nil {
		t.Fatal("expected error on 500")
	}
}
