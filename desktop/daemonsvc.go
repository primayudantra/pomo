package main

import (
	"pomo/internal/daemon"
)

// DaemonStatus represents the current state of the daemon.
type DaemonStatus struct {
	Running bool `json:"running"`
	PID     int  `json:"pid"`
}

// controller is the seam for tests.
type controller interface {
	spawn() (int, error)
	stop() (int, error)
	running() (int, bool)
}

// realCtl implements controller using the actual daemon package.
type realCtl struct{}

func (realCtl) spawn() (int, error)   { return daemon.Spawn() }
func (realCtl) stop() (int, error)    { return daemon.Stop() }
func (realCtl) running() (int, bool)  { return daemon.Running() }

// DaemonService provides methods to control the daemon.
type DaemonService struct {
	ctl controller
}

// NewDaemonService creates a new DaemonService wired to the real daemon.
func NewDaemonService() *DaemonService {
	return &DaemonService{ctl: realCtl{}}
}

// Status returns the current daemon status.
func (s *DaemonService) Status() DaemonStatus {
	pid, ok := s.ctl.running()
	return DaemonStatus{Running: ok, PID: pid}
}

// SetTracking turns daemon tracking on or off.
func (s *DaemonService) SetTracking(on bool) (DaemonStatus, error) {
	if on {
		if _, err := s.ctl.spawn(); err != nil {
			return s.Status(), err
		}
	} else {
		if _, err := s.ctl.stop(); err != nil {
			return s.Status(), err
		}
	}
	return s.Status(), nil
}
