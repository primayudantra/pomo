package cmd

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"pomo/internal/report"
)

var (
	reviewJSON         bool
	reviewIncludeNotes bool
)

var reviewCmd = &cobra.Command{
	Use:   "review [today|week|month|YYYY-Www|YYYY-MM]",
	Short: "Show focus vs plan and drift for a period",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		return runReview(args, reviewJSON, os.Stdout)
	},
}

func init() {
	reviewCmd.Flags().BoolVar(&reviewJSON, "json", false, "output machine-readable JSON")
	reviewCmd.Flags().BoolVar(&reviewIncludeNotes, "include-notes", false, "reserved; no effect yet")
	rootCmd.AddCommand(reviewCmd)
}

func runReview(args []string, jsonOut bool, w io.Writer) error {
	spec := ""
	if len(args) == 1 {
		spec = args[0]
	}
	win, err := report.ParseWindow(spec)
	if err != nil {
		return err
	}
	sum, err := report.Build(database, win)
	if err != nil {
		return err
	}
	if jsonOut {
		b, err := sum.JSON()
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(w, string(b))
		return err
	}
	_, err = fmt.Fprint(w, report.RenderText(sum))
	return err
}
