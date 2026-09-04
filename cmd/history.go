package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"pomo/internal/db"
	"pomo/internal/model"
)

var (
	histToday     bool
	histYesterday bool
	histWeek      bool
	histDate      string
	histTask      string
	histDetails   bool
)

var historyCmd = &cobra.Command{
	Use:   "history",
	Short: "Show session history",
	RunE: func(c *cobra.Command, args []string) error {
		f := db.SessionFilter{Task: histTask}
		now := time.Now()
		dayStart := func(t time.Time) time.Time {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		}
		switch {
		case histToday:
			from := dayStart(now)
			to := from.AddDate(0, 0, 1)
			f.From, f.To = &from, &to
		case histYesterday:
			from := dayStart(now).AddDate(0, 0, -1)
			to := dayStart(now)
			f.From, f.To = &from, &to
		case histWeek:
			from := dayStart(now).AddDate(0, 0, -7)
			f.From = &from
		case histDate != "":
			d, err := time.ParseInLocation("2006-01-02", histDate, time.Local)
			if err != nil {
				return fmt.Errorf("invalid --date, expected YYYY-MM-DD")
			}
			from := dayStart(d)
			to := from.AddDate(0, 0, 1)
			f.From, f.To = &from, &to
		}

		sessions, err := database.ListSessions(f)
		if err != nil {
			return err
		}
		if len(sessions) == 0 {
			fmt.Println("No sessions found.")
			return nil
		}

		if histDetails {
			for _, s := range sessions {
				fmt.Printf("#%d  %s — %s\n%s %s\n", s.ID, s.StartedAt.Format("15:04"), s.TaskName, durMin(s), statusMark(s.Status))
				if s.Note != "" {
					fmt.Printf("\nNote:\n%s\n", s.Note)
				}
				fmt.Println()
			}
			return nil
		}

		fmt.Printf("%-5s %-11s %-7s %-24s %-10s %s\n", "ID", "DATE", "TIME", "TASK", "DURATION", "STATUS")
		fmt.Println()
		for _, s := range sessions {
			fmt.Printf("%-5d %-11s %-7s %-24s %-10s %s\n",
				s.ID, s.StartedAt.Format("Jan 02"), s.StartedAt.Format("15:04"), truncate(s.TaskName, 24), durMin(s), statusMark(s.Status))
		}
		fmt.Println()
		fmt.Println("Delete an entry with `pomo history delete <id>`.")
		return nil
	},
}

var historyDeleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a session from history",
	Args:  cobra.ExactArgs(1),
	RunE: func(c *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid session id: %s", args[0])
		}
		if err := database.DeleteSession(id); err != nil {
			return err
		}
		fmt.Printf("Deleted session #%d.\n", id)
		return nil
	},
}

func durMin(s model.Session) string {
	d := s.ActualDuration
	if s.Status == model.StatusRunning {
		d = int(time.Since(s.StartedAt).Seconds())
	}
	return fmt.Sprintf("%dm", d/60)
}

func statusMark(status model.SessionStatus) string {
	switch status {
	case model.StatusCompleted:
		return "✓"
	case model.StatusRunning:
		return "▶"
	case model.StatusSkipped:
		return "⏭"
	case model.StatusCancelled:
		return "✗"
	case model.StatusInterrupted:
		return "!"
	}
	return "-"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func init() {
	historyCmd.Flags().BoolVar(&histToday, "today", false, "sessions from today")
	historyCmd.Flags().BoolVar(&histYesterday, "yesterday", false, "sessions from yesterday")
	historyCmd.Flags().BoolVar(&histWeek, "week", false, "sessions from last 7 days")
	historyCmd.Flags().StringVar(&histDate, "date", "", "sessions from a specific date (YYYY-MM-DD)")
	historyCmd.Flags().StringVar(&histTask, "task", "", "filter by task name")
	historyCmd.Flags().BoolVar(&histDetails, "details", false, "show notes")
	historyCmd.AddCommand(historyDeleteCmd)
	rootCmd.AddCommand(historyCmd)
}
