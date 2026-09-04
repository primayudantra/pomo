package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"pomo/internal/db"
	"pomo/internal/model"
)

var weekCmd = &cobra.Command{
	Use:   "week",
	Short: "Show this week's summary",
	RunE: func(c *cobra.Command, args []string) error {
		now := time.Now()
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7 // treat Sunday as end of week (Mon-start)
		}
		monday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(weekday - 1))
		sunday := monday.AddDate(0, 0, 7)

		sessions, err := database.ListSessions(db.SessionFilter{From: &monday, To: &sunday})
		if err != nil {
			return err
		}

		perDay := map[int]int{}
		totalFocus, pomodoros := 0, 0
		for _, s := range sessions {
			if s.Status == model.StatusRunning {
				continue
			}
			day := int(s.StartedAt.Sub(monday).Hours() / 24)
			perDay[day] += s.ActualDuration
			totalFocus += s.ActualDuration
			if s.PlannedDuration >= 60 {
				pomodoros++
			}
		}

		fmt.Printf("WEEK — %s → %s\n\n", monday.Format("Jan 02"), sunday.AddDate(0, 0, -1).Format("Jan 02"))
		names := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
		maxSecs := 1
		for _, v := range perDay {
			if v > maxSecs {
				maxSecs = v
			}
		}
		barWidth := 16
		for i := 0; i < 7; i++ {
			secs := perDay[i]
			filled := 0
			if secs > 0 {
				filled = int(float64(secs) / float64(maxSecs) * float64(barWidth))
				if filled == 0 {
					filled = 1
				}
			}
			bar := strings.Repeat("█", filled)
			fmt.Printf("%-5s %-16s %s\n", names[i], bar, fmtDurLong(secs))
		}

		fmt.Println()
		fmt.Printf("Total Focus\n%s\n\n", fmtDurLong(totalFocus))
		fmt.Printf("Pomodoros\n%d\n\n", pomodoros)
		avg := totalFocus / 7
		fmt.Printf("Average / Day\n%s\n", fmtDurLong(avg))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(weekCmd)
}
