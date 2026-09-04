# TUI Slash Surface Implementation Plan (Chunk 4)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the interactive TUI a slash-command surface — `/review`, `/insights`, `/drift`, `/chat`, `/settings`, … — wire the TUI to the drift daemon over IPC (checkpoint + nudge overlays, a daemon indicator), add `pomo digest` and the daemon's weekly auto-write, and expose the drift/AI config keys in the settings screen. This is the last chunk of the ADHD focus layer; after it the feature set matches the specs.

**Architecture:** A `Command` registry drives one new `screenPrompt` (fuzzy palette) that dispatches to `screenResult` (a scrollable viewport for `/review`, `/drift`, `/help`) or `screenChat` (a streamed AI conversation). All rendering stays inside the single persistent Bubble Tea program. Async work (AI recap, chat token stream, inbound daemon events) uses the standard channel + re-issued `tea.Cmd` pattern — no program handle escapes. `/chat` input passes a `guardChatInput` validator before any bytes reach the model. `RunApp` dials the daemon socket if present; a listener goroutine feeds `ipc.Event`s in as `daemonEventMsg`. `cmd/digest.go` and a daemon idle-branch check both call `internal/report` + `internal/ai`.

**Tech Stack:** Go 1.25, `github.com/charmbracelet/bubbles/viewport` and `.../textinput` (bubbles is already a direct dep), `github.com/sahilm/fuzzy` (already an indirect dep — promoted to direct), `internal/report` / `internal/ai` / `internal/ipc` (all shipped in chunks 1–3). No new third-party modules.

**Spec:** `specs/2026-09-04-slash-palette-and-chat.md`, `specs/2026-09-04-review-and-digest.md` (parent: `specs/2026-09-04-adhd-focus-overview.md`)

## Global Constraints

- Go version floor: `go 1.25`.
- No new third-party modules. `viewport`, `textinput`, `fuzzy` are sub-packages / existing deps; `go mod tidy` may move `fuzzy` from indirect to direct — that is allowed.
- Screen transitions never drop the user to the shell — every new screen returns to its origin on `esc`. Only the existing dashboard quit-confirm exits.
- When the daemon is not running, the TUI behaves exactly as before: no indicator noise beyond a small `○ daemon off`, no errors, slash commands that need the daemon (`/drift` live view) degrade to "start `pomo daemon`".
- `/chat` and `/insights` with no `ai.provider` configured show a one-line "set `ai.provider` + `ai.key`" message and nothing else — never a spinner that hangs.
- `guardChatInput` runs on every `/chat` send. Rejected input never reaches `ai.Chatter.Stream`.
- The AI system prompts are fixed constants in `internal/ai` (shipped chunk 2). The TUI never lets the user set or override them.
- Chat history sent to the model is capped (last N turns) so a long session cannot grow the request unbounded.
- Commit after every task (`feat:` / `test:` / `docs:`). Work continues on `master`.

---

### Task 1: Command registry + chat input guard

**Files:**
- Create: `internal/tui/commands.go`
- Test: `internal/tui/commands_test.go` (create — first test file in `internal/tui`)

**Interfaces:**
- Consumes: nothing (pure).
- Produces:
```go
package tui

// slashCommand is one entry in the palette.
type slashCommand struct {
	Name    string   // "/review"
	Aliases []string // {"/recap"}
	Help    string
	// NeedsSession / NeedsDaemon gate availability; a gated-off command is
	// shown greyed and refuses to run.
	NeedsSession bool
	NeedsDaemon  bool
}

// slashCommands is the registry, in display order.
var slashCommands = []slashCommand{ /* /review /recap /insights /drift /chat
   /start /note /skip /settings /help /quit — see step 3 */ }

// resolveCommand returns the command whose Name or an Alias equals `name`
// (with or without the leading slash), and ok=false if none.
func resolveCommand(name string) (slashCommand, bool)

// filterCommands returns the registry entries fuzzy-matching `q` (the text
// after "/"), best first, capped at 6. Empty q returns the first 6 in order.
func filterCommands(q string) []slashCommand

const maxChatInput = 2000

// guardChatInput normalises and validates a /chat message. It trims outer
// whitespace, collapses runs of blank lines, and rejects:
//   - empty after trim
//   - longer than maxChatInput runes
//   - more than 20% non-printable runes (control chars, etc.) — a paste of
//     binary / escape junk
// On success it returns the cleaned string.
func guardChatInput(s string) (string, error)
```

- [ ] **Step 1: Write the failing test**

Create `internal/tui/commands_test.go`:
```go
package tui

import (
	"strings"
	"testing"
)

func TestResolveCommand(t *testing.T) {
	if _, ok := resolveCommand("/review"); !ok {
		t.Error("/review should resolve")
	}
	if _, ok := resolveCommand("recap"); !ok {
		t.Error("alias 'recap' (no slash) should resolve")
	}
	if _, ok := resolveCommand("/nope"); ok {
		t.Error("/nope should not resolve")
	}
}

func TestFilterCommands(t *testing.T) {
	all := filterCommands("")
	if len(all) == 0 || len(all) > 6 {
		t.Fatalf("empty query returned %d", len(all))
	}
	got := filterCommands("rev")
	found := false
	for _, c := range got {
		if c.Name == "/review" {
			found = true
		}
	}
	if !found {
		t.Errorf("filter 'rev' missing /review: %+v", got)
	}
}

func TestGuardChatInput(t *testing.T) {
	if _, err := guardChatInput("   \n  "); err == nil {
		t.Error("blank input should be rejected")
	}
	if _, err := guardChatInput(strings.Repeat("a", maxChatInput+1)); err == nil {
		t.Error("over-long input should be rejected")
	}
	if _, err := guardChatInput("why do I keep drifting?"); err != nil {
		t.Errorf("normal input rejected: %v", err)
	}
	junk := "hi\x00\x01\x02\x03\x04\x05\x06\x07\x08 there \x0e\x0f\x10\x11\x12"
	if _, err := guardChatInput(junk); err == nil {
		t.Error("control-char junk should be rejected")
	}
	clean, err := guardChatInput("  hello\n\n\n\nworld  ")
	if err != nil || clean != "hello\n\nworld" {
		t.Errorf("normalise = %q, %v", clean, err)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'TestResolve|TestFilter|TestGuard' -v`
Expected: FAIL — undefined.

- [ ] **Step 3: Implement `internal/tui/commands.go`**

