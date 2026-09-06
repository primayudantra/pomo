package cmd

import "testing"

// Process-control helpers moved to internal/daemon; see
// internal/daemon/control_test.go for their coverage.
func TestDaemonCommandsRegistered(t *testing.T) {
	subs := map[string]bool{}
	for _, c := range daemonCmd.Commands() {
		subs[c.Name()] = true
	}
	for _, want := range []string{"run", "start", "stop", "status"} {
		if !subs[want] {
			t.Errorf("daemon subcommand %q not registered", want)
		}
	}
}
