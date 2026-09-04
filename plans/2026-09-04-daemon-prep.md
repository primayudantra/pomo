# Daemon Prep Implementation Plan (Chunk 2)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the three self-contained support packages the drift daemon needs — config keys (`internal/pomoconfig`), the IPC socket layer (`internal/ipc`), and the BYOK AI provider layer (`internal/ai`) — with no daemon and no behaviour change to existing commands.

**Architecture:** Additive typed config groups parsed in one pass from the `config` table. A newline-delimited-JSON Unix-domain-socket server/client in `internal/ipc` with fan-out broadcast and stale-socket cleanup. An `internal/ai` package exposing `Nudger` / `Recapper` / `Chatter` interfaces over a provider switch (`anthropic` | `openrouter`) implemented with raw `net/http` (the Anthropic Go SDK is Anthropic-only and we need OpenRouter too); an empty provider yields no-op implementations that return `ErrNoProvider`.

**Tech Stack:** Go 1.25, stdlib only (`net`, `net/http`, `encoding/json`, `bufio`, `context`, `net/http/httptest` for tests). No new dependencies. `modernc.org/sqlite` and `cobra` already present.

**Spec:** `specs/2026-09-04-schema-config-migration.md` §3–4, `specs/2026-09-04-nudge-and-ai.md` §3 & §5, `specs/2026-09-04-slash-palette-and-chat.md` §8 (parent: `specs/2026-09-04-adhd-focus-overview.md`)

## Global Constraints

- Go version floor: `go 1.25` — do not raise it.
- No new third-party dependencies. Stdlib + libraries already in `go.mod` only.
- No change to existing runtime behaviour: existing `pomo` commands, the timer, and TUI screens must behave identically. The only user-visible change is new keys listed by `pomo config` and accepted by `pomo config set`.
- SQLite config values are strings in the `config` table via `db.GetConfig`/`db.SetConfig`/`db.AllConfig` — unchanged.
- IPC socket path is `filepath.Join(db.Dir(), "daemon.sock")` — never hardcode.
- AI default model: `claude-haiku-4-5` (anthropic) / `anthropic/claude-3.5-haiku` (openrouter). Anthropic API version header: `anthropic-version: 2023-06-01`.
- Durations in config are Go duration strings (`"15s"`, `"3m"`, `"10m"`). Parse with `time.ParseDuration`; on parse failure keep the default, never error.
- `ai.key` is secret: mask it everywhere it is displayed (`sk-…` + last 4 chars).
- Commit after every task with a `feat:` / `test:` prefixed message. Work goes on `master` (continues the chunk-1 history).

---

### Task 1: New config groups in `internal/pomoconfig`

**Files:**
- Modify: `internal/pomoconfig/pomoconfig.go`
- Test: `internal/pomoconfig/pomoconfig_test.go` (create)

**Interfaces:**
- Consumes: `db.AllConfig() (map[string]string, error)` (existing), `dbtest.NewTemp` (chunk 1).
- Produces: `pomoconfig.Config` gains five nested structs and one bool, all populated by the existing `Load(d *db.DB) Config`:

```go
type DaemonConfig struct { Tick time.Duration }

type DriftConfig struct {
	Enabled           bool
	DistractGrace     time.Duration
	FsStale           time.Duration
	CheckpointTimeout time.Duration
	CheckpointPenalty time.Duration
	RecoverGrace      time.Duration
	FocusApps         []string // built-in list + user additions, lowercased
	DistractApps      []string
	NeutralApps       []string
	FocusTitleHints   []string
}

type NudgeConfig struct {
	Enabled       bool
	MaxPerSession int
	MinGap        time.Duration
}

type AIConfig struct {
	Provider string // "" | "anthropic" | "openrouter"
	Key      string
	Model    string // "" => provider default
}

type DigestConfig struct { Notify bool }
```
  `Config` also gains `CheckpointEnabled bool`.
- Exported helpers:
  - `pomoconfig.DefaultFocusApps() []string`, `DefaultDistractApps() []string`, `DefaultFocusTitleHints() []string` — the built-in lists (so other packages and tests can reference them).
  - `pomoconfig.MaskKey(s string) string` → `"(unset)"` for `""`, else first-3 + `"…"` + last-4 (or `"…"+s` if shorter than 8).

- [ ] **Step 1: Write the failing tests**

Create `internal/pomoconfig/pomoconfig_test.go`:
```go
package pomoconfig_test

import (
	"testing"
	"time"

	"pomo/internal/db/dbtest"
	"pomo/internal/pomoconfig"
)

func TestDefaultsForNewGroups(t *testing.T) {
	d := dbtest.NewTemp(t)
	c := pomoconfig.Load(d)

	if c.Daemon.Tick != 15*time.Second {
		t.Errorf("Daemon.Tick = %v, want 15s", c.Daemon.Tick)
	}
	if !c.Drift.Enabled || !c.Nudge.Enabled || !c.CheckpointEnabled {
		t.Errorf("enabled flags should default true: %+v", c)
	}
	if c.Drift.DistractGrace != 3*time.Minute || c.Drift.FsStale != 10*time.Minute {
		t.Errorf("drift durations wrong: %+v", c.Drift)
	}
	if c.Nudge.MaxPerSession != 4 || c.Nudge.MinGap != 5*time.Minute {
		t.Errorf("nudge defaults wrong: %+v", c.Nudge)
	}
	if c.AI.Provider != "" || c.AI.Key != "" {
		t.Errorf("AI should be empty by default: %+v", c.AI)
	}
	if c.Digest.Notify {
		t.Errorf("Digest.Notify should default false")
	}
	if len(c.Drift.DistractApps) == 0 || len(c.Drift.FocusApps) == 0 {
		t.Errorf("built-in app lists should be populated")
	}
}

func TestOverridesAndCSVAppend(t *testing.T) {
	d := dbtest.NewTemp(t)
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(d.SetConfig("daemon.tick", "30s"))
	must(d.SetConfig("drift.enabled", "false"))
	must(d.SetConfig("nudge.max_per_session", "2"))
	must(d.SetConfig("ai.provider", "anthropic"))
	must(d.SetConfig("ai.key", "sk-ant-secret"))
	must(d.SetConfig("drift.distract_apps", "figma, notion"))

	c := pomoconfig.Load(d)
	if c.Daemon.Tick != 30*time.Second {
		t.Errorf("tick override failed: %v", c.Daemon.Tick)
	}
	if c.Drift.Enabled {
		t.Errorf("drift.enabled=false not applied")
	}
	if c.Nudge.MaxPerSession != 2 {
		t.Errorf("nudge override failed: %d", c.Nudge.MaxPerSession)
	}
	if c.AI.Provider != "anthropic" || c.AI.Key != "sk-ant-secret" {
		t.Errorf("ai override failed: %+v", c.AI)
	}
	// user additions appended, built-ins retained
	has := func(list []string, want string) bool {
		for _, v := range list {
			if v == want {
				return true
			}
		}
		return false
	}
	if !has(c.Drift.DistractApps, "figma") || !has(c.Drift.DistractApps, "notion") {
		t.Errorf("csv additions missing: %v", c.Drift.DistractApps)
	}
	if !has(c.Drift.DistractApps, "chrome") {
		t.Errorf("built-in distract app dropped: %v", c.Drift.DistractApps)
	}
}

func TestBadValuesFallBackToDefault(t *testing.T) {
	d := dbtest.NewTemp(t)
	_ = d.SetConfig("daemon.tick", "not-a-duration")
	_ = d.SetConfig("nudge.max_per_session", "abc")
	_ = d.SetConfig("ai.provider", "openai") // unsupported

	c := pomoconfig.Load(d)
	if c.Daemon.Tick != 15*time.Second {
		t.Errorf("bad duration should fall back: %v", c.Daemon.Tick)
	}
	if c.Nudge.MaxPerSession != 4 {
		t.Errorf("bad int should fall back: %d", c.Nudge.MaxPerSession)
	}
	if c.AI.Provider != "" {
		t.Errorf("unsupported provider should fall back to empty: %q", c.AI.Provider)
	}
}

func TestMaskKey(t *testing.T) {
	cases := map[string]string{
		"":                 "(unset)",
		"short":            "…short",
		"sk-ant-abcdef1234": "sk-…1234",
	}
	for in, want := range cases {
		if got := pomoconfig.MaskKey(in); got != want {
			t.Errorf("MaskKey(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/pomoconfig/ -v`
