package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"pomo/internal/ai"
	"pomo/internal/notify"
	"pomo/internal/pomoconfig"
	"pomo/internal/report"
)

var (
	digestWeek   string
	digestNotify bool
)

var digestCmd = &cobra.Command{
	Use:   "digest [--week YYYY-Www]",
	Short: "Write this week's review digest to ~/.pomo/reviews/",
	RunE: func(c *cobra.Command, args []string) error {
		return runDigest(digestWeek, digestNotify, os.Stdout)
	},
}

func init() {
	digestCmd.Flags().StringVar(&digestWeek, "week", "", "ISO week (YYYY-Www); default this week")
	digestCmd.Flags().BoolVar(&digestNotify, "notify", false, "send a desktop notification when done")
	rootCmd.AddCommand(digestCmd)
}

func runDigest(weekSpec string, doNotify bool, w io.Writer) error {
	win := report.ThisWeek()
	if weekSpec != "" {
		parsed, err := report.ParseWindow(weekSpec)
		if err != nil {
			return err
		}
		win = parsed
	}

	cfg := pomoconfig.Load(database)
	recap := ""
	if cfg.AI.Provider != "" {
		sum, err := report.Build(database, win)
		if err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			_, r, _ := ai.New(ai.Config{Provider: cfg.AI.Provider, Key: cfg.AI.Key, Model: cfg.AI.Model})
			if text, rerr := r.Recap(ctx, recapContext(sum)); rerr == nil {
				recap = text
			}
			cancel()
		}
	}

	path, err := report.WriteWeeklyDigest(database, win, recap)
	if err != nil {
		return err
	}
	fmt.Fprintln(w, path)
	if doNotify {
		_ = notify.New().Send("pomo weekly digest", path)
	}
	return nil
}

func recapContext(sum report.Summary) ai.RecapContext {
	rc := ai.RecapContext{
		Label:          sum.Window.Label,
		FocusMinutes:   sum.FocusSeconds / 60,
		PlannedMinutes: sum.PlannedSeconds / 60,
		Completed:      sum.Completed,
		Planned:        sum.Planned,
		DriftMinutes:   sum.DriftSeconds / 60,
	}
	for i, ad := range sum.DriftByApp {
		if i == 3 {
			break
		}
		rc.TopDrift = append(rc.TopDrift, fmt.Sprintf("%s %s", ad.App, report.FmtDur(ad.Seconds)))
	}
	for _, ts := range sum.ByTag {
		rc.ByTag = append(rc.ByTag, fmt.Sprintf("%s %s", ts.Tag, report.FmtDur(ts.FocusSeconds)))
	}
	if sum.BestHour != nil {
		rc.BestHour = fmt.Sprintf("%02d:00", sum.BestHour.Hour)
	}
	return rc
}
