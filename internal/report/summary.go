package report

import (
	"sort"
	"strings"

	"pomo/internal/db"
	"pomo/internal/model"
)

type AppDrift struct {
	App     string
	Seconds int
}

type TagStat struct {
	Tag          string
	FocusSeconds int
}

type HourStat struct {
	Hour         int
	FocusSeconds int
	DriftSeconds int
}

type Summary struct {
	Window         Window
	Planned        int
	Completed      int
	Cancelled      int
	Skipped        int
	FocusSeconds   int
	PlannedSeconds int
	DriftSeconds   int
	DriftEpisodes  int
	DriftByApp     []AppDrift
	ByTag          []TagStat
	Streak         int
	BestHour       *HourStat
}

func isFocusStatus(s model.SessionStatus) bool {
	return s == model.StatusCompleted || s == model.StatusInterrupted
}

// appPrefix extracts the application name from a drift detail string.
func appPrefix(detail string) string {
	if detail == "" {
		return "idle"
	}
	if i := strings.Index(detail, " — "); i >= 0 {
		return detail[:i]
	}
	return detail
}

// Build aggregates sessions and drift events in the window into a Summary.
func Build(d *db.DB, w Window) (Summary, error) {
	sum := Summary{Window: w}

	from, to := w.From, w.To
	sessions, err := d.ListSessions(db.SessionFilter{From: &from, To: &to})
	if err != nil {
		return sum, err
	}

	tagFocus := map[string]int{}
	hourFocus := map[int]int{}
	for _, s := range sessions {
		if s.Status == model.StatusRunning {
			continue
		}
		sum.Planned++
		sum.PlannedSeconds += s.PlannedDuration
		switch s.Status {
		case model.StatusCompleted:
			sum.Completed++
		case model.StatusCancelled:
			sum.Cancelled++
		case model.StatusSkipped:
			sum.Skipped++
		}
		if isFocusStatus(s.Status) {
			sum.FocusSeconds += s.ActualDuration
			tagFocus[s.Tag] += s.ActualDuration
			hourFocus[s.StartedAt.Hour()] += s.ActualDuration
		}
	}

	events, err := d.ListDriftEvents(w.From, w.To)
	if err != nil {
		return sum, err
	}
	appDrift := map[string]int{}
	hourDrift := map[int]int{}
	for _, e := range events {
		sum.DriftSeconds += e.Seconds
		sum.DriftEpisodes++
		appDrift[appPrefix(e.Detail)] += e.Seconds
		hourDrift[e.StartedAt.Hour()] += e.Seconds
	}

	for app, secs := range appDrift {
		sum.DriftByApp = append(sum.DriftByApp, AppDrift{App: app, Seconds: secs})
	}
	sort.Slice(sum.DriftByApp, func(i, j int) bool {
		if sum.DriftByApp[i].Seconds != sum.DriftByApp[j].Seconds {
			return sum.DriftByApp[i].Seconds > sum.DriftByApp[j].Seconds
		}
		return sum.DriftByApp[i].App < sum.DriftByApp[j].App
	})

	for tag, secs := range tagFocus {
		sum.ByTag = append(sum.ByTag, TagStat{Tag: tag, FocusSeconds: secs})
	}
	sort.Slice(sum.ByTag, func(i, j int) bool {
		if sum.ByTag[i].FocusSeconds != sum.ByTag[j].FocusSeconds {
			return sum.ByTag[i].FocusSeconds > sum.ByTag[j].FocusSeconds
		}
		return sum.ByTag[i].Tag < sum.ByTag[j].Tag
	})

	for h, secs := range hourFocus {
		cand := HourStat{Hour: h, FocusSeconds: secs, DriftSeconds: hourDrift[h]}
		if sum.BestHour == nil ||
			cand.FocusSeconds > sum.BestHour.FocusSeconds ||
			(cand.FocusSeconds == sum.BestHour.FocusSeconds && cand.DriftSeconds < sum.BestHour.DriftSeconds) ||
			(cand.FocusSeconds == sum.BestHour.FocusSeconds && cand.DriftSeconds == sum.BestHour.DriftSeconds && cand.Hour < sum.BestHour.Hour) {
			c := cand
			sum.BestHour = &c
		}
	}

	sum.Streak, _ = Streaks(LoadDayStats(d))
	return sum, nil
}
