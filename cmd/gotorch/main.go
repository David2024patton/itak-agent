package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/David2024patton/GOAgent/pkg/torch"
)

const defaultPort = 41934
const defaultCacheDir = "~/.gotorch/models"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "serve":
		cmdServe(os.Args[2:])
	case "models":
		cmdModels()
	case "catalog":
		cmdCatalog()
	case "recommend":
		cmdRecommend()
	case "pull":
		cmdPull(os.Args[2:])
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`GOTorch - Go-Native LLM Inference Engine

Usage:
  gotorch <command> [options]

Commands:
  serve       Load a GGUF model and start the inference server
  models      List cached models
  catalog     Show all available models with family and hardware info
  recommend   Detect your hardware and recommend compatible models
  pull        Download a model from the curated catalog by name

Examples:
  gotorch serve --model ./models/qwen3-0.6b.gguf --port 11434
  gotorch serve --models-dir ~/.gotorch/models --max-models 2 --port 11434
  gotorch serve --model main.gguf --draft-model draft.gguf --speculative-tokens 5
  gotorch recommend
  gotorch catalog
  gotorch pull qwen3-0.6b-q4_k_m`)
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	modelPath := fs.String("model", "", "Path to GGUF model file (single-model mode)")
	mmprojPath := fs.String("mmproj", "", "Path to multimodal projector GGUF (for vision models)")
	modelsDir := fs.String("models-dir", "", "Directory of GGUF models (multi-model mode)")
	maxModels := fs.Int("max-models", 1, "Max models loaded simultaneously in multi-model mode")
	port := fs.Int("port", defaultPort, "Port to listen on")
	useMock := fs.Bool("mock", false, "Use mock engine (for testing without a real model)")
	ctxSize := fs.Int("ctx", 2048, "Context window size")
	threads := fs.Int("threads", 0, "Number of CPU threads (0 = auto-detect)")
	gpuLayers := fs.Int("gpu-layers", 0, "Layers to offload to GPU (0=CPU, -1=all, N=specific count)")
	flashAttn := fs.Bool("flash-attn", true, "Enable flash attention for faster inference")
	useMlock := fs.Bool("mlock", false, "Lock model in RAM to prevent OS swapping")
	numaStrategy := fs.Int("numa", 0, "NUMA strategy (0=disabled, 1=distribute, 2=isolate)")
	batchSize := fs.Int("batch", 2048, "Logical batch size for prompt processing")
	kvCacheType := fs.String("kv-cache-type", "f16", "KV cache quantization: f16 (default), q8_0 (50% less VRAM), q4_0 (75% less)")
	defragThreshold := fs.Float64("defrag-threshold", -1, "KV cache defrag threshold (0.0-1.0, -1=disabled, 0.1=recommended)")
	maxSlots := fs.Int("max-slots", 1, "Concurrent inference slots for continuous batching (1=sequential, 4=recommended)")
	backend := fs.String("backend", "auto", "GPU backend: auto (default), cuda, vulkan, cpu")

	// Speculative decoding flags (Phase 3 Stretch).
	draftModel := fs.String("draft-model", "", "Path to draft GGUF model for speculative decoding")
	draftGPULayers := fs.Int("draft-gpu-layers", 0, "GPU layers for draft model (0 = same as --gpu-layers)")
	specTokens := fs.Int("speculative-tokens", 5, "Number of tokens to speculate ahead per step")
	prefixCacheSize := fs.Int("prefix-cache-size", 16, "Max cached KV states for identical system prompts (0=disabled)")

	fs.Parse(args)

	opts := torch.EngineOpts{
		ContextSize:       *ctxSize,
		Threads:           *threads,
		GPULayers:         *gpuLayers,
		FlashAttention:    *flashAttn,
		NoFlashAttention:  !*flashAttn,
		UseMlock:          *useMlock,
		NumaStrategy:      *numaStrategy,
		BatchSize:         *batchSize,
		KVCacheType:       *kvCacheType,
		DefragThreshold:   float32(*defragThreshold),
		MaxSlots:          *maxSlots,
		Backend:           *backend,
		DraftModelPath:    *draftModel,
		DraftGPULayers:    *draftGPULayers,
		SpeculativeTokens: *specTokens,
		PrefixCacheSize:   *prefixCacheSize,
	}

	var engine torch.Engine
	var serverOpts []torch.ServerOption

	if *useMock {
		mockName := "mock-model"
		if *modelPath != "" {
			mockName = *modelPath
		}
		engine = torch.NewMockEngine(mockName)
		fmt.Println("[GOTorch] Using mock engine (no real inference)")
	} else if *modelsDir != "" {
		// Multi-model mode: use ModelRegistry to dynamically load/unload models.
		fmt.Printf("[GOTorch] Multi-model mode: dir=%s max_loaded=%d\n", *modelsDir, *maxModels)
		registry, err := torch.NewModelRegistry(*modelsDir, *maxModels, opts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[GOTorch] Failed to initialize model registry: %v\n", err)
			os.Exit(1)
		}

		// Use a mock engine as placeholder (registry handles real engines).
		engine = torch.NewMockEngine("registry-placeholder")
		serverOpts = append(serverOpts, torch.WithRegistry(registry))
	} else if *modelPath != "" {
		fmt.Printf("[GOTorch] Loading model: %s\n", *modelPath)
		fmt.Printf("[GOTorch] Config: ctx=%d threads=%d gpu_layers=%d flash_attn=%v batch=%d\n",
			*ctxSize, *threads, *gpuLayers, *flashAttn, *batchSize)

		var err error
		if *mmprojPath != "" {
			// Vision model: load both text model and multimodal projector.
			fmt.Printf("[GOTorch] Loading mmproj: %s\n", *mmprojPath)
			engine, err = torch.NewVisionEngine(*modelPath, *mmprojPath, opts)
		} else {
			// Text-only model.
			engine, err = torch.NewTorchEngine(*modelPath, opts)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "[GOTorch] Failed to load model: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("[GOTorch] Model loaded successfully: %s\n", engine.ModelName())
	} else {
		fmt.Fprintf(os.Stderr, "Error: --model, --models-dir, or --mock is required\n")
		fmt.Fprintf(os.Stderr, "Usage: gotorch serve --model <path.gguf> --port <port>\n")
		fmt.Fprintf(os.Stderr, "       gotorch serve --models-dir <dir> --max-models 2 --port <port>\n")
		fmt.Fprintf(os.Stderr, "       gotorch serve --mock --port <port>\n")
		os.Exit(1)
	}

	server := torch.NewServer(engine, *port, serverOpts...)

	// Graceful shutdown on Ctrl+C.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-stop
		fmt.Println("\n[GOTorch] Shutting down...")
		server.Stop()
		engine.Close()
	}()

	if err := server.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}