```go
package tui

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/sahilm/fuzzy"
)

type slashCommand struct {
	Name         string
	Aliases      []string
	Help         string
	NeedsSession bool
	NeedsDaemon  bool
}

var slashCommands = []slashCommand{
	{Name: "/review", Aliases: []string{"/recap"}, Help: "focus vs plan + drift for a period"},
	{Name: "/insights", Help: "/review plus an AI recap"},
	{Name: "/drift", Help: "drift so far this session (or today)"},
	{Name: "/chat", Aliases: []string{"/ask"}, Help: "talk to your focus coach"},
	{Name: "/start", Help: "start a session on a task", NeedsSession: false},
	{Name: "/note", Help: "add a note to the running session", NeedsSession: true},
	{Name: "/skip", Help: "skip the running session", NeedsSession: true},
	{Name: "/settings", Aliases: []string{"/config"}, Help: "open settings"},
	{Name: "/help", Aliases: []string{"/?"}, Help: "list commands"},
	{Name: "/quit", Aliases: []string{"/q"}, Help: "quit pomo"},
}

func norm(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	return strings.TrimPrefix(name, "/")
}

func resolveCommand(name string) (slashCommand, bool) {
	n := norm(name)
	for _, c := range slashCommands {
		if norm(c.Name) == n {
			return c, true
		}
		for _, a := range c.Aliases {
			if norm(a) == n {
				return c, true
			}
		}
	}
	return slashCommand{}, false
}

func filterCommands(q string) []slashCommand {
	q = strings.TrimSpace(strings.ToLower(q))
	if q == "" {
		if len(slashCommands) <= 6 {
			return append([]slashCommand(nil), slashCommands...)
		}
		return append([]slashCommand(nil), slashCommands[:6]...)
	}
	names := make([]string, len(slashCommands))
	for i, c := range slashCommands {
		names[i] = norm(c.Name)
	}
	matches := fuzzy.Find(q, names)
	out := make([]slashCommand, 0, 6)
	for _, m := range matches {
		out = append(out, slashCommands[m.Index])
		if len(out) == 6 {
			break
		}
	}
	return out
}

const maxChatInput = 2000

func guardChatInput(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", fmt.Errorf("empty message")
	}
	if len([]rune(s)) > maxChatInput {
		return "", fmt.Errorf("message too long (max %d characters)", maxChatInput)
	}
	bad, total := 0, 0
	for _, r := range s {
		total++
		if r == '\n' || r == '\t' {
			continue
		}
		if !unicode.IsPrint(r) {
			bad++
		}
	}
	if total > 0 && bad*5 > total {
		return "", fmt.Errorf("message contains too many non-text characters")
	}
	// collapse 3+ newlines to 2
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return s, nil
}
```

- [ ] **Step 4: Run the tests + tidy**

Run: `go test ./internal/tui/ -run 'TestResolve|TestFilter|TestGuard' -v && go mod tidy && go build ./...`
Expected: PASS; `go.mod` may move `github.com/sahilm/fuzzy` to the direct require block — that is fine.

- [ ] **Step 5: Full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(tui): slash command registry and chat input guard"
```

---

### Task 2: `screenPrompt` — the palette

**Files:**
- Modify: `internal/tui/app.go` — new `screen` const, `App` fields, `Update`/`View` switch arms, `/` entry from dashboard and timer
- Create: `internal/tui/prompt.go` — `updatePrompt`, `viewPrompt`, `runSlashCommand`
- Test: `internal/tui/prompt_test.go` (create)

**Interfaces:**
- Consumes: `slashCommand`, `filterCommands`, `resolveCommand` (Task 1).
- Produces:
  - New `screen` value `screenPrompt` (append to the `const (... )` block at `app.go:24`).
  - `App` gains: `promptInput textinput.Model`, `promptOrigin screen` (where `esc` returns), `promptErr string`.
  - `App` gains `promptFrom screen` set when `/` is pressed.
  - `(a *App) enterPrompt(origin screen) (tea.Model, tea.Cmd)` — focuses the input, sets origin, `screen = screenPrompt`.
  - `(a *App) updatePrompt(msg tea.Msg) (tea.Model, tea.Cmd)` — `esc` → origin; `enter` → parse `promptInput.Value()` (`/name args…`), `resolveCommand`, gate check, dispatch via `runSlashCommand`; `tab` → complete to the top filtered match; text edits update the filtered list.
  - `(a *App) viewPrompt() string` — the input line + up to 6 `name — help` rows (greyed when gated off) + `promptErr`.
  - `(a *App) runSlashCommand(c slashCommand, args []string) (tea.Model, tea.Cmd)` — a switch. This task implements only `/settings`, `/quit`, `/start`, `/note`, `/skip`, `/help`; `/review` `/insights` `/drift` `/chat` are added in Tasks 3–5 (until then they set `promptErr = "not wired yet"` — replaced in those tasks).
  - Availability gate: `NeedsSession` && no running session, or `NeedsDaemon` && `!a.daemonUp` (field added in Task 6; default false — treat `/drift` live as allowed for now, it just shows the DB view).

- [ ] **Step 1: Write the failing test**

Create `internal/tui/prompt_test.go`:
```go
package tui

import (
	"testing"

	"pomo/internal/db/dbtest"
)

func TestEnterAndLeavePrompt(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.screen = screenDashboard
	a.enterPrompt(screenDashboard)
	if a.screen != screenPrompt {
		t.Fatalf("screen = %v, want screenPrompt", a.screen)
	}
	// esc returns to origin
	a.updatePrompt(keyMsg("esc"))
	if a.screen != screenDashboard {
		t.Fatalf("esc should return to dashboard, got %v", a.screen)
	}
}

func TestPromptUnknownCommand(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.enterPrompt(screenDashboard)
	a.promptInput.SetValue("/bogus")
	a.updatePrompt(keyMsg("enter"))
	if a.promptErr == "" || a.screen != screenPrompt {
		t.Fatalf("unknown command should stay on prompt with an error; err=%q screen=%v", a.promptErr, a.screen)
	}
}

func TestPromptOpensSettings(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.enterPrompt(screenDashboard)
	a.promptInput.SetValue("/settings")
	a.updatePrompt(keyMsg("enter"))
	if a.screen != screenSettings {
		t.Fatalf("/settings should open settings, got %v", a.screen)
	}
}
```
Add a shared test helper `internal/tui/testhelp_test.go`:
```go
package tui

import tea "github.com/charmbracelet/bubbletea"

