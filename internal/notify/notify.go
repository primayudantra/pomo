// Package notify sends fire-and-forget desktop notifications for the daemon.
// It never opens a blocking GUI dialog and never returns an error for
// ordinary input — a missing notifier binary is a silent no-op.
package notify

import (
	"context"
	"os/exec"
	"time"
)

type Notifier interface {
	Send(title, body string) error
}

type execNotifier struct{}

func New() Notifier { return execNotifier{} }

func (execNotifier) Send(title, body string) error {
	argv := command(title, body)
	if argv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = exec.CommandContext(ctx, argv[0], argv[1:]...).Run()
	return nil
}

func onPath(bin string) bool {
	_, err := exec.LookPath(bin)
	return err == nil
}
