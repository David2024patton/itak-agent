package agent

import (
	"testing"
)

var testTools = []string{"shell", "file_read", "file_write", "http_fetch", "dir_list", "grep_search"}

func TestParseTextToolCalls_Empty(t *testing.T) {
	calls := parseTextToolCalls("", testTools)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls from empty string, got %d", len(calls))
	}
}

func TestParseTextToolCalls_NoToolCalls(t *testing.T) {
	calls := parseTextToolCalls("Here is the directory listing:\n- file1.txt\n- file2.go", testTools)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls from plain text, got %d", len(calls))
	}
}

func TestParseTextToolCalls_SingleJSON_ToolKey(t *testing.T) {
	content := `I'll list the files for you.
{"tool": "shell", "arguments": {"command": "ls -la"}}
`
	calls := parseTextToolCalls(content, testTools)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "shell" {
		t.Errorf("expected tool=shell, got %s", calls[0].Function.Name)
	}
}

func TestParseTextToolCalls_SingleJSON_NameKey(t *testing.T) {
	content := `{"name": "file_read", "arguments": {"path": "/tmp/test.txt"}}`
	calls := parseTextToolCalls(content, testTools)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Name != "file_read" {
		t.Errorf("expected tool=file_read, got %s", calls[0].Function.Name)
	}
}

func TestParseTextToolCalls_ParametersAlias(t *testing.T) {
	content := `{"tool": "shell", "parameters": {"command": "echo hello"}}`
	calls := parseTextToolCalls(content, testTools)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].Function.Arguments != `{"command":"echo hello"}` {
		t.Errorf("unexpected args: %s", calls[0].Function.Arguments)
	}
}

func TestParseTextToolCalls_ArgsAlias(t *testing.T) {
	content := `{"tool": "dir_list", "args": {"path": "."}}`
	calls := parseTextToolCalls(content, testTools)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
}

func TestParseTextToolCalls_MarkdownFenced(t *testing.T) {
	content := "```json\n{\"tool\": \"grep_search\", \"arguments\": {\"pattern\": \"func main\", \"path\": \".\"}}\n```"
	calls := parseTextToolCalls(content, testTools)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call from markdown-fenced JSON, got %d", len(calls))
	}
	if calls[0].Function.Name != "grep_search" {
		t.Errorf("expected grep_search, got %s", calls[0].Function.Name)
	}
}

func TestParseTextToolCalls_JSONArray(t *testing.T) {
	content := `I'll run two commands:
[
  {"tool": "shell", "arguments": {"command": "go build"}},
  {"tool": "shell", "arguments": {"command": "go test"}}
]`
	calls := parseTextToolCalls(content, testTools)
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls from JSON array, got %d", len(calls))
	}
}

func TestParseTextToolCalls_TaggedFormat(t *testing.T) {
	content := `Let me check the file.
<tool_call>{"name": "file_read", "arguments": {"path": "go.mod"}}</tool_call>
Then I'll check another:
<tool_call>{"name": "file_read", "arguments": {"path": "go.sum"}}</tool_call>`

	calls := parseTextToolCalls(content, testTools)
	if len(calls) != 2 {
		t.Fatalf("expected 2 tagged calls, got %d", len(calls))
	}
	if calls[0].Function.Name != "file_read" {
		t.Errorf("call 0: expected file_read, got %s", calls[0].Function.Name)
	}
}

func TestParseTextToolCalls_UnknownToolIgnored(t *testing.T) {
	content := `{"tool": "hack_system", "arguments": {"target": "root"}}`
	calls := parseTextToolCalls(content, testTools)
	if len(calls) != 0 {
		t.Errorf("expected 0 calls for unknown tool, got %d", len(calls))
	}
}

func TestParseTextToolCalls_MixedValidInvalid(t *testing.T) {
	content := `[
		{"tool": "shell", "arguments": {"command": "ls"}},
		{"tool": "nuke_everything", "arguments": {}},
		{"tool": "file_read", "arguments": {"path": "test.txt"}}
	]`
	calls := parseTextToolCalls(content, testTools)
	if len(calls) != 2 {
		t.Fatalf("expected 2 valid calls (ignoring unknown), got %d", len(calls))
	}
}

func TestParseTextToolCalls_HasToolCallID(t *testing.T) {
	content := `{"tool": "shell", "arguments": {"command": "echo hi"}}`
	calls := parseTextToolCalls(content, testTools)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(calls))
	}
	if calls[0].ID == "" {
		t.Error("expected non-empty tool call ID")
	}
	if calls[0].Type != "function" {
		t.Errorf("expected type=function, got %s", calls[0].Type)
	}
}

func TestParseTextToolCalls_SurroundingGarbage(t *testing.T) {
	content := `Sure! I will use the shell tool to list files.

Here is my tool call:

{"tool": "shell", "arguments": {"command": "dir"}}

This will show us the current directory contents.`

	calls := parseTextToolCalls(content, testTools)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call despite surrounding text, got %d", len(calls))
	}
}

func TestParseTextToolCalls_ThinkingTags(t *testing.T) {
	content := `<think>
I need to list the directory. I should use the dir_list tool with path ".".
</think>
{"tool": "dir_list", "arguments": {"path": "."}}`

	calls := parseTextToolCalls(content, testTools)
	if len(calls) != 1 {
		t.Fatalf("expected 1 call after stripping think tags, got %d", len(calls))
	}
	if calls[0].Function.Name != "dir_list" {
		t.Errorf("expected dir_list, got %s", calls[0].Function.Name)
	}
	// Make sure the path is "." not "/think".
	if calls[0].Function.Arguments != `{"path":"."}` {
		t.Errorf("expected path=., got %s", calls[0].Function.Arguments)
	}
}

func TestStripThinkingTags(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no tags", "hello world", "hello world"},
		{"simple", "<think>reason</think>answer", "answer"},
		{"multiline", "<think>\nI think...\nstep 1\n</think>\n{\"ok\": true}", "{\"ok\": true}"},
		{"unclosed", "<think>still thinking", ""},
		{"empty after strip", "<think>just thinking</think>", ""},
		{"multiple", "<think>a</think>mid<think>b</think>end", "midend"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripThinkingTags(tt.input)
			if got != tt.expected {
				t.Errorf("stripThinkingTags(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
