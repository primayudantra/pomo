# Review, Insights, Digest + `internal/report`

Date: 2026-09-04
Parent: `2026-09-04-adhd-focus-overview.md`

## 1. `internal/report`

New package that owns all aggregation + rendering, shared by the `/review`
slash command, the `/insights` recap, `pomo review --json`, `pomo digest`, and
the existing TUI stats screen (refactor `internal/tui/stats.go` to call it —
removes the current duplicated aggregation).

```go
package report

type Window struct { From, To time.Time; Label string } // "today", "2026-W36", ...

func Today() Window
func ThisWeek() Window   // ISO week, Monday start
func ThisMonth() Window
func ParseWindow(s string) (Window, error)  // "today"|"week"|"month"|"YYYY-Www"|"YYYY-MM"

type Summary struct {
    Window        Window
    Planned       int            // sessions
    Completed     int
    Cancelled     int
    Skipped       int
    FocusSeconds  int            // sum actual_duration of completed/interrupted focus
    PlannedSeconds int
    DriftSeconds  int
    DriftEpisodes int
    DriftByApp    []AppDrift     // sorted desc
    ByTag         []TagStat
    Streak        int            // consecutive days with >=1 completed session, ending at Window.To
    BestHour      *HourStat      // hour with most focus and least drift; nil if no data
}

type AppDrift struct { App string; Seconds int }
type TagStat  struct { Tag string; FocusSeconds int }
type HourStat struct { Hour int; FocusSeconds int; DriftSeconds int }

func Build(d *db.DB, w Window) (Summary, error)

func RenderText(s Summary) string          // lipgloss, for TUI screenResult
func RenderMarkdown(s Summary, recap string) string  // for digest files
func (s Summary) JSON() ([]byte, error)
```

`Build` runs two queries: `db.ListSessions(SessionFilter{From,To})` and a new
`db.ListDriftEvents(from, to)` (join not needed — `drift_events` carries
`session_id`, filter by `started_at`). Drift-by-app groups on `detail`'s app
prefix (`"Google Chrome — reddit.com"` → `"Google Chrome"`; `""` → `"idle"`).

## 2. `/review` and `/recap`

- Slash command → `report.Build(db, window)` → `report.RenderText` → `screenResult`.
- No AI. Footer line: `run /insights for an AI recap`.

## 3. `/insights`

- Same as `/review`, then a `tea.Cmd` calls `recapper.Recap(RecapContext{...})`
  built from the `Summary` (numbers only, no notes unless `--include-notes` which
  the slash form does not expose).
- Rendered as a `RECAP` block appended to the pane. Pending → `RECAP  …thinking`.
  Error / no provider → `RECAP  (unavailable: no AI provider set)` etc.

```go
type RecapContext struct {
    Label          string
    FocusMinutes   int
    PlannedMinutes int
    Completed, Planned int
    DriftMinutes   int
    TopDrift       []AppDrift   // top 3
    ByTag          []TagStat
    BestHour       *HourStat
    Notes          []string     // empty unless --include-notes
}
```

## 4. `pomo review` (CLI)

New `cmd/review.go`:

```
pomo review [today|week|month|YYYY-Www|YYYY-MM] [--json] [--insights] [--include-notes]
```

- Default window `today`, default output `report.RenderText` to stdout (no TUI).
- `--json` → `Summary.JSON()`.
- `--insights` → append recap (respects `--include-notes`).
- Exists for cron, editor plugins, and quick shell use. The daemon and `/review`
  both call `internal/report`, not this command.

## 5. `pomo digest` (CLI)

New `cmd/digest.go`:

```
pomo digest [--week YYYY-Www] [--notify]
```

- Default week = current ISO week.
- Builds `report.Build` for the week, calls `recapper.Recap` if a provider is
  set (8s timeout; on failure the recap section is omitted, not an error).
- Writes `report.RenderMarkdown(summary, recap)` to
  `~/.pomo/reviews/<YYYY-Www>.md`. Creates `~/.pomo/reviews/` if missing.
- Idempotent: overwrites the target week's file every run.
- `--notify` → `notify.Send("weekly digest ready", path)`.

### Markdown shape

```markdown
# Pomo — Week 2026-W36  (Sep 1 – Sep 7)

## Focus
| Day | Sessions | Focus | Drift |
|-----|----------|-------|-------|
| Mon | 4        | 1h42m | 12m   |
...
**Totals:** 18 sessions · 12 completed · 7h20m focus / 9h planned · 1h58m drift

## Drift
- Google Chrome — 1h04m
- Slack — 33m
- idle — 21m

## By tag
- backend — 3h10m
- docs — 2h05m
...

## Recap
<AI recap, or omitted>
```

## 6. Daemon weekly auto-write

In the daemon idle branch (`2026-09-04-daemon-drift-detection.md` §2):

```go
func maybeWeeklyDigest(d *db.DB) {
    if time.Now().Weekday() != time.Monday { return }
    lastWeek := report.ParseWindow(isoWeekString(time.Now().AddDate(0,0,-7)))
    path := reviewsPath(lastWeek.Label)   // ~/.pomo/reviews/2026-W35.md
    if fileExists(path) { return }
    runDigestForWindow(d, lastWeek, cfg.Digest.Notify)   // default notify=false
}
```

- Guard against running more than once per idle tick with an in-memory
  `lastDigestCheck` date.
- No notification by default (`digest.notify` config, default `false`) — the
  file just appears. Guilt-free review.

## 7. Config keys

`digest.notify` (bool, default false). Full table in schema spec.
