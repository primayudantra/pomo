package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"pomo/internal/db"
	"pomo/internal/model"
)

// dayStat aggregates one calendar day's finished-session activity.
type dayStat struct {
	Secs     int
	Sessions int
}

func dayKey(t time.Time) string { return t.Format("2006-01-02") }

// loadDayStats buckets every non-running session into its calendar day.
// The dataset is a single user's Pomodoro history, small enough to scan
// in full on every render of the analytics screen.
func loadDayStats(d *db.DB) map[string]dayStat {
	sessions, _ := d.ListSessions(db.SessionFilter{})
	stats := map[string]dayStat{}
	for _, s := range sessions {
		if s.Status == model.StatusRunning {
			continue
		}
		key := dayKey(s.StartedAt)
		ds := stats[key]
		ds.Secs += s.ActualDuration
		ds.Sessions++
		stats[key] = ds
	}
	return stats
}

// computeStreaks returns the current consecutive-active-day streak (today
// counts if it has activity yet, otherwise the streak can still be "alive"
// through yesterday) and the longest streak ever recorded.
func computeStreaks(stats map[string]dayStat) (current, longest int) {
	dates := make([]time.Time, 0, len(stats))
	for k, v := range stats {
		if v.Secs <= 0 {
			continue
		}
		if t, err := time.ParseInLocation("2006-01-02", k, time.Local); err == nil {
			dates = append(dates, t)
		}
	}
	if len(dates) == 0 {
		return 0, 0
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })

	longest, run := 1, 1
	for i := 1; i < len(dates); i++ {
		if dates[i].Sub(dates[i-1]).Hours() == 24 {
			run++
		} else {
			run = 1
		}
		if run > longest {
			longest = run
		}
	}

	now := time.Now()
	cursor := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	if stats[dayKey(cursor)].Secs <= 0 {
		cursor = cursor.AddDate(0, 0, -1)
		if stats[dayKey(cursor)].Secs <= 0 {
			return 0, longest
		}
	}
	for stats[dayKey(cursor)].Secs > 0 {
		current++
		cursor = cursor.AddDate(0, 0, -1)
	}
	return current, longest
}

// mostActiveDay returns the label and seconds of the single best day on
// record.
func mostActiveDay(stats map[string]dayStat) (string, int) {
	best, bestSecs := "-", 0
	for k, v := range stats {
		if v.Secs > bestSecs {
			t, err := time.ParseInLocation("2006-01-02", k, time.Local)
			if err != nil {
				continue
			}
			best, bestSecs = t.Format("Jan 02"), v.Secs
		}
	}
	return best, bestSecs
}

// heatCell renders one calendar-day cell: a dim dot when idle, else a
// square shaded from the accent color by how the day compares to the
// busiest day shown.
func heatCell(secs, max int) string {
	if secs <= 0 {
		return styleDim.Render("·")
	}
	t := float64(secs) / float64(max)
	col := lerpColor(rowBg, accent, 0.35+0.65*t)
	return lipgloss.NewStyle().Foreground(col).Render("■")
}

func heatLegend() string {
	var b strings.Builder
	b.WriteString(styleMuted.Render("Less "))
	b.WriteString(styleDim.Render("· "))
	for _, t := range []float64{0.33, 0.66, 1.0} {
		col := lerpColor(rowBg, accent, 0.35+0.65*t)
		b.WriteString(lipgloss.NewStyle().Foreground(col).Render("■ "))
	}
	b.WriteString(styleMuted.Render("More"))
	return b.String()
}

