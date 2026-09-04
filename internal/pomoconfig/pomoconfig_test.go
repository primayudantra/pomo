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
	_ = d.SetConfig("ai.provider", "openai")

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
		"":                  "(unset)",
		"short":             "…short",
		"sk-ant-abcdef1234": "sk-…1234",
	}
	for in, want := range cases {
		if got := pomoconfig.MaskKey(in); got != want {
			t.Errorf("MaskKey(%q) = %q, want %q", in, got, want)
		}
	}
}