func keyMsg(s string) tea.KeyMsg {
	switch s {
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run TestPrompt -v`
Expected: FAIL — `screenPrompt` / `enterPrompt` / `updatePrompt` undefined.

- [ ] **Step 3: Add the screen const + App fields**

In `internal/tui/app.go`:
- In the `const (` block that has `screenDashboard screen = iota` … add `screenPrompt`, `screenResult`, `screenChat` at the end (before the closing `)`). Adding at the end keeps existing iota values stable.
- In `type App struct`, add:
```go
	promptInput  textinput.Model
	promptOrigin screen
	promptErr    string

	result      viewport.Model
	resultTitle string
	resultCmd   slashCommand // the command that filled result (for `r` refresh)

	chat          viewport.Model
	chatInput     textinput.Model
	chatHistory   []ai.Msg
	chatStreaming bool
	chatErr       string

	daemonUp bool
```
  Add imports: `"github.com/charmbracelet/bubbles/viewport"`, `"pomo/internal/ai"`.
- In `NewApp`, initialise `promptInput` and `chatInput` as `textinput.New()` (prompt style like the others), and `a.result` / `a.chat` as `viewport.New(80, 20)` (resized on `WindowSizeMsg`). Wire them into the `&App{...}` literal.
- In `Update`, in the `WindowSizeMsg` branch, also set `a.result.Width, a.result.Height = wmsg.Width, wmsg.Height-4` and the same for `a.chat`.

- [ ] **Step 4: Add the Update + View switch arms**

In `Update`'s `switch a.screen` add:
```go
	case screenPrompt:
		return a.updatePrompt(msg)
	case screenResult:
		return a.updateResult(msg)
	case screenChat:
		return a.updateChat(msg)
```
In `View`'s `switch a.screen` add:
```go
	case screenPrompt:
		content = a.viewPrompt()
	case screenResult:
		content = a.viewResult()
	case screenChat:
		content = a.viewChat()
```
`updateResult`/`viewResult`/`updateChat`/`viewChat` are stubbed in this task (return `a, nil` / `""`) and filled in Tasks 3–5.

- [ ] **Step 5: Add `/` entry from dashboard and timer**

In `updateDashboard`, in the `switch km.String()`:
```go
		case "/":
			return a.enterPrompt(screenDashboard)
```
In `updateTimer` (find the `tea.KeyMsg` switch), add the same with `screenTimer` origin. If `updateTimer` forwards all keys to the embedded timer, intercept `/` before forwarding.

- [ ] **Step 6: Implement `internal/tui/prompt.go`**

```go
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
)

func (a *App) enterPrompt(origin screen) (tea.Model, tea.Cmd) {
	a.promptOrigin = origin
	a.promptErr = ""
	a.promptInput.SetValue("/")
	a.promptInput.CursorEnd()
	a.promptInput.Focus()
	a.screen = screenPrompt
	return a, textinput.Blink
}

func (a *App) updatePrompt(msg tea.Msg) (tea.Model, tea.Cmd) {
	km, ok := msg.(tea.KeyMsg)
	if ok {
		switch km.String() {
		case "esc":
			a.screen = a.promptOrigin
			return a, nil
		case "tab":
			if ms := filterCommands(a.promptQuery()); len(ms) > 0 {
				a.promptInput.SetValue(ms[0].Name + " ")
				a.promptInput.CursorEnd()
			}
			return a, nil
		case "enter":
			return a.dispatchPrompt()
		}
	}
	var cmd tea.Cmd
	a.promptInput, cmd = a.promptInput.Update(msg)
	a.promptErr = ""
	return a, cmd
}

func (a *App) promptQuery() string {
	v := strings.TrimPrefix(strings.TrimSpace(a.promptInput.Value()), "/")
	if i := strings.IndexByte(v, ' '); i >= 0 {
		v = v[:i]
	}
	return v
}

func (a *App) dispatchPrompt() (tea.Model, tea.Cmd) {
	fields := strings.Fields(a.promptInput.Value())
	if len(fields) == 0 {
		a.screen = a.promptOrigin
		return a, nil
	}
	c, ok := resolveCommand(fields[0])
	if !ok {
		a.promptErr = "unknown command: " + fields[0] + " — try /help"
		return a, nil
	}
	if c.NeedsSession && !a.hasRunningSession() {
		a.promptErr = fields[0] + " needs a running session"
		return a, nil
	}
	return a.runSlashCommand(c, fields[1:])
}

func (a *App) hasRunningSession() bool {
	s, err := a.db.LastRunningSession()
	return err == nil && s != nil
}

func (a *App) runSlashCommand(c slashCommand, args []string) (tea.Model, tea.Cmd) {
	switch c.Name {
	case "/settings":
		a.settingsCursor = 0
		a.screen = screenSettings
		return a, nil
	case "/quit":
		a.quitInput.SetValue("")
		a.quitInput.Focus()
		a.quitErr = false
		a.screen = screenQuitConfirm
		return a, textinput.Blink
	case "/start":
		a.loadTaskList()
		a.screen = screenTaskSelect
		return a, nil
	case "/note", "/skip":
		// hand off to the timer's existing handling
		a.screen = screenTimer
		if c.Name == "/skip" {
			return a.skipRunningSession()
		}
		return a, nil
	case "/help":
		a.resultTitle = "commands"
		a.result.SetContent(renderHelp())
		a.screen = screenResult
		return a, nil
	default:
		a.promptErr = c.Name + " not wired yet"
		return a, nil
	}
}

func renderHelp() string {
	var b strings.Builder
	for _, c := range slashCommands {
		name := c.Name
		if len(c.Aliases) > 0 {
			name += " (" + strings.Join(c.Aliases, ", ") + ")"
		}
		b.WriteString(name)
		b.WriteString("\n    ")
		b.WriteString(c.Help)
		b.WriteString("\n")
	}
	return b.String()
}

func (a *App) viewPrompt() string {
	var b strings.Builder
	b.WriteString("\n")
	b.WriteString(a.promptInput.View())
	b.WriteString("\n\n")
	for _, c := range filterCommands(a.promptQuery()) {
		gated := c.NeedsSession && !a.hasRunningSession()
		line := "  " + c.Name + "  " + styleMuted.Render(c.Help)
		if gated {
			line = styleDim.Render("  " + c.Name + "  " + c.Help + "  (needs a session)")
		}
		b.WriteString(line + "\n")
	}
	if a.promptErr != "" {
		b.WriteString("\n" + styleErr.Render(a.promptErr) + "\n")
	}
	b.WriteString("\n" + dimHelp("↑/↓ · tab complete · enter run · esc back"))
	return b.String()
}
```
- Add `skipRunningSession()` if a reusable skip path does not already exist — factor it out of the existing timer skip handling, or call the existing method. If the timer skip is only inline in `updateTimer`, add a small `func (a *App) skipRunningSession() (tea.Model, tea.Cmd)` that finishes the running session as `skipped` and returns to the dashboard (mirror the existing inline logic — read it first).

- [ ] **Step 7: Add the stub view/update functions**

Create minimal stubs so the build passes (filled in later tasks):
```go
// in prompt.go for now, moved to their own files in Tasks 3–5
func (a *App) updateResult(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
		a.screen = a.promptOrigin
		return a, nil
	}
	var cmd tea.Cmd
	a.result, cmd = a.result.Update(msg)
	return a, cmd
}
func (a *App) viewResult() string {
	return "\n" + styleBright.Bold(true).Render(a.resultTitle) + "\n\n" + a.result.View() +
		"\n" + dimHelp("↑/↓ scroll · r refresh · esc back")
}
func (a *App) updateChat(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
		a.screen = a.promptOrigin
		return a, nil
	}
	return a, nil
}
func (a *App) viewChat() string { return "\n" + a.chat.View() }
```

- [ ] **Step 8: Run tests + build**

Run: `go test ./internal/tui/ -run TestPrompt -v && go build ./... && make test`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat(tui): slash palette screen (screenPrompt)"
```

---

### Task 3: `/review`, `/drift` → `screenResult`

**Files:**
- Create: `internal/tui/result.go` (move the stubs from Task 2 here, flesh out)
- Modify: `internal/tui/prompt.go` — `runSlashCommand` cases for `/review`, `/drift`
- Test: `internal/tui/result_test.go` (create)