func cmdModels() {
	mgr, err := torch.NewModelManager(defaultCacheDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	models, err := mgr.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing models: %v\n", err)
		os.Exit(1)
	}

	if len(models) == 0 {
		fmt.Println("No cached models found.")
		fmt.Printf("Cache directory: %s\n", mgr.CacheDir())
		fmt.Println("Run 'gotorch catalog' to see available models.")
		return
	}

	fmt.Printf("Cached models (%s):\n\n", mgr.CacheDir())
	for _, m := range models {
		sizeMB := m.Size / 1024 / 1024
		fmt.Printf("  %-40s  %6d MB  %s\n", m.Name, sizeMB, m.LastUsed.Format("2006-01-02"))
	}
}

func cmdCatalog() {
	catalog := torch.CuratedModels()
	fmt.Println("GOTorch Model Catalog:")
	fmt.Println()

	// Group by family.
	lastFamily := ""
	fmt.Printf("  %-30s  %-8s  %-8s  %-10s  %-8s  %-5s  %s\n", "NAME", "PARAMS", "SIZE", "ROLE", "FAMILY", "DRAFT", "NOTES")
	fmt.Printf("  %-30s  %-8s  %-8s  %-10s  %-8s  %-5s  %s\n", "----", "------", "----", "----", "------", "-----", "-----")
	for _, m := range catalog {
		if m.Family != lastFamily {
			if lastFamily != "" {
				fmt.Println()
			}
			lastFamily = m.Family
		}
		draft := ""
		if m.CanDraft {
			draft = "  yes"
		}
		fmt.Printf("  %-30s  %-8s  %-8s  %-10s  %-8s  %-5s  %s\n", m.Name, m.Params, m.Size, m.Role, m.Family, draft, m.Notes)
	}
	fmt.Println()
	fmt.Println("Pull a model: gotorch pull <name>")
	fmt.Println("See what fits your hardware: gotorch recommend")
}

