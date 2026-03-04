package torch

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/David2024patton/GOAgent/pkg/torch/llama"
)

// LlamaEngine implements the Engine interface using the forked yzma/llama.cpp
// bindings via purego/ffi. No CGo required.
type LlamaEngine struct {
	model     llama.Model
	ctx       llama.Context
	vocab     llama.Vocab
	sampler   llama.Sampler
	modelName string
	modelPath string
	opts      EngineOpts
	mu        sync.Mutex
	loaded    bool
}

// NewLlamaEngine creates an engine that loads a GGUF model via llama.cpp.
// libPath is the directory containing the llama.cpp shared libraries.
// If empty, checks the GOTORCH_LIB environment variable.
func NewLlamaEngine(modelPath string, opts EngineOpts) (*LlamaEngine, error) {
	// Find the llama.cpp shared libraries.
	libPath := os.Getenv("GOTORCH_LIB")
	if libPath == "" {
		// Check common locations.
		candidates := []string{
			"./lib",
			"./libs",
			"~/.gotorch/lib",
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

	if libPath == "" {
		return nil, fmt.Errorf("llama.cpp libraries not found. Set GOTORCH_LIB env variable or run 'gotorch install'")
	}

	// Load the shared libraries.
	if err := llama.Load(libPath); err != nil {
		return nil, fmt.Errorf("load llama.cpp libraries from %s: %w", libPath, err)
	}

	// Initialize the backend.
	llama.Init()

	// Apply defaults.
	if opts.ContextSize == 0 {
		opts.ContextSize = 2048
	}
	if opts.Threads == 0 {
		opts.Threads = 4
	}

	// Load the model.
	modelParams := llama.ModelDefaultParams()
	modelParams.NGpuLayers = int32(opts.GPULayers)

	model, err := llama.ModelLoadFromFile(modelPath, modelParams)
	if err != nil {
		return nil, fmt.Errorf("load model %s: %w", modelPath, err)
	}

	// Create context.
	ctxParams := llama.ContextDefaultParams()
	ctxParams.NCtx = uint32(opts.ContextSize)
	ctxParams.NThreads = int32(opts.Threads)
	ctxParams.NThreadsBatch = int32(opts.Threads)

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

	return &LlamaEngine{
		model:     model,
		ctx:       ctx,
		vocab:     vocab,
		sampler:   sampler,
		modelName: name,
		modelPath: modelPath,
		opts:      opts,
		loaded:    true,
	}, nil
}

// Complete runs inference on the given messages and returns the generated text.
func (e *LlamaEngine) Complete(ctx context.Context, messages []ChatMessage, params CompletionParams) (string, error) {
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
	batch := llama.BatchGetOne(tokens)
	if _, err := llama.Decode(e.ctx, batch); err != nil {
		return "", fmt.Errorf("decode prompt: %w", err)
	}

	// Generate tokens.
	var result strings.Builder
	for i := 0; i < maxTokens; i++ {
		// Check context cancellation.
		select {
		case <-ctx.Done():
			return result.String(), ctx.Err()
		default:
		}

		// Sample next token.
		token := llama.SamplerSample(e.sampler, e.ctx, -1)

		// Check for end of generation.
		if llama.VocabIsEOG(e.vocab, token) {
			break
		}

		// Convert token to text.
		buf := make([]byte, 128)
		n := llama.TokenToPiece(e.vocab, token, buf, 0, true)
		if n > 0 {
			result.Write(buf[:n])
		}

		// Check stop sequences.
		currentText := result.String()
		for _, stop := range params.Stop {
			if strings.HasSuffix(currentText, stop) {
				// Remove the stop sequence from output.
				result.Reset()
				result.WriteString(currentText[:len(currentText)-len(stop)])
				return result.String(), nil
			}
		}

		// Prepare next batch with the sampled token.
		batch = llama.BatchGetOne([]llama.Token{token})
		if _, err := llama.Decode(e.ctx, batch); err != nil {
			return result.String(), fmt.Errorf("decode token %d: %w", i, err)
		}
	}

	return result.String(), nil
}

// ModelName returns the name of the loaded model.
func (e *LlamaEngine) ModelName() string {
	return e.modelName
}

// Close unloads the model and frees all resources.
func (e *LlamaEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.loaded {
		return nil
	}

	llama.Close()
	e.loaded = false
	return nil
}
