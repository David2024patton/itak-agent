package embed

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ModelInfo describes an available embedding model.
type ModelInfo struct {
	Name       string `json:"name"`
	Provider   string `json:"provider"`    // "ollama", "huggingface", "openai"
	Dimensions int    `json:"dimensions"`
	SizeMB     int    `json:"size_mb"`
	Installed  bool   `json:"installed"`
	FilePath   string `json:"file_path,omitempty"` // local GGUF path if installed
	Notes      string `json:"notes,omitempty"`
}

// ModelCatalog is the built-in list of recommended embedding models.
var ModelCatalog = []ModelInfo{
	{
		Name: "nomic-embed-text", Provider: "ollama",
		Dimensions: 768, SizeMB: 274,
		Notes: "Best general-purpose. Outperforms OpenAI ada-002 on MTEB.",
	},
	{
		Name: "mxbai-embed-large", Provider: "ollama",
		Dimensions: 1024, SizeMB: 670,
		Notes: "High quality, larger vectors. Great for code search.",
	},
	{
		Name: "all-minilm", Provider: "ollama",
		Dimensions: 384, SizeMB: 46,
		Notes: "Tiny and fast. Good for resource-constrained systems.",
	},
	{
		Name: "snowflake-arctic-embed", Provider: "ollama",
		Dimensions: 1024, SizeMB: 670,
		Notes: "Snowflake's retrieval model. Strong on technical content.",
	},
	{
		Name: "bge-m3", Provider: "ollama",
		Dimensions: 1024, SizeMB: 1180,
		Notes: "Multilingual. Supports 100+ languages.",
	},
}

// ModelManager handles downloading, listing, and switching embedding models.
type ModelManager struct {
	modelsDir string
	mu        sync.RWMutex
}

// NewModelManager creates a model manager instance.
// modelsDir is the directory to store downloaded models.
func NewModelManager(modelsDir string) *ModelManager {
	if modelsDir == "" {
		home, _ := os.UserHomeDir()
		modelsDir = filepath.Join(home, ".itakagent", "models", "embed")
	}
	os.MkdirAll(modelsDir, 0755)

	return &ModelManager{
		modelsDir: modelsDir,
	}
}

// ListModels returns all models from the catalog with install status.
func (m *ModelManager) ListModels() []ModelInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	models := make([]ModelInfo, len(ModelCatalog))
	copy(models, ModelCatalog)

	for i := range models {
		ggufPath := filepath.Join(m.modelsDir, models[i].Name+".gguf")
		blobPath := filepath.Join(m.modelsDir, models[i].Name)
		if fileExists(ggufPath) {
			models[i].Installed = true
			models[i].FilePath = ggufPath
		} else if fileExists(blobPath) {
			models[i].Installed = true
			models[i].FilePath = blobPath
		}
	}

	// Also scan for custom models on disk that aren't in catalog.
	entries, _ := os.ReadDir(m.modelsDir)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".gguf")
		found := false
		for _, cm := range models {
			if cm.Name == name {
				found = true
				break
			}
		}
		if !found {
			info, _ := entry.Info()
			models = append(models, ModelInfo{
				Name:      name,
				Provider:  "custom",
				Installed: true,
				FilePath:  filepath.Join(m.modelsDir, entry.Name()),
				SizeMB:    int(info.Size() / (1024 * 1024)),
			})
		}
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].Installed != models[j].Installed {
			return models[i].Installed
		}
		return models[i].Name < models[j].Name
	})

	return models
}

// GetModelPath returns the local file path for a model, or empty if not installed.
func (m *ModelManager) GetModelPath(name string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ggufPath := filepath.Join(m.modelsDir, name+".gguf")
	if fileExists(ggufPath) {
		return ggufPath
	}
	blobPath := filepath.Join(m.modelsDir, name)
	if fileExists(blobPath) {
		return blobPath
	}
	return ""
}