Expected: FAIL — undefined fields / helpers.

- [ ] **Step 3: Implement**

Replace `internal/pomoconfig/pomoconfig.go` with (keeps every existing field and the existing parsing exactly, adds the new groups):
```go
package pomoconfig

import (
	"strconv"
	"strings"
	"time"

	"pomo/internal/db"
	"pomo/internal/sound"
)

type Config struct {
	Focus              time.Duration
	ShortBreak         time.Duration
	LongBreak          time.Duration
	SessionsBeforeLong int
	AutoStartBreak     bool
	AutoStartFocus     bool
	Sound              bool
	SoundChoice        sound.ID
	Notifications      bool

	Daemon           DaemonConfig
	Drift            DriftConfig
	Nudge            NudgeConfig
	AI               AIConfig
	Digest           DigestConfig
	CheckpointEnabled bool
}

type DaemonConfig struct{ Tick time.Duration }

type DriftConfig struct {
	Enabled           bool
	DistractGrace     time.Duration
	FsStale           time.Duration
	CheckpointTimeout time.Duration
	CheckpointPenalty time.Duration
	RecoverGrace      time.Duration
	FocusApps         []string
	DistractApps      []string
	NeutralApps       []string
	FocusTitleHints   []string
}

type NudgeConfig struct {
	Enabled       bool
	MaxPerSession int
	MinGap        time.Duration
}

type AIConfig struct {
	Provider string
	Key      string
	Model    string
}

type DigestConfig struct{ Notify bool }

func DefaultFocusApps() []string {
	return []string{"terminal", "iterm", "alacritty", "kitty", "wezterm", "ghostty",
		"code", "vscode", "zed", "nvim", "vim", "emacs", "jetbrains", "xcode", "preview", "dash"}
}

func DefaultDistractApps() []string {
	return []string{"chrome", "safari", "firefox", "arc", "brave", "edge", "slack",
		"discord", "telegram", "whatsapp", "messages", "mail", "youtube", "twitter",
		"x.com", "reddit", "netflix", "steam"}
}

func DefaultFocusTitleHints() []string {
	return []string{"localhost", "github.com", "stackoverflow", "pomo", "docs."}
}

func defaults() Config {
	return Config{
		Focus:              25 * time.Minute,
		ShortBreak:         5 * time.Minute,
		LongBreak:          15 * time.Minute,
		SessionsBeforeLong: 4,
		AutoStartBreak:     true,
		AutoStartFocus:     false,
		Sound:              true,
		SoundChoice:        sound.Start,
		Notifications:      true,

		Daemon:            DaemonConfig{Tick: 15 * time.Second},
		CheckpointEnabled: true,
		Drift: DriftConfig{
			Enabled:           true,
			DistractGrace:     3 * time.Minute,
			FsStale:           10 * time.Minute,
			CheckpointTimeout: 20 * time.Second,
			CheckpointPenalty: 2 * time.Minute,
			RecoverGrace:      1 * time.Minute,
			FocusApps:         DefaultFocusApps(),
			DistractApps:      DefaultDistractApps(),
			NeutralApps:       nil,
			FocusTitleHints:   DefaultFocusTitleHints(),
		},
		Nudge: NudgeConfig{Enabled: true, MaxPerSession: 4, MinGap: 5 * time.Minute},
		AI:    AIConfig{},
		Digest: DigestConfig{Notify: false},
	}
}

func Load(d *db.DB) Config {
	c := defaults()
	m, err := d.AllConfig()
	if err != nil {
		return c
	}

	dur := func(key string, dst *time.Duration) {
		if v, ok := m[key]; ok {
			if parsed, err := time.ParseDuration(v); err == nil {
				*dst = parsed
			}
		}
	}
	boolean := func(key string, dst *bool) {
		if v, ok := m[key]; ok {
			*dst = v == "true"
		}
	}
	integer := func(key string, dst *int) {
		if v, ok := m[key]; ok {
			if n, err := strconv.Atoi(v); err == nil {
				*dst = n
			}
		}
	}
	appendCSV := func(key string, dst *[]string) {
		if v, ok := m[key]; ok {
			for _, part := range strings.Split(v, ",") {
				part = strings.ToLower(strings.TrimSpace(part))
				if part != "" {
					*dst = append(*dst, part)
				}
			}
		}
	}

	// --- existing keys (unchanged) ---
	dur("focus", &c.Focus)
	dur("short_break", &c.ShortBreak)
	dur("long_break", &c.LongBreak)
	integer("sessions_before_long", &c.SessionsBeforeLong)
	boolean("auto_start_break", &c.AutoStartBreak)
	boolean("auto_start_focus", &c.AutoStartFocus)
	boolean("sound", &c.Sound)
	if v, ok := m["sound_choice"]; ok && v != "" {
		c.SoundChoice = sound.ID(v)
	}
	boolean("notifications", &c.Notifications)

	// --- new keys ---
	dur("daemon.tick", &c.Daemon.Tick)
	boolean("checkpoint.enabled", &c.CheckpointEnabled)

	boolean("drift.enabled", &c.Drift.Enabled)
	dur("drift.distract_grace", &c.Drift.DistractGrace)
	dur("drift.fs_stale", &c.Drift.FsStale)
	dur("drift.checkpoint_timeout", &c.Drift.CheckpointTimeout)
	dur("drift.checkpoint_penalty", &c.Drift.CheckpointPenalty)
	dur("drift.recover_grace", &c.Drift.RecoverGrace)
	appendCSV("drift.focus_apps", &c.Drift.FocusApps)
	appendCSV("drift.distract_apps", &c.Drift.DistractApps)
	appendCSV("drift.neutral_apps", &c.Drift.NeutralApps)
	appendCSV("drift.focus_title_hints", &c.Drift.FocusTitleHints)

	boolean("nudge.enabled", &c.Nudge.Enabled)
	integer("nudge.max_per_session", &c.Nudge.MaxPerSession)
	dur("nudge.min_gap", &c.Nudge.MinGap)

	if v, ok := m["ai.provider"]; ok && (v == "anthropic" || v == "openrouter") {
		c.AI.Provider = v
	}
	if v, ok := m["ai.key"]; ok {
		c.AI.Key = v
	}
	if v, ok := m["ai.model"]; ok {
		c.AI.Model = v
	}

	boolean("digest.notify", &c.Digest.Notify)

	return c
}

// MaskKey renders a secret for display: "(unset)" when empty, else first 3 +
// "…" + last 4 characters.
func MaskKey(s string) string {
	if s == "" {
		return "(unset)"
	}
	if len(s) < 8 {
		return "…" + s
	}
	return s[:3] + "…" + s[len(s)-4:]
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/pomoconfig/ -v`
Expected: PASS (all four).

- [ ] **Step 5: Full suite + build**

Run: `make test && go build ./...`
Expected: PASS. Existing `pomoconfig.Load` callers in `cmd/` and `internal/tui/` still compile (only additions were made).

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(pomoconfig): add daemon/drift/nudge/ai/digest config groups"
```

---

### Task 2: `pomo config` display + `pomo config set` validation

**Files:**
- Modify: `cmd/config.go`
- Test: `cmd/config_test.go` (create)

**Interfaces:**
- Consumes: `pomoconfig.Load`, `pomoconfig.MaskKey` (Task 1); package-level `database`.
- Produces:
  - `pomo config` prints a `FOCUS / DRIFT` and `AI` section after the existing `POMODORO` block, with `ai.key` masked and a plaintext-storage warning line when `ai.provider` is set.
  - `pomo config set` validates the key against a known set and (for `ai.provider`) the value; unknown keys and bad enum values return an error instead of silently writing.
  - `cmd.validateConfigSet(key, value string) error` — exported-to-package helper, unit tested.

- [ ] **Step 1: Write the failing test**

Create `cmd/config_test.go`:
```go
package cmd

