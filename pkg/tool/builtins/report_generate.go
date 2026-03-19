package builtins

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Report represents a rich web report structure.
type Report struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	CreatedAt string `json:"created_at"`
	Sections  []ReportSection `json:"sections"`
}

// ReportSection represents a section within a report.
type ReportSection struct {
	Title   string `json:"title"`
	Content string `json:"content"` // Markdown/HTML content
	Chart   *ChartConfig `json:"chart,omitempty"` // Optional Chart.js config
}

// ChartConfig defines a configuration for Chart.js.
type ChartConfig struct {
	Type    string      `json:"type"` // "bar", "line", "pie", etc.
	Data    interface{} `json:"data"` // Chart.js data object
	Options interface{} `json:"options,omitempty"` // Chart.js options object
}

// ReportGenerateTool creates rich JSON reports for the Dashboard.
type ReportGenerateTool struct {
	DataDir string
}

func (r *ReportGenerateTool) Name() string { return "report_generate" }

func (r *ReportGenerateTool) Description() string {
	return "Generate a structured JSON report (with optional Chart.js data) for rendering in the iTaK Web Dashboard."
}

func (r *ReportGenerateTool) Schema() map[string]interface{} {
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"filename": map[string]interface{}{
				"type":        "string",
				"description": "Name of the output file (without .json extension)",
			},
			"report_json": map[string]interface{}{
				"type":        "string",
				"description": "JSON string representing the Report structure containing Title, Summary, and Sections array.",
			},
		},
		"required": []string{"filename", "report_json"},
	}
}

func (r *ReportGenerateTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	filename, ok := args["filename"].(string)
	if !ok || filename == "" {
		return "", fmt.Errorf("missing or invalid filename")
	}

	reportJSONStr, ok := args["report_json"].(string)
	if !ok || reportJSONStr == "" {
		return "", fmt.Errorf("missing or invalid report_json")
	}

	var report Report
	if err := json.Unmarshal([]byte(reportJSONStr), &report); err != nil {
		return "", fmt.Errorf("failed to parse report_json: %v", err)
	}

	// Set metadata if missing.
	if report.ID == "" {
		report.ID = filename
	}
	if report.CreatedAt == "" {
		report.CreatedAt = time.Now().Format(time.RFC3339)
	}

	// Re-encode to ensure clean JSON formatting.
	cleanJSON, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to re-encode report JSON: %v", err)
	}

	// Ensure output directory exists.
	outDir := filepath.Join(r.DataDir, "reports")
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %v", outDir, err)
	}

	outPath := filepath.Join(outDir, filename+".json")
	if err := os.WriteFile(outPath, cleanJSON, 0644); err != nil {
		return "", fmt.Errorf("failed to write file %s: %v", outPath, err)
	}

	return fmt.Sprintf("Successfully generated structured report: %s", outPath), nil
}
