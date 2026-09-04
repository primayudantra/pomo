package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"pomo/internal/db"
)

var database *db.DB

var rootCmd = &cobra.Command{
	Use:   "pomo [task]",
	Short: "Pomo — terminal Pomodoro productivity tracker",
	Args:  cobra.ArbitraryArgs,
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) > 0 {
			// Quick start: `pomo "Fix reconciliation"` == `pomo start "Fix reconciliation"`
			return runStart(c, args)
		}
		return runDashboard()
	},
}

func Execute() {
	d, err := db.Open()
	if err != nil {
		fmt.Fprintln(os.Stderr, "pomo: failed to open database:", err)
		os.Exit(1)
	}
	database = d
	defer database.Close()

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "pomo:", err)
		os.Exit(1)
	}
}
