package tasks

import (
	"time"
)

// Status represents the state of a Task.
type Status string

const (
	StatusTodo       Status = "Todo"
	StatusInProgress Status = "In Progress"
	StatusReview     Status = "Review"
	StatusDone       Status = "Done"
)

// TaskHistoryEntry represents a single event in a task's life.
type TaskHistoryEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	WorkerAgent string    `json:"worker_agent,omitempty"`
	Action      string    `json:"action"`
}

// Task represents a unit of work on the Kanban board.
type Task struct {
	ID            string             `json:"id"`
	Title         string             `json:"title"`
	Description   string             `json:"description"`
	Status        Status             `json:"status"`
	AssignedAgent string             `json:"assigned_agent,omitempty"`
	CreatedAt     time.Time          `json:"created_at"`
	UpdatedAt     time.Time          `json:"updated_at"`
	History       []TaskHistoryEntry `json:"history"`
}
