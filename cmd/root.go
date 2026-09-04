package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"pomo/internal/db"
	"pomo/internal/tui"
)

var database *db.DB

var rootCmd = &cobra.Command{
	Use:   "pomo [task]",
	Short: "Pomo — terminal Pomodoro productivity tracker",
	Args:  cobra.ArbitraryArgs,
	RunE: func(c *cobra.Command, args []string) error {
		if len(args) > 0 {
			// Quick start: open the app on the duration-confirm screen with the
			// task pre-filled. Nothing runs until the user hits enter — this also
			// keeps a mistyped subcommand (`pomo daemons tart`) from silently
			// launching a timer.
			return tui.RunAppQuickStart(database, strings.Join(args, " "), "")
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
