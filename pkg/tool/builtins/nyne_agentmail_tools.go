package builtins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// Nyne.ai credentials come from env vars NYNE_API_KEY and NYNE_API_SECRET,
// or can be set in the agent's config/credentials store.
func nyneHeaders() (string, string) {
	return os.Getenv("NYNE_API_KEY"), os.Getenv("NYNE_API_SECRET")
}

func nynePost(ctx context.Context, endpoint string, payload map[string]interface{}) (string, error) {
	apiKey, apiSecret := nyneHeaders()
	if apiKey == "" || apiSecret == "" {
		return "", fmt.Errorf("NYNE_API_KEY and NYNE_API_SECRET environment variables required")
	}

	b, _ := json.Marshal(payload)
	url := "https://api.nyne.ai" + endpoint

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	if err != nil {
		return "", err
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-API-Secret", apiSecret)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("nyne api: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("nyne api %d: %s", resp.StatusCode, string(body))
	}

	// Pretty-print the JSON for the agent.
	var pretty bytes.Buffer
	if json.Indent(&pretty, body, "", "  ") == nil {
		return pretty.String(), nil
	}
	return string(body), nil
}

// ── Person Enrichment ──────────────────────────────────────────────
// POST https://api.nyne.ai/person/enrichment
// Lookup by email, phone, social_media_url, or name+company+city.

type NynePersonEnrichTool struct{}

func (t *NynePersonEnrichTool) Name() string        { return "nyne_person_enrich" }
func (t *NynePersonEnrichTool) Description() string {
	return "Look up a person using Nyne.ai's identity graph. Provide an email, phone number, social media URL, or name + company. Returns full profile: work history, social profiles, interests, verified contacts, and buying intent signals."
}
func (t *NynePersonEnrichTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"email":            map[string]interface{}{"type": "string", "description": "Email address to look up"},
			"phone":            map[string]interface{}{"type": "string", "description": "Phone number to look up (E.164 format)"},
			"social_media_url": map[string]interface{}{"type": "string", "description": "LinkedIn or other social profile URL"},
			"name":             map[string]interface{}{"type": "string", "description": "Full name (use with company for best results)"},
			"company":          map[string]interface{}{"type": "string", "description": "Company name to narrow down name-based lookup"},
			"city":             map[string]interface{}{"type": "string", "description": "City to narrow down name-based lookup"},
			"lite":             map[string]interface{}{"type": "boolean", "description": "Lite mode: only returns name, company, LinkedIn (3 credits vs 6)"},
		},
	}
}
func (t *NynePersonEnrichTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	payload := map[string]interface{}{}

	if v := argStr(args, "email"); v != "" {
		payload["email"] = v
	}
	if v := argStr(args, "phone"); v != "" {
		payload["phone"] = v
	}
	if v := argStr(args, "social_media_url"); v != "" {
		payload["social_media_url"] = v
	}
	if v := argStr(args, "name"); v != "" {
		payload["name"] = v
	}
	if v := argStr(args, "company"); v != "" {
		payload["company"] = v
	}
	if v := argStr(args, "city"); v != "" {
		payload["city"] = v
	}
	if lite, ok := args["lite"].(bool); ok && lite {
		payload["lite_enrich"] = true
	}

	if len(payload) == 0 {
		return "", fmt.Errorf("provide at least one of: email, phone, social_media_url, or name")
	}

	return nynePost(ctx, "/person/enrichment", payload)
}

// ── Person Search ──────────────────────────────────────────────────
// POST https://api.nyne.ai/person/search
// Find people matching criteria: job title, company, location, signals.

type NynePersonSearchTool struct{}

