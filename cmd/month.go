package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"pomo/internal/db"
	"pomo/internal/model"
)

var monthCmd = &cobra.Command{
	Use:   "month",
	Short: "Show this month's statistics",
	RunE: func(c *cobra.Command, args []string) error {
		now := time.Now()
		from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		to := from.AddDate(0, 1, 0)

		sessions, err := database.ListSessions(db.SessionFilter{From: &from, To: &to})
		if err != nil {
			return err
		}

		totalFocus, pomodoros, completed, skipped := 0, 0, 0, 0
		byDay := map[string]int{}
		byTask := map[string]int{}
		for _, s := range sessions {
			if s.Status == model.StatusRunning {
				continue
			}
			totalFocus += s.ActualDuration
			if s.PlannedDuration >= 60 {
				pomodoros++
			}
			switch s.Status {
			case model.StatusCompleted:
				completed++
			case model.StatusSkipped:
				skipped++
			}
			byDay[s.StartedAt.Format("January 2")] += s.ActualDuration
			byTask[s.TaskName] += s.ActualDuration
		}

		daysInMonth := from.AddDate(0, 1, -1).Day()
		avgPerDay := totalFocus / daysInMonth

		bestDay, bestDaySecs := "-", 0
		for d, secs := range byDay {
			if secs > bestDaySecs {
				bestDay, bestDaySecs = d, secs
			}
		}
		bestTask, bestTaskSecs := "-", 0
		for t, secs := range byTask {
			if secs > bestTaskSecs {
				bestTask, bestTaskSecs = t, secs
			}
		}

		fmt.Printf("%s\n\n", now.Format("January 2006"))
		fmt.Printf("Total Focus       %s\n", fmtDurLong(totalFocus))
		fmt.Printf("Pomodoros         %d\n", pomodoros)
		fmt.Printf("Completed         %d\n", completed)
		fmt.Printf("Skipped           %d\n\n", skipped)
		fmt.Printf("Average / Day     %s\n\n", fmtDurLong(avgPerDay))
		fmt.Println("Most Focused Day")
		fmt.Printf("%s       %s\n\n", bestDay, fmtDurLong(bestDaySecs))
		fmt.Println("Most Focused Task")
		fmt.Printf("%s    %s\n", bestTask, fmtDurLong(bestTaskSecs))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(monthCmd)
}
