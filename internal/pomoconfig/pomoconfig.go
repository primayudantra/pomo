package pomoconfig

import (
	"strconv"
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
	}
}

func Load(d *db.DB) Config {
	c := defaults()
	m, err := d.AllConfig()
	if err != nil {
		return c
	}
	if v, ok := m["focus"]; ok {
		if dur, err := time.ParseDuration(v); err == nil {
			c.Focus = dur
		}
	}
	if v, ok := m["short_break"]; ok {
		if dur, err := time.ParseDuration(v); err == nil {
			c.ShortBreak = dur
		}
	}
	if v, ok := m["long_break"]; ok {
		if dur, err := time.ParseDuration(v); err == nil {
			c.LongBreak = dur
		}
	}
	if v, ok := m["sessions_before_long"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			c.SessionsBeforeLong = n
		}
	}
	if v, ok := m["auto_start_break"]; ok {
		c.AutoStartBreak = v == "true"
	}
	if v, ok := m["auto_start_focus"]; ok {
		c.AutoStartFocus = v == "true"
	}
	if v, ok := m["sound"]; ok {
		c.Sound = v == "true"
	}
	if v, ok := m["sound_choice"]; ok && v != "" {
		c.SoundChoice = sound.ID(v)
	}
	if v, ok := m["notifications"]; ok {
		c.Notifications = v == "true"
	}
	return c
}
