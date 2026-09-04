package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"pomo/internal/report"
)

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
func renderHeatmap(stats map[string]report.DayStat, weeks int) string {
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
		if s := stats[report.DayKey(d)].Secs; s > max {
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
			b.WriteString(heatCell(stats[report.DayKey(d)].Secs, max) + " ")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// breakdownDays renders a bar per calendar day for the last n days.
func breakdownDays(stats map[string]report.DayStat, n, width int) string {
	now := time.Now()
	secs := make([]int, n)
	labels := make([]string, n)
	max := 1
	for i := 0; i < n; i++ {
		d := now.AddDate(0, 0, -(n - 1 - i))
		secs[i] = stats[report.DayKey(d)].Secs
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
func breakdownWeeks(stats map[string]report.DayStat, n, width int) string {
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
			total += stats[report.DayKey(start.AddDate(0, 0, d))].Secs
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
func breakdownMonths(stats map[string]report.DayStat, n, width int) string {
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
