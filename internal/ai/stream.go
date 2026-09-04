package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (p *httpProvider) Stream(ctx context.Context, system string, msgs []Msg, onDelta func(string)) error {
	ctx, cancel := context.WithTimeout(ctx, chatTimeout)
	defer cancel()

	var url string
	var reqBody []byte
	switch p.cfg.Provider {
	case "anthropic":
		url = p.base() + "/v1/messages"
		m := make([]map[string]string, 0, len(msgs))
		for _, x := range msgs {
			m = append(m, map[string]string{"role": x.Role, "content": x.Content})
		}
		reqBody, _ = json.Marshal(map[string]any{
			"model": p.cfg.Model, "max_tokens": 400, "system": system,
			"messages": m, "stream": true,
		})
	case "openrouter":
		url = p.base() + "/api/v1/chat/completions"
		m := []map[string]string{{"role": "system", "content": system}}
		for _, x := range msgs {
			m = append(m, map[string]string{"role": x.Role, "content": x.Content})
		}
		reqBody, _ = json.Marshal(map[string]any{
			"model": p.cfg.Model, "max_tokens": 400, "messages": m, "stream": true,
		})
	default:
		return ErrNoProvider
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	if p.cfg.Provider == "anthropic" {
		req.Header.Set("x-api-key", p.cfg.Key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+p.cfg.Key)
		req.Header.Set("HTTP-Referer", "https://github.com/pomo")
		req.Header.Set("X-Title", "pomo")
	}

	resp, err := p.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 400))
		msg := fmt.Sprintf("%s: %s: %s", p.cfg.Provider, resp.Status, strings.TrimSpace(string(body)))
		if resp.StatusCode == 404 && p.cfg.Provider == "openrouter" {
			msg += fmt.Sprintf("  (model %q not on OpenRouter — set a valid slug via ai.model, or switch provider to anthropic if your key is sk-ant-…)", p.cfg.Model)
		}
		return errors.New(msg)
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 8192), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			return nil
		}
		if p.cfg.Provider == "anthropic" {
			var ev struct {
				Type  string `json:"type"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			}
			if json.Unmarshal([]byte(payload), &ev) != nil {
				continue
			}
			if ev.Type == "message_stop" {
				return nil
			}
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				onDelta(ev.Delta.Text)
			}
		} else {
			var ev struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(payload), &ev) != nil {
				continue
			}
			if len(ev.Choices) > 0 && ev.Choices[0].Delta.Content != "" {
				onDelta(ev.Choices[0].Delta.Content)
			}
		}
	}
	return sc.Err()
}
