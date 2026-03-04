package task

import (
	"fmt"
	"sync"
	"time"
)

// Status represents a task item's lifecycle state.
type Status string

const (
	StatusPending  Status = "pending"
	StatusRunning  Status = "running"
	StatusDone     Status = "done"
	StatusFailed   Status = "failed"
	StatusSkipped  Status = "skipped"
)

// Item is a single step in a task list.
type Item struct {
	ID          string    `json:"id"`
	Description string    `json:"description"`
	Agent       string    `json:"agent"`          // which agent handles this
	Status      Status    `json:"status"`
	Output      string    `json:"output,omitempty"`
	Error       string    `json:"error,omitempty"`
	StartedAt   time.Time `json:"started_at,omitempty"`
	DoneAt      time.Time `json:"done_at,omitempty"`
	SubTasks    []*Item   `json:"sub_tasks,omitempty"` // nested decomposition
}

// Duration returns how long the task ran.
func (t *Item) Duration() time.Duration {
	if t.StartedAt.IsZero() || t.DoneAt.IsZero() {
		return 0
	}
	return t.DoneAt.Sub(t.StartedAt)
}

// List is a mandatory checklist created for every user request.
// The orchestrator breaks down each request into items.
// Each item maps to a TaskPayload for a focused agent.
type List struct {
	mu          sync.RWMutex
	ID          string    `json:"id"`
	UserRequest string    `json:"user_request"`
	Items       []*Item   `json:"items"`
	CreatedAt   time.Time `json:"created_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	onUpdate    func(*List) // callback for dashboard/event updates
}

// NewList creates a task list for a user request.
func NewList(id, userRequest string) *List {
	return &List{
		ID:          id,
		UserRequest: userRequest,
		Items:       make([]*Item, 0),
		CreatedAt:   time.Now(),
	}
}

// OnUpdate sets a callback fired whenever a task item changes status.
func (l *List) OnUpdate(fn func(*List)) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.onUpdate = fn
}

// AddItem appends a task item to the list.
func (l *List) AddItem(id, description, agent string) *Item {
	l.mu.Lock()
	defer l.mu.Unlock()

	item := &Item{
		ID:          id,
		Description: description,
		Agent:       agent,
		Status:      StatusPending,
	}
	l.Items = append(l.Items, item)
	l.notify()
	return item
}

// AddSubTask adds a sub-task to an existing item.
func (l *List) AddSubTask(parentID, id, description, agent string) (*Item, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	parent := l.findItem(parentID)
	if parent == nil {
		return nil, fmt.Errorf("parent task %q not found", parentID)
	}

	sub := &Item{
		ID:          id,
		Description: description,
		Agent:       agent,
		Status:      StatusPending,
	}
	parent.SubTasks = append(parent.SubTasks, sub)
	l.notify()
	return sub, nil
}

// Start marks a task as running.
func (l *List) Start(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	item := l.findItemDeep(id)
	if item == nil {
		return fmt.Errorf("task %q not found", id)
	}

	item.Status = StatusRunning
	item.StartedAt = time.Now()
	l.notify()
	return nil
}

// Complete marks a task as done with output.
func (l *List) Complete(id, output string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	item := l.findItemDeep(id)
	if item == nil {
		return fmt.Errorf("task %q not found", id)
	}

	item.Status = StatusDone
	item.Output = output
	item.DoneAt = time.Now()
	l.notify()

	// Check if all items are done.
	if l.allDone() {
		l.CompletedAt = time.Now()
	}
	return nil
}

// Fail marks a task as failed.
func (l *List) Fail(id, errMsg string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	item := l.findItemDeep(id)
	if item == nil {
		return fmt.Errorf("task %q not found", id)
	}

	item.Status = StatusFailed
	item.Error = errMsg
	item.DoneAt = time.Now()
	l.notify()
	return nil
}

// Skip marks a task as skipped.
func (l *List) Skip(id string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	item := l.findItemDeep(id)
	if item == nil {
		return fmt.Errorf("task %q not found", id)
	}

	item.Status = StatusSkipped
	item.DoneAt = time.Now()
	l.notify()
	return nil
}

// Progress returns (completed, total) counts.
func (l *List) Progress() (int, int) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	total := 0
	done := 0
	l.walkItems(func(item *Item) {
		total++
		if item.Status == StatusDone || item.Status == StatusSkipped {
			done++
		}
	})
	return done, total
}

// IsComplete returns true if all items are done/skipped/failed.
func (l *List) IsComplete() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.allDone()
}

// PendingItems returns items that haven't started yet.
func (l *List) PendingItems() []*Item {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var pending []*Item
	l.walkItems(func(item *Item) {
		if item.Status == StatusPending {
			pending = append(pending, item)
		}
	})
	return pending
}

// Summary returns a human-readable summary of the task list.
func (l *List) Summary() string {
	l.mu.RLock()
	defer l.mu.RUnlock()

	done, total := 0, 0
	l.walkItems(func(item *Item) {
		total++
		if item.Status == StatusDone || item.Status == StatusSkipped {
			done++
		}
	})

	s := fmt.Sprintf("Task: %s\n", l.UserRequest)
	s += fmt.Sprintf("Progress: %d/%d\n", done, total)
	for i, item := range l.Items {
		icon := statusIcon(item.Status)
		s += fmt.Sprintf("  %d. %s %s [%s]\n", i+1, icon, item.Description, item.Agent)
		for j, sub := range item.SubTasks {
			subIcon := statusIcon(sub.Status)
			s += fmt.Sprintf("     %d.%d %s %s [%s]\n", i+1, j+1, subIcon, sub.Description, sub.Agent)
		}
	}
	return s
}

// --- Internal helpers ---

func (l *List) findItem(id string) *Item {
	for _, item := range l.Items {
		if item.ID == id {
			return item
		}
	}
	return nil
}

func (l *List) findItemDeep(id string) *Item {
	for _, item := range l.Items {
		if item.ID == id {
			return item
		}
		for _, sub := range item.SubTasks {
			if sub.ID == id {
				return sub
			}
		}
	}
	return nil
}

func (l *List) walkItems(fn func(*Item)) {
	for _, item := range l.Items {
		fn(item)
		for _, sub := range item.SubTasks {
			fn(sub)
		}
	}
}

func (l *List) allDone() bool {
	allComplete := true
	l.walkItems(func(item *Item) {
		if item.Status == StatusPending || item.Status == StatusRunning {
			allComplete = false
		}
	})
	return allComplete
}

func (l *List) notify() {
	if l.onUpdate != nil {
		l.onUpdate(l)
	}
}

func statusIcon(s Status) string {
	switch s {
	case StatusPending:
		return "[ ]"
	case StatusRunning:
		return "[/]"
	case StatusDone:
		return "[x]"
	case StatusFailed:
		return "[!]"
	case StatusSkipped:
		return "[-]"
	default:
		return "[?]"
	}
}
