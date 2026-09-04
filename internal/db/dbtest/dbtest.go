// Package dbtest provides a throwaway pomo database for unit tests.
package dbtest

import (
	"path/filepath"
	"testing"

	"pomo/internal/db"
)

// NewTemp returns a migrated *db.DB backed by a file under t.TempDir().
// It is closed automatically when the test finishes.
func NewTemp(t *testing.T) *db.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pomo.db")
	d, err := db.OpenAt(path)
	if err != nil {
		t.Fatalf("dbtest.NewTemp: %v", err)
	}
	t.Cleanup(func() { d.Close() })
	return d
}
