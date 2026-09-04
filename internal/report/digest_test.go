package report_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pomo/internal/db/dbtest"
	"pomo/internal/model"
	"pomo/internal/report"
)

func TestWriteWeeklyDigest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	d := dbtest.NewTemp(t)
	now := time.Now()
	id, _ := d.CreateSession(model.Session{
		TaskName: "t", PlannedDuration: 1500, Status: model.StatusRunning, StartedAt: now,
	})
	_ = d.FinishSession(id, model.StatusCompleted, 1500, "")

	w := report.ThisWeek()
	path, err := report.WriteWeeklyDigest(d, w, "the recap")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != w.Label+".md" {
		t.Fatalf("path = %q", path)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "the recap") || !strings.Contains(string(b), "# Pomo") {
		t.Fatalf("digest content wrong:\n%s", b)
	}
	if _, err := report.WriteWeeklyDigest(d, w, ""); err != nil {
		t.Fatalf("second write: %v", err)
	}
}