func (t *NynePersonSearchTool) Name() string        { return "nyne_person_search" }
func (t *NynePersonSearchTool) Description() string {
	return "Search for people using Nyne.ai. Find contacts by job title, company, location, or intent signals. Returns matching profiles with verified contact info."
}
func (t *NynePersonSearchTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query":    map[string]interface{}{"type": "string", "description": "Free-text search query (e.g. 'pest control owner Austin TX')"},
			"title":    map[string]interface{}{"type": "string", "description": "Job title to search for"},
			"company":  map[string]interface{}{"type": "string", "description": "Company name"},
			"location": map[string]interface{}{"type": "string", "description": "City, state, or region"},
			"limit":    map[string]interface{}{"type": "number", "description": "Max results (default 10)"},
		},
		"required": []string{"query"},
	}
}
func (t *NynePersonSearchTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	payload := map[string]interface{}{}

	if v := argStr(args, "query"); v != "" {
		payload["query"] = v
	}
	if v := argStr(args, "title"); v != "" {
		payload["title"] = v
	}
	if v := argStr(args, "company"); v != "" {
		payload["company"] = v
	}
	if v := argStr(args, "location"); v != "" {
		payload["location"] = v
	}
	if limit := argFloat(args, "limit"); limit > 0 {
		payload["limit"] = int(limit)
	}

	return nynePost(ctx, "/person/search", payload)
}

// ── Company Enrichment ─────────────────────────────────────────────
// POST https://api.nyne.ai/company/enrichment
// Look up company data: ownership, decision makers, org structure, funding.

type NyneCompanyEnrichTool struct{}

func (t *NyneCompanyEnrichTool) Name() string        { return "nyne_company_enrich" }
func (t *NyneCompanyEnrichTool) Description() string {
	return "Look up a company using Nyne.ai. Returns ownership, decision makers, org structure, sentiment, funding info, and engagement data."
}
func (t *NyneCompanyEnrichTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"domain":  map[string]interface{}{"type": "string", "description": "Company website domain (e.g. 'acme.com')"},
			"name":    map[string]interface{}{"type": "string", "description": "Company name"},
			"query":   map[string]interface{}{"type": "string", "description": "Free-text company search"},
		},
	}
}
func (t *NyneCompanyEnrichTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	payload := map[string]interface{}{}

	if v := argStr(args, "domain"); v != "" {
		payload["domain"] = v
	}
	if v := argStr(args, "name"); v != "" {
		payload["name"] = v
	}
	if v := argStr(args, "query"); v != "" {
		payload["query"] = v
	}

	if len(payload) == 0 {
		return "", fmt.Errorf("provide at least one of: domain, name, or query")
	}

	return nynePost(ctx, "/company/enrichment", payload)
}

// ── Person Interests ───────────────────────────────────────────────
// POST https://api.nyne.ai/person/interests
// Get a person's interests and buying signals.

type NynePersonInterestsTool struct{}

func (t *NynePersonInterestsTool) Name() string        { return "nyne_person_interests" }
func (t *NynePersonInterestsTool) Description() string {
	return "Get a person's interests and buying intent signals from Nyne.ai. Shows what topics, products, and services they actively engage with."
}
func (t *NynePersonInterestsTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"email":            map[string]interface{}{"type": "string", "description": "Email address"},
			"phone":            map[string]interface{}{"type": "string", "description": "Phone number"},
			"social_media_url": map[string]interface{}{"type": "string", "description": "Social profile URL"},
		},
	}
}
func (t *NynePersonInterestsTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	payload := map[string]interface{}{}
	if v := argStr(args, "email"); v != "" {
		payload["email"] = v
	}
	if v := argStr(args, "phone"); v != "" {
		payload["phone"] = v
	}
	if v := argStr(args, "social_media_url"); v != "" {
		payload["social_media_url"] = v
	}
	if len(payload) == 0 {
		return "", fmt.Errorf("provide at least one of: email, phone, or social_media_url")
	}
	return nynePost(ctx, "/person/interests", payload)
}

// ── AgentMail ──────────────────────────────────────────────────────
// AgentMail is an email API built for AI agents.
// https://agentmail.to
// Agents get their own email addresses and can send/receive/manage mail.

func agentmailHeaders() string {
	return os.Getenv("AGENTMAIL_API_KEY")
}

func agentmailRequest(ctx context.Context, method, endpoint string, payload interface{}) (string, error) {
	apiKey := agentmailHeaders()
	if apiKey == "" {
		return "", fmt.Errorf("AGENTMAIL_API_KEY environment variable required")
	}

	var body io.Reader
	if payload != nil {
		b, _ := json.Marshal(payload)
		body = bytes.NewReader(b)
	}

	url := "https://api.agentmail.to/v0" + endpoint
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("agentmail: %w", err)
	}
	defer resp.Body.Close()

	result, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("agentmail %d: %s", resp.StatusCode, string(result))
	}

	var pretty bytes.Buffer
	if json.Indent(&pretty, result, "", "  ") == nil {
		return pretty.String(), nil
	}
	return string(result), nil
}

