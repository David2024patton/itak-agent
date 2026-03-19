package tasks

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

type Manager struct {
	tasks     map[string]*Task
	mu        sync.RWMutex
	dir       string
	Approvals *ApprovalQueue
}

// NewManager creates a new Task Manager to handle Kanban board states.
func NewManager(workspaceDir string) (*Manager, error) {
	dir := filepath.Join(workspaceDir, "tasks")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create tasks dir: %w", err)
	}

	m := &Manager{
		tasks:     make(map[string]*Task),
		dir:       dir,
		Approvals: NewApprovalQueue(),
	}

	if err := m.loadAll(); err != nil {
		debug.Warn("Tasks", "Failed to load tasks: %v", err)
	}

	return m, nil
}

func (m *Manager) loadAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(m.dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		var t Task
		if err := json.Unmarshal(data, &t); err != nil {
			continue
		}

		m.tasks[t.ID] = &t
	}
	return nil
}

func (m *Manager) saveTask(t *Task) error {
	path := filepath.Join(m.dir, fmt.Sprintf("%s.json", t.ID))
	data, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (m *Manager) CreateTask(title, description string, priority Priority, labels []string, dueDate *time.Time, projectID string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("task_%d", time.Now().UnixNano())

	t := &Task{
		ID:          id,
		Title:       title,
		Description: description,
		Status:      StatusTodo,
		Priority:    priority,
		Labels:      labels,
		DueDate:     dueDate,
		ProjectID:   projectID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		History: []TaskHistoryEntry{
			{
				Timestamp: time.Now(),
				Action:    "Created task",
			},
		},
	}

	m.tasks[t.ID] = t
	if err := m.saveTask(t); err != nil {
		return nil, err
	}

	return t, nil
}

func (m *Manager) UpdateTask(id, title, description string, status Status, assignedAgent string, priority Priority, labels []string, dueDate *time.Time, subItems []SubItem, projectID string, blockedBy []string, blocks []string, recurPattern string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, exists := m.tasks[id]
	if !exists {
		return nil, fmt.Errorf("task not found")
	}

	t.Title = title
	t.Description = description

	if t.Status != status {
		t.History = append(t.History, TaskHistoryEntry{
			Timestamp:   time.Now(),
			Action:      fmt.Sprintf("Status changed from %s to %s", string(t.Status), string(status)),
			WorkerAgent: assignedAgent,
		})
	}
	t.Status = status

	if t.Priority != priority {
		pNames := map[Priority]string{0: "Low", 1: "Medium", 2: "High", 3: "Urgent"}
		t.History = append(t.History, TaskHistoryEntry{
			Timestamp: time.Now(),
			Action:    fmt.Sprintf("Priority changed to %s", pNames[priority]),
		})
	}
	t.Priority = priority
	t.Labels = labels
	t.DueDate = dueDate
	t.SubItems = subItems
	t.ProjectID = projectID
	t.BlockedBy = blockedBy
	t.Blocks = blocks
	t.RecurPattern = recurPattern

	if t.AssignedAgent != assignedAgent {
		t.History = append(t.History, TaskHistoryEntry{
			Timestamp:   time.Now(),
			Action:      fmt.Sprintf("Assigned to %s", assignedAgent),
			WorkerAgent: assignedAgent,
		})
	}
	t.AssignedAgent = assignedAgent
	t.UpdatedAt = time.Now()

	if err := m.saveTask(t); err != nil {
		return nil, err
	}

	return t, nil
}

func (m *Manager) UpdateTaskStatus(id string, status Status) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, exists := m.tasks[id]
	if !exists {
		return nil, fmt.Errorf("task not found")
	}

	// State machine guard: validate the transition.
	if err := ValidateTransition(t.Status, status); err != nil {
		return nil, fmt.Errorf("state machine rejected: %w", err)
	}

	if t.Status != status {
		t.History = append(t.History, TaskHistoryEntry{
			Timestamp: time.Now(),
			Action:    fmt.Sprintf("Status changed to %s", string(status)),
		})
	}

	oldStatus := t.Status
	t.Status = status
	t.UpdatedAt = time.Now()

	// Track wall-clock start when entering InProgress.
	if status == StatusInProgress && oldStatus != StatusInProgress {
		now := time.Now()
		if t.WallClockStart == nil {
			t.WallClockStart = &now
		}
	}

	// Auto-review: when a task enters Review, run automated checks.
	if status == StatusReview {
		review := RunReview(t)
		t.Review = &review
		if review.Passed {
			t.Status = StatusDone
			t.History = append(t.History, TaskHistoryEntry{
				Timestamp: time.Now(),
				Action:    "Auto-review passed, moved to Done",
			})
			debug.Info("tasks", "Task %s passed auto-review, advancing to Done", id)
		} else {
			t.Status = StatusInProgress
			t.History = append(t.History, TaskHistoryEntry{
				Timestamp: time.Now(),
				Action:    fmt.Sprintf("Auto-review failed: %d issues found", len(review.Issues)),
			})
			debug.Warn("tasks", "Task %s failed auto-review: %v", id, review.Issues)
		}
	}

	// Dependency cascade: when a task completes, unblock downstream tasks.
	if status == StatusDone {
		m.cascadeUnblock(t)
	}

	if err := m.saveTask(t); err != nil {
		return nil, err
	}

	return t, nil
}

