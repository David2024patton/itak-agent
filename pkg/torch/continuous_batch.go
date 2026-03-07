// continuous_batch.go implements request pipelining for the iTaKTorch scheduler.
// Since true continuous batching requires per-sequence KV cache management
// (not yet exposed via FFI), this implements "pipeline batching" where:
//   - The next request is pre-tokenized while the current request generates
//   - Prefix cache hits are resolved during the current request's generation
//   - Stats track pipeline efficiency for future optimization
//
// When full multi-sequence KV cache management is available (KvCacheSeqRm,
// KvCacheSeqCp), this can be upgraded to true continuous batching.
package torch

import (
	"sync"
	"sync/atomic"
)

// PipelineState tracks the pre-processing state of the next request.
type PipelineState struct {
	Request  *InferenceRequest
	Prompt   string  // Pre-built prompt
	Tokens   []int32 // Pre-tokenized
	CacheHit bool    // Whether prefix cache will hit
	Ready    bool    // Whether pre-processing is complete
}

// ContinuousBatcher manages request pipelining for the scheduler.
type ContinuousBatcher struct {
	mu           sync.Mutex
	pipeline     *PipelineState // Pre-processed next request (if any)
	pipelineHits atomic.Uint64  // Number of times pipeline had a ready request
	pipelineMiss atomic.Uint64  // Number of times pipeline was empty
}

// NewContinuousBatcher creates a new pipelining batcher.
func NewContinuousBatcher() *ContinuousBatcher {
	return &ContinuousBatcher{}
}

// PreProcess pre-tokenizes and checks prefix cache for a request.
// This runs while the previous request is still generating.
func (cb *ContinuousBatcher) PreProcess(req *InferenceRequest, promptBuilder func([]ChatMessage) string, tokenizer func(string, bool) []int32, cache *PrefixCache) {
	if req == nil {
		return
	}

	prompt := promptBuilder(req.Messages)
	tokens := tokenizer(prompt, true)

	// Check prefix cache.
	cacheHit := false
	if cache != nil {
		_, cacheHit = cache.Lookup(prompt)
	}

	cb.mu.Lock()
	cb.pipeline = &PipelineState{
		Request:  req,
		Prompt:   prompt,
		Tokens:   tokens,
		CacheHit: cacheHit,
		Ready:    true,
	}
	cb.mu.Unlock()
}

// TakePipeline returns the pre-processed state for the next request, if available.
// Returns nil if no pre-processed request is waiting.
func (cb *ContinuousBatcher) TakePipeline(req *InferenceRequest) *PipelineState {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.pipeline != nil && cb.pipeline.Request == req && cb.pipeline.Ready {
		state := cb.pipeline
		cb.pipeline = nil
		cb.pipelineHits.Add(1)
		return state
	}
	cb.pipelineMiss.Add(1)
	return nil
}

// PipelineStats returns hit/miss stats.
func (cb *ContinuousBatcher) PipelineStats() (hits, misses uint64) {
	return cb.pipelineHits.Load(), cb.pipelineMiss.Load()
}