// ── Email Send ─────────────────────────────────────────────────────

type AgentMailSendTool struct{}

func (t *AgentMailSendTool) Name() string        { return "email_send" }
func (t *AgentMailSendTool) Description() string {
	return "Send an email from the agent's email address via AgentMail. Supports to, cc, bcc, subject, body (text or HTML)."
}
func (t *AgentMailSendTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"to":       map[string]interface{}{"type": "string", "description": "Recipient email address"},
			"subject":  map[string]interface{}{"type": "string", "description": "Email subject line"},
			"body":     map[string]interface{}{"type": "string", "description": "Email body text"},
			"html":     map[string]interface{}{"type": "string", "description": "HTML body (optional, overrides body)"},
			"cc":       map[string]interface{}{"type": "string", "description": "CC recipient (optional)"},
			"from":     map[string]interface{}{"type": "string", "description": "Mailbox address to send from (optional, uses default)"},
			"reply_to": map[string]interface{}{"type": "string", "description": "Reply-to address (optional)"},
		},
		"required": []string{"to", "subject", "body"},
	}
}
func (t *AgentMailSendTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	to := argStr(args, "to")
	subject := argStr(args, "subject")
	body := argStr(args, "body")

	if to == "" || subject == "" || body == "" {
		return "", fmt.Errorf("to, subject, and body are required")
	}

	message := map[string]interface{}{
		"to":      []map[string]string{{"email": to}},
		"subject": subject,
		"text":    body,
	}

	if html := argStr(args, "html"); html != "" {
		message["html"] = html
	}
	if cc := argStr(args, "cc"); cc != "" {
		message["cc"] = []map[string]string{{"email": cc}}
	}
	if from := argStr(args, "from"); from != "" {
		message["from"] = map[string]string{"email": from}
	}
	if replyTo := argStr(args, "reply_to"); replyTo != "" {
		message["reply_to"] = map[string]string{"email": replyTo}
	}

	return agentmailRequest(ctx, "POST", "/messages/send", message)
}

// ── Email List ─────────────────────────────────────────────────────

type AgentMailListTool struct{}

func (t *AgentMailListTool) Name() string        { return "email_list" }
func (t *AgentMailListTool) Description() string {
	return "List recent emails in the agent's mailbox. Shows sender, subject, date, and preview for each message."
}
func (t *AgentMailListTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"mailbox": map[string]interface{}{"type": "string", "description": "Mailbox address to check (optional, uses default)"},
			"limit":   map[string]interface{}{"type": "number", "description": "Max messages to return (default 10)"},
			"folder":  map[string]interface{}{"type": "string", "description": "Folder: inbox (default), sent, drafts, trash"},
		},
	}
}
func (t *AgentMailListTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	mailbox := argStr(args, "mailbox")
	folder := argStr(args, "folder")
	limit := int(argFloat(args, "limit"))

	if folder == "" {
		folder = "inbox"
	}
	if limit <= 0 {
		limit = 10
	}

	endpoint := fmt.Sprintf("/mailboxes/%s/threads?limit=%d", mailbox, limit)
	if mailbox == "" {
		endpoint = fmt.Sprintf("/threads?limit=%d&folder=%s", limit, folder)
	}

	return agentmailRequest(ctx, "GET", endpoint, nil)
}

// ── Email Read ─────────────────────────────────────────────────────

type AgentMailReadTool struct{}

func (t *AgentMailReadTool) Name() string        { return "email_read" }
func (t *AgentMailReadTool) Description() string {
	return "Read a specific email thread by ID. Returns all messages in the thread with full body content."
}
func (t *AgentMailReadTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"thread_id": map[string]interface{}{"type": "string", "description": "Thread ID to read"},
		},
		"required": []string{"thread_id"},
	}
}
func (t *AgentMailReadTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	threadID := argStr(args, "thread_id")
	if threadID == "" {
		return "", fmt.Errorf("thread_id is required")
	}
	return agentmailRequest(ctx, "GET", "/threads/"+threadID, nil)
}

// Suppress unused import warnings.
var _ = strings.TrimSpace
