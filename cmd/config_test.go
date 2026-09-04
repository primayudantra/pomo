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
