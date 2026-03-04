package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/David2024patton/GOAgent/pkg/torch"
)

const defaultPort = 11434
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
  serve     Load a GGUF model and start the inference server
  models    List cached models
  catalog   Show available models from the curated list
  pull      Download a model from the curated catalog by name

Examples:
  gotorch serve --model ./models/qwen3-0.6b.gguf --port 11434
  gotorch serve --mock --port 11434
  gotorch models
  gotorch catalog
  gotorch pull qwen3-0.6b-q4_k_m`)
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	modelPath := fs.String("model", "", "Path to GGUF model file")
	port := fs.Int("port", defaultPort, "Port to listen on")
	useMock := fs.Bool("mock", false, "Use mock engine (for testing without a real model)")
	fs.Parse(args)

	var engine torch.Engine

	if *useMock {
		mockName := "mock-model"
		if *modelPath != "" {
			mockName = *modelPath
		}
		engine = torch.NewMockEngine(mockName)
		fmt.Println("[GOTorch] Using mock engine (no real inference)")
	} else if *modelPath != "" {
		// TODO: Replace with real llama.cpp engine when CGo is available.
		fmt.Fprintf(os.Stderr, "[GOTorch] Real llama.cpp engine not yet implemented.\n")
		fmt.Fprintf(os.Stderr, "[GOTorch] Use --mock flag for testing, or wait for CGo backend.\n")
		os.Exit(1)
	} else {
		fmt.Fprintf(os.Stderr, "Error: --model or --mock is required\n")
		fmt.Fprintf(os.Stderr, "Usage: gotorch serve --model <path.gguf> --port <port>\n")
		fmt.Fprintf(os.Stderr, "       gotorch serve --mock --port <port>\n")
		os.Exit(1)
	}

	server := torch.NewServer(engine, *port)

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
	fmt.Printf("  %-30s  %-8s  %-8s  %-10s  %s\n", "NAME", "PARAMS", "SIZE", "ROLE", "NOTES")
	fmt.Printf("  %-30s  %-8s  %-8s  %-10s  %s\n", "----", "------", "----", "----", "-----")
	for _, m := range catalog {
		fmt.Printf("  %-30s  %-8s  %-8s  %-10s  %s\n", m.Name, m.Params, m.Size, m.Role, m.Notes)
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
