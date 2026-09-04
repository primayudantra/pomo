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

	Daemon            DaemonConfig
	Drift             DriftConfig
	Nudge             NudgeConfig
	AI                AIConfig
	Digest            DigestConfig
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
		Nudge:  NudgeConfig{Enabled: true, MaxPerSession: 4, MinGap: 5 * time.Minute},
		AI:     AIConfig{},
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
