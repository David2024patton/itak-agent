package builtins

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGrepSearchFindsPattern(t *testing.T) {
	// Create a temp directory with a test file.
	dir := t.TempDir()
	content := "line one\nfoo bar baz\nline three\nfoo again\n"
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644)

	g := &GrepSearchTool{}

	result, err := g.Execute(context.Background(), map[string]interface{}{
		"pattern": "foo",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "foo bar baz") {
		t.Errorf("expected to find 'foo bar baz' in results, got: %s", result)
	}
	if !strings.Contains(result, "foo again") {
		t.Errorf("expected to find 'foo again' in results, got: %s", result)
	}
	if !strings.Contains(result, "2 match") {
		t.Errorf("expected 2 matches, got: %s", result)
	}
}

func TestGrepSearchNoMatches(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "empty.txt"), []byte("nothing here"), 0644)

	g := &GrepSearchTool{}

	result, err := g.Execute(context.Background(), map[string]interface{}{
		"pattern": "nonexistent-pattern-xyz",
		"path":    dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "No matches") {
		t.Errorf("expected no matches message, got: %s", result)
	}
}

func TestGrepSearchCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "test.go"), []byte("Hello World\nhello world\n"), 0644)

	g := &GrepSearchTool{}

	result, err := g.Execute(context.Background(), map[string]interface{}{
		"pattern":     "hello",
		"path":        dir,
		"ignore_case": true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "2 match") {
		t.Errorf("expected 2 case-insensitive matches, got: %s", result)
	}
}

func TestGrepSearchExtensionFilter(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "code.go"), []byte("func main() {}"), 0644)
	os.WriteFile(filepath.Join(dir, "readme.md"), []byte("func main() {}"), 0644)

	g := &GrepSearchTool{}

	result, err := g.Execute(context.Background(), map[string]interface{}{
		"pattern":    "func main",
		"path":       dir,
		"extensions": "go",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(result, "code.go") {
		t.Errorf("expected code.go in results, got: %s", result)
	}
	if strings.Contains(result, "readme.md") {
		t.Errorf("should not include readme.md when filtering for .go, got: %s", result)
	}
}

func TestGrepSearchMaxResults(t *testing.T) {
	dir := t.TempDir()
	// Create a file with many matching lines.
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = "match_line"
	}
	os.WriteFile(filepath.Join(dir, "many.txt"), []byte(strings.Join(lines, "\n")), 0644)

	g := &GrepSearchTool{}

	result, err := g.Execute(context.Background(), map[string]interface{}{
		"pattern":     "match_line",
		"path":        dir,
		"max_results": float64(5),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should say "truncated at 5".
	if !strings.Contains(result, "truncated") {
		t.Errorf("expected truncated message, got: %s", result)
	}
	// Count lines in the result.
	resultLines := strings.Split(strings.TrimSpace(result), "\n")
	// First line is header, rest are matches.
	matchLines := resultLines[1:]
	if len(matchLines) != 5 {
		t.Errorf("expected 5 match lines, got %d", len(matchLines))
	}
}

func TestGrepSearchSchema(t *testing.T) {
	g := &GrepSearchTool{}
	if g.Name() != "grep_search" {
		t.Errorf("expected name grep_search, got %s", g.Name())
	}
	schema := g.Schema()
	if schema["type"] != "object" {
		t.Error("schema type should be object")
	}
}
