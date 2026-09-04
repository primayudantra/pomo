package report_test

import (
	"testing"
	"time"

	"pomo/internal/db"
	"pomo/internal/db/dbtest"
	"pomo/internal/model"
	"pomo/internal/report"
)

func seedFinished(t *testing.T, d *db.DB, start time.Time, secs int) {
	t.Helper()
	id, err := d.CreateSession(model.Session{
		TaskName: "x", PlannedDuration: 1500, Status: model.StatusRunning, StartedAt: start,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.FinishSession(id, model.StatusCompleted, secs, ""); err != nil {
		t.Fatal(err)
	}
}

func midnightLocal(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func TestLoadDayStatsAndStreaks(t *testing.T) {
	d := dbtest.NewTemp(t)
	now := time.Now()
	seedFinished(t, d, midnightLocal(now.AddDate(0, 0, -2)).Add(9*time.Hour), 1500)
	seedFinished(t, d, midnightLocal(now.AddDate(0, 0, -1)).Add(9*time.Hour), 1500)
	seedFinished(t, d, midnightLocal(now).Add(9*time.Hour), 1500)

	stats := report.LoadDayStats(d)
	if len(stats) != 3 {
		t.Fatalf("got %d day buckets, want 3", len(stats))
	}
	cur, longest := report.Streaks(stats)
	if cur != 3 || longest != 3 {
		t.Fatalf("streaks: current=%d longest=%d, want 3/3", cur, longest)
	}
}
