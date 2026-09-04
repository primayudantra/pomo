//go:build darwin

package notify

import "fmt"

func command(title, body string) []string {
	if onPath("terminal-notifier") {
		return []string{"terminal-notifier", "-title", title, "-message", body, "-group", "pomo"}
	}
	script := fmt.Sprintf("display notification %q with title %q", body, title)
	return []string{"osascript", "-e", script}
}
