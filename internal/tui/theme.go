package tui

import "github.com/charmbracelet/lipgloss"

// Shared palette across dashboard, timer and the interactive app screens —
// warm terminal tones built around the tomato accent, one semantic teal for
// "done" states, everything else muted so the accent carries the eye.
var (
	accent     = lipgloss.Color("#ff6b52") // tomato — headers, focus bar
	accentSoft = lipgloss.Color("#ffab8f") // gradient far stop
	accent2    = lipgloss.Color("#5bd1b8") // teal — completion / "done" only
	border     = lipgloss.Color("#6b4136") // warm hairline border
	bright     = lipgloss.Color("#ede4dc") // primary text / values
	muted      = lipgloss.Color("#8f8079") // secondary labels, hints
	dim        = lipgloss.Color("#5f5854") // inactive glyphs, empty track
	rowBg      = lipgloss.Color("#2c1f1b") // selected list row fill
	errColor   = lipgloss.Color("#e5605a")

	styleAccent  = lipgloss.NewStyle().Foreground(accent)
	styleAccent2 = lipgloss.NewStyle().Foreground(accent2)
	styleBorder  = lipgloss.NewStyle().Foreground(border)
	styleBright  = lipgloss.NewStyle().Foreground(bright)
	styleMuted   = lipgloss.NewStyle().Foreground(muted)
	styleDim     = lipgloss.NewStyle().Foreground(dim)
	styleErr     = lipgloss.NewStyle().Foreground(errColor)
)

// gradientBar renders a filled/empty progress bar whose filled portion
// fades from accent to accentSoft, matching the mockup's focus-timer bar.
func gradientBar(pct float64, width int) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 1 {
		pct = 1
	}
	filled := int(pct * float64(width))
	if filled > width {
		filled = width
	}
	var b lipgloss.Style
	out := ""
	for i := 0; i < filled; i++ {
		t := 0.0
		if filled > 1 {
			t = float64(i) / float64(filled-1)
		}
		b = lipgloss.NewStyle().Foreground(lerpColor(accent, accentSoft, t))
		out += b.Render("█")
	}
	if width-filled > 0 {
		out += styleDim.Render(repeat("░", width-filled))
	}
	return out
}

func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}

// lerpColor blends two hex lipgloss colors at t in [0,1].
func lerpColor(a, b lipgloss.Color, t float64) lipgloss.Color {
	ar, ag, ab := hexRGB(string(a))
	br, bg, bb := hexRGB(string(b))
	r := ar + (br-ar)*t
	g := ag + (bg-ag)*t
	bl := ab + (bb-ab)*t
	return lipgloss.Color(rgbHex(r, g, bl))
}

func hexRGB(h string) (float64, float64, float64) {
	if len(h) != 7 {
		return 0, 0, 0
	}
	var r, g, b int
	_, _ = sscanfHex(h[1:3], &r)
	_, _ = sscanfHex(h[3:5], &g)
	_, _ = sscanfHex(h[5:7], &b)
	return float64(r), float64(g), float64(b)
}

func sscanfHex(s string, v *int) (int, error) {
	n := 0
	for _, c := range s {
		n *= 16
		switch {
		case c >= '0' && c <= '9':
			n += int(c - '0')
		case c >= 'a' && c <= 'f':
			n += int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			n += int(c-'A') + 10
		}
	}
	*v = n
	return 1, nil
}

func rgbHex(r, g, b float64) string {
	clamp := func(v float64) int {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return int(v)
	}
	const hexDigits = "0123456789abcdef"
	toHex := func(v int) string {
		return string([]byte{hexDigits[v>>4], hexDigits[v&0xf]})
	}
	return "#" + toHex(clamp(r)) + toHex(clamp(g)) + toHex(clamp(b))
}
