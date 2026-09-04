package cmd

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"pomo/internal/db"
	"pomo/internal/model"
)

var (
	statsTasks bool
	statsTag   string
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show productivity statistics",
	RunE: func(c *cobra.Command, args []string) error {
		if statsTasks {
			return runStatsTasks()
		}
		if statsTag != "" {
			return runStatsTag()
		}
		return runStatsOverview()
	},
}

func runStatsOverview() error {
	now := time.Now()
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	weekStart := dayStart.AddDate(0, 0, -7)

	todaySessions, err := database.ListSessions(db.SessionFilter{From: &dayStart})
	if err != nil {
		return err
	}
	weekSessions, err := database.ListSessions(db.SessionFilter{From: &weekStart})
	if err != nil {
		return err
	}

	summarize := func(sessions []model.Session) (focus, pomodoros, completed, skipped, longest int) {
		for _, s := range sessions {
			if s.Status == model.StatusRunning {
				continue
			}
			focus += s.ActualDuration
			if s.PlannedDuration >= 60 {
				pomodoros++
			}
			if s.ActualDuration > longest {
				longest = s.ActualDuration
			}
			switch s.Status {
			case model.StatusCompleted:
				completed++
			case model.StatusSkipped:
				skipped++
			}
		}
		return
	}

	tFocus, tPomo, tComp, tSkip, tLongest := summarize(todaySessions)
	wFocus, wPomo, wComp, wSkip, wLongest := summarize(weekSessions)

	avgSession := 0
	if wPomo > 0 {
		avgSession = wFocus / wPomo
	}
	longest := tLongest
	if wLongest > longest {
		longest = wLongest
	}

	fmt.Println("PRODUCTIVITY")
	fmt.Println()
	fmt.Println("Today")
	fmt.Println(strings.Repeat("─", 32))
	fmt.Println()
	fmt.Printf("Focus Time        %s\n", fmtDurLong(tFocus))
	fmt.Printf("Pomodoros         %d\n", tPomo)
	fmt.Printf("Completed         %d\n", tComp)
	fmt.Printf("Skipped           %d\n\n", tSkip)
	fmt.Println("This Week")
	fmt.Println(strings.Repeat("─", 32))
	fmt.Println()
	fmt.Printf("Focus Time        %s\n", fmtDurLong(wFocus))
	fmt.Printf("Pomodoros         %d\n", wPomo)
	fmt.Printf("Completed         %d\n", wComp)
	fmt.Printf("Skipped           %d\n\n", wSkip)
	fmt.Println("Average Session")
	fmt.Printf("                  %s\n\n", fmtDurLong(avgSession))
	fmt.Println("Longest Focus")
	fmt.Printf("                  %s\n", fmtDurLong(longest))
	return nil
}

func runStatsTasks() error {
	now := time.Now()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	monday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(weekday - 1))

	sessions, err := database.ListSessions(db.SessionFilter{From: &monday})
	if err != nil {
		return err
	}

	byTask := map[string]int{}
	var order []string
	for _, s := range sessions {
		if s.Status == model.StatusRunning {
			continue
		}
		if _, ok := byTask[s.TaskName]; !ok {
			order = append(order, s.TaskName)
		}
		byTask[s.TaskName] += s.ActualDuration
	}
	sort.Slice(order, func(i, j int) bool { return byTask[order[i]] > byTask[order[j]] })

	fmt.Println("FOCUS DISTRIBUTION — THIS WEEK")
	fmt.Println()
	if len(order) == 0 {
		fmt.Println("No sessions this week yet.")
		return nil
	}
	max := byTask[order[0]]
	barWidth := 22
	for _, t := range order {
		secs := byTask[t]
		filled := int(float64(secs) / float64(max) * float64(barWidth))
		if filled == 0 {
			filled = 1
		}
		fmt.Printf("%s\n%s  %s\n\n", t, strings.Repeat("█", filled), fmtDurLong(secs))
	}
	return nil
}

func runStatsTag() error {
	sessions, err := database.ListSessions(db.SessionFilter{Tag: statsTag})
	if err != nil {
		return err
	}
	focus, pomodoros := 0, 0
	for _, s := range sessions {
		if s.Status == model.StatusRunning {
			continue
		}
		focus += s.ActualDuration
		if s.PlannedDuration >= 60 {
			pomodoros++
		}
	}
	fmt.Println(strings.ToUpper(statsTag))
	fmt.Println()
	fmt.Printf("Focus Time       %s\n", fmtDurLong(focus))
	fmt.Printf("Pomodoros        %d\n", pomodoros)
	return nil
}

func init() {
	statsCmd.Flags().BoolVar(&statsTasks, "tasks", false, "show focus time distribution by task")
	statsCmd.Flags().StringVar(&statsTag, "tag", "", "filter stats by tag")
	rootCmd.AddCommand(statsCmd)
}
