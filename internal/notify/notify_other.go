//go:build !darwin

package notify

func command(title, body string) []string {
	if onPath("notify-send") {
		return []string{"notify-send", "-a", "pomo", title, body}
	}
	return nil
}
