package seed

// FocusAgent defines a specialized agent that comes pre-loaded with the system.
//
// What: Agent definitions for domain-specific AI specialists.
// Why:  Users should have access to a catalog of focused agents beyond the
//       core YAML agents (scout, operator, browser, researcher, coder, architect).
//       These can be activated on demand without editing YAML files.
// How:  Loaded by the API's /v1/agents endpoint and displayed in the dashboard's
//       Agents tab as a separate "Focus Agents" section.
type FocusAgent struct {
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Description string   `json:"description"`
	Category    string   `json:"category"`  // marketing, dev, data, creative, ops
	Source      string   `json:"source"`     // core, focus, custom
	Goals       []string `json:"goals"`
	Tools       []string `json:"tools"`
}

// GetFocusAgents returns the built-in catalog of specialized focus agents.
func GetFocusAgents() []FocusAgent {
	return []FocusAgent{
		{
			Name:        "seo",
			Role:        "SEO & Content Strategist",
			Description: "Analyzes websites for SEO issues, generates keyword strategies, writes meta descriptions, and audits content for search engine optimization.",
			Category:    "marketing",
			Source:      "focus",
			Goals:       []string{"seo_audit", "keyword_research", "content_optimization", "meta_generation"},
			Tools:       []string{"web_navigate", "web_extract", "web_search", "file_write", "memory_save"},
		},
		{
			Name:        "social",
			Role:        "Social Media Manager",
			Description: "Creates social media posts, schedules content, analyzes engagement metrics, and manages brand presence across platforms.",
			Category:    "marketing",
			Source:      "focus",
			Goals:       []string{"content_creation", "scheduling", "engagement_analysis", "brand_management"},
			Tools:       []string{"http_fetch", "file_write", "memory_save", "web_search"},
		},
		{
			Name:        "email",
			Role:        "Email Marketing Specialist",
			Description: "Writes email campaigns, creates drip sequences, designs templates, and optimizes open/click rates.",
			Category:    "marketing",
			Source:      "focus",
			Goals:       []string{"campaign_design", "copywriting", "segmentation", "analytics"},
			Tools:       []string{"http_fetch", "file_write", "memory_save"},
		},
		{
			Name:        "ads",
			Role:        "Paid Advertising Specialist",
			Description: "Creates ad copy for Google Ads, Facebook Ads, and other platforms. Optimizes campaigns for ROI and manages budgets.",
			Category:    "marketing",
			Source:      "focus",
			Goals:       []string{"ad_copywriting", "campaign_optimization", "budget_management", "audience_targeting"},
			Tools:       []string{"http_fetch", "file_write", "web_search", "memory_save"},
		},
		{
			Name:        "copywriter",
			Role:        "Professional Copywriter",
			Description: "Writes compelling copy for landing pages, sales pages, product descriptions, and brand messaging.",
			Category:    "creative",
			Source:      "focus",
			Goals:       []string{"persuasive_writing", "brand_voice", "conversion_optimization"},
			Tools:       []string{"file_write", "file_read", "web_search", "memory_save"},
		},
		{
			Name:        "designer",
			Role:        "UI/UX Designer",
			Description: "Creates wireframes, mockups, and design systems. Reviews interfaces for usability and accessibility.",
			Category:    "creative",
			Source:      "focus",
			Goals:       []string{"ui_design", "ux_audit", "design_systems", "accessibility"},
			Tools:       []string{"file_write", "file_read", "web_search", "memory_save"},
		},
		{
			Name:        "data",
			Role:        "Data Analyst",
			Description: "Analyzes datasets, creates visualizations, builds reports, and extracts insights from structured and unstructured data.",
			Category:    "data",
			Source:      "focus",
			Goals:       []string{"data_analysis", "visualization", "reporting", "pattern_detection"},
			Tools:       []string{"shell", "file_read", "file_write", "memory_save", "grep_search"},
		},
		{
			Name:        "devops",
			Role:        "DevOps Engineer",
			Description: "Manages CI/CD pipelines, Docker containers, Kubernetes deployments, and infrastructure as code.",
			Category:    "ops",
			Source:      "focus",
			Goals:       []string{"infrastructure", "deployment", "monitoring", "automation"},
			Tools:       []string{"shell", "file_read", "file_write", "memory_save"},
		},
		{
			Name:        "security",
			Role:        "Security Auditor",
			Description: "Audits code for vulnerabilities, reviews API security, checks for common attack vectors (OWASP Top 10), and recommends hardening measures.",
			Category:    "ops",
			Source:      "focus",
			Goals:       []string{"vulnerability_scanning", "code_audit", "penetration_testing", "compliance"},
			Tools:       []string{"file_read", "grep_search", "shell", "web_search", "memory_save"},
		},
		{
			Name:        "sales",
			Role:        "Sales Pipeline Manager",
			Description: "Manages leads, tracks deal stages, generates proposals, and provides sales forecasting and CRM data analysis.",
			Category:    "marketing",
			Source:      "focus",
			Goals:       []string{"lead_management", "proposal_generation", "forecasting", "crm_analysis"},
			Tools:       []string{"http_fetch", "file_write", "memory_save", "memory_recall"},
		},
		{
			Name:        "support",
			Role:        "Customer Support Agent",
			Description: "Handles customer inquiries, creates knowledge base articles, triages support tickets, and provides troubleshooting assistance.",
			Category:    "ops",
			Source:      "focus",
			Goals:       []string{"ticket_triage", "knowledge_base", "troubleshooting", "customer_communication"},
			Tools:       []string{"http_fetch", "file_write", "file_read", "web_search", "memory_save"},
		},
		{
			Name:        "legal",
			Role:        "Legal & Compliance Reviewer",
			Description: "Reviews contracts, terms of service, privacy policies, and compliance documents. Identifies legal risks and suggests improvements.",
			Category:    "ops",
			Source:      "focus",
			Goals:       []string{"contract_review", "compliance_check", "risk_assessment", "policy_drafting"},
			Tools:       []string{"file_read", "file_write", "web_search", "memory_save"},
		},
		{
			Name:        "translator",
			Role:        "Multilingual Translator",
			Description: "Translates content between languages, localizes marketing materials, and adapts brand messaging for different markets.",
			Category:    "creative",
			Source:      "focus",
			Goals:       []string{"translation", "localization", "cultural_adaptation"},
			Tools:       []string{"file_read", "file_write", "memory_save"},
		},
		{
			Name:        "writer",
			Role:        "Technical Writer",
			Description: "Creates documentation, API guides, README files, user manuals, and technical blog posts with clear structure and accuracy.",
			Category:    "creative",
			Source:      "focus",
			Goals:       []string{"documentation", "api_guides", "tutorials", "technical_blogging"},
			Tools:       []string{"file_read", "file_write", "grep_search", "web_search", "memory_save"},
		},
	}
}
