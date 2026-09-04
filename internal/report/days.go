package report

import (
	"sort"
	"time"

	"pomo/internal/db"
	"pomo/internal/model"
)

// DayStat aggregates one calendar day's finished-session activity.
type DayStat struct {
	Secs     int
	Sessions int
}

// DayKey is the local-date bucket key for a timestamp.
func DayKey(t time.Time) string { return t.Format("2006-01-02") }

// LoadDayStats buckets every non-running session into its calendar day. The
// dataset is a single user's history, small enough to scan in full.
func LoadDayStats(d *db.DB) map[string]DayStat {
	sessions, _ := d.ListSessions(db.SessionFilter{})
	stats := map[string]DayStat{}
	for _, s := range sessions {
		if s.Status == model.StatusRunning {
			continue
		}
		key := DayKey(s.StartedAt)
		ds := stats[key]
		ds.Secs += s.ActualDuration
		ds.Sessions++
		stats[key] = ds
	}
	return stats
}

// Streaks returns the current consecutive-active-day streak and the longest
// streak ever recorded.
func Streaks(stats map[string]DayStat) (current, longest int) {
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
	if stats[DayKey(cursor)].Secs <= 0 {
		cursor = cursor.AddDate(0, 0, -1)
		if stats[DayKey(cursor)].Secs <= 0 {
			return 0, longest
		}
	}
	for stats[DayKey(cursor)].Secs > 0 {
		current++
		cursor = cursor.AddDate(0, 0, -1)
	}
	return current, longest
}

// MostActiveDay returns the label ("Jan 02") and seconds of the best day on record.
func MostActiveDay(stats map[string]DayStat) (string, int) {
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