func (m *Manager) GetTasks() []*Task {
	m.mu.RLock()
	defer m.mu.RUnlock()

	all := make([]*Task, 0, len(m.tasks))
	for _, t := range m.tasks {
		tc := *t
		all = append(all, &tc)
	}
	return all
}

func (m *Manager) GetTask(id string) (*Task, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	t, exists := m.tasks[id]
	if !exists {
		return nil, fmt.Errorf("task not found")
	}
	tc := *t
	return &tc, nil
}

func (m *Manager) DeleteTask(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.tasks[id]; !exists {
		return fmt.Errorf("task not found")
	}

	delete(m.tasks, id)

	path := filepath.Join(m.dir, fmt.Sprintf("%s.json", id))
	return os.Remove(path)
}

// AddComment appends a comment to a task.
func (m *Manager) AddComment(taskID, author, text string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, exists := m.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found")
	}

	c := Comment{
		ID:        fmt.Sprintf("cmt_%d", time.Now().UnixNano()),
		Author:    author,
		Text:      text,
		CreatedAt: time.Now(),
	}
	t.Comments = append(t.Comments, c)
	t.History = append(t.History, TaskHistoryEntry{
		Timestamp:   time.Now(),
		Action:      fmt.Sprintf("Comment by %s", author),
		WorkerAgent: author,
	})
	t.UpdatedAt = time.Now()

	if err := m.saveTask(t); err != nil {
		return nil, err
	}
	return t, nil
}

// AddAttachment appends an attachment record to a task.
func (m *Manager) AddAttachment(taskID, name, path, mimeType string, size int64) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, exists := m.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found")
	}

	a := Attachment{
		ID:         fmt.Sprintf("att-%d", time.Now().UnixNano()),
		Name:       name,
		Path:       path,
		Size:       size,
		MimeType:   mimeType,
		UploadedAt: time.Now(),
	}
	t.Attachments = append(t.Attachments, a)
	t.History = append(t.History, TaskHistoryEntry{
		Timestamp: time.Now(),
		Action:    fmt.Sprintf("File attached: %s", name),
	})
	t.UpdatedAt = time.Now()

	if err := m.saveTask(t); err != nil {
		return nil, err
	}
	return t, nil
}

// AddReasoning appends a chain-of-thought entry to the task's scratchpad.
func (m *Manager) AddReasoning(taskID, step, toolCall, result string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, exists := m.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found")
	}

	t.Reasoning = append(t.Reasoning, ReasoningEntry{
		Timestamp: time.Now(),
		Step:      step,
		ToolCall:  toolCall,
		Result:    result,
	})
	t.UpdatedAt = time.Now()

	if err := m.saveTask(t); err != nil {
		return nil, err
	}
	return t, nil
}

// UpdateProgress sets the task's completion percentage (0-100).
func (m *Manager) UpdateProgress(taskID string, progress int) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, exists := m.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found")
	}

	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	t.Progress = progress
	t.UpdatedAt = time.Now()

	if err := m.saveTask(t); err != nil {
		return nil, err
	}
	return t, nil
}

