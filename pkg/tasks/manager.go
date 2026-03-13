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
	tasks map[string]*Task
	mu    sync.RWMutex
	dir   string
}

// NewManager creates a new Task Manager to handle Kanban board states.
func NewManager(workspaceDir string) (*Manager, error) {
	dir := filepath.Join(workspaceDir, "tasks")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create tasks dir: %w", err)
	}

	m := &Manager{
		tasks: make(map[string]*Task),
		dir:   dir,
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

func (m *Manager) CreateTask(title, description string) (*Task, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := fmt.Sprintf("task_%d", time.Now().UnixNano())

	t := &Task{
		ID:          id,
		Title:       title,
		Description: description,
		Status:      StatusTodo,
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

func (m *Manager) UpdateTask(id, title, description string, status Status, assignedAgent string) (*Task, error) {
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

	if t.Status != status {
		t.History = append(t.History, TaskHistoryEntry{
			Timestamp: time.Now(),
			Action:    fmt.Sprintf("Status changed to %s", string(status)),
		})
	}

	t.Status = status
	t.UpdatedAt = time.Now()

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
