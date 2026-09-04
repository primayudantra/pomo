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