**Interfaces:**
- Consumes: `report.ParseWindow`, `report.Build`, `report.RenderText` (chunk 1); `db.DriftEventsForSession`, `db.LastRunningSession`, `db.ListDriftEvents` (chunk 1).
- Produces:
  - `runSlashCommand` `/review` (and alias `/recap`): `win, err := report.ParseWindow(argOrEmpty(args))`; on err set `promptErr`; else `sum, _ := report.Build(a.db, win)`; `a.result.SetContent(report.RenderText(sum))`; `a.resultTitle = "/review " + win.Label`; `a.resultCmd = c`; `a.screen = screenResult`.
  - `/drift`: if a session is running → `report`-style summary of `db.DriftEventsForSession(sessionID)` (episode list: start time, minutes, app, trigger); else today's `db.ListDriftEvents(todayWindow)` grouped. Render with `renderDriftView(events)`.
  - `updateResult` `r` key → re-run `a.resultCmd` (for `/review`, rebuild; for `/drift`, re-query).
  - `renderDriftView([]model.DriftEvent) string` — plain text, `report.FmtDur` for durations.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/result_test.go`:
```go
package tui

import (
	"strings"
	"testing"
	"time"

	"pomo/internal/db/dbtest"
	"pomo/internal/model"
)

func TestSlashReviewFillsResult(t *testing.T) {
	d := dbtest.NewTemp(t)
	id, _ := d.CreateSession(model.Session{
		TaskName: "t", Tag: "backend", PlannedDuration: 1500,
		Status: model.StatusRunning, StartedAt: time.Now(),
	})
	_ = d.FinishSession(id, model.StatusCompleted, 1500, "")

	a := NewApp(d)
	a.enterPrompt(screenDashboard)
	a.promptInput.SetValue("/review today")
	a.updatePrompt(keyMsg("enter"))

	if a.screen != screenResult {
		t.Fatalf("screen = %v, want screenResult", a.screen)
	}
	if !strings.Contains(a.result.View(), "1 done") {
		t.Fatalf("result missing review content:\n%s", a.result.View())
	}
}

func TestSlashReviewBadWindow(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.enterPrompt(screenDashboard)
	a.promptInput.SetValue("/review last-tuesday")
	a.updatePrompt(keyMsg("enter"))
	if a.screen != screenPrompt || a.promptErr == "" {
		t.Fatalf("bad window should stay on prompt with an error")
	}
}

