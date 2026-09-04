package tui

import (
	"testing"

	"github.com/charmbracelet/bubbles/textinput"

	"pomo/internal/db/dbtest"
)

func typeInto(a *App, s string) {
	for _, r := range s {
		a.updateSettings(keyMsg(string(r)))
	}
}

func TestSettingsEditAIKey(t *testing.T) {
	d := dbtest.NewTemp(t)
	a := NewApp(d)
	a.screen = screenSettings
	a.settingsCursor = settingRowAIKey

	a.updateSettings(keyMsg("enter")) // start editing
	if !a.settingsEditing {
		t.Fatal("enter on AI Key row should start editing")
	}
	if a.settingsEdit.EchoMode != textinput.EchoPassword {
		t.Fatal("AI key edit must be masked (password echo)")
	}
	typeInto(a, "sk-ant-secret123")
	a.updateSettings(keyMsg("enter")) // save

	if a.settingsEditing {
		t.Fatal("enter should commit and leave edit mode")
	}
	if a.cfg.AI.Key != "sk-ant-secret123" {
		t.Fatalf("cfg not updated: %q", a.cfg.AI.Key)
	}
	if got := d.GetConfig("ai.key", ""); got != "sk-ant-secret123" {
		t.Fatalf("persisted value = %q", got)
	}
}

func TestSettingsEditAIModelCancel(t *testing.T) {
	d := dbtest.NewTemp(t)
	a := NewApp(d)
	a.screen = screenSettings
	a.settingsCursor = settingRowAIModel

	a.updateSettings(keyMsg("enter"))
	typeInto(a, "claude-opus-5")
	a.updateSettings(keyMsg("esc")) // cancel

	if a.settingsEditing {
		t.Fatal("esc should leave edit mode")
	}
	if a.cfg.AI.Model != "" {
		t.Fatalf("esc must not save: %q", a.cfg.AI.Model)
	}
	if d.GetConfig("ai.model", "") != "" {
		t.Fatal("esc must not persist")
	}
}
