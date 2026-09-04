package cmd

import (
	"fmt"
	"sort"
	"time"

	"github.com/spf13/cobra"

	"pomo/internal/db"
	"pomo/internal/model"
)

var todayCmd = &cobra.Command{
	Use:   "today",
	Short: "Show today's summary",
	RunE: func(c *cobra.Command, args []string) error {
		now := time.Now()
		from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		sessions, err := database.ListSessions(db.SessionFilter{From: &from})
		if err != nil {
			return err
		}

		focusSecs, pomodoros := 0, 0
		byTask := map[string]int{}
		var order []string
		for _, s := range sessions {
			if s.Status == model.StatusRunning {
				continue
			}
			focusSecs += s.ActualDuration
			if s.PlannedDuration >= 60 {
				pomodoros++
			}
			if _, ok := byTask[s.TaskName]; !ok {
				order = append(order, s.TaskName)
			}
			byTask[s.TaskName] += s.ActualDuration
		}

		fmt.Printf("TODAY — %s\n\n", now.Format("Jan 02"))
		fmt.Printf("Focus Time\n%s\n\n", fmtDurLong(focusSecs))
		fmt.Printf("Pomodoros\n%d\n\n", pomodoros)

		fmt.Println("Tasks")
		fmt.Println("────────────────────")
		fmt.Println()
		sort.Slice(order, func(i, j int) bool { return byTask[order[i]] > byTask[order[j]] })
		for _, t := range order {
			fmt.Printf("%-24s%s\n", t, fmtDurLong(byTask[t]))
		}

		fmt.Println()
		fmt.Println("Timeline")
		fmt.Println("────────────────────")
		fmt.Println()
		for _, s := range sessions {
			fmt.Printf("%s  %-24s %s\n", s.StartedAt.Format("15:04"), s.TaskName, durMin(s))
		}
		return nil
	},
}

func fmtDurLong(secs int) string {
	h := secs / 3600
	m := (secs % 3600) / 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

func init() {
	rootCmd.AddCommand(todayCmd)
}
