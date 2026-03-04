package llm

// ProviderEntry defines a pre-configured LLM provider with its endpoint details.
type ProviderEntry struct {
	Name       string // Human-readable name
	Slug       string // Config key (lowercase, no spaces)
	APIBase    string // Base URL for chat completions
	ModelsPath string // Path appended to APIBase for model listing (usually "/models")
	Category   string // "creator", "inference", or "cloud"
	Compatible bool   // true = OpenAI-compatible /chat/completions
	SignupURL  string // Where to get an API key
	Notes      string // Any special requirements
}

// providerCatalog is the master registry of all known LLM providers.
// Providers are ordered by category: creators first, then inference, then cloud.
var providerCatalog = []ProviderEntry{

	// ── Model Creators (Proprietary & Native) ──────────────────────

	{
		Name: "OpenAI", Slug: "openai",
		APIBase: "https://api.openai.com/v1", ModelsPath: "/models",
		Category: "creator", Compatible: true,
		SignupURL: "https://platform.openai.com/api-keys",
	},
	{
		Name: "Anthropic", Slug: "anthropic",
		APIBase: "https://api.anthropic.com/v1", ModelsPath: "/models",
		Category: "creator", Compatible: false,
		SignupURL: "https://console.anthropic.com/",
		Notes:     "Non-OpenAI format. Uses x-api-key header and Messages API.",
	},
	{
		Name: "Google Gemini", Slug: "google",
		APIBase: "https://generativelanguage.googleapis.com/v1beta/openai", ModelsPath: "/models",
		Category: "creator", Compatible: true,
		SignupURL: "https://aistudio.google.com/apikey",
		Notes:     "Uses OpenAI-compatible endpoint. Key passed as query param or Bearer token.",
	},
	{
		Name: "Mistral AI", Slug: "mistral",
		APIBase: "https://api.mistral.ai/v1", ModelsPath: "/models",
		Category: "creator", Compatible: true,
		SignupURL: "https://console.mistral.ai/api-keys",
	},
	{
		Name: "Meta AI", Slug: "meta",
		APIBase: "", ModelsPath: "",
		Category: "creator", Compatible: false,
		SignupURL: "https://llama.meta.com/",
		Notes:     "No direct API. Use via Azure, Together, Groq, or other hosts.",
	},
	{
		Name: "Cohere", Slug: "cohere",
		APIBase: "https://api.cohere.com/v2", ModelsPath: "/models",
		Category: "creator", Compatible: false,
		SignupURL: "https://dashboard.cohere.com/api-keys",
		Notes:     "Non-OpenAI format. Uses own Chat API and x-api-key header.",
	},
	{
		Name: "xAI (Grok)", Slug: "xai",
		APIBase: "https://api.x.ai/v1", ModelsPath: "/models",
		Category: "creator", Compatible: true,
		SignupURL: "https://console.x.ai/",
	},
	{
		Name: "DeepSeek", Slug: "deepseek",
		APIBase: "https://api.deepseek.com", ModelsPath: "/models",
		Category: "creator", Compatible: true,
		SignupURL: "https://platform.deepseek.com/api_keys",
	},
	{
		Name: "AI21 Labs", Slug: "ai21",
		APIBase: "https://api.ai21.com/studio/v1", ModelsPath: "/models",
		Category: "creator", Compatible: false,
		SignupURL: "https://studio.ai21.com/account/api-key",
		Notes:     "Non-OpenAI format. Uses own API schema.",
	},
	{
		Name: "Aleph Alpha", Slug: "aleph_alpha",
		APIBase: "https://api.aleph-alpha.com", ModelsPath: "/models_available",
		Category: "creator", Compatible: false,
		SignupURL: "https://app.aleph-alpha.com/",
		Notes:     "Non-OpenAI format. Uses own completion API.",
	},
	{
		Name: "Alibaba Qwen", Slug: "qwen",
		APIBase: "https://dashscope.aliyuncs.com/compatible-mode/v1", ModelsPath: "/models",
		Category: "creator", Compatible: true,
		SignupURL: "https://dashscope.console.aliyun.com/",
	},
	{
		Name: "01.AI (Yi)", Slug: "01ai",
		APIBase: "https://api.01.ai/v1", ModelsPath: "/models",
		Category: "creator", Compatible: true,
		SignupURL: "https://platform.01.ai/",
	},
	{
		Name: "Moonshot AI (Kimi)", Slug: "moonshot",
		APIBase: "https://api.moonshot.cn/v1", ModelsPath: "/models",
		Category: "creator", Compatible: true,
		SignupURL: "https://platform.moonshot.cn/",
	},
	{
		Name: "Tencent Hunyuan", Slug: "hunyuan",
		APIBase: "https://api.hunyuan.cloud.tencent.com/v1", ModelsPath: "/models",
		Category: "creator", Compatible: false,
		SignupURL: "https://cloud.tencent.com/product/hunyuan",
		Notes:     "Requires Tencent Cloud authentication.",
	},
	{
		Name: "Zhipu AI (GLM)", Slug: "zhipu",
		APIBase: "https://open.bigmodel.cn/api/paas/v4", ModelsPath: "/models",
		Category: "creator", Compatible: true,
		SignupURL: "https://open.bigmodel.cn/",
	},

	// ── Inference & Hosting Specialists ─────────────────────────────

	{
		Name: "Groq", Slug: "groq",
		APIBase: "https://api.groq.com/openai/v1", ModelsPath: "/models",
		Category: "inference", Compatible: true,
		SignupURL: "https://console.groq.com/keys",
	},
	{
		Name: "Together AI", Slug: "together",
		APIBase: "https://api.together.xyz/v1", ModelsPath: "/models",
		Category: "inference", Compatible: true,
		SignupURL: "https://api.together.xyz/settings/api-keys",
	},
	{
		Name: "Fireworks AI", Slug: "fireworks",
		APIBase: "https://api.fireworks.ai/inference/v1", ModelsPath: "/models",
		Category: "inference", Compatible: true,
		SignupURL: "https://fireworks.ai/account/api-keys",
	},
	{
		Name: "Cerebras", Slug: "cerebras",
		APIBase: "https://api.cerebras.ai/v1", ModelsPath: "/models",
		Category: "inference", Compatible: true,
		SignupURL: "https://cloud.cerebras.ai/",
	},
	{
		Name: "DeepInfra", Slug: "deepinfra",
		APIBase: "https://api.deepinfra.com/v1/openai", ModelsPath: "/models",
		Category: "inference", Compatible: true,
		SignupURL: "https://deepinfra.com/dash/api_keys",
	},
	{
		Name: "SambaNova", Slug: "sambanova",
		APIBase: "https://api.sambanova.ai/v1", ModelsPath: "/models",
		Category: "inference", Compatible: true,
		SignupURL: "https://cloud.sambanova.ai/",
	},
	{
		Name: "SiliconFlow", Slug: "siliconflow",
		APIBase: "https://api.siliconflow.cn/v1", ModelsPath: "/models",
		Category: "inference", Compatible: true,
		SignupURL: "https://cloud.siliconflow.cn/",
	},
	{
		Name: "Baseten", Slug: "baseten",
		APIBase: "", ModelsPath: "",
		Category: "inference", Compatible: false,
		SignupURL: "https://www.baseten.co/",
		Notes:     "Custom model deployment platform. Endpoint varies per deployment.",
	},
	{
		Name: "RunPod", Slug: "runpod",
		APIBase: "", ModelsPath: "",
		Category: "inference", Compatible: false,
		SignupURL: "https://www.runpod.io/",
		Notes:     "GPU-focused serverless. Endpoint varies per deployment.",
	},
	{
		Name: "Lambda Labs", Slug: "lambda",
		APIBase: "https://api.lambdalabs.com/v1", ModelsPath: "/models",
		Category: "inference", Compatible: true,
		SignupURL: "https://cloud.lambdalabs.com/",
	},
	{
		Name: "Novita AI", Slug: "novita",
		APIBase: "https://api.novita.ai/v3/openai", ModelsPath: "/models",
		Category: "inference", Compatible: true,
		SignupURL: "https://novita.ai/dashboard/key",
	},
	{
		Name: "Kluster.ai", Slug: "kluster",
		APIBase: "https://api.kluster.ai/v1", ModelsPath: "/models",
		Category: "inference", Compatible: true,
		SignupURL: "https://kluster.ai/",
	},
	{
		Name: "Inference.net", Slug: "inferencenet",
		APIBase: "https://api.inference.net/v1", ModelsPath: "/models",
		Category: "inference", Compatible: true,
		SignupURL: "https://inference.net/",
	},
	{
		Name: "FriendliAI", Slug: "friendli",
		APIBase: "https://inference.friendli.ai/v1", ModelsPath: "/models",
		Category: "inference", Compatible: true,
		SignupURL: "https://friendli.ai/",
	},
	{
		Name: "Nebuly", Slug: "nebuly",
		APIBase: "", ModelsPath: "",
		Category: "inference", Compatible: false,
		SignupURL: "https://www.nebuly.com/",
		Notes:     "Inference cost-optimization platform. Not a direct LLM API.",
	},

	// ── Cloud Ecosystems & Aggregators ──────────────────────────────

	{
		Name: "OpenRouter", Slug: "openrouter",
		APIBase: "https://openrouter.ai/api/v1", ModelsPath: "/models",
		Category: "cloud", Compatible: true,
		SignupURL: "https://openrouter.ai/keys",
		Notes:     "Unified API for 200+ models. Recommended aggregator.",
	},
	{
		Name: "Amazon Bedrock", Slug: "bedrock",
		APIBase: "", ModelsPath: "",
		Category: "cloud", Compatible: false,
		SignupURL: "https://aws.amazon.com/bedrock/",
		Notes:     "AWS SDK-based. Requires IAM credentials, not simple API key.",
	},
	{
		Name: "Azure OpenAI", Slug: "azure",
		APIBase: "", ModelsPath: "",
		Category: "cloud", Compatible: false,
		SignupURL: "https://azure.microsoft.com/en-us/products/ai-services/openai-service",
		Notes:     "OpenAI-compatible but requires resource-specific URL and deployment names.",
	},
	{
		Name: "Google Vertex AI", Slug: "vertex",
		APIBase: "", ModelsPath: "",
		Category: "cloud", Compatible: false,
		SignupURL: "https://cloud.google.com/vertex-ai",
		Notes:     "GCP service account auth. Not simple API key.",
	},
	{
		Name: "Hugging Face", Slug: "huggingface",
		APIBase: "https://api-inference.huggingface.co/v1", ModelsPath: "/models",
		Category: "cloud", Compatible: true,
		SignupURL: "https://huggingface.co/settings/tokens",
		Notes:     "Inference API with OpenAI-compatible mode.",
	},
	{
		Name: "Snowflake Cortex", Slug: "snowflake",
		APIBase: "", ModelsPath: "",
		Category: "cloud", Compatible: false,
		SignupURL: "https://www.snowflake.com/en/data-cloud/cortex/",
		Notes:     "Internal to Snowflake. Requires Snowflake account.",
	},
	{
		Name: "Databricks Mosaic AI", Slug: "databricks",
		APIBase: "", ModelsPath: "",
		Category: "cloud", Compatible: false,
		SignupURL: "https://www.databricks.com/product/machine-learning/mosaic-ai",
		Notes:     "Internal to Databricks workspace.",
	},
	{
		Name: "Perplexity", Slug: "perplexity",
		APIBase: "https://api.perplexity.ai", ModelsPath: "/models",
		Category: "cloud", Compatible: true,
		SignupURL: "https://www.perplexity.ai/settings/api",
	},
	{
		Name: "Cloudflare Workers AI", Slug: "cloudflare",
		APIBase: "", ModelsPath: "",
		Category: "cloud", Compatible: false,
		SignupURL: "https://dash.cloudflare.com/",
		Notes:     "Requires account ID in URL. Edge-based inference.",
	},
	{
		Name: "Vellum", Slug: "vellum",
		APIBase: "", ModelsPath: "",
		Category: "cloud", Compatible: false,
		SignupURL: "https://www.vellum.ai/",
		Notes:     "Agentic platform, not direct LLM inference.",
	},

	// ── Self-Hosted / Local ────────────────────────────────────────

	{
		Name: "NVIDIA NIM", Slug: "nvidia_nim",
		APIBase: "https://integrate.api.nvidia.com/v1", ModelsPath: "/models",
		Category: "inference", Compatible: true,
		SignupURL: "https://build.nvidia.com/",
	},
	{
		Name: "Ollama (Local)", Slug: "ollama",
		APIBase: "http://localhost:11434/v1", ModelsPath: "/models",
		Category: "inference", Compatible: true,
		SignupURL: "https://ollama.com/",
		Notes:     "Local inference. No API key needed. Change api_base if remote.",
	},
}