// renderHeatmap draws a GitHub-style contribution grid: weeks as columns
// (Monday-start), the last `weeks` of them, ending on the current week.
func renderHeatmap(stats map[string]dayStat, weeks int) string {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	wd := int(today.Weekday())
	if wd == 0 {
		wd = 7
	}
	thisMonday := today.AddDate(0, 0, -(wd - 1))
	startMonday := thisMonday.AddDate(0, 0, -7*(weeks-1))

	max := 1
	for i := 0; i < weeks*7; i++ {
		d := startMonday.AddDate(0, 0, i)
		if d.After(today) {
			break
		}
		if s := stats[dayKey(d)].Secs; s > max {
			max = s
		}
	}

	var months strings.Builder
	months.WriteString("    ")
	lastMonth := time.Month(0)
	for w := 0; w < weeks; w++ {
		m := startMonday.AddDate(0, 0, 7*w)
		if m.Month() != lastMonth {
			months.WriteString(styleMuted.Render(fmt.Sprintf("%-2s", m.Format("Jan"))))
			lastMonth = m.Month()
		} else {
			months.WriteString("  ")
		}
	}

	dayLabel := map[int]string{0: "Mon", 2: "Wed", 4: "Fri"}
	var b strings.Builder
	b.WriteString(months.String() + "\n")
	for row := 0; row < 7; row++ {
		label := dayLabel[row]
		b.WriteString(styleMuted.Render(fmt.Sprintf("%-4s", label)))
		for w := 0; w < weeks; w++ {
			d := startMonday.AddDate(0, 0, 7*w+row)
			if d.After(today) {
				b.WriteString("  ")
				continue
			}
			b.WriteString(heatCell(stats[dayKey(d)].Secs, max) + " ")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// breakdownDays renders a bar per calendar day for the last n days.
func breakdownDays(stats map[string]dayStat, n, width int) string {
	now := time.Now()
	secs := make([]int, n)
	labels := make([]string, n)
	max := 1
	for i := 0; i < n; i++ {
		d := now.AddDate(0, 0, -(n - 1 - i))
		secs[i] = stats[dayKey(d)].Secs
		labels[i] = d.Format("Mon 02")
		if secs[i] > max {
			max = secs[i]
		}
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		pct := float64(secs[i]) / float64(max)
		b.WriteString(fmt.Sprintf("%-8s%s  %s\n", labels[i], gradientBar(pct, width), fmtDur(secs[i])))
	}
	return b.String()
}

// breakdownWeeks renders a bar per Monday-start week for the last n weeks.
func breakdownWeeks(stats map[string]dayStat, n, width int) string {
	now := time.Now()
	wd := int(now.Weekday())
	if wd == 0 {
		wd = 7
	}
	thisMonday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -(wd - 1))

	secs := make([]int, n)
	labels := make([]string, n)
	max := 1
	for i := 0; i < n; i++ {
		start := thisMonday.AddDate(0, 0, -7*(n-1-i))
		total := 0
		for d := 0; d < 7; d++ {
			total += stats[dayKey(start.AddDate(0, 0, d))].Secs
		}
		secs[i] = total
		labels[i] = start.Format("Jan 02")
		if total > max {
			max = total
		}
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		pct := float64(secs[i]) / float64(max)
		b.WriteString(fmt.Sprintf("%-8s%s  %s\n", labels[i], gradientBar(pct, width), fmtDur(secs[i])))
	}
	return b.String()
}

// breakdownMonths renders a bar per calendar month for the last n months.
func breakdownMonths(stats map[string]dayStat, n, width int) string {
	totals := map[string]int{}
	for k, v := range stats {
		t, err := time.ParseInLocation("2006-01-02", k, time.Local)
		if err != nil {
			continue
		}
		totals[t.Format("2006-01")] += v.Secs
	}

	now := time.Now()
	secs := make([]int, n)
	labels := make([]string, n)
	max := 1
	for i := 0; i < n; i++ {
		m := now.AddDate(0, -(n - 1 - i), 0)
		key := m.Format("2006-01")
		secs[i] = totals[key]
		labels[i] = m.Format("Jan 2006")
		if secs[i] > max {
			max = secs[i]
		}
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		pct := float64(secs[i]) / float64(max)
		b.WriteString(fmt.Sprintf("%-10s%s  %s\n", labels[i], gradientBar(pct, width), fmtDur(secs[i])))
	}
	return b.String()
}
