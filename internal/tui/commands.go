package tui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/sahilm/fuzzy"
)

type slashCommand struct {
	Name         string
	Aliases      []string
	Help         string
	NeedsSession bool
	NeedsDaemon  bool
}

var slashCommands = []slashCommand{
	{Name: "/review", Aliases: []string{"/recap"}, Help: "focus vs plan + drift for a period"},
	{Name: "/insights", Help: "/review plus an AI recap"},
	{Name: "/drift", Help: "drift so far this session (or today)"},
	{Name: "/chat", Aliases: []string{"/ask"}, Help: "talk to your focus coach"},
	{Name: "/start", Help: "start a session on a task"},
	{Name: "/skip", Help: "skip the running session", NeedsSession: true},
	{Name: "/settings", Aliases: []string{"/config"}, Help: "open settings"},
	{Name: "/help", Aliases: []string{"/?"}, Help: "list commands"},
	{Name: "/quit", Aliases: []string{"/q"}, Help: "quit pomo"},
}

func norm(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	return strings.TrimPrefix(name, "/")
}

func resolveCommand(name string) (slashCommand, bool) {
	n := norm(name)
	for _, c := range slashCommands {
		if norm(c.Name) == n {
			return c, true
		}
		for _, a := range c.Aliases {
			if norm(a) == n {
				return c, true
			}
		}
	}
	return slashCommand{}, false
}

func filterCommands(q string) []slashCommand {
	q = strings.TrimSpace(strings.ToLower(q))
	if q == "" {
		if len(slashCommands) <= 6 {
			return append([]slashCommand(nil), slashCommands...)
		}
		return append([]slashCommand(nil), slashCommands[:6]...)
	}
	names := make([]string, len(slashCommands))
	for i, c := range slashCommands {
		names[i] = norm(c.Name)
	}
	matches := fuzzy.Find(q, names)
	out := make([]slashCommand, 0, 6)
	for _, m := range matches {
		out = append(out, slashCommands[m.Index])
		if len(out) == 6 {
			break
		}
	}
	return out
}

const maxChatInput = 2000

// guardChatInput normalises and validates a /chat message before it reaches
// the model: trims, collapses 3+ newlines, rejects empty / over-long /
// control-char-heavy input.
func guardChatInput(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty message")
	}
	if len([]rune(s)) > maxChatInput {
		return "", fmt.Errorf("message too long (max %d characters)", maxChatInput)
	}
	bad, total := 0, 0
	for _, r := range s {
		total++
		if r == '\n' || r == '\t' {
			continue
		}
		if !unicode.IsPrint(r) {
			bad++
		}
	}
	if total > 0 && bad*5 > total {
		return "", fmt.Errorf("message contains too many non-text characters")
	}
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s, nil
}
