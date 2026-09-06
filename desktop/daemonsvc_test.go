package main

import (
	"errors"
	"testing"
)

type fakeCtl struct {
	pid     int
	up      bool
	spawnErr error
}

func (f *fakeCtl) spawn() (int, error) {
	if f.spawnErr != nil {
		return 0, f.spawnErr
	}
	f.up, f.pid = true, 111
	return 111, nil
}
func (f *fakeCtl) stop() (int, error) { p := f.pid; f.up, f.pid = false, 0; return p, nil }
func (f *fakeCtl) running() (int, bool) { return f.pid, f.up }

func TestSetTracking(t *testing.T) {
	s := &DaemonService{ctl: &fakeCtl{}}
	if s.Status().Running {
		t.Fatal("should start down")
	}
	st, err := s.SetTracking(true)
	if err != nil || !st.Running || st.PID != 111 {
		t.Fatalf("on: %+v %v", st, err)
	}
	st, _ = s.SetTracking(false)
	if st.Running {
		t.Fatal("should be down after off")
	}
}

func TestSetTrackingSpawnError(t *testing.T) {
	s := &DaemonService{ctl: &fakeCtl{spawnErr: errors.New("no binary")}}
	if _, err := s.SetTracking(true); err == nil {
		t.Fatal("expected spawn error to propagate")
	}
}
