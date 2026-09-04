package model

import "time"

type Task struct {
	ID        int64
	Name      string
	Tag       string
	Done      bool
	CreatedAt time.Time
}

type SessionStatus string

const (
	StatusRunning     SessionStatus = "running"
	StatusCompleted   SessionStatus = "completed"
	StatusCancelled   SessionStatus = "cancelled"
	StatusSkipped     SessionStatus = "skipped"
	StatusInterrupted SessionStatus = "interrupted"
)

type Session struct {
	ID              int64
	TaskID          int64
	TaskName        string
	Tag             string
	PlannedDuration int // seconds
	ActualDuration  int // seconds
	Status          SessionStatus
	Note            string
	StartedAt       time.Time
	CompletedAt     *time.Time
	CreatedAt       time.Time
	RepoPath        string
	RepoBranch      string
}
