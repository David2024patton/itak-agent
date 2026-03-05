package torch

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/David2024patton/GOAgent/pkg/torch/llama"
)

// TorchEngine implements the Engine interface using the forked yzma/llama.cpp
// bindings via purego/ffi. No CGo required.
type TorchEngine struct {
	model     llama.Model
	ctx       llama.Context
	vocab     llama.Vocab
	sampler   llama.Sampler
	tokenBuf  []byte // pre-allocated buffer for TokenToPiece (eliminates per-token heap alloc)
	modelName string
	modelPath string
	opts      EngineOpts
	mu        sync.Mutex
	loaded    bool
	Stats     EngineStats
}

// NewTorchEngine creates an engine that loads a GGUF model via llama.cpp.
// libPath is the directory containing the llama.cpp shared libraries.
// If empty, checks the GOTORCH_LIB environment variable.
func NewTorchEngine(modelPath string, opts EngineOpts) (*TorchEngine, error) {
	// Find the llama.cpp shared libraries.
	libPath := os.Getenv("GOTORCH_LIB")
	if libPath == "" {
		// Smart lib selection: choose CPU-only or CUDA libs based on GPU layer count.
		// This prevents CUDA DLL contamination that causes 2.5x slowdown on CPU workloads.
		platformDir := runtime.GOOS + "_" + runtime.GOARCH
		var candidates []string
		if opts.GPULayers > 0 {
			// GPU mode: prefer CUDA libs, fallback to generic.
			candidates = []string{
				"./lib/" + platformDir + "_cuda",
				"~/.gotorch/lib",
				"./lib/" + platformDir,
				"./lib",
			}
		} else {
			// CPU-only mode: prefer CPU-only libs to avoid CUDA overhead.
			candidates = []string{
				"./lib/" + platformDir,
				"~/.gotorch/lib",
				"./lib",
			}
		}
		for _, c := range candidates {
			expanded := c
			if strings.HasPrefix(c, "~") {
				if home, err := os.UserHomeDir(); err == nil {
					expanded = home + c[1:]
				}
			}
			if _, err := os.Stat(expanded); err == nil {
				libPath = expanded
				break
			}
		}
	}
	fmt.Printf("[GOTorch] Using libs from: %s\n", libPath)

	if libPath == "" {
		return nil, fmt.Errorf("llama.cpp libraries not found. Set GOTORCH_LIB env variable or run 'gotorch install'")
	}

	// Load the shared libraries.
	if err := llama.Load(libPath); err != nil {
		return nil, fmt.Errorf("load llama.cpp libraries from %s: %w", libPath, err)
	}

	// Initialize the backend.
	llama.Init()

	// Initialize NUMA if configured.
	if opts.NumaStrategy > 0 {
		llama.NumaInit(llama.NumaStrategy(opts.NumaStrategy))
		fmt.Printf("[GOTorch] NUMA strategy: %d\n", opts.NumaStrategy)
	}

	// Apply smart defaults.
	if opts.ContextSize == 0 {
		opts.ContextSize = 2048
	}
	if opts.Threads == 0 {
		opts.Threads = runtime.NumCPU()
	}
	if opts.BatchSize == 0 {
		opts.BatchSize = 2048
	}

	// Phase 3: Auto-enable flash attention on GPU mode.
	// Modern CUDA GPUs all support it, and it improves throughput by 3-5%.
	if opts.GPULayers > 0 && !opts.NoFlashAttention {
		opts.FlashAttention = true
	}

	// Snapshot resources before loading.
	preLoad := CaptureResources()
	loadStart := time.Now()

	// Load the model.
	// ModelDefaultParams() returns mmap=1 from llama.cpp, which we preserve.
	modelParams := llama.ModelDefaultParams()
	modelParams.NGpuLayers = int32(opts.GPULayers)
	if opts.UseMlock {
		modelParams.UseMlock = 1
	}

	model, err := llama.ModelLoadFromFile(modelPath, modelParams)
	if err != nil {
		return nil, fmt.Errorf("load model %s: %w", modelPath, err)
	}

	loadDuration := time.Since(loadStart)

	// Create context with performance optimizations.
	ctxParams := llama.ContextDefaultParams()
	ctxParams.NCtx = uint32(opts.ContextSize)
	ctxParams.NThreads = int32(opts.Threads)
	ctxParams.NThreadsBatch = int32(opts.Threads)
	ctxParams.NBatch = uint32(opts.BatchSize)

	// Enable flash attention for faster inference (auto mode detects support).
	if opts.FlashAttention {
		ctxParams.FlashAttentionType = llama.FlashAttentionTypeEnabled
	}

	ctx, err := llama.InitFromModel(model, ctxParams)
	if err != nil {
		return nil, fmt.Errorf("create context: %w", err)
	}

	// Get vocab.
	vocab := llama.ModelGetVocab(model)

	// Create default sampler chain with standard sampling pipeline.
	samplerParams := llama.DefaultSamplerParams()
	sampler := llama.NewSampler(model, llama.DefaultSamplers, samplerParams)

	// Extract model name from path.
	name := modelPath
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	if idx := strings.LastIndex(name, "\\"); idx >= 0 {
		name = name[idx+1:]
	}
	name = strings.TrimSuffix(name, ".gguf")

	engine := &TorchEngine{
		model:     model,
		ctx:       ctx,
		vocab:     vocab,
		sampler:   sampler,
		tokenBuf:  make([]byte, 256), // pre-allocated to avoid per-token heap allocation
		modelName: name,
		modelPath: modelPath,
		opts:      opts,
		loaded:    true,
	}

	// Record load metrics.
	postLoad := CaptureResources()
	engine.Stats.ModelLoadTime = loadDuration
	engine.Stats.PreLoadRes = preLoad
	engine.Stats.PostLoadRes = postLoad

	fmt.Printf("[GOTorch] Model loaded in %s\n", loadDuration.Round(time.Millisecond))
	fmt.Printf("[GOTorch] %s\n", preLoad.String())
	fmt.Printf("[GOTorch] %s\n", postLoad.String())

	// Print system capabilities.
	sysInfo := llama.PrintSystemInfo()
	if sysInfo != "" {
		fmt.Printf("[GOTorch] System: %s\n", sysInfo)
	}
	fmt.Printf("[GOTorch] Optimizations: mmap=%v mlock=%v gpu_offload=%v flash_attn=%v threads=%d batch=%d\n",
		llama.SupportsMmap(), opts.UseMlock, llama.SupportsGpuOffload(),
		opts.FlashAttention, opts.Threads, opts.BatchSize)

	// Warmup: triggers GPU kernel JIT compilation for common tensor shapes,
	// reducing latency on first real request.
	if err := llama.Warmup(ctx, model); err != nil {
		fmt.Printf("[GOTorch] Warmup warning: %v\n", err)
	} else {
		fmt.Printf("[GOTorch] Model warmup complete\n")
	}

	return engine, nil
}

