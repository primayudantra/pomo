package notify

import (
	"strings"
	"testing"
)

func TestCommandIncludesTitleAndBody(t *testing.T) {
	argv := command("Focus", "back to it")
	if argv == nil {
		t.Skip("no notifier binary on this machine")
	}
	joined := strings.Join(argv, " ")
	if !strings.Contains(joined, "Focus") || !strings.Contains(joined, "back to it") {
		t.Fatalf("argv missing title/body: %v", argv)
	}
	if strings.Contains(joined, "display dialog") {
		t.Fatalf("notifier must not use a blocking dialog: %v", argv)
	}
}

func TestSendNeverErrorsHard(t *testing.T) {
	if err := New().Send("t", "b"); err != nil {
		t.Fatalf("Send returned %v, want nil", err)
	}
}
