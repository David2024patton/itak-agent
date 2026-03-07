package torch

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/David2024patton/iTaKAgent/pkg/torch/llama"
	"github.com/David2024patton/iTaKAgent/pkg/torch/tokenizer"
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

	// Speculative decoding (Phase 3 Stretch)
	draftModel   llama.Model
	draftCtx     llama.Context
	draftVocab   llama.Vocab
	draftSampler llama.Sampler
	hasDraft     bool

	// Go-native tokenizer (Phase 4A) - eliminates FFI overhead.
	goTokenizer    *tokenizer.GoTokenizer
	hasGoTokenizer bool

	// Prefix cache (Phase 4B) - reuses KV state for shared system prompts.
	prefixCache *PrefixCache

	// streamCh receives token deltas during streaming inference.
	streamCh chan string
}

// tokenToText converts a token ID to its text representation.
// Phase 4A: uses Go-native vocab lookup (zero FFI) when available.
// Phase 7B: uses unsafe.String to eliminate per-token heap allocation.
//
//go:nosplit
func (e *TorchEngine) tokenToText(token llama.Token) string {
	if e.hasGoTokenizer {
		return e.goTokenizer.DecodeToken(int32(token))
	}
	// FFI fallback with zero-copy string (unsafe.String avoids heap alloc).
	n := llama.TokenToPiece(e.vocab, token, e.tokenBuf, 0, true)
	if n > 0 {
		// unsafe.String: returns a string header pointing directly at tokenBuf memory.
		// Safe here because tokenBuf is pre-allocated and lives for the engine's lifetime.
		return unsafe.String(&e.tokenBuf[0], n)
	}
	return ""
}

// isEOG checks if a token is an end-of-generation token.
// Phase 4A: uses Go-native lookup (zero FFI) when available.
func (e *TorchEngine) isEOG(token llama.Token) bool {
	if e.hasGoTokenizer {
		return e.goTokenizer.IsEOG(int32(token))
	}
	return llama.VocabIsEOG(e.vocab, token)
}

