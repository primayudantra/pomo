package cmd

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"pomo/internal/ai"
	"pomo/internal/daemon"
	"pomo/internal/db"
	"pomo/internal/ipc"
	"pomo/internal/notify"
	"pomo/internal/pomoconfig"
	"pomo/internal/watch"
)

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "Background drift-detection daemon",
}

var daemonRunCmd = &cobra.Command{
	Use:   "run",
	Short: "Run the drift loop in the foreground (used by `daemon start`)",
	RunE:  runDaemon,
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the daemon as a detached background process",
	RunE: func(c *cobra.Command, args []string) error {
		pid, err := daemon.Spawn()
		if err != nil {
			return err
		}
		fmt.Printf("daemon running (pid %d), logging to %s\n", pid, daemon.LogPath())
		return nil
	},
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the daemon",
	RunE: func(c *cobra.Command, args []string) error {
		pid, err := daemon.Stop()
		if err != nil {
			return err
		}
		if pid == 0 {
			fmt.Println("daemon not running")
		} else {
			fmt.Printf("daemon stopped (pid %d)\n", pid)
		}
		return nil
	},
}

var daemonStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show daemon status",
	RunE: func(c *cobra.Command, args []string) error {
		pid, ok := daemon.Running()
		if !ok {
			fmt.Println("daemon: not running")
			return nil
		}
		fmt.Printf("daemon: running (pid %d)\n", pid)
		st, has := daemon.ReadStatus()
		if !has {
			fmt.Println("  (no status file yet)")
			return nil
		}
		fmt.Printf("  watch backend  %s (supported: %t)\n", st.WatchBackend, st.WatchSupported)
		if !st.LastTick.IsZero() {
			fmt.Printf("  last tick      %s ago\n", time.Since(st.LastTick).Round(time.Second))
		}
		if st.WatchingID != 0 {
			fmt.Printf("  watching       session #%d\n", st.WatchingID)
		} else {
			fmt.Println("  watching       (idle)")
		}
		return nil
	},
}

func init() {
	daemonCmd.AddCommand(daemonRunCmd, daemonStartCmd, daemonStopCmd, daemonStatusCmd)
	rootCmd.AddCommand(daemonCmd)
}

func runDaemon(c *cobra.Command, args []string) error {
	if err := os.MkdirAll(db.Dir(), 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(daemon.LockPath(), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("daemon already running (lock held)")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	cfg := pomoconfig.Load(database)

	var loop *daemon.Loop
	srv, err := ipc.Serve(ipc.SocketPath(), func(e ipc.Event) {
		if loop != nil {
			loop.HandleEvent(e)
		}
	})
	if err != nil {
		return err
	}
	defer srv.Close()

	nudger, recapper, _ := ai.New(ai.Config{
		Provider: cfg.AI.Provider, Key: cfg.AI.Key, Model: cfg.AI.Model,
	})
	loop = daemon.NewLoop(daemon.Deps{
		DB:     database,
		Watch:  watch.New(),
		Notify: notify.New(),
		IPC:    srv,
		AI:     nudger,
		Recap:  recapper,
		Cfg:    cfg,
	})

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(cfg.Daemon.Tick)
	defer ticker.Stop()

	fmt.Fprintf(os.Stderr, "pomo daemon: started, tick %s, watch %s\n", cfg.Daemon.Tick, watch.New().Name())
	_ = daemon.WriteStatus(loop.StatusSnapshot())
	for {
		select {
		case <-sigc:
			fmt.Fprintln(os.Stderr, "pomo daemon: shutting down")
			daemon.RemoveStatus()
			return nil
		case <-ticker.C:
			loop.Tick()
			_ = daemon.WriteStatus(loop.StatusSnapshot())
		}
	}
}