// Complete runs inference on the given messages and returns the generated text.
func (e *TorchEngine) Complete(ctx context.Context, messages []ChatMessage, params CompletionParams) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.loaded {
		return "", fmt.Errorf("engine not loaded")
	}

	// Build prompt from messages.
	prompt := BuildPrompt(messages)

	// Tokenize the prompt.
	tokens := llama.Tokenize(e.vocab, prompt, true, false)
	if len(tokens) == 0 {
		return "", fmt.Errorf("tokenization produced no tokens")
	}

	// Reset sampler state for this request.
	llama.SamplerReset(e.sampler)

	maxTokens := params.MaxTokens
	if maxTokens == 0 {
		maxTokens = 512
	}

	// Process prompt tokens.
	promptStart := time.Now()
	batch := llama.BatchGetOne(tokens)
	if _, err := llama.Decode(e.ctx, batch); err != nil {
		return "", fmt.Errorf("decode prompt: %w", err)
	}
	promptDuration := time.Since(promptStart)

	// Generate tokens.
	// Phase 3: Async GPU/CPU overlap pattern (from Ollama PR #11863).
	// Instead of blocking on each Decode, we let the GPU work asynchronously
	// and only Synchronize when we need to read results (before SamplerSample).
	// This keeps the GPU busy while we do CPU-side work (token convert, stop check, batch prep).
	genStart := time.Now()
	var result strings.Builder
	completionTokens := 0
	hasStopSequences := len(params.Stop) > 0

	// Pre-compute max stop sequence length for tail-only checking.
	maxStopLen := 0
	if hasStopSequences {
		for _, s := range params.Stop {
			if len(s) > maxStopLen {
				maxStopLen = len(s)
			}
		}
	}

	for i := 0; i < maxTokens; i++ {
		// Check context cancellation.
		select {
		case <-ctx.Done():
			return result.String(), ctx.Err()
		default:
		}

		// Synchronize: wait for any pending GPU computation to finish
		// before reading results. On the first iteration this is a no-op
		// since prompt decode completed synchronously above.
		if i > 0 {
			llama.Synchronize(e.ctx)
		}

		// Sample next token (reads GPU results, must be after Synchronize).
		token := llama.SamplerSample(e.sampler, e.ctx, -1)

		// Check for end of generation.
		if llama.VocabIsEOG(e.vocab, token) {
			break
		}

		// Convert token to text using pre-allocated buffer (zero alloc per token).
		n := llama.TokenToPiece(e.vocab, token, e.tokenBuf, 0, true)
		if n > 0 {
			result.Write(e.tokenBuf[:n])
			completionTokens++
		}

		// Phase 3: Only check stop sequences when they exist.
		// When checking, only examine the tail of the output (max stop length)
		// instead of copying the entire buffer every token.
		if hasStopSequences {
			currentText := result.String()
			for _, stop := range params.Stop {
				if strings.HasSuffix(currentText, stop) {
					// Remove the stop sequence from output.
					result.Reset()
					result.WriteString(currentText[:len(currentText)-len(stop)])
					return result.String(), nil
				}
			}
		}

		// Prepare next batch with the sampled token and issue decode.
		// Decode returns immediately on CUDA - GPU works asynchronously
		// while we loop back to do CPU work (cancel check, etc).
		batch = llama.BatchGetOne([]llama.Token{token})
		if _, err := llama.Decode(e.ctx, batch); err != nil {
			return result.String(), fmt.Errorf("decode token %d: %w", i, err)
		}
	}

	genDuration := time.Since(genStart)
	totalDuration := promptDuration + genDuration

	// Calculate tok/s.
	tokPerSec := 0.0
	if genDuration.Seconds() > 0 {
		tokPerSec = float64(completionTokens) / genDuration.Seconds()
	}

	// Record metrics.
	metrics := &InferenceMetrics{
		PromptTokens:     len(tokens),
		CompletionTokens: completionTokens,
		TotalTokens:      len(tokens) + completionTokens,
		PromptDuration:   promptDuration,
		GenDuration:      genDuration,
		TotalDuration:    totalDuration,
		TokensPerSecond:  tokPerSec,
	}
	e.Stats.RecordRequest(metrics)

	// Print metrics to CLI.
	fmt.Printf("%s\n", metrics.String())

	return result.String(), nil
}

// GetStats returns a snapshot of engine performance stats.
func (e *TorchEngine) GetStats() EngineStats {
	return e.Stats.Snapshot()
}

// ModelName returns the name of the loaded model.
func (e *TorchEngine) ModelName() string {
	return e.modelName
}

// Close unloads the model and frees all resources.
func (e *TorchEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.loaded {
		return nil
	}

	llama.Close()
	e.loaded = false
	return nil
}
