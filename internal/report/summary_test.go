package report_test

import (
	"testing"
	"time"

	"pomo/internal/db/dbtest"
	"pomo/internal/model"
	"pomo/internal/report"
)

func TestBuildAggregates(t *testing.T) {
	d := dbtest.NewTemp(t)
	day := time.Date(2026, 9, 4, 0, 0, 0, 0, time.Local)
	w := report.Window{From: day, To: day.AddDate(0, 0, 1), Label: "test"}

	mk := func(status model.SessionStatus, tag string, hour, planned, actual int) int64 {
		id, err := d.CreateSession(model.Session{
			TaskName: "t", Tag: tag, PlannedDuration: planned,
			Status: model.StatusRunning, StartedAt: day.Add(time.Duration(hour) * time.Hour),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := d.FinishSession(id, status, actual, ""); err != nil {
			t.Fatal(err)
		}
		return id
	}
	s1 := mk(model.StatusCompleted, "backend", 10, 1500, 1500)
	mk(model.StatusCompleted, "backend", 11, 1500, 1200)
	mk(model.StatusCancelled, "docs", 14, 1500, 300)

	id, _ := d.OpenDriftEpisode(s1, day.Add(10*time.Hour+5*time.Minute), "foreground", "Google Chrome — reddit.com")
	d.CloseDriftEpisode(id, day.Add(10*time.Hour+33*time.Minute), 28*60)
	id2, _ := d.OpenDriftEpisode(s1, day.Add(10*time.Hour+40*time.Minute), "foreground", "Slack")
	d.CloseDriftEpisode(id2, day.Add(10*time.Hour+49*time.Minute), 9*60)

	sum, err := report.Build(d, w)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if sum.Planned != 3 || sum.Completed != 2 || sum.Cancelled != 1 {
		t.Fatalf("counts: %+v", sum)
	}
	if sum.FocusSeconds != 2700 {
		t.Fatalf("FocusSeconds = %d, want 2700", sum.FocusSeconds)
	}
	if sum.PlannedSeconds != 4500 {
		t.Fatalf("PlannedSeconds = %d, want 4500", sum.PlannedSeconds)
	}
	if sum.DriftSeconds != 37*60 || sum.DriftEpisodes != 2 {
		t.Fatalf("drift: %ds / %d episodes", sum.DriftSeconds, sum.DriftEpisodes)
	}
	if len(sum.DriftByApp) != 2 || sum.DriftByApp[0].App != "Google Chrome" || sum.DriftByApp[0].Seconds != 28*60 {
		t.Fatalf("DriftByApp = %+v", sum.DriftByApp)
	}
	if len(sum.ByTag) == 0 || sum.ByTag[0].Tag != "backend" || sum.ByTag[0].FocusSeconds != 2700 {
		t.Fatalf("ByTag = %+v", sum.ByTag)
	}
	if sum.BestHour == nil || sum.BestHour.Hour != 10 {
		t.Fatalf("BestHour = %+v, want hour 10", sum.BestHour)
	}
}

func TestBuildEmptyWindow(t *testing.T) {
	d := dbtest.NewTemp(t)
	sum, err := report.Build(d, report.Window{
		From: time.Now().AddDate(0, 0, -1), To: time.Now(), Label: "empty",
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if sum.Planned != 0 || sum.BestHour != nil || len(sum.DriftByApp) != 0 {
		t.Fatalf("empty window not empty: %+v", sum)
	}
}
