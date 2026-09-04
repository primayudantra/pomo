package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"pomo/internal/db/dbtest"
	"pomo/internal/model"
)

func TestRunReviewText(t *testing.T) {
	d := dbtest.NewTemp(t)
	database = d
	t.Cleanup(func() { database = nil })

	now := time.Now()
	id, err := d.CreateSession(model.Session{
		TaskName: "t", Tag: "backend", PlannedDuration: 1500,
		Status: model.StatusRunning, StartedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.FinishSession(id, model.StatusCompleted, 1500, ""); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := runReview([]string{"today"}, false, &buf); err != nil {
		t.Fatalf("runReview: %v", err)
	}
	if !strings.Contains(buf.String(), "1 done") {
		t.Fatalf("output missing '1 done':\n%s", buf.String())
	}
}

func TestRunReviewJSON(t *testing.T) {
	d := dbtest.NewTemp(t)
	database = d
	t.Cleanup(func() { database = nil })

	var buf bytes.Buffer
	if err := runReview([]string{"today"}, true, &buf); err != nil {
		t.Fatalf("runReview: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(buf.String()), "{") {
		t.Fatalf("expected JSON object, got:\n%s", buf.String())
	}
}

func TestRunReviewBadWindow(t *testing.T) {
	d := dbtest.NewTemp(t)
	database = d
	t.Cleanup(func() { database = nil })
	if err := runReview([]string{"not-a-window"}, false, new(bytes.Buffer)); err == nil {
		t.Fatal("expected error for bad window")
	}
}
