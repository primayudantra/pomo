//go:build windows

package daemon

import "errors"

var errUnsupported = errors.New("daemon control is not supported on this platform")

// PidAlive is unsupported on Windows.
func PidAlive(pid int) bool { return false }

// Spawn is unsupported on Windows.
func Spawn() (int, error) { return 0, errUnsupported }

// Stop is unsupported on Windows.
func Stop() (int, error) { return 0, errUnsupported }