import "testing"

func TestValidateConfigSet(t *testing.T) {
	ok := [][2]string{
		{"focus", "25m"},
		{"daemon.tick", "15s"},
		{"drift.enabled", "false"},
		{"nudge.max_per_session", "3"},
		{"ai.provider", "anthropic"},
		{"ai.provider", "openrouter"},
		{"ai.key", "sk-ant-x"},
		{"drift.distract_apps", "figma,notion"},
	}
	for _, c := range ok {
		if err := validateConfigSet(c[0], c[1]); err != nil {
			t.Errorf("validateConfigSet(%q,%q) = %v, want nil", c[0], c[1], err)
		}
	}

	bad := [][2]string{
		{"not.a.key", "x"},
		{"ai.provider", "openai"},
		{"daemon.tick", "fifteen"},
		{"nudge.max_per_session", "lots"},
		{"drift.enabled", "yes"},
	}
	for _, c := range bad {
		if err := validateConfigSet(c[0], c[1]); err == nil {
			t.Errorf("validateConfigSet(%q,%q) = nil, want error", c[0], c[1])
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./cmd/ -run TestValidateConfigSet -v`
Expected: FAIL — `validateConfigSet` undefined.

- [ ] **Step 3: Implement**

Replace `cmd/config.go` with:
```go
package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"pomo/internal/pomoconfig"
)

// configKeys maps every settable key to a validator for its value.
var configKeys = map[string]func(string) error{
	"focus":                vDuration,
	"short_break":          vDuration,
	"long_break":           vDuration,
	"sessions_before_long": vInt,
	"auto_start_break":     vBool,
	"auto_start_focus":     vBool,
	"sound":                vBool,
	"sound_choice":         vAny,
	"notifications":        vBool,

	"daemon.tick":               vDuration,
	"checkpoint.enabled":        vBool,
	"drift.enabled":             vBool,
	"drift.distract_grace":      vDuration,
	"drift.fs_stale":            vDuration,
	"drift.checkpoint_timeout":  vDuration,
	"drift.checkpoint_penalty":  vDuration,
	"drift.recover_grace":       vDuration,
	"drift.focus_apps":          vAny,
	"drift.distract_apps":       vAny,
	"drift.neutral_apps":        vAny,
	"drift.focus_title_hints":   vAny,
	"nudge.enabled":             vBool,
	"nudge.max_per_session":     vInt,
	"nudge.min_gap":             vDuration,
	"ai.provider":               vProvider,
	"ai.key":                    vAny,
	"ai.model":                  vAny,
	"digest.notify":             vBool,
}

func vAny(string) error { return nil }
func vBool(s string) error {
	if s != "true" && s != "false" {
		return fmt.Errorf("want 'true' or 'false', got %q", s)
	}
	return nil
}
func vInt(s string) error {
	if _, err := strconv.Atoi(s); err != nil {
		return fmt.Errorf("want an integer, got %q", s)
	}
	return nil
}
func vDuration(s string) error {
	if _, err := time.ParseDuration(s); err != nil {
		return fmt.Errorf("want a duration like '15s' or '3m', got %q", s)
	}
	return nil
}
func vProvider(s string) error {
	if s != "" && s != "anthropic" && s != "openrouter" {
		return fmt.Errorf("want '' , 'anthropic' or 'openrouter', got %q", s)
	}
	return nil
}

func validateConfigSet(key, value string) error {
	v, ok := configKeys[key]
	if !ok {
		return fmt.Errorf("unknown config key %q", key)
	}
	return v(value)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "View Pomodoro configuration",
	RunE: func(c *cobra.Command, args []string) error {
		cfg := pomoconfig.Load(database)
		fmt.Println("POMODORO")
		fmt.Println()
		fmt.Printf("Focus                 %s\n", cfg.Focus)
		fmt.Printf("Short Break           %s\n", cfg.ShortBreak)
		fmt.Printf("Long Break            %s\n", cfg.LongBreak)
		fmt.Printf("Sessions Before Long  %d\n\n", cfg.SessionsBeforeLong)
		fmt.Printf("Auto Start Break      %t\n", cfg.AutoStartBreak)
		fmt.Printf("Auto Start Focus      %t\n\n", cfg.AutoStartFocus)
		fmt.Printf("Sound                 %t\n", cfg.Sound)
		fmt.Printf("Sound Choice          %s\n", cfg.SoundChoice)
		fmt.Printf("Notifications         %t\n\n", cfg.Notifications)

		fmt.Println("FOCUS / DRIFT")
		fmt.Println()
		fmt.Printf("Drift Detection       %t\n", cfg.Drift.Enabled)
		fmt.Printf("Checkpoint Prompt     %t\n", cfg.CheckpointEnabled)
		fmt.Printf("Nudges                %t (max %d/session, min gap %s)\n",
			cfg.Nudge.Enabled, cfg.Nudge.MaxPerSession, cfg.Nudge.MinGap)
		fmt.Printf("Daemon Tick           %s\n\n", cfg.Daemon.Tick)

		fmt.Println("AI")
		fmt.Println()
		provider := cfg.AI.Provider
		if provider == "" {
			provider = "(off)"
		}
		fmt.Printf("Provider              %s\n", provider)
		fmt.Printf("Key                   %s\n", pomoconfig.MaskKey(cfg.AI.Key))
		model := cfg.AI.Model
		if model == "" {
			model = "(provider default)"
		}
		fmt.Printf("Model                 %s\n", model)
		if cfg.AI.Provider != "" {
			fmt.Println()
			fmt.Println("note: ai.key is stored in plaintext at ~/.pomo/pomo.db")
		}
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set a config value (see `pomo config` for the full list)",
	Args:  cobra.ExactArgs(2),
	RunE: func(c *cobra.Command, args []string) error {
		if err := validateConfigSet(args[0], args[1]); err != nil {
			return err
		}
		if err := database.SetConfig(args[0], args[1]); err != nil {
			return err
		}
		shown := args[1]
		if args[0] == "ai.key" {
			shown = pomoconfig.MaskKey(args[1])
		}
		fmt.Printf("Set %s = %s\n", args[0], shown)
		return nil
	},
}

func init() {
	configCmd.AddCommand(configSetCmd)
	rootCmd.AddCommand(configCmd)
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./cmd/ -run TestValidateConfigSet -v`
Expected: PASS.

- [ ] **Step 5: Full suite + smoke**

Run:
```bash
make test && go build -o pomo . && HOME=$(mktemp -d) sh -c './pomo config set ai.provider anthropic; ./pomo config set ai.key sk-ant-secret123; ./pomo config; ./pomo config set bogus.key x; echo "exit=$?"'
```
Expected: suite PASS; `pomo config` shows `Provider anthropic`, `Key sk-…c123`, the plaintext note; `set bogus.key x` prints an error and non-zero exit.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(cmd): validate config keys, show drift/ai sections with masked key"
```

---

### Task 3: `internal/ipc` — event socket

**Files:**
- Create: `internal/ipc/ipc.go`
- Test: `internal/ipc/ipc_test.go` (create)

**Interfaces:**
- Consumes: nothing outside stdlib.
- Produces:
```go
package ipc

type Event struct {
	Type      string   `json:"type"`
	SessionID int64    `json:"id,omitempty"`
	RepoPath  string   `json:"repo_path,omitempty"`
	Text      string   `json:"text,omitempty"`
	Level     int      `json:"level,omitempty"`
	Actions   []string `json:"actions,omitempty"`
	Answer    string   `json:"answer,omitempty"`
	Action    string   `json:"action,omitempty"`
}

// Server accepts multiple clients and fans out Broadcast to all of them.
type Server struct { /* unexported */ }

// Serve removes any stale socket at path, listens, and returns a running
// Server. onRecv is called for every Event a client sends (may be nil).
func Serve(path string, onRecv func(Event)) (*Server, error)
func (s *Server) Broadcast(e Event)
func (s *Server) Close() error

// Client is one connection to a Server.
type Client struct { /* unexported */ }

// Dial connects to the socket at path. onRecv is called for every Event the
// server broadcasts (may be nil).
func Dial(path string, onRecv func(Event)) (*Client, error)
func (c *Client) Send(e Event) error
func (c *Client) Close() error

// SocketPath returns the standard socket location.
func SocketPath() string   // filepath.Join(db.Dir(), "daemon.sock")
```
  Wire format: one JSON object per line (`json.Encoder` writes a trailing `\n`; read with `bufio.Scanner`). A malformed line is skipped, not fatal. Client disconnect is non-fatal for the server and vice-versa.

- [ ] **Step 1: Write the failing tests**

Create `internal/ipc/ipc_test.go`:
```go
package ipc_test

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"pomo/internal/ipc"
)

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition not met within 2s")
}

func TestBroadcastReachesAllClients(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.sock")
	srv, err := ipc.Serve(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	var mu sync.Mutex
	got := map[string]int{}
	recv := func(name string) func(ipc.Event) {
		return func(e ipc.Event) {
			mu.Lock()
			got[name+":"+e.Type]++
			mu.Unlock()
		}
	}
	c1, err := ipc.Dial(path, recv("c1"))
	if err != nil {
		t.Fatal(err)
	}
	defer c1.Close()
	c2, err := ipc.Dial(path, recv("c2"))
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()

	// give the server time to register both connections
	time.Sleep(100 * time.Millisecond)
	srv.Broadcast(ipc.Event{Type: "nudge", Text: "hi"})

	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return got["c1:nudge"] == 1 && got["c2:nudge"] == 1
	})
}

func TestClientSendReachesServerHandler(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.sock")
	var mu sync.Mutex
	var last ipc.Event
	srv, err := ipc.Serve(path, func(e ipc.Event) {
		mu.Lock()
		last = e
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	c, err := ipc.Dial(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.Send(ipc.Event{Type: "checkpoint-answer", Answer: "y"}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return last.Type == "checkpoint-answer" && last.Answer == "y"
	})
}

func TestServeCleansStaleSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.sock")
	srv1, err := ipc.Serve(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	srv1.Close() // leaves the socket file behind on some platforms

	srv2, err := ipc.Serve(path, nil) // must not fail with "address already in use"
	if err != nil {
		t.Fatalf("Serve over stale socket: %v", err)
	}
	srv2.Close()
}

func TestOneClientDisconnectDoesNotBreakOthers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "d.sock")
	srv, err := ipc.Serve(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	c1, _ := ipc.Dial(path, nil)
	var mu sync.Mutex
	n := 0
	c2, _ := ipc.Dial(path, func(ipc.Event) { mu.Lock(); n++; mu.Unlock() })
	defer c2.Close()

	time.Sleep(100 * time.Millisecond)
	c1.Close()
	time.Sleep(50 * time.Millisecond)
	srv.Broadcast(ipc.Event{Type: "watching"})

	waitFor(t, func() bool { mu.Lock(); defer mu.Unlock(); return n == 1 })
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/ipc/ -v`
Expected: FAIL — package `ipc` does not exist.

- [ ] **Step 3: Implement**

Create `internal/ipc/ipc.go`:
```go
// Package ipc is the newline-delimited-JSON Unix-domain-socket channel
// between the pomo daemon (server) and the pomo TUI (client).
package ipc

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sync"

	"pomo/internal/db"
)

type Event struct {
	Type      string   `json:"type"`
	SessionID int64    `json:"id,omitempty"`
	RepoPath  string   `json:"repo_path,omitempty"`
	Text      string   `json:"text,omitempty"`
	Level     int      `json:"level,omitempty"`
	Actions   []string `json:"actions,omitempty"`
	Answer    string   `json:"answer,omitempty"`
	Action    string   `json:"action,omitempty"`
}

// SocketPath is the standard daemon socket location.
func SocketPath() string { return filepath.Join(db.Dir(), "daemon.sock") }

type Server struct {
	ln      net.Listener
	mu      sync.Mutex
	conns   map[net.Conn]struct{}
	onRecv  func(Event)
	closing bool
}

// Serve removes any stale socket at path, then listens on it.
func Serve(path string, onRecv func(Event)) (*Server, error) {
	// Clean a stale socket: if nothing is listening, remove the file.
	if _, err := os.Stat(path); err == nil {
		if c, derr := net.Dial("unix", path); derr == nil {
			c.Close()
			return nil, errors.New("ipc: another server is already listening on " + path)
		}
		_ = os.Remove(path)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	s := &Server{ln: ln, conns: map[net.Conn]struct{}{}, onRecv: onRecv}
	go s.acceptLoop()
	return s, nil
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return // listener closed
		}
		s.mu.Lock()
		s.conns[conn] = struct{}{}
		s.mu.Unlock()
		go s.readLoop(conn)
	}
}

func (s *Server) readLoop(conn net.Conn) {
	defer s.drop(conn)
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 0, 4096), 1<<20)
	for sc.Scan() {
		var e Event
		if json.Unmarshal(sc.Bytes(), &e) != nil {
			continue
		}
		if s.onRecv != nil {
			s.onRecv(e)
		}
	}
}

func (s *Server) drop(conn net.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
	conn.Close()
}

// Broadcast sends e to every connected client. Write failures drop that client.
func (s *Server) Broadcast(e Event) {
	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	line = append(line, '\n')
	s.mu.Lock()
	targets := make([]net.Conn, 0, len(s.conns))
	for c := range s.conns {
		targets = append(targets, c)
	}
	s.mu.Unlock()
	for _, c := range targets {
		if _, err := c.Write(line); err != nil {
			s.drop(c)
		}
	}
}

func (s *Server) Close() error {
	s.mu.Lock()
	s.closing = true
	for c := range s.conns {
		c.Close()
	}
	s.mu.Unlock()
	err := s.ln.Close()
	_ = os.Remove(s.ln.Addr().String())
	return err
}

type Client struct {
	conn net.Conn
	enc  *json.Encoder
	mu   sync.Mutex
}

// Dial connects to the socket at path.
func Dial(path string, onRecv func(Event)) (*Client, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return nil, err
	}
	c := &Client{conn: conn, enc: json.NewEncoder(conn)}
	go func() {
		sc := bufio.NewScanner(conn)
		sc.Buffer(make([]byte, 0, 4096), 1<<20)
		for sc.Scan() {
			var e Event
			if json.Unmarshal(sc.Bytes(), &e) != nil {
				continue
			}
			if onRecv != nil {
				onRecv(e)
			}
		}
	}()
	return c, nil
}

func (c *Client) Send(e Event) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enc.Encode(e) // Encoder appends '\n'
}

func (c *Client) Close() error { return c.conn.Close() }
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/ipc/ -v -race`
Expected: PASS (all four), no race warnings.

- [ ] **Step 5: Full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(ipc): newline-JSON unix-socket server/client with fan-out"
```

---

### Task 4: `internal/ai` — types, `New`, no-op provider

**Files:**
- Create: `internal/ai/ai.go`
- Test: `internal/ai/ai_test.go` (create)

**Interfaces:**
- Consumes: nothing outside stdlib.
- Produces:
```go
package ai

import "context"

var ErrNoProvider = errors.New("ai: no provider configured")

type Config struct {
	Provider string // "" | "anthropic" | "openrouter"
	Key      string
	Model    string // "" => provider default
	BaseURL  string // "" => real endpoint; set by tests
}

// NudgeContext is the data a nudge line is generated from.
type NudgeContext struct {
	Task           string
	Tag            string
	DistractApp    string
	DistractDetail string
	DriftMinutes   int
	SessionMinutes int
	Level          int
	Hour           int
}

// RecapContext is the aggregated data a review recap is generated from.
type RecapContext struct {
	Label          string
	FocusMinutes   int
	PlannedMinutes int
	Completed      int
	Planned        int
	DriftMinutes   int
	TopDrift       []string // "Google Chrome 28m"
	ByTag          []string // "backend 38m"
	BestHour       string   // "10:00" or ""
	Notes          []string // usually empty
}

type Msg struct {
	Role    string // "user" | "assistant"
	Content string
}

type Nudger interface{ Line(ctx context.Context, nc NudgeContext) (string, error) }
type Recapper interface{ Recap(ctx context.Context, rc RecapContext) (string, error) }
type Chatter interface {
	// Stream calls onDelta for each token chunk. system is the system prompt.
	Stream(ctx context.Context, system string, msgs []Msg, onDelta func(string)) error
}

// New returns the three interfaces for cfg. Provider "" yields no-op
// implementations whose methods return ErrNoProvider (Stream immediately,
// Line/Recap with an empty string).
func New(cfg Config) (Nudger, Recapper, Chatter)

// DefaultModel returns the model id for a provider when cfg.Model is "".
func DefaultModel(provider string) string
```

- [ ] **Step 1: Write the failing test**

Create `internal/ai/ai_test.go`:
```go
package ai_test

import (
	"context"
	"errors"
	"testing"

	"pomo/internal/ai"
)

func TestNoProviderIsNoOp(t *testing.T) {
	n, r, c := ai.New(ai.Config{})

	if _, err := n.Line(context.Background(), ai.NudgeContext{Task: "x"}); !errors.Is(err, ai.ErrNoProvider) {
		t.Errorf("Line err = %v, want ErrNoProvider", err)
	}
	if _, err := r.Recap(context.Background(), ai.RecapContext{Label: "today"}); !errors.Is(err, ai.ErrNoProvider) {
		t.Errorf("Recap err = %v, want ErrNoProvider", err)
	}
	called := false
	err := c.Stream(context.Background(), "sys", []ai.Msg{{Role: "user", Content: "hi"}}, func(string) { called = true })
	if !errors.Is(err, ai.ErrNoProvider) {
		t.Errorf("Stream err = %v, want ErrNoProvider", err)
	}
	if called {
		t.Error("Stream should not call onDelta with no provider")
	}
}

func TestDefaultModel(t *testing.T) {
	if ai.DefaultModel("anthropic") != "claude-haiku-4-5" {
		t.Errorf("anthropic default = %q", ai.DefaultModel("anthropic"))
	}
	if ai.DefaultModel("openrouter") != "anthropic/claude-3.5-haiku" {
		t.Errorf("openrouter default = %q", ai.DefaultModel("openrouter"))
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/ai/ -v`
Expected: FAIL — package `ai` does not exist.

- [ ] **Step 3: Implement**

Create `internal/ai/ai.go`:
```go
// Package ai is the BYOK provider layer for pomo's optional AI features:
// nudge lines, review recaps, and the /chat stream. Providers are "anthropic"
// and "openrouter"; an empty provider yields no-op implementations.
package ai

import (
	"context"
	"errors"
	"net/http"
	"time"
)

var ErrNoProvider = errors.New("ai: no provider configured")

type Config struct {
	Provider string
	Key      string
	Model    string
	BaseURL  string
}

type NudgeContext struct {
	Task           string
	Tag            string
	DistractApp    string
	DistractDetail string
	DriftMinutes   int
	SessionMinutes int
	Level          int
	Hour           int
}

type RecapContext struct {
	Label          string
	FocusMinutes   int
	PlannedMinutes int
	Completed      int
	Planned        int
	DriftMinutes   int
	TopDrift       []string
	ByTag          []string
	BestHour       string
	Notes          []string
}

type Msg struct {
	Role    string
	Content string
}

type Nudger interface {
	Line(ctx context.Context, nc NudgeContext) (string, error)
}
type Recapper interface {
	Recap(ctx context.Context, rc RecapContext) (string, error)
}
type Chatter interface {
	Stream(ctx context.Context, system string, msgs []Msg, onDelta func(string)) error
}

func DefaultModel(provider string) string {
	switch provider {
	case "anthropic":
		return "claude-haiku-4-5"
	case "openrouter":
		return "anthropic/claude-3.5-haiku"
	default:
		return ""
	}
}

// New returns the three interfaces for cfg.
func New(cfg Config) (Nudger, Recapper, Chatter) {
	if cfg.Provider != "anthropic" && cfg.Provider != "openrouter" {
		n := noop{}
		return n, n, n
	}
	if cfg.Model == "" {
		cfg.Model = DefaultModel(cfg.Provider)
	}
	c := &httpProvider{cfg: cfg, hc: &http.Client{}}
	return c, c, c
}

type noop struct{}

func (noop) Line(context.Context, NudgeContext) (string, error)  { return "", ErrNoProvider }
func (noop) Recap(context.Context, RecapContext) (string, error) { return "", ErrNoProvider }
func (noop) Stream(context.Context, string, []Msg, func(string)) error {
	return ErrNoProvider
}

// httpProvider implements all three interfaces against a real HTTP endpoint.
// Transport details are in anthropic.go / openrouter.go.
type httpProvider struct {
	cfg Config
	hc  *http.Client
}

// systemNudge / systemRecap / systemChat are the fixed system prompts.
const (
	systemNudge = "You are a terse, non-judgmental focus buddy for a developer. " +
		"Reply with ONE sentence, 15 words max, no emoji, no exclamation marks."
	systemRecap = "You summarise a developer's focus session data. 3 short sentences, " +
		"then one line starting with '→ Try:' with a concrete suggestion. No praise, no fluff."
	systemChat = "You are a terse focus coach for a developer. Ground every answer in the " +
		"supplied session and drift data. 4 sentences max. Be concrete."
)

// perCall timeouts.
var (
	nudgeTimeout = 3 * time.Second
	recapTimeout = 8 * time.Second
	chatTimeout  = 30 * time.Second
)
```
(This task leaves `Line`/`Recap`/`Stream` on `httpProvider` undefined — Tasks 5–7 add them in `anthropic.go` / `openrouter.go`. The package will not compile until Task 5; that is expected and the test for THIS task only exercises `noop`. To keep the build green between tasks, add temporary stubs now and delete them in Task 5:)
```go
// --- temporary stubs, removed in Task 5 ---
func (p *httpProvider) Line(context.Context, NudgeContext) (string, error)  { return "", ErrNoProvider }
func (p *httpProvider) Recap(context.Context, RecapContext) (string, error) { return "", ErrNoProvider }
func (p *httpProvider) Stream(context.Context, string, []Msg, func(string)) error {
	return ErrNoProvider
}
```

- [ ] **Step 4: Run the tests**

Run: `go test ./internal/ai/ -v`
Expected: PASS (both).

- [ ] **Step 5: Full suite + build**

Run: `make test && go build ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add -A
git commit -m "feat(ai): package skeleton, interfaces, no-op provider"
```

---

### Task 5: `internal/ai` — Anthropic transport (Line + Recap)

**Files:**
- Create: `internal/ai/anthropic.go`
- Modify: `internal/ai/ai.go` (delete the temporary stubs from Task 4)
- Create: `internal/ai/prompt.go` (context → user-message text)
- Test: `internal/ai/anthropic_test.go` (create)

**Interfaces:**
- Consumes: `httpProvider` (Task 4), `NudgeContext` / `RecapContext`.
- Produces:
  - `httpProvider.Line` and `httpProvider.Recap` fully implemented, dispatching on `p.cfg.Provider` (this task: the `"anthropic"` branch; `"openrouter"` branch added in Task 6 — until then it returns `fmt.Errorf("openrouter not implemented")`, replaced in Task 6).
  - `renderNudgeUser(nc NudgeContext) string` and `renderRecapUser(rc RecapContext) string` in `prompt.go` — compact `key: value` lines.
  - `anthropicComplete(ctx, hc, cfg, system, user string, maxTokens int) (string, error)` — one non-streaming `POST /v1/messages`, returns `.content[0].text`.
  - Endpoint base: `cfg.BaseURL` if set, else `https://api.anthropic.com`.

- [ ] **Step 1: Write the failing test**

Create `internal/ai/anthropic_test.go`:
```go
package ai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pomo/internal/ai"
)

func TestAnthropicLine(t *testing.T) {
	var gotAuth, gotVersion, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"content":[{"type":"text","text":"take a lap and come back"}]}`)
	}))
	defer srv.Close()

	n, _, _ := ai.New(ai.Config{Provider: "anthropic", Key: "sk-ant-test", BaseURL: srv.URL})
	line, err := n.Line(context.Background(), ai.NudgeContext{
		Task: "fix bug", DistractApp: "Google Chrome", DriftMinutes: 20, Level: 2, Hour: 15,
	})
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	if line != "take a lap and come back" {
		t.Fatalf("line = %q", line)
	}
	if gotAuth != "sk-ant-test" || gotVersion != "2023-06-01" {
		t.Fatalf("headers: auth=%q version=%q", gotAuth, gotVersion)
	}
	var body struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
		System    string `json:"system"`
	}
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatalf("body not json: %v", err)
	}
	if body.Model != "claude-haiku-4-5" || body.MaxTokens != 120 {
		t.Fatalf("body model/max = %q/%d", body.Model, body.MaxTokens)
	}
	if !strings.Contains(body.System, "focus buddy") {
		t.Fatalf("system prompt missing: %q", body.System)
	}
	if !strings.Contains(gotBody, "fix bug") || !strings.Contains(gotBody, "Google Chrome") {
		t.Fatalf("user payload missing context: %s", gotBody)
	}
}

