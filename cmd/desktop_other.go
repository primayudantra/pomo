//go:build !darwin

package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

var desktopCmd = &cobra.Command{
	Use:   "desktop",
	Short: "Open the pomo desktop app",
	Args:  cobra.ArbitraryArgs,
	RunE: func(c *cobra.Command, args []string) error {
		return errors.New("pomo desktop is macOS-only")
	},
}

func init() {
	rootCmd.AddCommand(desktopCmd)
}
