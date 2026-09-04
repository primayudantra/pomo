package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"pomo/internal/pomoconfig"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View Pomodoro configuration",
	RunE: func(c *cobra.Command, args []string) error {
		cfg := pomoconfig.Load(database)
		fmt.Println("POMODORO")
		fmt.Println()
		fmt.Printf("Focus                 %s\n", cfg.Focus)
		fmt.Printf("Short Break           %s\n", cfg.ShortBreak)
		fmt.Printf("Long Break            %s\n", cfg.LongBreak)
		fmt.Printf("Sessions Before Long  %d\n\n", cfg.SessionsBeforeLong)
		fmt.Printf("Auto Start Break      %t\n", cfg.AutoStartBreak)
		fmt.Printf("Auto Start Focus      %t\n\n", cfg.AutoStartFocus)
		fmt.Printf("Sound                 %t\n", cfg.Sound)
		fmt.Printf("Sound Choice          %s\n", cfg.SoundChoice)
		fmt.Printf("Notifications         %t\n", cfg.Notifications)
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a config value (focus, short_break, long_break, sessions_before_long, auto_start_break, auto_start_focus, sound, sound_choice, notifications)",
	Args:  cobra.ExactArgs(2),
	RunE: func(c *cobra.Command, args []string) error {
		if err := database.SetConfig(args[0], args[1]); err != nil {
			return err
		}
		fmt.Printf("Set %s = %s\n", args[0], args[1])
		return nil
	},
}

func init() {
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}