func TestAnthropicRecap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"content":[{"type":"text","text":"you drifted a lot.\n→ Try: mornings"}]}`)
	}))
	defer srv.Close()

	_, r, _ := ai.New(ai.Config{Provider: "anthropic", Key: "k", BaseURL: srv.URL})
	out, err := r.Recap(context.Background(), ai.RecapContext{Label: "today", FocusMinutes: 62, DriftMinutes: 41})
	if err != nil {
		t.Fatalf("Recap: %v", err)
	}
	if !strings.Contains(out, "→ Try:") {
		t.Fatalf("recap = %q", out)
	}
}

func TestAnthropicHTTPErrorSurfaces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(429)
		io.WriteString(w, `{"error":{"message":"rate limited"}}`)
	}))
	defer srv.Close()

	n, _, _ := ai.New(ai.Config{Provider: "anthropic", Key: "k", BaseURL: srv.URL})
	if _, err := n.Line(context.Background(), ai.NudgeContext{Task: "x"}); err == nil {
		t.Fatal("expected error on 429")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/ai/ -run TestAnthropic -v`
Expected: FAIL — `Line` returns `ErrNoProvider` (the Task 4 stub).

- [ ] **Step 3: Delete the Task 4 stubs**

In `internal/ai/ai.go`, remove the block marked `// --- temporary stubs, removed in Task 5 ---` (the three `httpProvider` methods).

