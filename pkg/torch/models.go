package torch

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ModelManager handles downloading, caching, and listing GGUF model files.
type ModelManager struct {
	cacheDir   string
	httpClient *http.Client
}

// NewModelManager creates a model manager with the given cache directory.
// Creates the directory if it doesn't exist.
func NewModelManager(cacheDir string) (*ModelManager, error) {
	// Expand ~ to home directory.
	if strings.HasPrefix(cacheDir, "~") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("get home dir: %w", err)
		}
		cacheDir = filepath.Join(home, cacheDir[1:])
	}

	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("create cache dir %s: %w", cacheDir, err)
	}

	return &ModelManager{
		cacheDir: cacheDir,
		httpClient: &http.Client{
			Timeout: 0, // No timeout for large downloads.
		},
	}, nil
}

// CacheDir returns the absolute path to the model cache directory.
func (m *ModelManager) CacheDir() string {
	return m.cacheDir
}

// Download fetches a GGUF file from a URL and saves it to the cache.
// Returns the local file path. If the file already exists, skips download.
func (m *ModelManager) Download(url string, name string) (string, error) {
	if !strings.HasSuffix(name, ".gguf") {
		name = name + ".gguf"
	}

	destPath := filepath.Join(m.cacheDir, name)

	// Skip if already downloaded.
	if info, err := os.Stat(destPath); err == nil && info.Size() > 0 {
		return destPath, nil
	}

	resp, err := m.httpClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}

	// Write to temp file first, then rename (atomic on most filesystems).
	tmpPath := destPath + ".downloading"
	out, err := os.Create(tmpPath)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	written, err := io.Copy(out, resp.Body)
	out.Close()
	if err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("write model data: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("rename temp file: %w", err)
	}

	fmt.Printf("[GOTorch] Downloaded %s (%d MB)\n", name, written/1024/1024)
	return destPath, nil
}

// List returns all cached models sorted by name.
func (m *ModelManager) List() ([]ModelEntry, error) {
	entries, err := os.ReadDir(m.cacheDir)
	if err != nil {
		return nil, fmt.Errorf("read cache dir: %w", err)
	}

	var models []ModelEntry
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".gguf") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		models = append(models, ModelEntry{
			Name:     strings.TrimSuffix(entry.Name(), ".gguf"),
			Path:     filepath.Join(m.cacheDir, entry.Name()),
			Size:     info.Size(),
			LastUsed: info.ModTime(),
		})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})

	return models, nil
}

// GetPath returns the full path to a cached model by name.
func (m *ModelManager) GetPath(name string) (string, error) {
	if !strings.HasSuffix(name, ".gguf") {
		name = name + ".gguf"
	}

	path := filepath.Join(m.cacheDir, name)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("model %q not found in cache", name)
	}
	return path, nil
}

// Remove deletes a cached model by name.
func (m *ModelManager) Remove(name string) error {
	if !strings.HasSuffix(name, ".gguf") {
		name = name + ".gguf"
	}

	path := filepath.Join(m.cacheDir, name)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove model %q: %w", name, err)
	}
	return nil
}

// ModelIndex is a catalog entry from our curated model list.
type ModelIndex struct {
	Name   string `json:"name"`
	URL    string `json:"url"`
	Size   string `json:"size"`
	Params string `json:"params"`
	Role   string `json:"role"` // "chat", "code", "embed", "vision", "reasoning"
	Notes  string `json:"notes"`
}

// CuratedModels returns the built-in list of recommended small models.
func CuratedModels() []ModelIndex {
	return []ModelIndex{
		{
			Name:   "qwen3-0.6b-q4_k_m",
			URL:    "https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q4_K_M.gguf",
			Size:   "~400MB",
			Params: "0.6B",
			Role:   "chat",
			Notes:  "Smallest Qwen3. Fast router/classifier. Good for testing.",
		},
		{
			Name:   "qwen3-1.7b-q4_k_m",
			URL:    "https://huggingface.co/unsloth/Qwen3-1.7B-GGUF/resolve/main/Qwen3-1.7B-Q4_K_M.gguf",
			Size:   "~1.2GB",
			Params: "1.7B",
			Role:   "chat",
			Notes:  "Solid balance of smart + small. Good tool calling.",
		},
		{
			Name:   "qwen3-4b-q4_k_m",
			URL:    "https://huggingface.co/unsloth/Qwen3-4B-GGUF/resolve/main/Qwen3-4B-Q4_K_M.gguf",
			Size:   "~2.5GB",
			Params: "4B",
			Role:   "chat",
			Notes:  "Best brain under 3GB. Recommended standard tier.",
		},
		{
			Name:   "qwen2.5-coder-3b-q4_k_m",
			URL:    "https://huggingface.co/bartowski/Qwen2.5-Coder-3B-Instruct-GGUF/resolve/main/Qwen2.5-Coder-3B-Instruct-Q4_K_M.gguf",
			Size:   "~2GB",
			Params: "3B",
			Role:   "code",
			Notes:  "Purpose-built for code generation.",
		},
		{
			Name:   "nomic-embed-text-v2-moe",
			URL:    "https://huggingface.co/nomic-ai/nomic-embed-text-v2-moe-GGUF/resolve/main/nomic-embed-text-v2-moe.Q8_0.gguf",
			Size:   "~275MB",
			Params: "MoE",
			Role:   "embed",
			Notes:  "MoE embedding. Smallest footprint, great quality.",
		},
	}
}

// SaveCatalog writes the curated model list to a JSON file in the cache dir.
func (m *ModelManager) SaveCatalog() error {
	catalog := CuratedModels()
	data, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal catalog: %w", err)
	}
	path := filepath.Join(m.cacheDir, "catalog.json")
	return os.WriteFile(path, data, 0644)
}

// DownloadProgress reports download progress.
type DownloadProgress struct {
	Name       string
	TotalBytes int64
	DoneBytes  int64
	StartedAt  time.Time
}

// Percent returns the download completion percentage.
func (p *DownloadProgress) Percent() float64 {
	if p.TotalBytes == 0 {
		return 0
	}
	return float64(p.DoneBytes) / float64(p.TotalBytes) * 100
}
