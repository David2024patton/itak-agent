package builtins

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ══════════════════════════════════════════════════════════════════
// Workflow Management Tools
//
// These tools let an LLM agent create, list, update, and execute
// visual node-based workflows through the REST API. A focused
// "workflow-builder" agent uses these to translate natural language
// requests into concrete workflow graphs.
// ══════════════════════════════════════════════════════════════════

// workflowBaseURL returns the local API base for workflow endpoints.
func workflowBaseURL(port int) string {
	return fmt.Sprintf("http://localhost:%d/v1/workflows", port)
}

// ── workflow_list ─────────────────────────────────────────────────

type WorkflowListTool struct {
	APIPort int
}

func (t *WorkflowListTool) Name() string { return "workflow_list" }
func (t *WorkflowListTool) Description() string {
	return "List all saved workflows. Returns each workflow's ID, name, status, node count, edge count, and run count. No arguments required."
}
func (t *WorkflowListTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (t *WorkflowListTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	resp, err := http.Get(workflowBaseURL(t.APIPort))
	if err != nil {
		return "", fmt.Errorf("failed to list workflows: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	// Parse and return a compact summary.
	var result struct {
		Pipelines []struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			Description string `json:"description"`
			Status      string `json:"status"`
			Nodes       []interface{} `json:"nodes"`
			Edges       []interface{} `json:"edges"`
			RunCount    int    `json:"run_count"`
		} `json:"pipelines"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return string(body), nil
	}

	if len(result.Pipelines) == 0 {
		return "No workflows found. Use workflow_create to create one.", nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d workflow(s):\n", len(result.Pipelines)))
	for _, w := range result.Pipelines {
		sb.WriteString(fmt.Sprintf("  [ID:%d] %s (status: %s, nodes: %d, edges: %d, runs: %d)\n",
			w.ID, w.Name, w.Status, len(w.Nodes), len(w.Edges), w.RunCount))
		if w.Description != "" {
			sb.WriteString(fmt.Sprintf("    Description: %s\n", w.Description))
		}
	}
	return sb.String(), nil
}

// ── workflow_get ──────────────────────────────────────────────────

type WorkflowGetTool struct {
	APIPort int
}

func (t *WorkflowGetTool) Name() string { return "workflow_get" }
func (t *WorkflowGetTool) Description() string {
	return "Get the full details of a workflow by its numeric ID, including all nodes (with positions, types, configs) and edges (connections). Args: id (integer workflow ID)."
}
func (t *WorkflowGetTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{"type": "number", "description": "Workflow ID to retrieve"},
		},
		"required": []string{"id"},
	}
}

func (t *WorkflowGetTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	id := int(argFloat(args, "id"))
	if id <= 0 {
		return "", fmt.Errorf("missing required argument: id (positive integer)")
	}

	url := fmt.Sprintf("%s/%d", workflowBaseURL(t.APIPort), id)
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("failed to get workflow %d: %w", id, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 404 {
		return fmt.Sprintf("Workflow ID %d not found.", id), nil
	}

	// Pretty-print the JSON.
	var buf bytes.Buffer
	if err := json.Indent(&buf, body, "", "  "); err != nil {
		return string(body), nil
	}
	return buf.String(), nil
}

// ── workflow_create ───────────────────────────────────────────────

type WorkflowCreateTool struct {
	APIPort int
}

func (t *WorkflowCreateTool) Name() string { return "workflow_create" }
func (t *WorkflowCreateTool) Description() string {
	return `Create a new visual workflow with nodes and edges. Returns the created workflow ID.

Node types: prompt, agent, webhook, api_call, websocket, condition, transform, delay, loop, merge, code, error_handler, schedule, db_query, email, notification, human, note.

Each node needs: id (unique string like "n1"), type (one of above), label (display name), x (integer x position), y (integer y position), config (object with type-specific fields).

Example node configs:
  - prompt: {"prompt": "Summarize the daily news"}
  - agent: {"agent_id": "researcher", "prompt": "Find trending topics"}
  - webhook: {"method": "POST", "url": "https://example.com/hook"}
  - api_call: {"url": "https://api.example.com/data", "method": "GET"}
  - condition: {"field": "status", "operator": "equals", "value": "success"}
  - delay: {"seconds": 30}
  - email: {"to": "user@example.com", "subject": "Report Ready"}
  - notification: {"channel": "discord", "message": "Alert!"}
  - code: {"language": "javascript", "code": "return data.filter(x => x.active)"}
  - schedule: {"cron": "0 9 * * *"}

Edges connect nodes: each edge has source (node id) and target (node id).
For condition nodes, set sourcePort to "out_true" or "out_false".

Args: name, description, nodes (JSON array), edges (JSON array).`
}
func (t *WorkflowCreateTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"name":        map[string]interface{}{"type": "string", "description": "Workflow name"},
			"description": map[string]interface{}{"type": "string", "description": "What this workflow does"},
			"nodes":       map[string]interface{}{"type": "string", "description": "JSON array of node objects [{id, type, label, x, y, config}]"},
			"edges":       map[string]interface{}{"type": "string", "description": "JSON array of edge objects [{source, target, sourcePort?}]"},
		},
		"required": []string{"name", "nodes", "edges"},
	}
}

func (t *WorkflowCreateTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	name := argStr(args, "name")
	if name == "" {
		return "", fmt.Errorf("missing required argument: name")
	}

	description := argStr(args, "description")
	nodesStr := argStr(args, "nodes")
	edgesStr := argStr(args, "edges")

	// Parse JSON strings into actual arrays.
	var nodes []interface{}
	if err := json.Unmarshal([]byte(nodesStr), &nodes); err != nil {
		return "", fmt.Errorf("invalid nodes JSON: %w", err)
	}

	var edges []interface{}
	if edgesStr != "" {
		if err := json.Unmarshal([]byte(edgesStr), &edges); err != nil {
			return "", fmt.Errorf("invalid edges JSON: %w", err)
		}
	}

	payload := map[string]interface{}{
		"name":        name,
		"description": description,
		"status":      "draft",
		"nodes":       nodes,
		"edges":       edges,
	}
	body, _ := json.Marshal(payload)

	resp, err := http.Post(workflowBaseURL(t.APIPort), "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create workflow: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	var result struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return string(respBody), nil
	}

	return fmt.Sprintf("Workflow %q created successfully (ID: %d). It has %d nodes and %d edges. Status: draft. Open it in the dashboard at /#workflows to see and edit visually.",
		name, result.ID, len(nodes), len(edges)), nil
}

// ── workflow_update ───────────────────────────────────────────────

type WorkflowUpdateTool struct {
	APIPort int
}

func (t *WorkflowUpdateTool) Name() string { return "workflow_update" }
func (t *WorkflowUpdateTool) Description() string {
	return "Update an existing workflow's nodes, edges, name, description, or status. Args: id (required), name (optional), description (optional), status (optional: draft/active/paused), nodes (optional JSON array), edges (optional JSON array)."
}
func (t *WorkflowUpdateTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id":          map[string]interface{}{"type": "number", "description": "Workflow ID to update"},
			"name":        map[string]interface{}{"type": "string", "description": "New name (optional)"},
			"description": map[string]interface{}{"type": "string", "description": "New description (optional)"},
			"status":      map[string]interface{}{"type": "string", "description": "New status: draft, active, or paused (optional)"},
			"nodes":       map[string]interface{}{"type": "string", "description": "JSON array of updated nodes (optional)"},
			"edges":       map[string]interface{}{"type": "string", "description": "JSON array of updated edges (optional)"},
		},
		"required": []string{"id"},
	}
}

func (t *WorkflowUpdateTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	id := int(argFloat(args, "id"))
	if id <= 0 {
		return "", fmt.Errorf("missing required argument: id (positive integer)")
	}

	payload := make(map[string]interface{})

	if v := argStr(args, "name"); v != "" {
		payload["name"] = v
	}
	if v := argStr(args, "description"); v != "" {
		payload["description"] = v
	}
	if v := argStr(args, "status"); v != "" {
		payload["status"] = v
	}

	if nodesStr := argStr(args, "nodes"); nodesStr != "" {
		var nodes []interface{}
		if err := json.Unmarshal([]byte(nodesStr), &nodes); err != nil {
			return "", fmt.Errorf("invalid nodes JSON: %w", err)
		}
		payload["nodes"] = nodes
	}

	if edgesStr := argStr(args, "edges"); edgesStr != "" {
		var edges []interface{}
		if err := json.Unmarshal([]byte(edgesStr), &edges); err != nil {
			return "", fmt.Errorf("invalid edges JSON: %w", err)
		}
		payload["edges"] = edges
	}

	if len(payload) == 0 {
		return "Nothing to update. Provide at least one field: name, description, status, nodes, or edges.", nil
	}

	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("%s/%d", workflowBaseURL(t.APIPort), id)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to build update request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to update workflow %d: %w", id, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return fmt.Sprintf("Workflow ID %d not found.", id), nil
	}

	return fmt.Sprintf("Workflow ID %d updated successfully.", id), nil
}

// ── workflow_execute ──────────────────────────────────────────────

type WorkflowExecuteTool struct {
	APIPort int
}

func (t *WorkflowExecuteTool) Name() string { return "workflow_execute" }
func (t *WorkflowExecuteTool) Description() string {
	return "Execute (run) a workflow by its ID. The workflow will be processed in topological order, executing each node in sequence. Args: id (integer workflow ID)."
}
func (t *WorkflowExecuteTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"id": map[string]interface{}{"type": "number", "description": "Workflow ID to execute"},
		},
		"required": []string{"id"},
	}
}

func (t *WorkflowExecuteTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	id := int(argFloat(args, "id"))
	if id <= 0 {
		return "", fmt.Errorf("missing required argument: id (positive integer)")
	}

	url := fmt.Sprintf("%s/%d/execute", workflowBaseURL(t.APIPort), id)
	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		return "", fmt.Errorf("failed to execute workflow %d: %w", id, err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 404 {
		return fmt.Sprintf("Workflow ID %d not found.", id), nil
	}

	var result struct {
		Status    string `json:"status"`
		RunCount  int    `json:"run_count"`
		ExecOrder []string `json:"execution_order"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return string(body), nil
	}

	return fmt.Sprintf("Workflow ID %d executed successfully.\nStatus: %s\nTotal runs: %d\nExecution order: %s",
		id, result.Status, result.RunCount, strings.Join(result.ExecOrder, " → ")), nil
}