- [ ] **Step 4: Implement the prompt renderers**

Create `internal/ai/prompt.go`:
```go
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
```

- [ ] **Step 5: Implement the Anthropic transport + dispatch**

Create `internal/ai/anthropic.go`:
```go
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (p *httpProvider) base() string {
	if p.cfg.BaseURL != "" {
		return p.cfg.BaseURL
	}
	switch p.cfg.Provider {
	case "openrouter":
		return "https://openrouter.ai"
	default:
		return "https://api.anthropic.com"
	}
}

func (p *httpProvider) Line(ctx context.Context, nc NudgeContext) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, nudgeTimeout)
	defer cancel()
	return p.complete(ctx, systemNudge, renderNudgeUser(nc), 120)
}

func (p *httpProvider) Recap(ctx context.Context, rc RecapContext) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, recapTimeout)
	defer cancel()
	return p.complete(ctx, systemRecap, renderRecapUser(rc), 250)
}

// complete runs one non-streaming completion via the configured provider.
func (p *httpProvider) complete(ctx context.Context, system, user string, maxTokens int) (string, error) {
	switch p.cfg.Provider {
	case "anthropic":
		return p.anthropicComplete(ctx, system, user, maxTokens)
	case "openrouter":
		return p.openrouterComplete(ctx, system, user, maxTokens)
	default:
		return "", ErrNoProvider
	}
}

func (p *httpProvider) anthropicComplete(ctx context.Context, system, user string, maxTokens int) (string, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"model":      p.cfg.Model,
		"max_tokens": maxTokens,
		"system":     system,
		"messages":   []map[string]string{{"role": "user", "content": user}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base()+"/v1/messages", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", p.cfg.Key)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("anthropic: %s: %s", resp.Status, truncate(string(body), 200))
	}
	var out struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("anthropic: bad response: %w", err)
	}
	for _, c := range out.Content {
		if c.Type == "text" && c.Text != "" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic: empty response")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
```

