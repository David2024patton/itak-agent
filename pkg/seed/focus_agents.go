package seed

import (
	_ "embed"
	"encoding/json"
	"log"
)

//go:embed agency_catalog.json
var agencyCatalogJSON []byte

// FocusAgent describes a specialist agent from the catalog.
type FocusAgent struct {
	Name        string   `json:"name"`
	Role        string   `json:"role"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Division    string   `json:"division"`
	Source      string   `json:"source"`
	Goals       []string `json:"goals,omitempty"`
	Tools       []string `json:"tools"`
}

// agencyAgents is the parsed catalog, loaded once at import time.
var agencyAgents []FocusAgent

func init() {
	if err := json.Unmarshal(agencyCatalogJSON, &agencyAgents); err != nil {
		log.Printf("[seed] WARNING: failed to parse agency_catalog.json: %v", err)
	}
}

// GetFocusAgents returns the full catalog of specialized agents.
// This merges the built-in iTaK agents with the agency-agents catalog.
func GetFocusAgents() []FocusAgent {
	all := make([]FocusAgent, 0, len(builtinAgents)+len(agencyAgents))
	all = append(all, builtinAgents...)
	all = append(all, agencyAgents...)
	return all
}

// GetAgentsByDivision returns agents filtered by division name.
func GetAgentsByDivision(division string) []FocusAgent {
	var result []FocusAgent
	for _, a := range GetFocusAgents() {
		if a.Division == division {
			result = append(result, a)
		}
	}
	return result
}

// GetDivisions returns all unique division names.
func GetDivisions() []string {
	seen := map[string]bool{}
	var divs []string
	for _, a := range GetFocusAgents() {
		if !seen[a.Division] {
			seen[a.Division] = true
			divs = append(divs, a.Division)
		}
	}
	return divs
}

// builtinAgents are the original iTaK focus agents (always included).
var builtinAgents = []FocusAgent{
	{
		Name:        "seo-optimizer",
		Role:        "SEO Specialist",
		Description: "Analyzes websites for SEO, generates meta tags, keyword strategies, and content optimization plans.",
		Category:    "marketing",
		Division:    "Marketing",
		Source:      "focus",
		Goals:       []string{"Improve search rankings", "Generate keyword research", "Audit on-page SEO"},
		Tools:       []string{"http_fetch", "file_write", "web_search", "memory_save"},
	},
	{
		Name:        "social-media-manager",
		Role:        "Social Media Manager",
		Description: "Creates social media strategies, content calendars, and engagement plans across platforms.",
		Category:    "marketing",
		Division:    "Marketing",
		Source:      "focus",
		Goals:       []string{"Build social presence", "Create content calendars", "Analyze engagement metrics"},
		Tools:       []string{"file_write", "web_search", "memory_save"},
	},
	{
		Name:        "email-marketer",
		Role:        "Email Marketing Specialist",
		Description: "Designs email campaigns, drip sequences, and newsletter strategies with A/B testing frameworks.",
		Category:    "marketing",
		Division:    "Marketing",
		Source:      "focus",
		Goals:       []string{"Increase open rates", "Design drip campaigns", "Segment audiences"},
		Tools:       []string{"file_write", "web_search", "memory_save"},
	},
	{
		Name:        "ad-manager",
		Role:        "Advertising Manager",
		Description: "Plans and optimizes ad campaigns across Google, Facebook, and other platforms. Manages budgets.",
		Category:    "marketing",
		Division:    "Paid Media",
		Source:      "focus",
		Goals:       []string{"Optimize ROAS", "Create ad copy", "Analyze campaign performance"},
		Tools:       []string{"http_fetch", "file_write", "web_search", "memory_save"},
	},
	{
		Name:        "copywriter",
		Role:        "Copywriter",
		Description: "Writes compelling copy for landing pages, sales pages, product descriptions, and brand messaging.",
		Category:    "creative",
		Division:    "Design",
		Source:      "focus",
		Goals:       []string{"Write converting copy", "Develop brand voice", "Create CTAs"},
		Tools:       []string{"file_write", "file_read", "web_search", "memory_save"},
	},
	{
		Name:        "designer",
		Role:        "UI/UX Designer",
		Description: "Creates wireframes, mockups, and design systems. Reviews interfaces for usability and accessibility.",
		Category:    "creative",
		Division:    "Design",
		Source:      "focus",
		Goals:       []string{"Design user interfaces", "Create design systems", "Conduct UX reviews"},
		Tools:       []string{"file_write", "file_read", "web_search", "memory_save"},
	},
	{
		Name:        "data-analyst",
		Role:        "Data Analyst",
		Description: "Analyzes datasets, creates visualizations, builds reports, and extracts insights from structured and unstructured data.",
		Category:    "data",
		Division:    "Support",
		Source:      "focus",
		Goals:       []string{"Analyze data trends", "Build dashboards", "Generate reports"},
		Tools:       []string{"shell", "file_read", "file_write", "memory_save", "grep_search"},
	},
	{
		Name:        "devops-engineer",
		Role:        "DevOps Engineer",
		Description: "Manages CI/CD pipelines, Docker containers, Kubernetes clusters, and cloud infrastructure.",
		Category:    "ops",
		Division:    "Engineering",
		Source:      "focus",
		Goals:       []string{"Automate deployments", "Monitor infrastructure", "Optimize pipelines"},
		Tools:       []string{"shell", "file_read", "file_write", "web_search"},
	},
	{
		Name:        "security-auditor",
		Role:        "Security Auditor",
		Description: "Performs security assessments, vulnerability scans, and compliance checks on applications and infrastructure.",
		Category:    "ops",
		Division:    "Engineering",
		Source:      "focus",
		Goals:       []string{"Identify vulnerabilities", "Review access controls", "Audit compliance"},
		Tools:       []string{"shell", "file_read", "grep_search", "web_search"},
	},
	{
		Name:        "sales-pipeline",
		Role:        "Sales Pipeline Manager",
		Description: "Tracks leads, manages CRM data, creates sales reports, and provides sales forecasting and data analysis.",
		Category:    "marketing",
		Division:    "Sales",
		Source:      "focus",
		Goals:       []string{"Track pipeline health", "Forecast revenue", "Qualify leads"},
		Tools:       []string{"http_fetch", "file_write", "memory_save", "memory_recall"},
	},
	{
		Name:        "customer-support",
		Role:        "Customer Support Agent",
		Description: "Handles customer inquiries, creates KB articles, and manages support ticket workflows.",
		Category:    "ops",
		Division:    "Support",
		Source:      "focus",
		Goals:       []string{"Resolve tickets", "Build knowledge base", "Improve CSAT"},
		Tools:       []string{"file_read", "file_write", "web_search", "memory_save"},
	},
	{
		Name:        "legal-reviewer",
		Role:        "Legal Reviewer",
		Description: "Reviews contracts, terms of service, and compliance documents. Identifies legal risks and issues.",
		Category:    "ops",
		Division:    "Support",
		Source:      "focus",
		Goals:       []string{"Review contracts", "Check compliance", "Identify legal risks"},
		Tools:       []string{"file_read", "web_search", "memory_save"},
	},
	{
		Name:        "translator",
		Role:        "Translator & Localizer",
		Description: "Translates content between languages, localizes marketing materials, and adapts brand messaging for different markets.",
		Category:    "creative",
		Division:    "Specialized",
		Source:      "focus",
		Goals:       []string{"Translate content", "Localize marketing", "Adapt for markets"},
		Tools:       []string{"file_read", "file_write", "memory_save"},
	},
	{
		Name:        "technical-writer",
		Role:        "Technical Writer",
		Description: "Creates documentation, API guides, README files, user manuals, and technical blog posts with clear structure and accuracy.",
		Category:    "creative",
		Division:    "Engineering",
		Source:      "focus",
		Goals:       []string{"Write documentation", "Create API guides", "Maintain READMEs"},
		Tools:       []string{"file_read", "file_write", "grep_search", "web_search", "memory_save"},
	},
	{
		Name:        "workflow-builder",
		Role:        "Workflow Architect",
		Description: "Designs and builds visual agent workflows from natural language descriptions. Translates user requests into node graphs with proper node types (prompt, agent, webhook, api_call, condition, etc.), connections, and configurations. Can list, create, update, and execute workflows.",
		Category:    "ops",
		Division:    "Engineering",
		Source:      "focus",
		Goals:       []string{"Build workflow graphs from descriptions", "Connect nodes with proper logic", "Configure node parameters"},
		Tools:       []string{"workflow_list", "workflow_get", "workflow_create", "workflow_update", "workflow_execute", "memory_save"},
	},
	{
		Name:        "phone-receptionist",
		Role:        "Phone Receptionist",
		Description: "Handles incoming phone calls as an AI receptionist. Greets callers, understands their needs through natural conversation, schedules appointments, answers FAQ from company knowledge, and transfers to humans when needed. Works with FieldRoutes, Odoo, and other business APIs.",
		Category:    "ops",
		Division:    "Operations",
		Source:      "focus",
		Goals:       []string{"Answer calls professionally", "Schedule work orders and appointments", "Route calls to the right department"},
		Tools:       []string{"voice_speak", "voice_gather", "voice_transfer", "voice_hold", "voice_hangup", "voice_record", "voice_call_list", "voice_make_call", "nyne_person_enrich", "nyne_company_enrich", "nyne_person_interests", "email_send", "memory_search", "memory_save", "web_search"},
	},
}
