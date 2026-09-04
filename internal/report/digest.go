package report

import (
	"os"
	"path/filepath"

	"pomo/internal/db"
)

// WeeklyDigestPath is where a week's digest markdown lives.
func WeeklyDigestPath(label string) string {
	return filepath.Join(db.Dir(), "reviews", label+".md")
}

// WriteWeeklyDigest builds the summary for window w and writes the digest
// markdown (with an optional recap) to WeeklyDigestPath(w.Label). Idempotent.
func WriteWeeklyDigest(d *db.DB, w Window, recap string) (string, error) {
	sum, err := Build(d, w)
	if err != nil {
		return "", err
	}
	path := WeeklyDigestPath(w.Label)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(RenderMarkdown(sum, recap)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