- [ ] **Step 6: Add the openrouter stub so the package compiles**

Also in `internal/ai/anthropic.go` (removed in Task 6):
```go
// --- temporary stub, removed in Task 6 ---
func (p *httpProvider) openrouterComplete(ctx context.Context, system, user string, maxTokens int) (string, error) {
	return "", fmt.Errorf("openrouter: not implemented")
}
func (p *httpProvider) Stream(ctx context.Context, system string, msgs []Msg, onDelta func(string)) error {
	return fmt.Errorf("stream: not implemented")
}
```

- [ ] **Step 7: Run the tests**

Run: `go test ./internal/ai/ -run 'TestAnthropic|TestNoProvider|TestDefaultModel' -v`
Expected: PASS.

- [ ] **Step 8: Full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "feat(ai): anthropic non-streaming transport for Line and Recap"
```

---

### Task 6: `internal/ai` — OpenRouter transport

**Files:**
- Create: `internal/ai/openrouter.go`
- Modify: `internal/ai/anthropic.go` (delete the `openrouterComplete` temp stub; keep the `Stream` stub until Task 7)
- Test: `internal/ai/openrouter_test.go` (create)

**Interfaces:**
- Consumes: `httpProvider`.
- Produces: `httpProvider.openrouterComplete(ctx, system, user string, maxTokens int) (string, error)` — one `POST /api/v1/chat/completions`, OpenAI shape, returns `.choices[0].message.content`. Headers: `Authorization: Bearer <key>`, `HTTP-Referer: https://github.com/pomo`, `X-Title: pomo`.

- [ ] **Step 1: Write the failing test**

Create `internal/ai/openrouter_test.go`:
```go
package ai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pomo/internal/ai"
)

func TestOpenRouterLine(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"close the tab"}}]}`)
	}))
	defer srv.Close()

	n, _, _ := ai.New(ai.Config{Provider: "openrouter", Key: "or-key", BaseURL: srv.URL})
	line, err := n.Line(context.Background(), ai.NudgeContext{Task: "ship it", DriftMinutes: 12})
	if err != nil {
		t.Fatalf("Line: %v", err)
	}
	if line != "close the tab" {
		t.Fatalf("line = %q", line)
	}
	if gotAuth != "Bearer or-key" {
		t.Fatalf("auth = %q", gotAuth)
	}
	var body struct {
		Model     string           `json:"model"`
		MaxTokens int              `json:"max_tokens"`
		Messages  []map[string]any `json:"messages"`
	}
	if err := json.Unmarshal([]byte(gotBody), &body); err != nil {
		t.Fatal(err)
	}
	if body.Model != "anthropic/claude-3.5-haiku" || body.MaxTokens != 120 {
		t.Fatalf("model/max = %q/%d", body.Model, body.MaxTokens)
	}
	if len(body.Messages) != 2 || body.Messages[0]["role"] != "system" {
		t.Fatalf("messages shape wrong: %v", body.Messages)
	}
	if !strings.Contains(gotBody, "ship it") {
		t.Fatalf("user content missing: %s", gotBody)
	}
}

func TestOpenRouterHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	_, r, _ := ai.New(ai.Config{Provider: "openrouter", Key: "k", BaseURL: srv.URL})
	if _, err := r.Recap(context.Background(), ai.RecapContext{Label: "today"}); err == nil {
		t.Fatal("expected error on 500")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/ai/ -run TestOpenRouter -v`
Expected: FAIL — stub returns "not implemented".

- [ ] **Step 3: Delete the stub**

In `internal/ai/anthropic.go`, remove the `openrouterComplete` function marked `// --- temporary stub, removed in Task 6 ---` (leave the `Stream` stub).

- [ ] **Step 4: Implement**

Create `internal/ai/openrouter.go`:
```go
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (p *httpProvider) openrouterComplete(ctx context.Context, system, user string, maxTokens int) (string, error) {
	reqBody, _ := json.Marshal(map[string]any{
		"model":      p.cfg.Model,
		"max_tokens": maxTokens,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.base()+"/api/v1/chat/completions", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.Key)
	req.Header.Set("HTTP-Referer", "https://github.com/pomo")
	req.Header.Set("X-Title", "pomo")

	resp, err := p.hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("openrouter: %s: %s", resp.Status, truncate(string(body), 200))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", fmt.Errorf("openrouter: bad response: %w", err)
	}
	if len(out.Choices) == 0 || out.Choices[0].Message.Content == "" {
		return "", fmt.Errorf("openrouter: empty response")
	}
	return out.Choices[0].Message.Content, nil
}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/ai/ -run TestOpenRouter -v`
Expected: PASS (both).

- [ ] **Step 6: Full suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(ai): openrouter non-streaming transport"
```

---

### Task 7: `internal/ai` — streaming `Chatter`

**Files:**
- Create: `internal/ai/stream.go`
- Modify: `internal/ai/anthropic.go` (delete the `Stream` temp stub)
- Test: `internal/ai/stream_test.go` (create)

**Interfaces:**
- Consumes: `httpProvider`.
- Produces: `httpProvider.Stream(ctx, system string, msgs []Msg, onDelta func(string)) error` — one streaming `POST` (chat timeout 30s), parses SSE, calls `onDelta` per text chunk.
  - Anthropic: `POST /v1/messages` with `"stream": true`; SSE `data:` lines carry JSON; on `{"type":"content_block_delta","delta":{"type":"text_delta","text":"..."}}` emit `delta.text`; stop on `{"type":"message_stop"}`.
  - OpenRouter: `POST /api/v1/chat/completions` with `"stream": true`; SSE `data:` lines; on `{"choices":[{"delta":{"content":"..."}}]}` emit `delta.content`; stop on `data: [DONE]`.
  - `msgs` is the running conversation; prepend nothing — `system` goes to the top-level `system` field (anthropic) or a leading `{"role":"system"}` message (openrouter).
  - A non-2xx status returns an error before any `onDelta` call. A mid-stream transport error returns that error (partial deltas already delivered stay delivered).

- [ ] **Step 1: Write the failing test**

Create `internal/ai/stream_test.go`:
```go
package ai_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"pomo/internal/ai"
)

func TestStreamAnthropic(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl, _ := w.(http.Flusher)
		lines := []string{
			`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"hel"}}`,
			`data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"lo"}}`,
			`data: {"type":"message_stop"}`,
		}
		for _, l := range lines {
			io.WriteString(w, l+"\n\n")
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer srv.Close()

	_, _, c := ai.New(ai.Config{Provider: "anthropic", Key: "k", BaseURL: srv.URL})
	var got strings.Builder
	err := c.Stream(context.Background(), "sys", []ai.Msg{{Role: "user", Content: "hi"}}, func(d string) {
		got.WriteString(d)
	})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got.String() != "hello" {
		t.Fatalf("streamed %q, want 'hello'", got.String())
	}
}

func TestStreamOpenRouter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fl, _ := w.(http.Flusher)
		for _, l := range []string{
			`data: {"choices":[{"delta":{"content":"wor"}}]}`,
			`data: {"choices":[{"delta":{"content":"ld"}}]}`,
			`data: [DONE]`,
		} {
			io.WriteString(w, l+"\n\n")
			if fl != nil {
				fl.Flush()
			}
		}
	}))
	defer srv.Close()

	_, _, c := ai.New(ai.Config{Provider: "openrouter", Key: "k", BaseURL: srv.URL})
	var got strings.Builder
	if err := c.Stream(context.Background(), "sys", []ai.Msg{{Role: "user", Content: "hi"}}, func(d string) { got.WriteString(d) }); err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if got.String() != "world" {
		t.Fatalf("streamed %q", got.String())
	}
}

func TestStreamHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()
	_, _, c := ai.New(ai.Config{Provider: "anthropic", Key: "k", BaseURL: srv.URL})
	called := false
	err := c.Stream(context.Background(), "s", []ai.Msg{{Role: "user", Content: "x"}}, func(string) { called = true })
	if err == nil {
		t.Fatal("expected error on 401")
	}
	if called {
		t.Fatal("onDelta called despite error")
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/ai/ -run TestStream -v`
Expected: FAIL — stub returns "not implemented".

- [ ] **Step 3: Delete the stub**

In `internal/ai/anthropic.go`, remove the `Stream` method marked as the temporary stub.

- [ ] **Step 4: Implement**

Create `internal/ai/stream.go`:
```go
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

func (p *httpProvider) Stream(ctx context.Context, system string, msgs []Msg, onDelta func(string)) error {
	ctx, cancel := context.WithTimeout(ctx, chatTimeout)
	defer cancel()

	var url string
	var reqBody []byte
	switch p.cfg.Provider {
	case "anthropic":
		url = p.base() + "/v1/messages"
		m := make([]map[string]string, 0, len(msgs))
		for _, x := range msgs {
			m = append(m, map[string]string{"role": x.Role, "content": x.Content})
		}
		reqBody, _ = json.Marshal(map[string]any{
			"model": p.cfg.Model, "max_tokens": 400, "system": system,
			"messages": m, "stream": true,
		})
	case "openrouter":
		url = p.base() + "/api/v1/chat/completions"
		m := []map[string]string{{"role": "system", "content": system}}
		for _, x := range msgs {
			m = append(m, map[string]string{"role": x.Role, "content": x.Content})
		}
		reqBody, _ = json.Marshal(map[string]any{
			"model": p.cfg.Model, "max_tokens": 400, "messages": m, "stream": true,
		})
	default:
		return ErrNoProvider
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}
	req.Header.Set("content-type", "application/json")
	if p.cfg.Provider == "anthropic" {
		req.Header.Set("x-api-key", p.cfg.Key)
		req.Header.Set("anthropic-version", "2023-06-01")
	} else {
		req.Header.Set("Authorization", "Bearer "+p.cfg.Key)
		req.Header.Set("HTTP-Referer", "https://github.com/pomo")
		req.Header.Set("X-Title", "pomo")
	}

	resp, err := p.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s: stream status %s", p.cfg.Provider, resp.Status)
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 0, 8192), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			return nil
		}
		if p.cfg.Provider == "anthropic" {
			var ev struct {
				Type  string `json:"type"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			}
			if json.Unmarshal([]byte(payload), &ev) != nil {
				continue
			}
			if ev.Type == "message_stop" {
				return nil
			}
			if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
				onDelta(ev.Delta.Text)
			}
		} else {
			var ev struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if json.Unmarshal([]byte(payload), &ev) != nil {
				continue
			}
			if len(ev.Choices) > 0 && ev.Choices[0].Delta.Content != "" {
				onDelta(ev.Choices[0].Delta.Content)
			}
		}
	}
	return sc.Err()
}
```

- [ ] **Step 5: Run the tests**

Run: `go test ./internal/ai/ -run TestStream -v -race`
Expected: PASS (all three), no races.

- [ ] **Step 6: Full suite + vet**

Run: `make test && go vet ./...`
Expected: PASS, no vet complaints.

- [ ] **Step 7: Commit**

```bash
git add -A
git commit -m "feat(ai): streaming Chatter for anthropic and openrouter"
```

---

### Task 8: Update CLAUDE.md + spec status

**Files:**
- Modify: `CLAUDE.md`
- Modify: `specs/README.md`

**Interfaces:**
- Consumes: nothing.
- Produces: docs mention the three new packages.

- [ ] **Step 1: Edit CLAUDE.md**

Under **Architecture**, add:
```
- **`internal/ipc`** — newline-delimited-JSON Unix-domain-socket channel
  (`~/.pomo/daemon.sock`) between the planned daemon (server, fan-out
  `Broadcast`) and the TUI (client). Not yet wired to anything.
- **`internal/ai`** — BYOK provider layer (`anthropic` | `openrouter`, raw
  `net/http`) behind `Nudger` / `Recapper` / `Chatter`. Empty provider →
  no-op returning `ErrNoProvider`. Not yet wired to anything.
```

Under **Conventions**, add:
```
- Config keys are grouped (`drift.*`, `nudge.*`, `ai.*`, `daemon.*`,
  `digest.*`); `pomoconfig.Load` parses them in one pass, bad values fall
  back to the default. `pomo config set` validates against `configKeys` in
  `cmd/config.go`. `ai.key` is masked (`pomoconfig.MaskKey`) wherever shown.
```

- [ ] **Step 2: Mark spec sections done in specs/README.md**

Append to `specs/README.md`:
```
## Progress

- Chunk 1 (schema + `internal/report` + `pomo review`): done — `plans/2026-09-04-report-foundation.md`.
- Chunk 2 (config keys + `internal/ipc` + `internal/ai`): done — `plans/2026-09-04-daemon-prep.md`.
- Chunk 3 (daemon + `internal/watch` + drift scoring + `internal/nudge` + `internal/notify`): not started.
- Chunk 4 (TUI slash palette + `/chat` + `pomo digest` + daemon wiring): not started.
```

- [ ] **Step 3: Verify + commit**

Run: `make test`
Expected: PASS.
```bash
git add -A
git commit -m "docs: note internal/ipc and internal/ai in CLAUDE.md"
```

---

## Self-Review

**1. Spec coverage**

`specs/2026-09-04-schema-config-migration.md` §3 (config keys):
- Every key in the §3 table — Task 1 (`Load` parsing) + Task 2 (`configKeys` validation). Cross-checked each row: `daemon.tick`, `drift.enabled`, `drift.distract_grace`, `drift.fs_stale`, `drift.checkpoint_timeout`, `drift.checkpoint_penalty`, `drift.recover_grace`, `drift.focus_apps`, `drift.distract_apps`, `drift.neutral_apps`, `drift.focus_title_hints`, `nudge.enabled`, `nudge.max_per_session`, `nudge.min_gap`, `ai.provider`, `ai.key`, `ai.model`, `digest.notify`, `checkpoint.enabled`. ✓
- CSV append-not-replace semantics — Task 1 `appendCSV` + `TestOverridesAndCSVAppend`. ✓
- `pomo config` masked-key display + plaintext warning — Task 2. ✓
- `/settings` screen rows — **deferred to chunk 4** (TUI work). Noted, not a gap.

`specs/2026-09-04-schema-config-migration.md` §4 (`/settings`): deferred to chunk 4.

`specs/2026-09-04-nudge-and-ai.md` §3 (`internal/ai`):
- `Config`, `Nudger`/`Recapper`/`Chatter`, `New`, `DefaultModel`, no-op path — Task 4. ✓
- Anthropic transport, `x-api-key` + `anthropic-version`, `.content[0].text` — Task 5. ✓
- OpenRouter transport, `Authorization: Bearer`, `.choices[0].message.content` — Task 6. ✓
- Per-call max_tokens (120 / 250 / 400) + timeouts (3s / 8s / 30s) — Tasks 5, 7. ✓
- Streaming SSE both providers, `[DONE]` / `message_stop` termination — Task 7. ✓
- Compact `key: value` user payloads, no raw notes unless passed — Task 5 `prompt.go` (`RecapContext.Notes` only rendered if populated; caller controls). ✓
- `nudge`-package canned fallback + escalation state machine — **chunk 3** (`internal/nudge`). The `ai` no-op returning `ErrNoProvider` is the hook it needs. Noted, not a gap.
- §5 security note (plaintext key, masked display) — Task 2. ✓

`specs/2026-09-04-slash-palette-and-chat.md` §8 (`internal/ipc`):
- `Event` struct with exact json tags — Task 3. ✓
- `Serve` multi-client + `Broadcast` fan-out + non-fatal disconnect — Task 3 + tests. ✓
- Stale-socket cleanup on startup (`net.Dial` probe → `os.Remove`) — Task 3 `Serve` + `TestServeCleansStaleSocket`. ✓
- `SocketPath()` = `filepath.Join(db.Dir(), "daemon.sock")` — Task 3. ✓
- Client round-trip (`checkpoint-answer`) — Task 3 `TestClientSendReachesServerHandler`. ✓
- TUI-side client usage, retry-on-session-start — **chunk 4** (TUI). Noted, not a gap.

**2. Placeholder scan** — the only stubs are explicitly labelled temporary with the task that removes them (Task 4 → removed Task 5; Task 5 openrouter/stream stubs → removed Tasks 6/7). Each is real compilable code returning a clear error, and every removal is a named step. No "TODO"/"handle errors"/"similar to Task N".

**3. Type consistency**
- `pomoconfig.Config` sub-structs (`DaemonConfig`, `DriftConfig`, `NudgeConfig`, `AIConfig`, `DigestConfig`) — defined Task 1, referenced only within Task 1/2. `cfg.Drift.Enabled`, `cfg.Nudge.MaxPerSession`, `cfg.AI.Provider`, `cfg.Daemon.Tick`, `cfg.Digest.Notify`, `cfg.CheckpointEnabled` — consistent between defaults, Load, and the `pomo config` printer. ✓
- `pomoconfig.MaskKey` — Task 1 def, Task 2 use. ✓
- `ipc.Event` field names + json tags — Task 3 def, matches `specs/2026-09-04-slash-palette-and-chat.md` §8 verbatim. ✓
- `ipc.Serve(path string, onRecv func(Event)) (*Server, error)`, `Dial(path string, onRecv func(Event)) (*Client, error)`, `(*Server).Broadcast(Event)`, `(*Client).Send(Event) error` — def Task 3, used in Task 3 tests. Signatures match. ✓
- `ai.Config` fields (`Provider`, `Key`, `Model`, `BaseURL`) — Task 4 def; `BaseURL` used by every transport test (Tasks 5–7) and by `httpProvider.base()` (Task 5). ✓
- `ai.New` returns `(Nudger, Recapper, Chatter)` — Task 4; all tests destructure `n, r, c := ai.New(...)`. ✓
- `Nudger.Line(context.Context, NudgeContext)`, `Recapper.Recap(context.Context, RecapContext)`, `Chatter.Stream(context.Context, string, []Msg, func(string))` — Task 4 def, implemented Tasks 5 (`Line`/`Recap`) and 7 (`Stream`), exercised by tests with matching arg lists. ✓
- `httpProvider.complete` dispatches to `anthropicComplete` / `openrouterComplete` — `complete` + `anthropicComplete` defined Task 5, `openrouterComplete` Task 6 (stub Task 5). Names consistent. ✓
- `truncate` helper — defined once in Task 5 `anthropic.go`, used in Task 6 `openrouter.go`. Same package, fine. ✓
- `systemNudge`/`systemRecap`/`systemChat`, `nudgeTimeout`/`recapTimeout`/`chatTimeout` — defined Task 4 `ai.go`, used Tasks 5/7. ✓

No inconsistencies found.
