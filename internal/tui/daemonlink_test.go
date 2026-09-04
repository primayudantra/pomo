package tui

import (
	"testing"

	"pomo/internal/db/dbtest"
	"pomo/internal/ipc"
)

func TestDaemonEventUpdatesState(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))

	m, _ := a.Update(daemonEventMsg{ipc.Event{Type: "watching", SessionID: 3}})
	a = m.(*App)
	if !a.daemonUp {
		t.Fatal("watching event should set daemonUp")
	}

	m, _ = a.Update(daemonEventMsg{ipc.Event{Type: "nudge", Text: "still on it?", Level: 1, Actions: []string{"snooze", "drifted"}}})
	a = m.(*App)
	if a.nudgeOverlay == nil || a.nudgeOverlay.text != "still on it?" {
		t.Fatalf("nudge event should set the overlay: %+v", a.nudgeOverlay)
	}

	m, _ = a.Update(daemonEventMsg{ipc.Event{Type: "checkpoint", Text: "on task?"}})
	a = m.(*App)
	if !a.checkpointActive {
		t.Fatal("checkpoint event should raise the checkpoint overlay")
	}
}

func TestCheckpointAnswerClearsOverlay(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.screen = screenTimer
	a.checkpointActive = true
	a.updateTimer(keyMsg("y")) // no ipcClient; must not panic, must clear
	if a.checkpointActive {
		t.Fatal("answering the checkpoint should clear it")
	}
}
