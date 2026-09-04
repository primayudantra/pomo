package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (p *httpProvider) base() string {
	if p.cfg.BaseURL != "" {
		return p.cfg.BaseURL
	}
	switch p.cfg.Provider {
	case "openrouter":
		return "https://openrouter.ai"
	default:
		return "https://api.anthropic.com"
	}
}

func (p *httpProvider) Line(ctx context.Context, nc NudgeContext) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, nudgeTimeout)
	defer cancel()
	return p.complete(ctx, systemNudge, renderNudgeUser(nc), 120)
}

func (p *httpProvider) Recap(ctx context.Context, rc RecapContext) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, recapTimeout)
	defer cancel()
	return p.complete(ctx, systemRecap, renderRecapUser(rc), 250)
}

// complete runs one non-streaming completion via the configured provider.
func (p *httpProvider) complete(ctx context.Context, system, user string, maxTokens int) (string, error) {
	switch p.cfg.Provider {
	case "anthropic":
		return p.anthropicComplete(ctx, system, user, maxTokens)
	case "openrouter":
		return p.openrouterComplete(ctx, system, user, maxTokens)
	default:
		return "", ErrNoProvider
	}
}

func (p *httpProvider) anthropicComplete(ctx context.Context, system, user string, maxTokens int) (string, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"model":      p.cfg.Model,
		"max_tokens": maxTokens,
		"system":     system,
		"messages":   []map[string]string{{"role": "user", "content": user}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base()+"/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", p.cfg.Key)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("anthropic: %s: %s", resp.Status, truncate(string(body), 200))
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("anthropic: bad response: %w", err)
	}
	for _, c := range out.Content {
		if c.Type == "text" && c.Text != "" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic: empty response")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// --- temporary stubs, removed in Tasks 6 and 7 ---
func (p *httpProvider) openrouterComplete(ctx context.Context, system, user string, maxTokens int) (string, error) {
	return "", fmt.Errorf("openrouter: not implemented")
}

func (p *httpProvider) Stream(ctx context.Context, system string, msgs []Msg, onDelta func(string)) error {
	return fmt.Errorf("stream: not implemented")
}
