//go:build darwin

package cmd

import (
	"fmt"
	"os"
	"syscall"

	"github.com/spf13/cobra"

	"pomo/internal/pomoexec"
)

var desktopCmd = &cobra.Command{
	Use:   "desktop",
	Short: "Open the pomo desktop app",
	Args:  cobra.ArbitraryArgs,
	RunE: func(c *cobra.Command, args []string) error {
		bin, err := pomoexec.Find("pomo-desktop")
		if err != nil {
			return fmt.Errorf("%w\nbuild it with: make desktop-build", err)
		}
		argv := append([]string{bin}, args...)
		return syscall.Exec(bin, argv, os.Environ())
	},
}

func init() {
	rootCmd.AddCommand(desktopCmd)
}
