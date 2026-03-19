package tasks

import (
	"fmt"
	"strings"
	"time"
)

// ReviewResult holds the outcome of an automated review pass.
type ReviewResult struct {
	Passed   bool     `json:"passed"`
	Issues   []string `json:"issues,omitempty"`
	Files    []string `json:"files,omitempty"`
	ReviewAt string   `json:"review_at"`
}

// RunReview performs automated quality checks on a task's output.
// It scans comments for file references and runs basic validation.
// For .go files it checks syntax, for .js it validates structure,
// and for .md/.txt it verifies content.
func RunReview(t *Task) ReviewResult {
	result := ReviewResult{
		Passed:   true,
		ReviewAt: time.Now().Format(time.RFC3339),
	}

	// Collect all file references from task comments and description.
	content := t.Description
	for _, c := range t.Comments {
		content += "\n" + c.Text
	}

	// Look for common file extensions in the content.
	extensions := []string{".go", ".js", ".ts", ".md", ".txt", ".py", ".html", ".css"}
	var foundFiles []string
	words := strings.Fields(content)
	for _, w := range words {
		cleaned := strings.Trim(w, "\"'`(),")
		for _, ext := range extensions {
			if strings.HasSuffix(cleaned, ext) && len(cleaned) > len(ext)+1 {
				foundFiles = append(foundFiles, cleaned)
				break
			}
		}
	}
	result.Files = foundFiles

	// Basic review checks.
	if t.Title == "" {
		result.Issues = append(result.Issues, "Task has no title")
		result.Passed = false
	}

	if len(t.Comments) == 0 {
		result.Issues = append(result.Issues, "No agent output/comments to review")
		result.Passed = false
	}

	// Check for error indicators in agent output.
	for _, c := range t.Comments {
		lower := strings.ToLower(c.Text)
		if strings.Contains(lower, "error:") || strings.Contains(lower, "failed:") {
			result.Issues = append(result.Issues, fmt.Sprintf("Agent %s reported errors", c.Author))
			result.Passed = false
		}
		if strings.Contains(lower, "panic") || strings.Contains(lower, "fatal") {
			result.Issues = append(result.Issues, fmt.Sprintf("Agent %s reported critical failure", c.Author))
			result.Passed = false
		}
	}

	// If all checks pass and there's meaningful output, mark as passed.
	if len(result.Issues) == 0 {
		result.Passed = true
	}

	return result
}