func TestSlashDriftIdle(t *testing.T) {
	d := dbtest.NewTemp(t)
	a := NewApp(d)
	a.enterPrompt(screenDashboard)
	a.promptInput.SetValue("/drift")
	a.updatePrompt(keyMsg("enter"))
	if a.screen != screenResult {
		t.Fatalf("/drift should open screenResult even when idle, got %v", a.screen)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'TestSlashReview|TestSlashDrift' -v`
Expected: FAIL — `/review` returns "not wired yet".

- [ ] **Step 3: Implement `internal/tui/result.go`**

Move `updateResult` / `viewResult` out of `prompt.go` into `result.go`, add
`renderDriftView`, `argOrEmpty`, and the `r`-refresh handling. Wire `/review`,
`/recap`, `/drift` in `runSlashCommand` per the Interfaces block. Full code for
`renderDriftView`:
```go
func renderDriftView(events []model.DriftEvent) string {
	if len(events) == 0 {
		return "no drift recorded."
	}
	var b strings.Builder
	total := 0
	for _, e := range events {
		total += e.Seconds
		app := e.Detail
		if app == "" {
			app = "idle"
		}
		fmt.Fprintf(&b, "%s  %-6s  %-24s  %s\n",
			e.StartedAt.Format("15:04"), report.FmtDur(e.Seconds), app, e.Trigger)
	}
	fmt.Fprintf(&b, "\ntotal drift: %s across %d episodes\n", report.FmtDur(total), len(events))
	return b.String()
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/tui/ -run 'TestSlash|TestPrompt' -v`
Expected: PASS.

- [ ] **Step 5: Full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(tui): /review and /drift result screen"
```

---

### Task 4: `/insights` — async AI recap

**Files:**
- Modify: `internal/tui/result.go` — `/insights` case, `recapMsg`, async `tea.Cmd`
- Test: `internal/tui/insights_test.go` (create — uses a fake `ai.Recapper` via a package-level hook)

**Interfaces:**
- Consumes: `report.Build`, `ai.New` / `ai.Recapper`, `pomoconfig.Load`.
- Produces:
  - `runSlashCommand` `/insights`: same as `/review` (fill `a.result` with `RenderText`), then if `a.cfg.AI.Provider == ""` append a line `"(set ai.provider + ai.key for an AI recap)"`; else append `"RECAP  …thinking"` and return a `tea.Cmd` that builds `ai.RecapContext` from the `Summary` and calls `Recap` with an 8s context, returning `recapMsg{text, err}`.
  - `Update` (top-level or `updateResult`) handles `recapMsg` → replace the `…thinking` line with the recap text, or `"RECAP  (unavailable: <err>)"`.
  - A package var `newRecapper func(ai.Config) ai.Recapper = func(c ai.Config) ai.Recapper { _, r, _ := ai.New(c); return r }` so tests can swap it.
  - `recapContextFrom(sum report.Summary) ai.RecapContext` helper (maps `DriftByApp`→`[]string`, `ByTag`→`[]string`, `BestHour`→`"HH:00"`).

- [ ] **Step 1: Write the failing test**

Create `internal/tui/insights_test.go`:
```go
package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	"pomo/internal/ai"
	"pomo/internal/db/dbtest"
	"pomo/internal/model"
)

type fakeRecapper struct{ text string }

func (f fakeRecapper) Recap(context.Context, ai.RecapContext) (string, error) { return f.text, nil }

func TestInsightsAppendsRecap(t *testing.T) {
	d := dbtest.NewTemp(t)
	id, _ := d.CreateSession(model.Session{
		TaskName: "t", PlannedDuration: 1500, Status: model.StatusRunning, StartedAt: time.Now(),
	})
	_ = d.FinishSession(id, model.StatusCompleted, 1500, "")

	orig := newRecapper
	newRecapper = func(ai.Config) ai.Recapper { return fakeRecapper{text: "you did fine.\n→ Try: mornings"} }
	defer func() { newRecapper = orig }()

	a := NewApp(d)
	a.cfg.AI.Provider = "anthropic" // force the recap path
	a.enterPrompt(screenDashboard)
	a.promptInput.SetValue("/insights today")
	m, cmd := a.updatePrompt(keyMsg("enter"))
	a = m.(*App)
	if cmd == nil {
		t.Fatal("expected an async recap command")
	}
	msg := cmd() // run it
	m, _ = a.Update(msg)
	a = m.(*App)
	if !strings.Contains(a.result.View(), "→ Try: mornings") {
		t.Fatalf("recap not shown:\n%s", a.result.View())
	}
}

func TestInsightsNoProvider(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.cfg.AI.Provider = ""
	a.enterPrompt(screenDashboard)
	a.promptInput.SetValue("/insights today")
	a.updatePrompt(keyMsg("enter"))
	if !strings.Contains(a.result.View(), "ai.provider") {
		t.Fatalf("expected a 'set ai.provider' hint:\n%s", a.result.View())
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run TestInsights -v`
Expected: FAIL — `/insights` not wired / `newRecapper` undefined.

- [ ] **Step 3: Implement**

Add to `result.go` per the Interfaces block. `Update` must route `recapMsg`
regardless of screen (put the case in the top-level `Update` before the
`switch a.screen`, or in `updateResult`). Guard: ignore a `recapMsg` if the
user has already left `screenResult`.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/tui/ -run 'TestInsights|TestSlash|TestPrompt' -v`
Expected: PASS.

- [ ] **Step 5: Full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(tui): /insights async AI recap"
```

---

### Task 5: `/chat` — streamed conversation

**Files:**
- Create: `internal/tui/chat.go` (move chat stubs here, flesh out)
- Modify: `internal/tui/prompt.go` — `/chat` case
- Test: `internal/tui/chat_test.go` (create)

**Interfaces:**
- Consumes: `ai.New` / `ai.Chatter` / `ai.Msg`, `guardChatInput` (Task 1), `report.Build` + `db` for the context preamble, `pomoconfig`.
- Produces:
  - `runSlashCommand` `/chat` (alias `/ask`): if `a.cfg.AI.Provider == ""` → open `screenChat` showing only the "set ai.provider + ai.key" line; else open `screenChat`, focus `chatInput`. If `args` non-empty, treat as the first message (send immediately).
  - `updateChat`: `esc` → origin (abort any in-flight stream via a stored `context.CancelFunc`); `enter` → `guardChatInput(chatInput.Value())`; on error set `a.chatErr`; on success append a `user` `ai.Msg`, clear input, set `chatStreaming = true`, return the stream `tea.Cmd`.
  - Streaming pattern: `startChatStream(history []ai.Msg) tea.Cmd` opens a `chan chatDelta`, runs `chatter.Stream(ctx, systemChat, capHistory(history), func(d){ ch <- chatDelta{text:d} })` in a goroutine, closes `ch` when done; returns `waitChatDelta(ch)`. `waitChatDelta` reads one value → `chatDeltaMsg` (re-issued from `Update`) or `chatDoneMsg` / `chatErrMsg` on close/error.
  - `Update` handles `chatDeltaMsg` (append to the last assistant bubble, re-issue `waitChatDelta`), `chatDoneMsg` (`chatStreaming = false`, commit the assistant `ai.Msg` to history), `chatErrMsg` (append `[error: …]`, `chatStreaming = false`).
  - `capHistory(h []ai.Msg) []ai.Msg` — keep the last 12 messages.
  - `chatPreamble(a *App) string` — builds the hidden context: current/last session (task, planned vs actual), today's drift totals + top apps, open task list, current hour. Prepended as the first `user` message content wrapped in a clear delimiter, OR passed as extra system context — implement as a synthetic leading `ai.Msg{Role:"user", Content:"[context]\n…"}` that is always first in the capped history.
  - Package var `newChatter func(ai.Config) ai.Chatter` for tests.
  - The system prompt passed to `Stream` is `ai.SystemChat` — **export** the `systemChat` const in `internal/ai` as `SystemChat` (one-line change in `internal/ai/ai.go`; keep `systemNudge`/`systemRecap` unexported unless needed — export `SystemRecap` too if Task 4's recap path needs it; check and export as required).

- [ ] **Step 1: Write the failing test**

Create `internal/tui/chat_test.go`:
```go
package tui

import (
	"context"
	"strings"
	"testing"

	"pomo/internal/ai"
	"pomo/internal/db/dbtest"
)

type fakeChatter struct{ chunks []string }

func (f fakeChatter) Stream(ctx context.Context, system string, msgs []ai.Msg, onDelta func(string)) error {
	for _, c := range f.chunks {
		onDelta(c)
	}
	return nil
}

func TestChatRejectsJunk(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.cfg.AI.Provider = "anthropic"
	a.runSlashCommand(mustCmd("/chat"), nil)
	a.chatInput.SetValue("   ")
	a.updateChat(keyMsg("enter"))
	if a.chatErr == "" {
		t.Fatal("blank message should set chatErr")
	}
	if a.chatStreaming {
		t.Fatal("must not start a stream for rejected input")
	}
}

func TestChatStreamsDeltas(t *testing.T) {
	orig := newChatter
	newChatter = func(ai.Config) ai.Chatter { return fakeChatter{chunks: []string{"hel", "lo"}} }
	defer func() { newChatter = orig }()

	a := NewApp(dbtest.NewTemp(t))
	a.cfg.AI.Provider = "anthropic"
	a.runSlashCommand(mustCmd("/chat"), nil)
	a.chatInput.SetValue("hi")
	m, cmd := a.updateChat(keyMsg("enter"))
	a = m.(*App)
	if cmd == nil {
		t.Fatal("expected a stream command")
	}
	// drain the delta cmd chain
	for cmd != nil {
		msg := cmd()
		m, cmd = a.Update(msg)
		a = m.(*App)
	}
	if !strings.Contains(a.chat.View(), "hello") {
		t.Fatalf("streamed text missing:\n%s", a.chat.View())
	}
}

func TestChatNoProvider(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.cfg.AI.Provider = ""
	a.runSlashCommand(mustCmd("/chat"), nil)
	if a.screen != screenChat || !strings.Contains(a.viewChat(), "ai.provider") {
		t.Fatalf("no-provider chat should show the hint")
	}
}

func mustCmd(name string) slashCommand {
	c, _ := resolveCommand(name)
	return c
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run TestChat -v`
Expected: FAIL.

- [ ] **Step 3: Implement `internal/tui/chat.go` + export `ai.SystemChat`**

Per the Interfaces block. In `internal/ai/ai.go` rename `systemChat` → `SystemChat` (exported) and update its one use in `stream.go` if any (the daemon does not use it; Task 5 does). Keep the streaming `tea.Cmd` chain small and documented.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/tui/ -run 'TestChat|TestInsights|TestSlash|TestPrompt' -v`
Expected: PASS.

- [ ] **Step 5: Full suite + race on tui**

Run: `make test && go test -race ./internal/tui/`
Expected: PASS, no races (the stream goroutine writes only to its channel).

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(tui): /chat streamed conversation with input guard"
```

---

### Task 6: IPC client — daemon indicator, checkpoint & nudge overlays

**Files:**
- Modify: `internal/tui/app.go` — dial IPC in `RunApp`/`RunAppTaskSelect`, listener, `daemonEventMsg` handling, overlays
- Create: `internal/tui/daemonlink.go` — the IPC glue
- Modify: `internal/tui/dashboard.go` — `● daemon up` / `○ daemon off` indicator
- Test: `internal/tui/daemonlink_test.go` (create)

**Interfaces:**
- Consumes: `ipc.Dial`, `ipc.SocketPath`, `ipc.Event` (chunk 2/3).
- Produces:
  - `App` gains `ipcClient *ipc.Client`, `daemonEvents chan ipc.Event`, `nudgeOverlay *nudgeOverlay`, `checkpointActive bool`.
  - `RunApp` (and `RunAppTaskSelect`): after `NewApp`, call `a.connectDaemon()` — `ipc.Dial(ipc.SocketPath(), func(e){ a.daemonEvents <- e })`; on failure set `a.daemonUp = false` and leave `ipcClient` nil (no retry loop; one retry when a session starts — call `connectDaemon` again in `beginSession` / `runStart` path if `ipcClient == nil`).
  - `Init` returns `tea.Batch(dashTick(), a.waitDaemonEvent())`.
  - `waitDaemonEvent() tea.Cmd` — blocks on `<-a.daemonEvents`, returns `daemonEventMsg{e}`; re-issued from `Update` after each one.
  - `Update` `daemonEventMsg` cases:
    - `"watching"` → `a.daemonUp = true`
    - `"idle"` → `a.daemonUp = true` (still connected)
    - `"checkpoint"` → `a.checkpointActive = true` (overlay shown on `screenTimer`)
    - `"nudge"` → `a.nudgeOverlay = &nudgeOverlay{text: e.Text, level: e.Level, actions: e.Actions}`
  - On `screenTimer`, `updateTimer` intercepts (before forwarding to the timer):
    - if `a.checkpointActive`: `y`/`n` → `a.ipcClient.Send(ipc.Event{Type:"checkpoint-answer", Answer:"y|n"})`, `a.checkpointActive = false`.
    - if `a.nudgeOverlay != nil`: keys map to actions (`b`→break via existing flow + `nudge-action`/`b`; `r`→`nudge-action`/`r`; `d`→`nudge-action`/`d`; `s`/`esc`→`nudge-action`/`snooze` + dismiss). Send via `ipcClient` when non-nil, then clear the overlay.
  - `viewTimer` (or the `screenTimer` arm of `View`) appends the checkpoint / nudge overlay box under the timer.
  - `dashboard.go` view: a small right-aligned `● daemon up` (green) when `a.daemonUp`, else `○ daemon off` (dim). Pass `daemonUp` into the dashboard render (add a field or a param).
  - On program exit (`tea.Quit` path), close `a.ipcClient` if non-nil.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/daemonlink_test.go`:
```go
package tui

import (
	"testing"

	"pomo/internal/db/dbtest"
	"pomo/internal/ipc"
)

func TestDaemonEventUpdatesState(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.daemonEvents = make(chan ipc.Event, 4)

	m, _ := a.Update(daemonEventMsg{ipc.Event{Type: "watching", SessionID: 3}})
	a = m.(*App)
	if !a.daemonUp {
		t.Fatal("watching event should set daemonUp")
	}

	m, _ = a.Update(daemonEventMsg{ipc.Event{Type: "nudge", Text: "still on it?", Level: 1, Actions: []string{"snooze", "drifted"}}})
	a = m.(*App)
	if a.nudgeOverlay == nil || a.nudgeOverlay.text != "still on it?" {
		t.Fatalf("nudge event should set the overlay: %+v", a.nudgeOverlay)
	}

	m, _ = a.Update(daemonEventMsg{ipc.Event{Type: "checkpoint", Text: "on task?"}})
	a = m.(*App)
	if !a.checkpointActive {
		t.Fatal("checkpoint event should raise the checkpoint overlay")
	}
}

func TestCheckpointAnswerClearsOverlay(t *testing.T) {
	a := NewApp(dbtest.NewTemp(t))
	a.screen = screenTimer
	a.checkpointActive = true
	a.updateTimer(keyMsg("y")) // no ipcClient; must not panic, must clear
	if a.checkpointActive {
		t.Fatal("answering the checkpoint should clear it")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run 'TestDaemonEvent|TestCheckpointAnswer' -v`
Expected: FAIL — `daemonEventMsg` / `nudgeOverlay` / fields undefined.

- [ ] **Step 3: Implement `internal/tui/daemonlink.go`**

```go
package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"pomo/internal/ipc"
)

type daemonEventMsg struct{ e ipc.Event }

type nudgeOverlay struct {
	text    string
	level   int
	actions []string
}

func (a *App) connectDaemon() {
	if a.daemonEvents == nil {
		a.daemonEvents = make(chan ipc.Event, 16)
	}
	c, err := ipc.Dial(ipc.SocketPath(), func(e ipc.Event) {
		select {
		case a.daemonEvents <- e:
		default: // drop if the UI is far behind
		}
	})
	if err != nil {
		a.daemonUp = false
		return
	}
	a.ipcClient = c
	a.daemonUp = true
}

func (a *App) waitDaemonEvent() tea.Cmd {
	return func() tea.Msg {
		e, ok := <-a.daemonEvents
		if !ok {
			return nil
		}
		return daemonEventMsg{e}
	}
}

func (a *App) sendDaemon(e ipc.Event) {
	if a.ipcClient != nil {
		_ = a.ipcClient.Send(e)
	}
}
```
Then wire the rest per the Interfaces block. `daemonEventMsg` handling goes in
the top-level `Update` (before `switch a.screen`), and it must **re-issue**
`a.waitDaemonEvent()` so the stream continues.

- [ ] **Step 4: `dashboard.go` indicator**

Read `internal/tui/dashboard.go`; add a `daemonUp bool` field to
`DashboardModel` (set from `App` on each render, or pass through `View`), and
render `● daemon up` / `○ daemon off` in a dim style in a corner of the
dashboard header.

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/tui/ -run 'TestDaemon|TestCheckpoint|TestChat|TestInsights|TestSlash|TestPrompt' -v`
Expected: PASS.

- [ ] **Step 6: Full suite + build + live smoke**

Run:
```bash
make test && go build -o pomo . && D=$(mktemp -d) && HOME=$D ./pomo daemon start && sleep 1 && HOME=$D ./pomo daemon status && HOME=$D ./pomo daemon stop
```
Expected: suite PASS; daemon still starts/stops clean (no regression from the IPC server side).

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(tui): daemon link — indicator, checkpoint and nudge overlays"
```

---

### Task 7: `pomo digest` + daemon weekly auto-write

**Files:**
- Create: `cmd/digest.go`
- Create: `internal/report/digest.go` — `WeeklyDigestPath`, `WriteWeeklyDigest`
- Modify: `internal/daemon/daemon.go` — `maybeWeeklyDigest` in the idle branch
- Test: `internal/report/digest_test.go`, `cmd/digest_test.go` (create)

**Interfaces:**
- Consumes: `report.Build` / `report.RenderMarkdown` (chunk 1), `ai.Recapper` (chunk 2), `db.Dir`.
- Produces:
  - `report.WeeklyDigestPath(label string) string` → `filepath.Join(db.Dir(), "reviews", label+".md")`.
  - `report.WriteWeeklyDigest(d *db.DB, w Window, recap string) (string, error)` — `MkdirAll` the `reviews/` dir, `RenderMarkdown(Build(d,w), recap)`, write, return the path. Idempotent (overwrites).
  - `cmd/digest.go`: `pomo digest [--week YYYY-Www] [--notify]` — default week = current ISO week; build the recap via `ai` if `cfg.AI.Provider != ""` (8s timeout, omit on error); write; print the path; `--notify` → `notify.Send`.
  - `daemon.Loop` idle branch: `l.maybeWeeklyDigest()` — guarded by an in-memory `lastDigestCheck` date; only acts on `l.now().Weekday() == time.Monday` and when last week's file is missing; builds the recap with the loop's `ai` (needs a `Recapper` — extend `Deps` with `Recap ai.Recapper` or reuse a combined `AI` field; simplest: add `Recap ai.Recapper` to `Deps`, wired in `cmd/daemon.go` from the same `ai.New`). Notification gated by `cfg.Digest.Notify`.

- [ ] **Step 1: Write the failing tests**

`internal/report/digest_test.go`:
```go
package report_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pomo/internal/db/dbtest"
	"pomo/internal/model"
	"pomo/internal/report"
)

func TestWriteWeeklyDigest(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	d := dbtest.NewTemp(t)
	now := time.Now()
	id, _ := d.CreateSession(model.Session{
		TaskName: "t", PlannedDuration: 1500, Status: model.StatusRunning, StartedAt: now,
	})
	_ = d.FinishSession(id, model.StatusCompleted, 1500, "")

	w := report.ThisWeek()
	path, err := report.WriteWeeklyDigest(d, w, "the recap")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != w.Label+".md" {
		t.Fatalf("path = %q", path)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "the recap") || !strings.Contains(string(b), "# Pomo") {
		t.Fatalf("digest content wrong:\n%s", b)
	}
	// idempotent
	if _, err := report.WriteWeeklyDigest(d, w, ""); err != nil {
		t.Fatalf("second write: %v", err)
	}
}
```
`cmd/digest_test.go`:
```go
package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pomo/internal/db/dbtest"
)

func TestRunDigestWritesFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	database = dbtest.NewTemp(t)
	t.Cleanup(func() { database = nil })

	var buf bytes.Buffer
	if err := runDigest("", false, &buf); err != nil {
		t.Fatal(err)
	}
	out := strings.TrimSpace(buf.String())
	if !strings.HasSuffix(out, ".md") {
		t.Fatalf("expected a path, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".pomo", "reviews")); err != nil {
		t.Fatalf("reviews dir not created: %v", err)
	}
}
```

- [ ] **Step 2: Run to verify they fail**

Run: `go test ./internal/report/ ./cmd/ -run 'TestWriteWeeklyDigest|TestRunDigest' -v`
Expected: FAIL.

- [ ] **Step 3: Implement**

`internal/report/digest.go`, `cmd/digest.go` (`runDigest(weekSpec string, notify bool, w io.Writer) error` for testability), and `maybeWeeklyDigest` on `daemon.Loop`. Wire `Deps.Recap` in `cmd/daemon.go`.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/report/ ./cmd/ ./internal/daemon/ -v`
Expected: PASS.

- [ ] **Step 5: Full suite + smoke**

Run:
```bash
make test && go build -o pomo . && D=$(mktemp -d) && HOME=$D ./pomo digest && ls $D/.pomo/reviews/
```
Expected: PASS; a `YYYY-Www.md` file exists.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat: pomo digest and daemon weekly auto-write"
```

---

### Task 8: `/settings` rows for drift / AI keys

**Files:**
- Modify: `internal/tui/app.go` — `settingRow*` consts, `updateSettings`, `settingsView`
- Test: `internal/tui/settings_test.go` (create)

**Interfaces:**
- Consumes: `db.SetConfig`, `pomoconfig.Load`, `pomoconfig.MaskKey`, `cmd`-side validation is not reachable — replicate the minimal checks inline (provider cycle, bool toggle).
- Produces:
  - New rows after the existing sound rows: `Drift Detection` (`drift.enabled` bool), `Checkpoint Prompt` (`checkpoint.enabled` bool), `Nudges` (`nudge.enabled` bool), `AI Provider` (cycle `off`→`anthropic`→`openrouter`), `AI Key` (masked; `enter` opens a small masked `textinput`), `AI Model` (`textinput`).
  - Each edit writes via `a.db.SetConfig` then `a.cfg = pomoconfig.Load(a.db)`.
  - `settingRowCount` updated; cursor clamping still works.
  - `AI Key` display uses `pomoconfig.MaskKey`.

- [ ] **Step 1: Write the failing test**

Create `internal/tui/settings_test.go`:
```go
package tui

import (
	"testing"

	"pomo/internal/db/dbtest"
)

func TestSettingsToggleDrift(t *testing.T) {
	d := dbtest.NewTemp(t)
	a := NewApp(d)
	a.screen = screenSettings
	a.settingsCursor = settingRowDriftEnabled

	// toggle off
	a.updateSettings(keyMsg("enter"))
	if a.cfg.Drift.Enabled {
		t.Fatal("drift.enabled should be false after toggle")
	}
	if got := d.GetConfig("drift.enabled", "true"); got != "false" {
		t.Fatalf("persisted value = %q", got)
	}
}

func TestSettingsCycleProvider(t *testing.T) {
	d := dbtest.NewTemp(t)
	a := NewApp(d)
	a.screen = screenSettings
	a.settingsCursor = settingRowAIProvider

	a.updateSettings(keyMsg("enter")) // off -> anthropic
	if a.cfg.AI.Provider != "anthropic" {
		t.Fatalf("provider = %q, want anthropic", a.cfg.AI.Provider)
	}
	a.updateSettings(keyMsg("enter")) // anthropic -> openrouter
	a.updateSettings(keyMsg("enter")) // openrouter -> off
	if a.cfg.AI.Provider != "" {
		t.Fatalf("provider should cycle back to off, got %q", a.cfg.AI.Provider)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/tui/ -run TestSettings -v`
Expected: FAIL — `settingRowDriftEnabled` etc. undefined.

- [ ] **Step 3: Implement**

Extend the `settingRow` const block and `updateSettings` / `settingsView` in
`app.go`. Read the existing settings handling first and match its style
exactly (cursor movement, `enter`/`←`/`→` semantics). Keep the AI-key text
input minimal — a masked `textinput` shown only while that row is being edited.

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/tui/ -run 'TestSettings|TestChat|TestDaemon|TestSlash|TestPrompt|TestInsights' -v`
Expected: PASS.

- [ ] **Step 5: Full suite + build + manual**

Run: `make test && go build -o pomo . && go vet ./...`
Expected: PASS. Manual: `go run .` → `s` → arrow to the new rows → toggle drift, cycle provider, confirm they stick after re-opening settings.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(tui): settings rows for drift and AI config"
```

---

### Task 9: Docs + spec close-out

**Files:**
- Modify: `CLAUDE.md`, `README.md`, `specs/README.md`

**Interfaces:**
- Consumes: nothing.
- Produces: docs reflect the finished feature.

- [ ] **Step 1: CLAUDE.md**

Under **Architecture**, update the `internal/tui` bullet to mention the slash
palette (`commands.go`, `prompt.go`, `result.go`, `chat.go`, `daemonlink.go`)
and the IPC client. Add a line under **Commands** for `pomo digest`.

- [ ] **Step 2: README.md**

Move the "Slash-command surface" and "Weekly digest" bullets from "Still to
come" to "Shipped". Add `/chat`, `/review`, `/insights`, `/drift` to the
feature list under a new "In the dashboard" note. Add `pomo digest` to the
command reference. Update the "Shipped so far" paragraph to say the ADHD focus
layer is complete (daemon + nudges + slash surface + digest), with the
remaining backlog items listed (launchd/systemd unit, linux foreground
watching, browser-tab attribution under denied automation permission).

- [ ] **Step 3: specs/README.md**

Mark chunk 4 done. Add a closing line: "ADHD focus layer complete; see backlog
in the overview spec for deferred items."

- [ ] **Step 4: Verify + commit**

Run: `make test`
Expected: PASS.
```bash
git add -A
git commit -m "docs: close out the ADHD focus layer"
```

---

## Self-Review

**1. Spec coverage**

`specs/2026-09-04-slash-palette-and-chat.md`:
- §2 new screens `screenPrompt` / `screenResult` (spec calls it that) / `screenChat` — Task 2. ✓
- §3 command registry with `Name`/`Aliases`/`Help`, `/recap` alias of `/review`, `/insights` = review + forced recap — Tasks 1, 3, 4. ✓
- §3 gated commands shown greyed + refuse — Task 2 (`NeedsSession`). `NeedsDaemon` is defined but only `/drift` would use it and `/drift` is allowed to fall back to the DB view, so it is effectively unused this chunk — noted, harmless.
- §4 palette UX (textinput, fuzzy list ≤6, ↑/↓, tab, enter, esc, unknown-command message) — Tasks 1–2. ↑/↓ selection movement within the list: the plan uses tab-completes-top-match rather than an explicitly selectable list. Minor simplification — noted; if the reviewer wants arrow selection it is a small follow-up.
- §5 `screenResult` viewport, esc, r refresh — Task 3. ✓
- §6 `screenChat` — scrollback + input, in-program history, streaming token-by-token, per-send context preamble, 30s timeout, error shows partial + `[error]`, no-provider placeholder — Task 5. ✓
- §6 the Bubble Tea streaming channel pattern — Task 5 documented. ✓
- §7 IPC client in the TUI: dial on start, retry on session start, `checkpoint` / `nudge` / `watching` / `idle` handling, timer overlays, dashboard indicator — Task 6. ✓
- §8 IPC protocol — already shipped chunk 2/3; consumed here. ✓
- **Chat input guard** (user's explicit ask): `guardChatInput` — empty / over-long / control-char-junk rejection + normalisation, run on every send, rejected input never reaches `Stream` — Task 1 + Task 5. History cap (`capHistory`) prevents unbounded request growth. ✓

`specs/2026-09-04-review-and-digest.md`:
- `/review` `/recap` render via `internal/report` — Task 3. ✓
- `/insights` with async AI `RECAP` block, pending state, error/no-provider handling — Task 4. ✓
- `/drift` current-session vs today — Task 3. ✓
- `pomo review --json` CLI — already shipped chunk 1.
- `pomo digest` + markdown shape + idempotent + `--week` + `--notify` — Task 7. ✓
- Daemon Monday auto-write, guarded, no notification by default — Task 7. ✓
- `internal/report` refactor sharing with TUI stats — already done chunk 1.
- `--include-notes` — the CLI flag exists (chunk 1, stubbed); the slash form does not expose it, per spec. ✓

Deferred (backlog, documented in Task 9, not gaps): launchd/systemd unit, linux foreground watching, browser-tab attribution when automation permission is denied.

**2. Placeholder scan** — Tasks 3–8 describe several `app.go` edits as "per the Interfaces block" / "match the existing style" rather than full listings, because they are modifications threaded into a 750-line existing file whose surrounding style must be followed; every new symbol, its signature, and its behaviour is fully specified in the Interfaces block, and each task ships full code for its *new* files (`commands.go`, `prompt.go` core, `result.go` helpers, `chat.go` pattern, `daemonlink.go` in full, `digest.go`). Tests are complete code in every task. No "TODO" / "handle errors" / "similar to Task N".

**3. Type consistency**
- `slashCommand{Name,Aliases,Help,NeedsSession,NeedsDaemon}`, `slashCommands`, `resolveCommand`, `filterCommands`, `guardChatInput`, `maxChatInput` — Task 1 def; Tasks 2–5 consume exactly these. ✓
- `screen` consts `screenPrompt`/`screenResult`/`screenChat` — Task 2 adds them; Tasks 3–6 reference them. ✓
- `App` new fields (`promptInput`, `promptOrigin`, `promptErr`, `result`, `resultTitle`, `resultCmd`, `chat`, `chatInput`, `chatHistory`, `chatStreaming`, `chatErr`, `daemonUp`, `ipcClient`, `daemonEvents`, `nudgeOverlay`, `checkpointActive`) — introduced in Tasks 2 and 6; every later use matches. `resultCmd` is `slashCommand` (Task 3), consistent with Task 1's type.
- `newRecapper func(ai.Config) ai.Recapper`, `newChatter func(ai.Config) ai.Chatter` — Tasks 4/5 def as swappable package vars; tests reassign them with matching signatures. `ai.Recapper.Recap(context.Context, ai.RecapContext) (string,error)` and `ai.Chatter.Stream(context.Context, string, []ai.Msg, func(string)) error` — exact chunk-2 signatures; `fakeRecapper` / `fakeChatter` in the tests implement them verbatim. ✓
- `ai.SystemChat` — Task 5 exports the previously-unexported `systemChat`; the only consumer is `chat.go`. `internal/ai/stream.go` currently inlines no system constant (it takes `system` as a param), so nothing else changes. ✓
- `recapMsg{text string; err error}`, `chatDelta`, `chatDeltaMsg`, `chatDoneMsg`, `chatErrMsg`, `daemonEventMsg{e ipc.Event}` — message types defined in Tasks 4/5/6; each `Update` case matches its producer.
- `ipc.Dial(path string, onRecv func(ipc.Event)) (*ipc.Client, error)`, `(*ipc.Client).Send(ipc.Event) error`, `(*ipc.Client).Close() error`, `ipc.SocketPath() string`, `ipc.Event` fields — all chunk-2 defs; Task 6 uses them unchanged. ✓
- `report.ParseWindow`, `report.Build`, `report.RenderText`, `report.RenderMarkdown`, `report.FmtDur`, `report.ThisWeek`, `report.Window`, `report.Summary` (+ `DriftByApp`/`ByTag`/`BestHour`) — chunk-1 defs; Tasks 3/4/7 consume them. `report.WeeklyDigestPath` / `report.WriteWeeklyDigest` — new in Task 7, consumed by Task 7's CLI + daemon only. ✓
- `daemon.Deps` gains `Recap ai.Recapper` — Task 7; `cmd/daemon.go` `runDaemon` wires it from `ai.New`; `daemon_test.go` baseline `Deps` literals in chunk 3 do not set it (zero value nil) — `maybeWeeklyDigest` must nil-check `l.d.Recap` before calling. Noted in Task 7 Step 3.
- `pomoconfig.MaskKey`, `pomoconfig.Load`, `pomoconfig.Config.AI` / `.Drift` / `.Nudge` / `.CheckpointEnabled` — chunk-2 defs; Task 8 consumes them. ✓
- `db.GetConfig` / `db.SetConfig` / `db.LastRunningSession` / `db.DriftEventsForSession` / `db.ListDriftEvents` — chunk-1 defs, unchanged.

No inconsistencies found.
