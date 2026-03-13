package embed

// Config holds the embedding engine configuration.
// Maps directly to the "embeddings:" block in itakagent.yaml.
type Config struct {
	// Provider selects the embedding backend: "gemini", "local", "ollama".
	Provider string `yaml:"provider"`

	// Model is the embedding model name.
	// Gemini: "gemini-embedding-2-preview" or "gemini-embedding-001"
	// Local:  "nomic-embed-text", "bge-m3", "snowflake-arctic-embed"
	Model string `yaml:"model"`

	// APIKey for cloud providers (Gemini). Supports ${ENV_VAR} expansion.
	APIKey string `yaml:"api_key,omitempty"`

	// APIBase overrides the default API endpoint.
	// Gemini default: "https://generativelanguage.googleapis.com"
	APIBase string `yaml:"api_base,omitempty"`

	// Dimensions controls the output vector size (128-3072).
	// Recommended: 768 (balanced), 1536 (quality), 3072 (max)
	Dimensions int `yaml:"dimensions,omitempty"`

	// Fallback provider name. If primary fails, try this.
	// Example: provider=gemini, fallback=local
	Fallback string `yaml:"fallback,omitempty"`

	// LocalEndpoint is the URL for local embedding (Ollama or Torch).
	// Default: "http://localhost:11434"
	LocalEndpoint string `yaml:"local_endpoint,omitempty"`

	// LocalModel overrides the model name for the local fallback.
	LocalModel string `yaml:"local_model,omitempty"`
}

// Defaults fills in missing config values.
func (c *Config) Defaults() {
	if c.Provider == "" {
		c.Provider = "gemini"
	}
	if c.Model == "" {
		switch c.Provider {
		case "gemini":
			c.Model = "gemini-embedding-2-preview"
		default:
			c.Model = "nomic-embed-text"
		}
	}
	if c.Dimensions == 0 {
		c.Dimensions = 768
	}
	if c.LocalEndpoint == "" {
		c.LocalEndpoint = "http://localhost:11434"
	}
	if c.LocalModel == "" {
		c.LocalModel = "nomic-embed-text"
	}
	if c.APIBase == "" {
		c.APIBase = "https://generativelanguage.googleapis.com"
	}
}
