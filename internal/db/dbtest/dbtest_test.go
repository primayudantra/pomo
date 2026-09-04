package dbtest_test

import (
	"testing"

	"pomo/internal/db/dbtest"
)

func TestNewTempIsUsable(t *testing.T) {
	d := dbtest.NewTemp(t)
	if _, err := d.AddTask("hello", ""); err != nil {
		t.Fatalf("AddTask on temp db: %v", err)
	}
	tasks, err := d.ListOpenTasks()
	if err != nil {
		t.Fatalf("ListOpenTasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Name != "hello" {
		t.Fatalf("got %+v, want one task named hello", tasks)
	}
}
