package tasks

import (
	"fmt"
	"sync"
	"time"
)

// ApprovalAction defines what kind of destructive action needs approval.
type ApprovalAction string

const (
	ApprovalDelete  ApprovalAction = "delete"
	ApprovalArchive ApprovalAction = "archive"
	ApprovalModify  ApprovalAction = "modify_critical"
)

// ApprovalStatus tracks the state of an approval request.
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalRejected ApprovalStatus = "rejected"
)

// ApprovalRequest represents a pending human-in-the-loop verification.
type ApprovalRequest struct {
	ID          string         `json:"id"`
	TaskID      string         `json:"task_id"`
	Action      ApprovalAction `json:"action"`
	Description string         `json:"description"`
	Requester   string         `json:"requester"` // agent name
	Status      ApprovalStatus `json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	ResolvedAt  *time.Time     `json:"resolved_at,omitempty"`
	ResolvedBy  string         `json:"resolved_by,omitempty"`
}

// ApprovalQueue manages pending approval requests with thread-safe access.
type ApprovalQueue struct {
	mu       sync.RWMutex
	requests map[string]*ApprovalRequest
	counter  int
}

// NewApprovalQueue creates a new approval queue.
func NewApprovalQueue() *ApprovalQueue {
	return &ApprovalQueue{
		requests: make(map[string]*ApprovalRequest),
	}
}

// Request creates a new pending approval and returns its ID.
func (q *ApprovalQueue) Request(taskID string, action ApprovalAction, description, requester string) string {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.counter++
	id := fmt.Sprintf("apr-%d", q.counter)

	q.requests[id] = &ApprovalRequest{
		ID:          id,
		TaskID:      taskID,
		Action:      action,
		Description: description,
		Requester:   requester,
		Status:      ApprovalPending,
		CreatedAt:   time.Now(),
	}

	return id
}

// Approve marks a request as approved.
func (q *ApprovalQueue) Approve(id, resolvedBy string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	req, ok := q.requests[id]
	if !ok {
		return fmt.Errorf("approval %s not found", id)
	}
	if req.Status != ApprovalPending {
		return fmt.Errorf("approval %s already resolved", id)
	}

	now := time.Now()
	req.Status = ApprovalApproved
	req.ResolvedAt = &now
	req.ResolvedBy = resolvedBy
	return nil
}

// Reject marks a request as rejected.
func (q *ApprovalQueue) Reject(id, resolvedBy string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	req, ok := q.requests[id]
	if !ok {
		return fmt.Errorf("approval %s not found", id)
	}
	if req.Status != ApprovalPending {
		return fmt.Errorf("approval %s already resolved", id)
	}

	now := time.Now()
	req.Status = ApprovalRejected
	req.ResolvedAt = &now
	req.ResolvedBy = resolvedBy
	return nil
}

// Pending returns all pending approval requests.
func (q *ApprovalQueue) Pending() []*ApprovalRequest {
	q.mu.RLock()
	defer q.mu.RUnlock()

	var pending []*ApprovalRequest
	for _, r := range q.requests {
		if r.Status == ApprovalPending {
			pending = append(pending, r)
		}
	}
	return pending
}

// All returns all approval requests.
func (q *ApprovalQueue) All() []*ApprovalRequest {
	q.mu.RLock()
	defer q.mu.RUnlock()

	all := make([]*ApprovalRequest, 0, len(q.requests))
	for _, r := range q.requests {
		all = append(all, r)
	}
	return all
}

// Get returns a single approval request by ID.
func (q *ApprovalQueue) Get(id string) (*ApprovalRequest, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()
	r, ok := q.requests[id]
	return r, ok
}