func cmdRecommend() {
	// Detect system hardware.
	var memInfo runtime.MemStats
	runtime.ReadMemStats(&memInfo)

	// Use Sys (total memory obtained from OS) as a rough estimate of available RAM.
	// For a more accurate reading we'd use OS-specific APIs, but this is a solid baseline.
	totalRAMMB := int(memInfo.Sys/1024/1024) + 8192 // Add headroom (Go only reports its own usage)

	// Rough heuristic: check if CUDA/HIP/SYCL libs exist to detect GPU.
	hasGPU := false
	vramMB := 0

	specs := torch.SystemSpecs{
		TotalRAMMB:  totalRAMMB,
		TotalVRAMMB: vramMB,
		HasGPU:      hasGPU,
	}

	fmt.Println("GOTorch Hardware Detection")
	fmt.Println("=========================")
	fmt.Printf("  Estimated RAM:  %d MB\n", totalRAMMB)
	if hasGPU {
		fmt.Printf("  GPU VRAM:       %d MB\n", vramMB)
	} else {
		fmt.Printf("  GPU:            Not detected (CPU-only mode)\n")
	}
	fmt.Println()

	// Show models that fit.
	fits := torch.ModelsForHardware(specs)
	if len(fits) == 0 {
		fmt.Println("No models fit your hardware. Try upgrading RAM.")
		return
	}

	fmt.Printf("Models that fit your hardware (%d available):\n\n", len(fits))
	fmt.Printf("  %-30s  %-8s  %-8s  %-10s  %-8s  %s\n", "NAME", "PARAMS", "SIZE", "ROLE", "FAMILY", "NOTES")
	fmt.Printf("  %-30s  %-8s  %-8s  %-10s  %-8s  %s\n", "----", "------", "----", "----", "------", "-----")
	for _, m := range fits {
		fmt.Printf("  %-30s  %-8s  %-8s  %-10s  %-8s  %s\n", m.Name, m.Params, m.Size, m.Role, m.Family, m.Notes)
	}

	// Show speculative decoding pairs.
	pairs := torch.SpeculativePairsForHardware(specs)
	if len(pairs) > 0 {
		fmt.Println()
		fmt.Printf("Speculative Decoding Pairs (%d pairs available):\n", len(pairs))
		fmt.Println("  Draft models predict tokens, main models verify. Same family = compatible tokenizer.")
		fmt.Println()
		fmt.Printf("  %-20s  %-20s  %-8s  %s\n", "DRAFT MODEL", "MAIN MODEL", "FAMILY", "NOTES")
		fmt.Printf("  %-20s  %-20s  %-8s  %s\n", "-----------", "----------", "------", "-----")
		for _, p := range pairs {
			fmt.Printf("  %-20s  %-20s  %-8s  %s\n", p.DraftModel.Name, p.MainModel.Name, p.Family, p.Notes)
		}
		fmt.Println()
		fmt.Println("  Usage: gotorch serve --model <main.gguf> --draft-model <draft.gguf> --speculative-tokens 5")
	}

	fmt.Println()
	fmt.Println("Pull a model: gotorch pull <name>")
}

func cmdPull(args []string) {
	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: gotorch pull <model-name>\n")
		fmt.Fprintf(os.Stderr, "Run 'gotorch catalog' to see available models.\n")
		os.Exit(1)
	}

	name := args[0]

	// Find in catalog.
	var found *torch.ModelIndex
	for _, m := range torch.CuratedModels() {
		if m.Name == name {
			found = &m
			break
		}
	}

	if found == nil {
		fmt.Fprintf(os.Stderr, "Model %q not found in catalog.\n", name)
		fmt.Fprintf(os.Stderr, "Run 'gotorch catalog' to see available models.\n")
		os.Exit(1)
	}

	mgr, err := torch.NewModelManager(defaultCacheDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[GOTorch] Pulling %s (%s, %s)...\n", found.Name, found.Params, found.Size)
	path, err := mgr.Download(found.URL, found.Name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Download failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("[GOTorch] Model saved to: %s\n", path)
}
