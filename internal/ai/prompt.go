package ai

import (
	"fmt"
	"strings"
)

func renderNudgeUser(nc NudgeContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "task: %s\n", nc.Task)
	if nc.Tag != "" {
		fmt.Fprintf(&b, "tag: %s\n", nc.Tag)
	}
	if nc.DistractApp != "" {
		fmt.Fprintf(&b, "distraction: %s", nc.DistractApp)
		if nc.DistractDetail != "" {
			fmt.Fprintf(&b, " (%s)", nc.DistractDetail)
		}
		b.WriteByte('\n')
	}
	fmt.Fprintf(&b, "drift_minutes: %d\n", nc.DriftMinutes)
	fmt.Fprintf(&b, "session_minutes: %d\n", nc.SessionMinutes)
	fmt.Fprintf(&b, "nudge_level: %d\n", nc.Level)
	fmt.Fprintf(&b, "hour: %02d\n", nc.Hour)
	return b.String()
}

func renderRecapUser(rc RecapContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "window: %s\n", rc.Label)
	fmt.Fprintf(&b, "focus_minutes: %d / planned_minutes: %d\n", rc.FocusMinutes, rc.PlannedMinutes)
	fmt.Fprintf(&b, "sessions: %d completed of %d\n", rc.Completed, rc.Planned)
	fmt.Fprintf(&b, "drift_minutes: %d\n", rc.DriftMinutes)
	if len(rc.TopDrift) > 0 {
		fmt.Fprintf(&b, "top_drift: %s\n", strings.Join(rc.TopDrift, ", "))
	}
	if len(rc.ByTag) > 0 {
		fmt.Fprintf(&b, "by_tag: %s\n", strings.Join(rc.ByTag, ", "))
	}
	if rc.BestHour != "" {
		fmt.Fprintf(&b, "best_hour: %s\n", rc.BestHour)
	}
	for _, n := range rc.Notes {
		fmt.Fprintf(&b, "note: %s\n", n)
	}
	return b.String()
}
