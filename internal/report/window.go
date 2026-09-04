// Package report aggregates pomo session and drift-event history into
// windowed summaries and renders them as text, markdown, or JSON. It is the
// single source of aggregation logic shared by the CLI, the TUI, and the
// daemon's weekly digest.
package report

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Window is a half-open time range [From, To) with a display label.
type Window struct {
	From  time.Time
	To    time.Time
	Label string
}

func midnight(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

// Today is the current local calendar day.
func Today() Window {
	from := midnight(time.Now())
	return Window{From: from, To: from.AddDate(0, 0, 1), Label: "today"}
}

// ThisWeek is the current ISO week (Monday start), local time.
func ThisWeek() Window { return WeekAt(time.Now()) }

// WeekAt is the ISO week (Monday start, local time) containing t.
func WeekAt(t time.Time) Window {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	from := midnight(t).AddDate(0, 0, -(wd - 1))
	y, w := from.ISOWeek()
	return Window{From: from, To: from.AddDate(0, 0, 7), Label: fmt.Sprintf("%d-W%02d", y, w)}
}

// ThisMonth is the current local calendar month.
func ThisMonth() Window {
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	return Window{From: from, To: from.AddDate(0, 1, 0), Label: from.Format("2006-01")}
}

// ParseWindow resolves a window spec. Accepted: "" / "today", "week",
// "month", "YYYY-Www" (ISO week), "YYYY-MM".
func ParseWindow(s string) (Window, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "today":
		return Today(), nil
	case "week":
		return ThisWeek(), nil
	case "month":
		return ThisMonth(), nil
	}
	if y, w, ok := parseISOWeek(s); ok {
		from := isoWeekStart(y, w)
		return Window{From: from, To: from.AddDate(0, 0, 7), Label: fmt.Sprintf("%d-W%02d", y, w)}, nil
	}
	if t, err := time.ParseInLocation("2006-01", strings.TrimSpace(s), time.Local); err == nil {
		return Window{From: t, To: t.AddDate(0, 1, 0), Label: t.Format("2006-01")}, nil
	}
	return Window{}, fmt.Errorf("unrecognised window %q (want today|week|month|YYYY-Www|YYYY-MM)", s)
}

func parseISOWeek(s string) (year, week int, ok bool) {
	parts := strings.Split(strings.ToUpper(strings.TrimSpace(s)), "-W")
	if len(parts) != 2 {
		return 0, 0, false
	}
	y, err1 := strconv.Atoi(parts[0])
	w, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || w < 1 || w > 53 {
		return 0, 0, false
	}
	return y, w, true
}

// isoWeekStart returns the local midnight of the Monday of ISO week (year, week).
func isoWeekStart(year, week int) time.Time {
	// Jan 4th is always in ISO week 1.
	jan4 := time.Date(year, 1, 4, 0, 0, 0, 0, time.Local)
	wd := int(jan4.Weekday())
	if wd == 0 {
		wd = 7
	}
	week1Monday := jan4.AddDate(0, 0, -(wd - 1))
	return week1Monday.AddDate(0, 0, 7*(week-1))
}
