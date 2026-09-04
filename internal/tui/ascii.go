package tui

import "strings"

// focusFrame draws a tiny figure hunched over a laptop, with a blinking
// terminal cursor standing in for "typing" motion — tick increments every
// second the timer runs (paused or not), so the cursor keeps blinking even
// while the timer is paused.
func focusFrame(tick int) string {
	cursor := " "
	if tick%2 == 0 {
		cursor = "_"
	}
	art := []string{
		`    .-""-.`,
		`   ( o  o )`,
		`    \ ‿‿ /`,
		`   /|    |\`,
		`  d |    | b`,
		`    '----'`,
		``,
		`   typing` + cursor,
	}
	return strings.Join(art, "\n")
}

// breakFrame draws the same figure leaning back with a coffee cup, its
// steam wisp cycling position each tick to read as rising steam.
func breakFrame(tick int) string {
	steam := []string{
		`    )  `,
		`   (   `,
		`    )  `,
		`     ( `,
	}[tick%4]
	art := []string{
		`   ` + steam,
		`   ( ^‿^)`,
		`    )   )__`,
		`   (       )`,
		`    '-----'`,
		``,
		`   ☕ on break`,
	}
	return strings.Join(art, "\n")
}