// SpawnChildTask creates a child task linked to a parent.
// The child inherits project ID and agent config from the parent.
func (m *Manager) SpawnChildTask(parentID, title, description string, priority Priority) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	parent, exists := m.tasks[parentID]
	if !exists {
		return nil, fmt.Errorf("parent task not found")
	}

	id := fmt.Sprintf("task_%d", time.Now().UnixNano())

	child := &Task{
		ID:            id,
		Title:         title,
		Description:   description,
		Status:        StatusTodo,
		Priority:      priority,
		ProjectID:     parent.ProjectID,
		ParentTaskID:  parentID,
		AgentConfig:   parent.AgentConfig,
		AutoExecute:   parent.AutoExecute,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		History: []TaskHistoryEntry{
			{
				Timestamp: time.Now(),
				Action:    fmt.Sprintf("Spawned as child of %s", parentID),
			},
		},
	}

	// Link parent to child.
	parent.ChildTaskIDs = append(parent.ChildTaskIDs, id)
	parent.History = append(parent.History, TaskHistoryEntry{
		Timestamp: time.Now(),
		Action:    fmt.Sprintf("Spawned child task: %s (%s)", title, id),
	})
	parent.UpdatedAt = time.Now()

	m.tasks[id] = child
	if err := m.saveTask(child); err != nil {
		return nil, err
	}
	if err := m.saveTask(parent); err != nil {
		return nil, err
	}

	debug.Info("tasks", "Spawned child task %s (%s) under parent %s", id, title, parentID)
	return child, nil
}

// CompleteChildTask marks a child as done and checks if the parent can resume.
// If all children are Done, the parent task moves from Waiting back to InProgress.
func (m *Manager) CompleteChildTask(childID string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	child, exists := m.tasks[childID]
	if !exists {
		return nil, fmt.Errorf("child task not found")
	}

	if child.ParentTaskID == "" {
		return child, nil // not a child, nothing to cascade
	}

	parent, exists := m.tasks[child.ParentTaskID]
	if !exists {
		return child, nil // orphan child, parent deleted
	}

	// Check if all siblings are done.
	allDone := true
	for _, sibID := range parent.ChildTaskIDs {
		sib, ok := m.tasks[sibID]
		if !ok {
			continue
		}
		if sib.Status != StatusDone {
			allDone = false
			break
		}
	}

	if allDone && (parent.Status == StatusWaiting || parent.Status == StatusInProgress) {
		parent.Status = StatusInProgress
		parent.History = append(parent.History, TaskHistoryEntry{
			Timestamp: time.Now(),
			Action:    "All child tasks completed, parent resumed",
		})
		parent.UpdatedAt = time.Now()
		if err := m.saveTask(parent); err != nil {
			debug.Warn("tasks", "Failed to save parent after child completion: %v", err)
		}
		debug.Info("tasks", "All children of %s complete, parent resumed", parent.ID)
	}

	return child, nil
}

// EnforceBudget checks if a task has exceeded its token or wall-clock limits.
// Returns true if the task was paused/escalated, false if within budget.
func (m *Manager) EnforceBudget(taskID string, tokensUsed int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, exists := m.tasks[taskID]
	if !exists {
		return false, fmt.Errorf("task not found")
	}

	t.TokensUsed += tokensUsed
	t.UpdatedAt = time.Now()

	// Check token budget.
	if t.AgentConfig != nil && t.AgentConfig.MaxTokens > 0 && t.TokensUsed >= t.AgentConfig.MaxTokens {
		t.Status = StatusPaused
		t.EscalationNote = fmt.Sprintf("Token budget exceeded: %d / %d tokens used", t.TokensUsed, t.AgentConfig.MaxTokens)
		t.History = append(t.History, TaskHistoryEntry{
			Timestamp: time.Now(),
			Action:    fmt.Sprintf("Paused: token budget exceeded (%d/%d)", t.TokensUsed, t.AgentConfig.MaxTokens),
		})
		debug.Warn("tasks", "Task %s paused: token budget exceeded (%d/%d)", taskID, t.TokensUsed, t.AgentConfig.MaxTokens)
		if err := m.saveTask(t); err != nil {
			return true, err
		}
		return true, nil
	}

	// Check wall-clock TTL.
	if t.AgentConfig != nil && t.AgentConfig.TTLSeconds > 0 && t.WallClockStart != nil {
		elapsed := time.Since(*t.WallClockStart)
		ttl := time.Duration(t.AgentConfig.TTLSeconds) * time.Second
		if elapsed >= ttl {
			t.Status = StatusPaused
			t.EscalationNote = fmt.Sprintf("Wall-clock TTL exceeded: %s elapsed (limit: %s)", elapsed.Round(time.Second), ttl)
			t.History = append(t.History, TaskHistoryEntry{
				Timestamp: time.Now(),
				Action:    fmt.Sprintf("Paused: wall-clock TTL exceeded (%s / %s)", elapsed.Round(time.Second), ttl),
			})
			debug.Warn("tasks", "Task %s paused: TTL exceeded (%s/%s)", taskID, elapsed.Round(time.Second), ttl)
			if err := m.saveTask(t); err != nil {
				return true, err
			}
			return true, nil
		}
	}

	if err := m.saveTask(t); err != nil {
		return false, err
	}
	return false, nil
}

