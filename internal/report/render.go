package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// FmtDur formats a second count as "1h02m" or "12m".
func FmtDur(secs int) string {
	d := time.Duration(secs) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh%02dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// JSON renders the summary as indented JSON.
func (s Summary) JSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// RenderText renders a plain-text summary block for terminal display.
func RenderText(s Summary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "FOCUS  %s\n", s.Window.Label)
	fmt.Fprintf(&b, "  %d planned · %d done · %d cancelled · %d skipped\n",
		s.Planned, s.Completed, s.Cancelled, s.Skipped)
	fmt.Fprintf(&b, "  %s focus / %s planned\n", FmtDur(s.FocusSeconds), FmtDur(s.PlannedSeconds))
	fmt.Fprintf(&b, "\nDRIFT  %s · %d episodes\n", FmtDur(s.DriftSeconds), s.DriftEpisodes)
	for _, a := range s.DriftByApp {
		fmt.Fprintf(&b, "  %-16s %s\n", a.App, FmtDur(a.Seconds))
	}
	if len(s.ByTag) > 0 {
		b.WriteString("\nBY TAG\n")
		for _, tg := range s.ByTag {
			name := tg.Tag
			if name == "" {
				name = "(untagged)"
			}
			fmt.Fprintf(&b, "  %-16s %s\n", name, FmtDur(tg.FocusSeconds))
		}
	}
	fmt.Fprintf(&b, "\nstreak %d", s.Streak)
	if s.BestHour != nil {
		fmt.Fprintf(&b, "  ·  best hour %02d:00 (%s focus, %s drift)",
			s.BestHour.Hour, FmtDur(s.BestHour.FocusSeconds), FmtDur(s.BestHour.DriftSeconds))
	}
	b.WriteString("\n")
	return b.String()
}

// RenderMarkdown renders the weekly-digest markdown. An empty recap omits the
// Recap section.
func RenderMarkdown(s Summary, recap string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Pomo — %s\n\n", s.Window.Label)

	b.WriteString("## Focus\n\n")
	fmt.Fprintf(&b, "**Totals:** %d sessions · %d completed · %s focus / %s planned · %s drift\n\n",
		s.Planned, s.Completed, FmtDur(s.FocusSeconds), FmtDur(s.PlannedSeconds), FmtDur(s.DriftSeconds))

	if len(s.DriftByApp) > 0 {
		b.WriteString("## Drift\n\n")
		for _, a := range s.DriftByApp {
			fmt.Fprintf(&b, "- %s — %s\n", a.App, FmtDur(a.Seconds))
		}
		b.WriteString("\n")
	}

	if len(s.ByTag) > 0 {
		b.WriteString("## By tag\n\n")
		for _, tg := range s.ByTag {
			name := tg.Tag
			if name == "" {
				name = "(untagged)"
			}
			fmt.Fprintf(&b, "- %s — %s\n", name, FmtDur(tg.FocusSeconds))
		}
		b.WriteString("\n")
	}

	if strings.TrimSpace(recap) != "" {
		fmt.Fprintf(&b, "## Recap\n\n%s\n", strings.TrimSpace(recap))
	}
	return b.String()
}
