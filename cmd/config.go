package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"pomo/internal/pomoconfig"
)

// configKeys maps every settable key to a validator for its value.
var configKeys = map[string]func(string) error{
	"focus":                vDuration,
	"short_break":          vDuration,
	"long_break":           vDuration,
	"sessions_before_long": vInt,
	"auto_start_break":     vBool,
	"auto_start_focus":     vBool,
	"sound":                vBool,
	"sound_choice":         vAny,
	"notifications":        vBool,

	"daemon.tick":              vDuration,
	"checkpoint.enabled":       vBool,
	"drift.enabled":            vBool,
	"drift.distract_grace":     vDuration,
	"drift.fs_stale":           vDuration,
	"drift.checkpoint_timeout": vDuration,
	"drift.checkpoint_penalty": vDuration,
	"drift.recover_grace":      vDuration,
	"drift.focus_apps":         vAny,
	"drift.distract_apps":      vAny,
	"drift.neutral_apps":       vAny,
	"drift.focus_title_hints":  vAny,
	"nudge.enabled":            vBool,
	"nudge.max_per_session":    vInt,
	"nudge.min_gap":            vDuration,
	"ai.provider":              vProvider,
	"ai.key":                   vAny,
	"ai.model":                 vAny,
	"digest.notify":            vBool,
}

func vAny(string) error { return nil }
func vBool(s string) error {
	if s != "true" && s != "false" {
		return fmt.Errorf("want 'true' or 'false', got %q", s)
	}
	return nil
}
func vInt(s string) error {
	if _, err := strconv.Atoi(s); err != nil {
		return fmt.Errorf("want an integer, got %q", s)
	}
	return nil
}
func vDuration(s string) error {
	if _, err := time.ParseDuration(s); err != nil {
		return fmt.Errorf("want a duration like '15s' or '3m', got %q", s)
	}
	return nil
}
func vProvider(s string) error {
	if s != "" && s != "anthropic" && s != "openrouter" {
		return fmt.Errorf("want '', 'anthropic' or 'openrouter', got %q", s)
	}
	return nil
}

func validateConfigSet(key, value string) error {
	v, ok := configKeys[key]
	if !ok {
		return fmt.Errorf("unknown config key %q", key)
	}
	return v(value)
}

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
		fmt.Printf("Notifications         %t\n\n", cfg.Notifications)

		fmt.Println("FOCUS / DRIFT")
		fmt.Println()
		fmt.Printf("Drift Detection       %t\n", cfg.Drift.Enabled)
		fmt.Printf("Checkpoint Prompt     %t\n", cfg.CheckpointEnabled)
		fmt.Printf("Nudges                %t (max %d/session, min gap %s)\n",
			cfg.Nudge.Enabled, cfg.Nudge.MaxPerSession, cfg.Nudge.MinGap)
		fmt.Printf("Daemon Tick           %s\n\n", cfg.Daemon.Tick)

		fmt.Println("AI")
		fmt.Println()
		provider := cfg.AI.Provider
		if provider == "" {
			provider = "(off)"
		}
		fmt.Printf("Provider              %s\n", provider)
		fmt.Printf("Key                   %s\n", pomoconfig.MaskKey(cfg.AI.Key))
		model := cfg.AI.Model
		if model == "" {
			model = "(provider default)"
		}
		fmt.Printf("Model                 %s\n", model)
		if cfg.AI.Provider != "" {
			fmt.Println()
			fmt.Println("note: ai.key is stored in plaintext at ~/.pomo/pomo.db")
		}
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a config value (see `pomo config` for the full list)",
	Args:  cobra.ExactArgs(2),
	RunE: func(c *cobra.Command, args []string) error {
		if err := validateConfigSet(args[0], args[1]); err != nil {
			return err
		}
		if err := database.SetConfig(args[0], args[1]); err != nil {
			return err
		}
		shown := args[1]
		if args[0] == "ai.key" {
			shown = pomoconfig.MaskKey(args[1])
		}
		fmt.Printf("Set %s = %s\n", args[0], shown)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}
