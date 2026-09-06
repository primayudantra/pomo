package main

import (
	"time"

	"pomo/internal/db"
	"pomo/internal/model"
)

// SessionRow is a single session in the Today view.
type SessionRow struct {
	Task      string `json:"task"`
	Minutes   int    `json:"minutes"` // actual_duration / 60, rounded
	Status    string `json:"status"`
	StartedAt string `json:"startedAt"` // RFC3339
}

// TodayView is a read-only summary of today's sessions.
type TodayView struct {
	Date         string       `json:"date"`
	FocusMinutes int          `json:"focusMinutes"`
	Count        int          `json:"count"`
	Sessions     []SessionRow `json:"sessions"`
}

// SessionService exposes today's sessions to the frontend.
type SessionService struct{ db *db.DB }

// NewSessionService builds a SessionService over d.
func NewSessionService(d *db.DB) *SessionService { return &SessionService{db: d} }

// Today returns today's sessions plus completed focus totals.
func (s *SessionService) Today() (TodayView, error) {
	now := time.Now()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 0, 1)

	sessions, err := s.db.ListSessions(db.SessionFilter{From: &start, To: &end})
	if err != nil {
		return TodayView{}, err
	}

	v := TodayView{Date: now.Format("Mon 2 Jan"), Sessions: make([]SessionRow, 0, len(sessions))}
	for _, sess := range sessions {
		v.Sessions = append(v.Sessions, SessionRow{
			Task:      sess.TaskName,
			Minutes:   int((float64(sess.ActualDuration) / 60.0) + 0.5),
			Status:    string(sess.Status),
			StartedAt: sess.StartedAt.Format(time.RFC3339),
		})
		if sess.Status == model.StatusCompleted {
			v.Count++
			v.FocusMinutes += int((float64(sess.ActualDuration) / 60.0) + 0.5)
		}
	}
	return v, nil
}