// PullModel downloads a model from the Ollama registry.
// Returns a channel that streams progress updates.
func (m *ModelManager) PullModel(name string) (<-chan PullProgress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	ch := make(chan PullProgress, 32)

	go func() {
		defer close(ch)

		ch <- PullProgress{Status: "starting", Progress: 0}

		// Use Ollama's blob API to download the model.
		// First, get the manifest to find the GGUF layer.
		manifest, err := fetchOllamaManifest(name)
		if err != nil {
			ch <- PullProgress{Status: "error", Error: err.Error()}
			return
		}

		// Find the model layer (largest blob, usually the GGUF file).
		var modelDigest string
		var modelSize int64
		for _, layer := range manifest.Layers {
			if layer.Size > modelSize {
				modelSize = layer.Size
				modelDigest = layer.Digest
			}
		}

		if modelDigest == "" {
			ch <- PullProgress{Status: "error", Error: "no model layer found in manifest"}
			return
		}

		ch <- PullProgress{
			Status:   "downloading",
			Progress: 5,
			Total:    modelSize,
		}

		// Download the blob.
		destPath := filepath.Join(m.modelsDir, name+".gguf")
		err = downloadOllamaBlob(name, modelDigest, destPath, func(downloaded, total int64) {
			pct := float64(downloaded) / float64(total) * 95
			ch <- PullProgress{
				Status:     "downloading",
				Progress:   int(pct) + 5,
				Downloaded: downloaded,
				Total:      total,
			}
		})
		if err != nil {
			ch <- PullProgress{Status: "error", Error: err.Error()}
			return
		}

		ch <- PullProgress{Status: "complete", Progress: 100}
		log.Printf("[embed] Model %q downloaded to %s", name, destPath)
	}()

	return ch, nil
}

// PullProgress is a progress update during model download.
type PullProgress struct {
	Status     string `json:"status"`               // "starting", "downloading", "complete", "error"
	Progress   int    `json:"progress"`              // 0-100 percent
	Downloaded int64  `json:"downloaded,omitempty"`
	Total      int64  `json:"total,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ── Ollama Registry Helpers ──────────────────────────────────────

type ollamaManifest struct {
	Layers []ollamaLayer `json:"layers"`
}

type ollamaLayer struct {
	Digest    string `json:"digest"`
	Size      int64  `json:"size"`
	MediaType string `json:"mediaType"`
}

func fetchOllamaManifest(model string) (*ollamaManifest, error) {
	// Resolve model name. "nomic-embed-text" becomes "library/nomic-embed-text".
	parts := strings.SplitN(model, "/", 2)
	var namespace, name string
	if len(parts) == 2 {
		namespace = parts[0]
		name = parts[1]
	} else {
		namespace = "library"
		name = model
	}

	url := fmt.Sprintf("https://registry.ollama.ai/v2/%s/%s/manifests/latest", namespace, name)

	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("manifest error (status %d): %s", resp.StatusCode, string(body))
	}

	var manifest ollamaManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode manifest: %w", err)
	}

	return &manifest, nil
}

func downloadOllamaBlob(model, digest, destPath string, progress func(downloaded, total int64)) error {
	parts := strings.SplitN(model, "/", 2)
	var namespace, name string
	if len(parts) == 2 {
		namespace = parts[0]
		name = parts[1]
	} else {
		namespace = "library"
		name = model
	}

	url := fmt.Sprintf("https://registry.ollama.ai/v2/%s/%s/blobs/%s", namespace, name, digest)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("download blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("blob download error (status %d): %s", resp.StatusCode, string(body))
	}

	total := resp.ContentLength

	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	buf := make([]byte, 64*1024) // 64KB buffer
	var downloaded int64

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, writeErr := f.Write(buf[:n]); writeErr != nil {
				return fmt.Errorf("write file: %w", writeErr)
			}
			downloaded += int64(n)
			if progress != nil {
				progress(downloaded, total)
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return fmt.Errorf("read blob: %w", readErr)
		}
	}

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
