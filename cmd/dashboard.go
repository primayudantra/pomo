package cmd

import "pomo/internal/tui"

func runDashboard() error {
	return tui.RunApp(database)
}
