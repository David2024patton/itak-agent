package task

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var taskIDCounter uint64

// Tracker manages all active and completed task lists.
// This is the central state for the dashboard and API.
type Tracker struct {
	mu       sync.RWMutex
	active   map[string]*List
	history  []*List
	maxHist  int
	onUpdate func(*List) // global update callback
}

// NewTracker creates a task tracker.
func NewTracker(maxHistory int) *Tracker {
	if maxHistory <= 0 {
		maxHistory = 100
	}
	return &Tracker{
		active:  make(map[string]*List),
		maxHist: maxHistory,
	}
}

// OnUpdate sets a global callback for all task list updates.
// Useful for broadcasting to WebTransport/SSE clients.
func (t *Tracker) OnUpdate(fn func(*List)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onUpdate = fn
}

// Create starts a new task list for a user request.
// Returns the list for the caller to add items to.
func (t *Tracker) Create(userRequest string) *List {
	t.mu.Lock()
	defer t.mu.Unlock()

	seq := atomic.AddUint64(&taskIDCounter, 1)
	id := fmt.Sprintf("task-%d-%d", time.Now().UnixNano(), seq)
	list := NewList(id, userRequest)

	// Attach global update callback.
	if t.onUpdate != nil {
		cb := t.onUpdate
		list.OnUpdate(cb)
	}

	t.active[id] = list
	return list
}

// Get returns an active task list by ID.
func (t *Tracker) Get(id string) (*List, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	l, ok := t.active[id]
	return l, ok
}

// Active returns all active (incomplete) task lists.
func (t *Tracker) Active() []*List {
	t.mu.RLock()
	defer t.mu.RUnlock()

	lists := make([]*List, 0, len(t.active))
	for _, l := range t.active {
		lists = append(lists, l)
	}
	return lists
}

// Archive moves a completed task list from active to history.
func (t *Tracker) Archive(id string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	list, ok := t.active[id]
	if !ok {
		return fmt.Errorf("task list %q not found", id)
	}

	delete(t.active, id)
	t.history = append(t.history, list)

	// Trim history if over limit.
	if len(t.history) > t.maxHist {
		t.history = t.history[len(t.history)-t.maxHist:]
	}
	return nil
}

// History returns the most recent N completed task lists.
func (t *Tracker) History(n int) []*List {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if n > len(t.history) {
		n = len(t.history)
	}
	result := make([]*List, n)
	copy(result, t.history[len(t.history)-n:])
	return result
}

// Stats returns aggregate stats across all active tasks.
func (t *Tracker) Stats() TrackerStats {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := TrackerStats{
		ActiveLists:  len(t.active),
		TotalHistory: len(t.history),
	}

	for _, l := range t.active {
		done, total := l.Progress()
		stats.TotalItems += total
		stats.CompletedItems += done
	}
	return stats
}

// TrackerStats is a snapshot of the tracker's state.
type TrackerStats struct {
	ActiveLists    int `json:"active_lists"`
	TotalHistory   int `json:"total_history"`
	TotalItems     int `json:"total_items"`
	CompletedItems int `json:"completed_items"`
}
