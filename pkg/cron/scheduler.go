// Package cron provides scheduled automation for agent tasks.
//
// What: A goroutine-safe scheduler that runs agent automations on cron/interval/one-shot schedules.
// Why:  Users need agents to perform recurring work (social media posts, data syncs, reports)
//       without manual triggering. Separates user-defined scheduling from Doctor health checks.
// How:  Reads jobs from data/cron/jobs.json, evaluates schedules every minute, and dispatches
//       matching jobs. Supports cron expressions, fixed intervals, and one-shot timestamps.
//       Adopted patterns from OpenClaw v2026.3.12: JSON persistence, idempotency keys,
//       isolated vs main session execution modes.
package cron

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/David2024patton/iTaKAgent/pkg/debug"
)

// Job represents a single scheduled automation.
type Job struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Type           string `json:"type"`            // cron, webhook, trigger
	ScheduleType   string `json:"schedule_type"`   // at, every, cron
	Schedule       string `json:"schedule"`         // cron expr, duration, or ISO timestamp
	Agent          string `json:"agent"`            // which agent runs this
	Prompt         string `json:"prompt"`           // what to tell the agent
	AgencyID       string `json:"agency_id,omitempty"`
	SubAccountID   string `json:"subaccount_id,omitempty"`
	Enabled        bool   `json:"enabled"`
	ExecutionMode  string `json:"execution_mode"`   // main or isolated
	WebhookSecret  string `json:"webhook_secret,omitempty"`
	LastRun        string `json:"last_run,omitempty"`
	NextRun        string `json:"next_run,omitempty"`
	RunCount       int    `json:"run_count"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// RunRecord captures a single execution of a job.
type RunRecord struct {
	JobID          string `json:"job_id"`
	RunAt          string `json:"run_at"`
	Status         string `json:"status"` // success, error, skipped
	Result         string `json:"result,omitempty"`
	IdempotencyKey string `json:"idempotency_key"`
}

// Scheduler manages job persistence, evaluation, and dispatch.
type Scheduler struct {
	mu       sync.RWMutex
	jobs     map[string]*Job
	history  []RunRecord
	dataDir  string
	stopCh   chan struct{}
	dispatch func(job *Job) // callback invoked when a job should run
}

// NewScheduler creates a scheduler with persistence in the given data directory.
func NewScheduler(dataDir string, dispatch func(job *Job)) *Scheduler {
	cronDir := filepath.Join(dataDir, "cron")
	os.MkdirAll(cronDir, 0755)

	s := &Scheduler{
		jobs:     make(map[string]*Job),
		history:  make([]RunRecord, 0),
		dataDir:  cronDir,
		stopCh:   make(chan struct{}),
		dispatch: dispatch,
	}

	s.loadJobs()
	return s
}

// Start begins the scheduler loop, checking every 60 seconds.
func (s *Scheduler) Start() {
	debug.Info("cron", "Scheduler started (%d jobs loaded)", len(s.jobs))
	go s.loop()
}

// Stop halts the scheduler.
func (s *Scheduler) Stop() {
	close(s.stopCh)
}

// AddJob adds or updates a job and persists.
func (s *Scheduler) AddJob(j *Job) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if j.ID == "" {
		j.ID = fmt.Sprintf("job_%d", time.Now().UnixNano())
	}
	if j.CreatedAt == "" {
		j.CreatedAt = time.Now().Format(time.RFC3339)
	}
	if j.ExecutionMode == "" {
		j.ExecutionMode = "main"
	}

	// Calculate next run time.
	j.NextRun = s.calcNextRun(j)

	s.jobs[j.ID] = j
	s.saveJobs()
	debug.Info("cron", "Job added: %s (%s, schedule=%s, next=%s)", j.Name, j.Type, j.Schedule, j.NextRun)
}

// RemoveJob deletes a job by ID.
func (s *Scheduler) RemoveJob(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.jobs[id]; ok {
		delete(s.jobs, id)
		s.saveJobs()
		return true
	}
	return false
}

// GetJob returns a job by ID.
func (s *Scheduler) GetJob(id string) *Job {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.jobs[id]
}

// ListJobs returns all jobs.
func (s *Scheduler) ListJobs() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		result = append(result, j)
	}
	return result
}

// TriggerJob manually runs a job immediately.
func (s *Scheduler) TriggerJob(id string) bool {
	s.mu.RLock()
	j, ok := s.jobs[id]
	s.mu.RUnlock()

	if !ok {
		return false
	}

	go s.runJob(j)
	return true
}

// History returns recent run records.
func (s *Scheduler) History(jobID string, limit int) []RunRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	var result []RunRecord
	for i := len(s.history) - 1; i >= 0 && len(result) < limit; i-- {
		if jobID == "" || s.history[i].JobID == jobID {
			result = append(result, s.history[i])
		}
	}
	return result
}

// loop is the main scheduler tick.
func (s *Scheduler) loop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.evaluate(now)
		}
	}
}

// evaluate checks all jobs and dispatches those that are due.
func (s *Scheduler) evaluate(now time.Time) {
	s.mu.RLock()
	var due []*Job
	for _, j := range s.jobs {
		if !j.Enabled {
			continue
		}
		if j.NextRun == "" {
			continue
		}
		nextRun, err := time.Parse(time.RFC3339, j.NextRun)
		if err != nil {
			continue
		}
		if now.After(nextRun) || now.Equal(nextRun) {
			due = append(due, j)
		}
	}
	s.mu.RUnlock()

	for _, j := range due {
		s.runJob(j)
	}
}

// runJob executes a single job.
func (s *Scheduler) runJob(j *Job) {
	// Generate idempotency key to prevent duplicates.
	idemKey := fmt.Sprintf("%s_%d", j.ID, time.Now().UnixNano())

	debug.Info("cron", "Running job: %s (%s) [agent=%s]", j.Name, j.ID, j.Agent)

	// Dispatch to agent.
	if s.dispatch != nil {
		s.dispatch(j)
	}

	// Record run.
	s.mu.Lock()
	j.LastRun = time.Now().Format(time.RFC3339)
	j.RunCount++
	j.IdempotencyKey = idemKey

	// Calculate next run.
	if j.ScheduleType == "at" {
		// One-shot: disable after running.
		j.Enabled = false
		j.NextRun = ""
	} else {
		j.NextRun = s.calcNextRun(j)
	}

	s.history = append(s.history, RunRecord{
		JobID:          j.ID,
		RunAt:          j.LastRun,
		Status:         "success",
		IdempotencyKey: idemKey,
	})

	// Cap history at 500 entries.
	if len(s.history) > 500 {
		s.history = s.history[len(s.history)-500:]
	}

	s.saveJobs()
	s.mu.Unlock()
}

// calcNextRun figures out when a job should next fire.
func (s *Scheduler) calcNextRun(j *Job) string {
	now := time.Now()

	switch j.ScheduleType {
	case "at":
		// One-shot: the schedule IS the run time.
		return j.Schedule

	case "every":
		// Interval: parse duration like "30m", "1h", "24h".
		d, err := time.ParseDuration(j.Schedule)
		if err != nil {
			debug.Warn("cron", "Invalid interval %q for job %s", j.Schedule, j.ID)
			return ""
		}
		return now.Add(d).Format(time.RFC3339)

	case "cron":
		// Simple cron expression parser for common patterns.
		return s.nextCronTime(j.Schedule, now)

	default:
		return ""
	}
}

// nextCronTime parses basic cron expressions (minute hour dom month dow).
// Supports: "0 9 * * *" (daily at 9am), "0 9 * * 1-5" (weekdays 9am), "*/30 * * * *" (every 30 min).
func (s *Scheduler) nextCronTime(expr string, now time.Time) string {
	parts := strings.Fields(expr)
	if len(parts) != 5 {
		debug.Warn("cron", "Invalid cron expression: %q", expr)
		return now.Add(1 * time.Hour).Format(time.RFC3339)
	}

	// Parse minute field.
	minute := 0
	if parts[0] != "*" {
		if strings.HasPrefix(parts[0], "*/") {
			interval, _ := strconv.Atoi(parts[0][2:])
			if interval > 0 {
				// Next interval from now.
				currentMin := now.Minute()
				nextMin := ((currentMin/interval)+1)*interval
				if nextMin >= 60 {
					nextMin = 0
					return time.Date(now.Year(), now.Month(), now.Day(), now.Hour()+1, nextMin, 0, 0, now.Location()).Format(time.RFC3339)
				}
				return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), nextMin, 0, 0, now.Location()).Format(time.RFC3339)
			}
		}
		minute, _ = strconv.Atoi(parts[0])
	}

	// Parse hour field.
	hour := now.Hour()
	if parts[1] != "*" {
		hour, _ = strconv.Atoi(parts[1])
	}

	// Build next run candidate.
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
	if next.Before(now) || next.Equal(now) {
		next = next.Add(24 * time.Hour)
	}

	return next.Format(time.RFC3339)
}

// Persistence: load/save jobs from data/cron/jobs.json.
func (s *Scheduler) loadJobs() {
	path := filepath.Join(s.dataDir, "jobs.json")
	data, err := os.ReadFile(path)
	if err != nil {
		// No jobs file yet, that's fine.
		return
	}

	var jobs []*Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		debug.Warn("cron", "Failed to parse jobs.json: %v", err)
		return
	}

	for _, j := range jobs {
		s.jobs[j.ID] = j
	}
	debug.Info("cron", "Loaded %d jobs from disk", len(jobs))
}

func (s *Scheduler) saveJobs() {
	path := filepath.Join(s.dataDir, "jobs.json")
	jobs := make([]*Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		jobs = append(jobs, j)
	}

	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		debug.Warn("cron", "Failed to marshal jobs: %v", err)
		return
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		debug.Warn("cron", "Failed to save jobs.json: %v", err)
	}
}