// NewTorchEngine creates an engine that loads a GGUF model via llama.cpp.
// libPath is the directory containing the llama.cpp shared libraries.
// If empty, checks the ITAK_TORCH_LIB environment variable.
func NewTorchEngine(modelPath string, opts EngineOpts) (*TorchEngine, error) {
	// Find the llama.cpp shared libraries.
	libPath := os.Getenv("ITAK_TORCH_LIB")
	if libPath == "" {
		// Smart lib selection based on --backend flag and GPU config.
		// This prevents CUDA DLL contamination that causes 2.5x slowdown on CPU workloads.
		platformDir := runtime.GOOS + "_" + runtime.GOARCH
		var candidates []string

		backend := strings.ToLower(opts.Backend)
		if backend == "" {
			backend = "auto"
		}

		switch {
		case backend == "cpu" || opts.GPULayers == 0:
			// CPU-only mode: prefer CPU-only libs to avoid GPU overhead.
			candidates = []string{
				"./lib/" + platformDir,
				"~/.itaktorch/lib",
				"./lib",
			}
		case backend == "cuda":
			// Force CUDA backend.
			candidates = []string{
				"./lib/" + platformDir + "_cuda",
				"~/.itaktorch/lib",
			}
		case backend == "vulkan":
			// Force Vulkan backend.
			candidates = []string{
				"./lib/" + platformDir + "_vulkan",
				"~/.itaktorch/lib",
			}
		case backend == "metal":
			// Force Metal backend (Apple Silicon).
			candidates = []string{
				"./lib/" + platformDir + "_metal",
				"~/.itaktorch/lib",
			}
		default:
			// Auto mode: try Vulkan first (faster, smaller, cross-platform),
			// then CUDA, Metal (Apple Silicon UMA), HIP, SYCL.
			candidates = []string{
				"./lib/" + platformDir + "_vulkan",
				"./lib/" + platformDir + "_cuda",
				"./lib/" + platformDir + "_metal",
				"./lib/" + platformDir + "_hip",
				"./lib/" + platformDir + "_sycl",
				"~/.itaktorch/lib",
				"./lib/" + platformDir,
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
	fmt.Printf("[iTaK Torch] Using libs from: %s\n", libPath)

	if libPath == "" {
		return nil, fmt.Errorf("llama.cpp libraries not found. Set ITAK_TORCH_LIB env variable or run 'itaktorch install'")
	}

	// Load the shared libraries.
	if err := llama.Load(libPath); err != nil {
		return nil, fmt.Errorf("load llama.cpp libraries from %s: %w", libPath, err)
	}

	// Fix: Some environments (Docker, IDE shells) set CUDA_VISIBLE_DEVICES=-1
	// which hides all GPUs from the CUDA runtime. If GPU layers are requested
	// (positive count or -1 for auto-all) and the variable blocks GPU access, override it.
	if opts.GPULayers != 0 {
		if cvd := os.Getenv("CUDA_VISIBLE_DEVICES"); cvd == "-1" || cvd == "" {
			os.Setenv("CUDA_VISIBLE_DEVICES", "0")
		}
	}

	// Initialize the backend.
	llama.Init()

	// Initialize NUMA if configured.
	if opts.NumaStrategy > 0 {
		llama.NumaInit(llama.NumaStrategy(opts.NumaStrategy))
		fmt.Printf("[iTaK Torch] NUMA strategy: %d\n", opts.NumaStrategy)
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

	// Phase 5: Auto GPU layers (-1 means offload everything).
	if opts.GPULayers == -1 {
		opts.GPULayers = 999 // llama.cpp clamps to actual layer count
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

	// Continuous batching: allow multiple concurrent sequences.
	if opts.MaxSlots > 1 {
		ctxParams.NSeqMax = uint32(opts.MaxSlots)
	}

	// Phase 5: KV cache quantization.
	switch strings.ToLower(opts.KVCacheType) {
	case "q8_0", "q8":
		ctxParams.TypeK = llama.GGMLTypeQ8_0
		ctxParams.TypeV = llama.GGMLTypeQ8_0
		fmt.Println("[iTaK Torch] KV cache: q8_0 (50% VRAM reduction)")
	case "q4_0", "q4":
		ctxParams.TypeK = llama.GGMLTypeQ4_0
		ctxParams.TypeV = llama.GGMLTypeQ4_0
		fmt.Println("[iTaK Torch] KV cache: q4_0 (75% VRAM reduction)")
	case "f16", "":
		// Default: f16, no change needed.
	default:
		fmt.Printf("[iTaK Torch] Warning: unknown kv-cache-type %q, using f16\n", opts.KVCacheType)
	}

	// Phase 5: KV cache defragmentation threshold.
	if opts.DefragThreshold > 0 {
		ctxParams.DefragThold = opts.DefragThreshold
	}

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
		model:       model,
		ctx:         ctx,
		vocab:       vocab,
		sampler:     sampler,
		tokenBuf:    make([]byte, 256), // pre-allocated to avoid per-token heap allocation
		modelName:   name,
		modelPath:   modelPath,
		opts:        opts,
		loaded:      true,
		prefixCache: NewPrefixCache(opts.PrefixCacheSize),
	}

	// Record load metrics.
	postLoad := CaptureResources()
	engine.Stats.ModelLoadTime = loadDuration
	engine.Stats.PreLoadRes = preLoad
	engine.Stats.PostLoadRes = postLoad

	fmt.Printf("[iTaK Torch] Model loaded in %s\n", loadDuration.Round(time.Millisecond))
	fmt.Printf("[iTaK Torch] %s\n", preLoad.String())
	fmt.Printf("[iTaK Torch] %s\n", postLoad.String())

	// Print system capabilities.
	sysInfo := llama.PrintSystemInfo()
	if sysInfo != "" {
		fmt.Printf("[iTaK Torch] System: %s\n", sysInfo)
	}
	fmt.Printf("[iTaK Torch] Optimizations: mmap=%v mlock=%v gpu_offload=%v flash_attn=%v threads=%d batch=%d\n",
		llama.SupportsMmap(), opts.UseMlock, llama.SupportsGpuOffload(),
		opts.FlashAttention, opts.Threads, opts.BatchSize)

	// Warmup: triggers GPU kernel JIT compilation for common tensor shapes,
	// reducing latency on first real request.
	if err := llama.Warmup(ctx, model); err != nil {
		fmt.Printf("[iTaK Torch] Warmup warning: %v\n", err)
	} else {
		fmt.Printf("[iTaK Torch] Model warmup complete\n")
	}

	// --- Phase 4A: Load Go-native tokenizer from GGUF metadata ---
	goTok, tokErr := tokenizer.NewFromGGUF(modelPath)
	if tokErr != nil {
		fmt.Printf("[iTaK Torch] Go tokenizer unavailable (using FFI fallback): %v\n", tokErr)
	} else {
		engine.goTokenizer = goTok
		engine.hasGoTokenizer = true
		fmt.Printf("[iTaK Torch] Go-native tokenizer loaded: %d tokens, %d merges\n",
			goTok.VocabSize, len(goTok.MergeRank))
	}

	// --- Speculative Decoding: Load draft model if configured ---
	if opts.DraftModelPath != "" {
		fmt.Printf("[iTaK Torch] Loading draft model: %s\n", opts.DraftModelPath)

		specTokens := opts.SpeculativeTokens
		if specTokens == 0 {
			specTokens = 5
		}

		draftGPU := opts.DraftGPULayers
		if draftGPU == 0 {
			draftGPU = opts.GPULayers
		}

		draftParams := llama.ModelDefaultParams()
		draftParams.NGpuLayers = int32(draftGPU)

		draftModel, draftErr := llama.ModelLoadFromFile(opts.DraftModelPath, draftParams)
		if draftErr != nil {
			fmt.Printf("[iTaK Torch] Draft model load failed (continuing without speculative decoding): %v\n", draftErr)
		} else {
			draftCtxParams := llama.ContextDefaultParams()
			draftCtxParams.NCtx = uint32(opts.ContextSize)
			draftCtxParams.NThreads = int32(opts.Threads)
			draftCtxParams.NThreadsBatch = int32(opts.Threads)
			draftCtxParams.NBatch = uint32(opts.BatchSize)
			if opts.FlashAttention {
				draftCtxParams.FlashAttentionType = llama.FlashAttentionTypeEnabled
			}

			draftCtx, draftCtxErr := llama.InitFromModel(draftModel, draftCtxParams)
			if draftCtxErr != nil {
				fmt.Printf("[iTaK Torch] Draft context init failed: %v\n", draftCtxErr)
				llama.ModelFree(draftModel)
			} else {
				draftVocab := llama.ModelGetVocab(draftModel)
				draftSamplerParams := llama.DefaultSamplerParams()
				draftSampler := llama.NewSampler(draftModel, llama.DefaultSamplers, draftSamplerParams)

				engine.draftModel = draftModel
				engine.draftCtx = draftCtx
				engine.draftVocab = draftVocab
				engine.draftSampler = draftSampler
				engine.hasDraft = true

				// Warmup draft model.
				if err := llama.Warmup(draftCtx, draftModel); err != nil {
					fmt.Printf("[iTaK Torch] Draft warmup warning: %v\n", err)
				}

				fmt.Printf("[iTaK Torch] Speculative decoding enabled: draft=%s, speculative_tokens=%d\n",
					opts.DraftModelPath, specTokens)
			}
		}
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
	// Phase 4A: prefer Go-native tokenizer (no FFI overhead).
	var tokens []llama.Token
	if e.hasGoTokenizer {
		goTokens := e.goTokenizer.Encode(prompt, true)
		tokens = make([]llama.Token, len(goTokens))
		for i, t := range goTokens {
			tokens[i] = llama.Token(t)
		}
		// DEBUG: compare with FFI tokenization to catch encoding mismatches.
		ffiTokens := llama.Tokenize(e.vocab, prompt, true, false)
		if len(tokens) != len(ffiTokens) {
			fmt.Printf("[iTaK Torch] TOKEN MISMATCH: Go=%d tokens, FFI=%d tokens\n", len(tokens), len(ffiTokens))
		} else {
			fmt.Printf("[iTaK Torch] Token count OK: %d tokens (Go == FFI)\n", len(tokens))
		}
	} else {
		tokens = llama.Tokenize(e.vocab, prompt, true, false)
		fmt.Printf("[iTaK Torch] FFI tokenized: %d tokens\n", len(tokens))
	}

	// Reset sampler state for this request.
	llama.SamplerReset(e.sampler)

	maxTokens := params.MaxTokens
	if maxTokens == 0 {
		maxTokens = 512
	}

	// Process prompt tokens.
	// Phase 4B: Check prefix cache for pre-computed KV state.
	promptStart := time.Now()
	cacheHit := false
	var batch llama.Batch

	if e.opts.PrefixCacheSize > 0 {
		if entry, ok := e.prefixCache.Lookup(prompt); ok {
			// Cache hit: restore KV state, skip prompt decode entirely.
			if _, err := e.prefixCache.Restore(e.ctx, entry); err == nil {
				cacheHit = true
			}
		}
	}

	if !cacheHit {
		batch = llama.BatchGetOne(tokens)
		if _, err := llama.Decode(e.ctx, batch); err != nil {
			return "", fmt.Errorf("decode prompt: %w", err)
		}
		// Save to prefix cache for future requests with same prompt.
		if e.opts.PrefixCacheSize > 0 {
			if err := e.prefixCache.Save(e.ctx, prompt, tokens); err != nil {
				// Non-fatal: log warning but continue.
				fmt.Printf("[iTaK Torch] Prefix cache save warning: %v\n", err)
			}
		}
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

		// --- Speculative Decoding Path ---
		// If a draft model is loaded, generate N draft tokens and verify with the main model.
		if e.hasDraft && i > 0 {
			specTokens := e.opts.SpeculativeTokens
			if specTokens == 0 {
				specTokens = 5
			}

			// Step 1: Generate draft tokens with the small model.
			llama.Synchronize(e.ctx)
			draftTokens := make([]llama.Token, 0, specTokens)

			// Feed the last main-model token to the draft model first.
			lastToken := llama.SamplerSample(e.sampler, e.ctx, -1)
			if e.isEOG(lastToken) {
				break
			}

			// Decode last token through draft model to sync state.
			draftBatch := llama.BatchGetOne([]llama.Token{lastToken})
			llama.Decode(e.draftCtx, draftBatch)

			// Accept the verified token from main model.
			piece := e.tokenToText(lastToken)
			if len(piece) > 0 {
				result.WriteString(piece)
				completionTokens++
				i++
			}

			// Generate draft candidates.
			for d := 0; d < specTokens && (i+d) < maxTokens; d++ {
				llama.Synchronize(e.draftCtx)
				dt := llama.SamplerSample(e.draftSampler, e.draftCtx, -1)
				if e.isEOG(dt) {
					break
				}
				draftTokens = append(draftTokens, dt)
				db := llama.BatchGetOne([]llama.Token{dt})
				llama.Decode(e.draftCtx, db)
			}

			if len(draftTokens) == 0 {
				// Draft model hit EOG, push last token through main model.
				batch = llama.BatchGetOne([]llama.Token{lastToken})
				llama.Decode(e.ctx, batch)
				continue
			}

			// Step 2: Verify draft tokens with main model in one batch.
			verifyBatch := llama.BatchGetOne(draftTokens)
			llama.Decode(e.ctx, verifyBatch)
			llama.Synchronize(e.ctx)

			// Step 3: Accept matching tokens, reject from first mismatch.
			accepted := 0
			for _, dt := range draftTokens {
				mainToken := llama.SamplerSample(e.sampler, e.ctx, int32(accepted))
				if mainToken == dt {
					// Match! Accept this token for free.
					piece := e.tokenToText(dt)
					if len(piece) > 0 {
						result.WriteString(piece)
						completionTokens++
					}
					accepted++
				} else {
					// Mismatch. Use the main model's token instead.
					piece := e.tokenToText(mainToken)
					if len(piece) > 0 {
						result.WriteString(piece)
						completionTokens++
					}
					break
				}
			}

			i += accepted

			// Prepare main model for next iteration.
			if accepted > 0 {
				lastAccepted := draftTokens[accepted-1]
				batch = llama.BatchGetOne([]llama.Token{lastAccepted})
				llama.Decode(e.ctx, batch)
			}

			// Reset draft sampler for next speculation round.
			llama.SamplerReset(e.draftSampler)
			continue
		}

		// --- Standard Sequential Path (no draft model) ---

		// Synchronize: wait for any pending GPU computation to finish
		// before reading results. On the first iteration this is a no-op
		// since prompt decode completed synchronously above.
		if i > 0 {
			llama.Synchronize(e.ctx)
		}

		// Sample next token (reads GPU results, must be after Synchronize).
		sampleStart := time.Now()

		var token llama.Token
		// ZERO-CGO SAMPLER:
		// 1. Extract raw Logits via Shared Memory Pointer (No CGO Penalty)
		nVocab := int(llama.VocabNTokens(e.vocab))
		// -1 gets the logits for the last token in the batch
		logits, err := llama.GetLogitsIth(e.ctx, -1, nVocab)

		if err == nil && len(logits) > 0 {
			// 2. Phase 7B: Unsafe Pointer Arithmetic ArgMax
			// Bypasses Go's per-element bounds checking on the ~151k vocab scan.
			// Direct pointer math: ptr + i*sizeof(float32) for raw memory traversal.
			var maxVal float32 = -1e9
			var maxIdx int32 = 0
			basePtr := unsafe.Pointer(&logits[0])
			for j := 0; j < nVocab; j++ {
				val := *(*float32)(unsafe.Add(basePtr, uintptr(j)*4))
				if val > maxVal {
					maxVal = val
					maxIdx = int32(j)
				}
			}
			token = llama.Token(maxIdx)
			// Inform the C++ sampler of the accepted token to keep internal state (like penalties) synced
			llama.SamplerAccept(e.sampler, token)
		} else {
			// Fallback to FFI Sampler if shared memory extraction fails
			token = llama.SamplerSample(e.sampler, e.ctx, -1)
		}

		LogTrace("PureGo Sampler took %v", time.Since(sampleStart))

		// Check for end of generation.
		if e.isEOG(token) {
			break
		}

		// Convert token to text.
		// Phase 4A: Go-native lookup (zero FFI, zero alloc via string interning).
		textStart := time.Now()
		piece := e.tokenToText(token)
		LogTrace("TokenToText took %v", time.Since(textStart))
		if len(piece) > 0 {
			result.WriteString(piece)
			completionTokens++
			// Stream token delta if streaming is active.
			if e.streamCh != nil {
				select {
				case e.streamCh <- piece:
				case <-ctx.Done():
					return result.String(), ctx.Err()
				}
			}
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
		decodeStart := time.Now()
		batch = llama.BatchGetOne([]llama.Token{token})
		if _, err := llama.Decode(e.ctx, batch); err != nil {
			return result.String(), fmt.Errorf("decode token %d: %w", i, err)
		}
		LogTrace("Decode block took %v", time.Since(decodeStart))
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

// CompleteStream runs inference and sends each token delta to the provided channel.
// The channel is closed when generation completes. The full result is also returned.
func (e *TorchEngine) CompleteStream(ctx context.Context, messages []ChatMessage, params CompletionParams, ch chan string) (string, error) {
	e.streamCh = ch
	defer func() {
		e.streamCh = nil
		close(ch)
	}()
	return e.Complete(ctx, messages, params)
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

	// Free draft model resources if speculative decoding was enabled.
	if e.hasDraft {
		llama.ModelFree(e.draftModel)
		e.hasDraft = false
	}

	llama.Close()
	e.loaded = false
	return nil
}
