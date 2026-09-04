package report_test

import (
	"testing"
	"time"

	"pomo/internal/report"
)

func TestParseWindowKeywords(t *testing.T) {
	for _, s := range []string{"", "today", "week", "month"} {
		w, err := report.ParseWindow(s)
		if err != nil {
			t.Fatalf("ParseWindow(%q): %v", s, err)
		}
		if !w.To.After(w.From) {
			t.Fatalf("ParseWindow(%q): To %v not after From %v", s, w.To, w.From)
		}
	}
}

func TestParseWindowISOWeek(t *testing.T) {
	w, err := report.ParseWindow("2026-W36")
	if err != nil {
		t.Fatalf("ParseWindow: %v", err)
	}
	// ISO week 36 of 2026 starts Monday 2026-08-31.
	wantFrom := time.Date(2026, 8, 31, 0, 0, 0, 0, time.Local)
	if !w.From.Equal(wantFrom) {
		t.Fatalf("From = %v, want %v", w.From, wantFrom)
	}
	if w.To.Sub(w.From) != 7*24*time.Hour {
		t.Fatalf("week span = %v, want 168h", w.To.Sub(w.From))
	}
	if w.Label != "2026-W36" {
		t.Fatalf("Label = %q", w.Label)
	}
}

func TestParseWindowMonth(t *testing.T) {
	w, err := report.ParseWindow("2026-02")
	if err != nil {
		t.Fatalf("ParseWindow: %v", err)
	}
	if w.From != time.Date(2026, 2, 1, 0, 0, 0, 0, time.Local) ||
		w.To != time.Date(2026, 3, 1, 0, 0, 0, 0, time.Local) {
		t.Fatalf("Feb 2026 window wrong: %+v", w)
	}
}

func TestParseWindowGarbage(t *testing.T) {
	if _, err := report.ParseWindow("last-tuesday"); err == nil {
		t.Fatal("expected error for garbage input")
	}
}
