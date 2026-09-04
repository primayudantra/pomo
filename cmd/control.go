package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"pomo/internal/model"
)

var pauseCmd = &cobra.Command{
	Use:   "pause",
	Short: "Pause the running Pomodoro (press [p] inside the timer instead if it's in this terminal)",
	RunE: func(c *cobra.Command, args []string) error {
		s, err := database.LastRunningSession()
		if err != nil || s == nil {
			fmt.Println("No active session.")
			return nil
		}
		fmt.Println("Use [p] inside the running timer to pause. From another terminal, pausing isn't supported yet.")
		return nil
	},
}

var resumeCmd = &cobra.Command{
	Use:   "resume",
	Short: "Resume a paused Pomodoro",
	RunE: func(c *cobra.Command, args []string) error {
		fmt.Println("Use [r] inside the running timer to resume.")
		return nil
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the active Pomodoro session",
	RunE: func(c *cobra.Command, args []string) error {
		s, err := database.LastRunningSession()
		if err != nil || s == nil {
			fmt.Println("No active session.")
			return nil
		}
		actual := int(time.Since(s.StartedAt).Seconds())
		if err := database.FinishSession(s.ID, model.StatusCancelled, actual, ""); err != nil {
			return err
		}
		fmt.Printf("Stopped: %s (%dm)\n", s.TaskName, actual/60)
		return nil
	},
}

var skipCmd = &cobra.Command{
	Use:   "skip",
	Short: "Skip the active Pomodoro session",
	RunE: func(c *cobra.Command, args []string) error {
		s, err := database.LastRunningSession()
		if err != nil || s == nil {
			fmt.Println("No active session.")
			return nil
		}
		actual := int(time.Since(s.StartedAt).Seconds())
		if err := database.FinishSession(s.ID, model.StatusSkipped, actual, ""); err != nil {
			return err
		}
		fmt.Printf("Skipped: %s\n", s.TaskName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pauseCmd, resumeCmd, stopCmd, skipCmd)
}
