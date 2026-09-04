package report_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"pomo/internal/report"
)

func sampleSummary() report.Summary {
	day := time.Date(2026, 9, 4, 0, 0, 0, 0, time.Local)
	return report.Summary{
		Window:         report.Window{From: day, To: day.AddDate(0, 0, 1), Label: "today"},
		Planned:        4,
		Completed:      3,
		Cancelled:      1,
		FocusSeconds:   62 * 60,
		PlannedSeconds: 100 * 60,
		DriftSeconds:   41 * 60,
		DriftEpisodes:  5,
		DriftByApp:     []report.AppDrift{{App: "Google Chrome", Seconds: 28 * 60}, {App: "Slack", Seconds: 9 * 60}, {App: "idle", Seconds: 4 * 60}},
		ByTag:          []report.TagStat{{Tag: "backend", FocusSeconds: 38 * 60}, {Tag: "docs", FocusSeconds: 15 * 60}},
		Streak:         4,
		BestHour:       &report.HourStat{Hour: 10, FocusSeconds: 18 * 60, DriftSeconds: 0},
	}
}

func TestFmtDur(t *testing.T) {
	cases := map[int]string{0: "0m", 12 * 60: "12m", 62 * 60: "1h02m", 3600: "1h00m"}
	for secs, want := range cases {
		if got := report.FmtDur(secs); got != want {
			t.Errorf("FmtDur(%d) = %q, want %q", secs, got, want)
		}
	}
}

func TestRenderTextContainsKeyNumbers(t *testing.T) {
	out := report.RenderText(sampleSummary())
	for _, want := range []string{"today", "3 done", "1h02m", "41m", "Google Chrome", "backend", "10:00", "streak 4"} {
		if !strings.Contains(out, want) {
			t.Errorf("RenderText missing %q\n---\n%s", want, out)
		}
	}
}

func TestRenderMarkdownOmitsRecapWhenEmpty(t *testing.T) {
	with := report.RenderMarkdown(sampleSummary(), "some recap")
	without := report.RenderMarkdown(sampleSummary(), "")
	if !strings.Contains(with, "## Recap") || !strings.Contains(with, "some recap") {
		t.Error("recap section missing when recap provided")
	}
	if strings.Contains(without, "## Recap") {
		t.Error("recap section present when recap empty")
	}
}

func TestJSONStableKeys(t *testing.T) {
	b, err := sampleSummary().JSON()
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"Window", "Planned", "Completed", "FocusSeconds", "DriftByApp", "ByTag", "Streak", "BestHour"} {
		if _, ok := m[k]; !ok {
			t.Errorf("JSON missing key %q", k)
		}
	}
}
