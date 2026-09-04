package db_test

import (
	"testing"
	"time"

	"pomo/internal/db/dbtest"
)

func TestDriftEpisodeLifecycle(t *testing.T) {
	d := dbtest.NewTemp(t)
	start := time.Date(2026, 9, 4, 10, 0, 0, 0, time.UTC)

	id, err := d.OpenDriftEpisode(1, start, "foreground", "Google Chrome")
	if err != nil {
		t.Fatalf("OpenDriftEpisode: %v", err)
	}
	if err := d.UpdateDriftEpisode(id, 45, "Google Chrome — reddit.com"); err != nil {
		t.Fatalf("UpdateDriftEpisode: %v", err)
	}
	if err := d.CloseDriftEpisode(id, start.Add(90*time.Second), 90); err != nil {
		t.Fatalf("CloseDriftEpisode: %v", err)
	}

	got, err := d.ListDriftEvents(start.Add(-time.Hour), start.Add(time.Hour))
	if err != nil {
		t.Fatalf("ListDriftEvents: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d events, want 1", len(got))
	}
	e := got[0]
	if e.SessionID != 1 || e.Seconds != 90 || e.Trigger != "foreground" ||
		e.Detail != "Google Chrome — reddit.com" || e.EndedAt == nil {
		t.Fatalf("unexpected event: %+v", e)
	}
}

func TestListDriftEventsWindowExcludesOutside(t *testing.T) {
	d := dbtest.NewTemp(t)
	base := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	if _, err := d.OpenDriftEpisode(1, base.Add(-2*time.Hour), "fs_stale", ""); err != nil {
		t.Fatal(err)
	}
	inWindow, err := d.OpenDriftEpisode(1, base.Add(10*time.Minute), "fs_stale", "")
	if err != nil {
		t.Fatal(err)
	}
	got, err := d.ListDriftEvents(base, base.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != inWindow {
		t.Fatalf("window filter wrong: got %+v", got)
	}
}