// slugIndex is a fast lookup map built once.
var slugIndex map[string]*ProviderEntry

func init() {
	slugIndex = make(map[string]*ProviderEntry, len(providerCatalog))
	for i := range providerCatalog {
		slugIndex[providerCatalog[i].Slug] = &providerCatalog[i]
	}
}

// GetProvider returns a provider entry by its config slug.
func GetProvider(slug string) (ProviderEntry, bool) {
	p, ok := slugIndex[slug]
	if !ok {
		return ProviderEntry{}, false
	}
	return *p, true
}

// AllProviders returns every provider in the catalog.
func AllProviders() []ProviderEntry {
	out := make([]ProviderEntry, len(providerCatalog))
	copy(out, providerCatalog)
	return out
}

// CompatibleProviders returns only OpenAI-compatible providers with a non-empty APIBase.
func CompatibleProviders() []ProviderEntry {
	var out []ProviderEntry
	for _, p := range providerCatalog {
		if p.Compatible && p.APIBase != "" {
			out = append(out, p)
		}
	}
	return out
}

// BuildProviderConfigs takes a map of slug -> api_key and returns ProviderConfigs
// for all compatible providers that have a key set.
func BuildProviderConfigs(keys map[string]string) []ProviderConfig {
	var configs []ProviderConfig
	for _, p := range providerCatalog {
		key, ok := keys[p.Slug]
		if !ok || key == "" || !p.Compatible || p.APIBase == "" {
			continue
		}
		configs = append(configs, ProviderConfig{
			Provider: p.Slug,
			Model:    "", // filled in by config or model discovery
			APIBase:  p.APIBase,
			APIKey:   key,
		})
	}
	return configs
}