// WakeTask resumes a Waiting task when its webhook fires.
func (m *Manager) WakeTask(taskID, webhookID string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, exists := m.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found")
	}

	if t.Status != StatusWaiting {
		return nil, fmt.Errorf("task is not in Waiting status (current: %s)", t.Status)
	}

	// Verify webhook ID matches if one was set.
	if t.WakeOnWebhook != "" && t.WakeOnWebhook != webhookID {
		return nil, fmt.Errorf("webhook ID mismatch: expected %s, got %s", t.WakeOnWebhook, webhookID)
	}

	t.Status = StatusInProgress
	t.History = append(t.History, TaskHistoryEntry{
		Timestamp: time.Now(),
		Action:    fmt.Sprintf("Woken by webhook: %s", webhookID),
	})
	t.UpdatedAt = time.Now()

	if err := m.saveTask(t); err != nil {
		return nil, err
	}

	debug.Info("tasks", "Task %s woken by webhook %s", taskID, webhookID)
	return t, nil
}

// SetConfidence sets the agent's self-assessed confidence score on a task.
func (m *Manager) SetConfidence(taskID string, score int) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, exists := m.tasks[taskID]
	if !exists {
		return nil, fmt.Errorf("task not found")
	}

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	t.ConfidenceScore = score
	t.UpdatedAt = time.Now()

	if err := m.saveTask(t); err != nil {
		return nil, err
	}
	return t, nil
}

// cascadeUnblock is called internally when a task completes.
// It removes this task's ID from the BlockedBy list of all tasks it blocks,
// and auto-transitions newly unblocked tasks from Blocked to Todo.
// Must be called while holding m.mu.
func (m *Manager) cascadeUnblock(completed *Task) {
	if len(completed.Blocks) == 0 {
		return
	}

	for _, blockedID := range completed.Blocks {
		blocked, exists := m.tasks[blockedID]
		if !exists {
			continue
		}

		// Remove completed task from the blocked task's BlockedBy list.
		newBlockedBy := make([]string, 0, len(blocked.BlockedBy))
		for _, dep := range blocked.BlockedBy {
			if dep != completed.ID {
				newBlockedBy = append(newBlockedBy, dep)
			}
		}
		blocked.BlockedBy = newBlockedBy

		// If no more blockers and task is Blocked, move to Todo.
		if len(blocked.BlockedBy) == 0 && blocked.Status == StatusBlocked {
			blocked.Status = StatusTodo
			blocked.History = append(blocked.History, TaskHistoryEntry{
				Timestamp: time.Now(),
				Action:    fmt.Sprintf("Unblocked: dependency %s completed", completed.ID),
			})
			debug.Info("tasks", "Task %s unblocked by completion of %s", blockedID, completed.ID)
		}

		blocked.UpdatedAt = time.Now()
		m.saveTask(blocked)
	}
}

// TrackModifiedFile records a file path that was changed during task execution.
func (m *Manager) TrackModifiedFile(taskID, filePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	t, exists := m.tasks[taskID]
	if !exists {
		return fmt.Errorf("task not found")
	}

	// Avoid duplicates.
	for _, f := range t.ModifiedFiles {
		if f == filePath {
			return nil
		}
	}

	t.ModifiedFiles = append(t.ModifiedFiles, filePath)
	t.UpdatedAt = time.Now()
	return m.saveTask(t)
}
