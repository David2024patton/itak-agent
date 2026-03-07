// scheduler.go implements a request queue with priority ordering and response channels
// for concurrent multi-request serving. Since llama.cpp uses a single inference context,
// requests are processed sequentially but HTTP connections can wait concurrently.
package torch

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// RequestPriority controls processing order.
type RequestPriority int

const (
	PriorityNormal RequestPriority = 0
	PriorityHigh   RequestPriority = 1
)

// InferenceRequest represents a queued completion request.
type InferenceRequest struct {
	ID       uint64
	Messages []ChatMessage
	Params   CompletionParams
	Priority RequestPriority
	Created  time.Time
	Ctx      context.Context

	// Response channel - scheduler writes result here.
	ResultCh chan InferenceResult

	// StreamCh delivers token deltas for SSE streaming.
	// When non-nil, each generated token piece is sent to this channel.
	// The channel is closed when generation completes.
	StreamCh chan string
}

// InferenceResult is the outcome of a queued request.
type InferenceResult struct {
	Text    string
	Metrics *InferenceMetrics
	Err     error
}

// SchedulerStats exposes queue and throughput metrics.
type SchedulerStats struct {
	QueueDepth      int
	TotalProcessed  uint64
	TotalDropped    uint64
	AvgWaitMs       float64
	AvgProcessingMs float64
}

// Scheduler manages a queue of inference requests and processes them.
// When maxSlots > 1, uses continuous batching via BatchEngine.
// When maxSlots == 1, processes requests sequentially (original behavior).
type Scheduler struct {
	engine      Engine
	batchEngine *BatchEngine // nil for sequential mode.
	queue       chan *InferenceRequest
	maxQueue    int

	// Metrics.
	nextID         atomic.Uint64
	totalProcessed atomic.Uint64
	totalDropped   atomic.Uint64
	totalWaitNs    atomic.Int64
	totalProcNs    atomic.Int64

	// Lifecycle.
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewScheduler creates a scheduler with the given queue capacity.
func NewScheduler(engine Engine, maxQueue int) *Scheduler {
	if maxQueue <= 0 {
		maxQueue = 64
	}
	return &Scheduler{
		engine:   engine,
		queue:    make(chan *InferenceRequest, maxQueue),
		maxQueue: maxQueue,
		stopCh:   make(chan struct{}),
	}
}

// NewBatchScheduler creates a scheduler with continuous batching support.
func NewBatchScheduler(engine *TorchEngine, maxQueue, maxSlots int) *Scheduler {
	if maxQueue <= 0 {
		maxQueue = 64
	}
	s := &Scheduler{
		engine:   engine,
		queue:    make(chan *InferenceRequest, maxQueue),
		maxQueue: maxQueue,
		stopCh:   make(chan struct{}),
	}
	if maxSlots > 1 {
		s.batchEngine = NewBatchEngine(engine, maxSlots)
	}
	return s
}

// Start launches the scheduler processing loop.
func (s *Scheduler) Start() {
	s.wg.Add(1)
	if s.batchEngine != nil {
		go s.processBatchLoop()
		fmt.Printf("[iTaK Torch] Scheduler started (continuous batching, queue: %d)\n", s.maxQueue)
	} else {
		go s.processLoop()
		fmt.Printf("[iTaK Torch] Scheduler started (queue capacity: %d)\n", s.maxQueue)
	}
}

// Stop gracefully shuts down the scheduler, finishing the current request.
func (s *Scheduler) Stop() {
	close(s.stopCh)
	s.wg.Wait()
	if s.batchEngine != nil {
		s.batchEngine.Close()
	}
	fmt.Printf("[iTaK Torch] Scheduler stopped (processed: %d, dropped: %d)\n",
		s.totalProcessed.Load(), s.totalDropped.Load())
}

// Submit queues a request for processing. Returns immediately.
// If the queue is full, the request is dropped with an error.
func (s *Scheduler) Submit(req *InferenceRequest) {
	req.ID = s.nextID.Add(1)
	req.Created = time.Now()
	req.ResultCh = make(chan InferenceResult, 1)

	select {
	case s.queue <- req:
		// Queued successfully.
	default:
		// Queue full - reject immediately.
		s.totalDropped.Add(1)
		req.ResultCh <- InferenceResult{
			Err: fmt.Errorf("server overloaded: %d requests queued", s.maxQueue),
		}
	}
}

// Stats returns current scheduler metrics.
func (s *Scheduler) Stats() SchedulerStats {
	processed := s.totalProcessed.Load()
	avgWait := float64(0)
	avgProc := float64(0)
	if processed > 0 {
		avgWait = float64(s.totalWaitNs.Load()) / float64(processed) / 1e6 // ns -> ms
		avgProc = float64(s.totalProcNs.Load()) / float64(processed) / 1e6 // ns -> ms
	}
	return SchedulerStats{
		QueueDepth:      len(s.queue),
		TotalProcessed:  processed,
		TotalDropped:    s.totalDropped.Load(),
		AvgWaitMs:       avgWait,
		AvgProcessingMs: avgProc,
	}
}

// QueueDepth returns the current number of pending requests.
func (s *Scheduler) QueueDepth() int {
	return len(s.queue)
}

// processLoop is the main scheduler goroutine. It processes one request at a time.
func (s *Scheduler) processLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.stopCh:
			// Drain remaining requests with cancellation errors.
			for {
				select {
				case req := <-s.queue:
					req.ResultCh <- InferenceResult{Err: fmt.Errorf("server shutting down")}
				default:
					return
				}
			}
		case req := <-s.queue:
			s.processRequest(req)
		}
	}
}

// processRequest handles a single inference request.
func (s *Scheduler) processRequest(req *InferenceRequest) {
	waitDuration := time.Since(req.Created)
	s.totalWaitNs.Add(waitDuration.Nanoseconds())

	// Check if the client already disconnected while waiting in queue.
	select {
	case <-req.Ctx.Done():
		req.ResultCh <- InferenceResult{Err: req.Ctx.Err()}
		s.totalDropped.Add(1)
		return
	default:
	}

	// Run inference.
	procStart := time.Now()
	var result string
	var err error

	// Use streaming path if StreamCh is set and engine supports it.
	if req.StreamCh != nil {
		if te, ok := s.engine.(*TorchEngine); ok {
			result, err = te.CompleteStream(req.Ctx, req.Messages, req.Params, req.StreamCh)
		} else {
			// Fallback: non-streaming engine, close stream channel.
			result, err = s.engine.Complete(req.Ctx, req.Messages, req.Params)
			close(req.StreamCh)
		}
	} else {
		result, err = s.engine.Complete(req.Ctx, req.Messages, req.Params)
	}

	procDuration := time.Since(procStart)
	s.totalProcNs.Add(procDuration.Nanoseconds())

	// Get metrics from engine.
	stats := s.engine.GetStats()

	req.ResultCh <- InferenceResult{
		Text:    result,
		Metrics: stats.LastMetrics,
		Err:     err,
	}

	s.totalProcessed.Add(1)
}

// processBatchLoop delegates to the BatchEngine for continuous batching.
func (s *Scheduler) processBatchLoop() {
	defer s.wg.Done()
	s.batchEngine.Run(s.queue, s.stopCh)
}
