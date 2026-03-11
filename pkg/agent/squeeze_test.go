package agent

import (
	"strings"
	"testing"

	"github.com/David2024patton/iTaKAgent/pkg/llm"
)

func TestSummarizeToolOutputShortInput(t *testing.T) {
	// Short input should pass through unchanged.
	input := "hello world"
	got := SummarizeToolOutput(input, 100)
	if got != input {
		t.Errorf("expected unchanged output for short input, got %q", got)
	}
}

func TestSummarizeToolOutputTruncation(t *testing.T) {
	// Long input should be truncated to roughly maxChars.
	input := strings.Repeat("x", 1000)
	got := SummarizeToolOutput(input, 200)
	if len(got) > 250 { // some overhead from summary format
		t.Errorf("expected output <= 250 chars, got %d", len(got))
	}
}

func TestSummarizeToolOutputDisabled(t *testing.T) {
	input := "some output"
	// maxChars=0 means disabled.
	got := SummarizeToolOutput(input, 0)
	if got != input {
		t.Errorf("expected unchanged output when disabled, got %q", got)
	}
}

func TestValidateJSONResponseClean(t *testing.T) {
	input := `{"tool": "shell", "arguments": {"command": "ls"}}`
	got, err := ValidateJSONResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != input {
		t.Errorf("expected unchanged for valid JSON, got %q", got)
	}
}

func TestValidateJSONResponseCodeFence(t *testing.T) {
	input := "```json\n{\"tool\": \"shell\"}\n```"
	got, err := ValidateJSONResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(got, "{") {
		t.Errorf("expected JSON object, got %q", got)
	}
}

func TestValidateJSONResponseTrailingComma(t *testing.T) {
	input := `{"tool": "shell", "arguments": {"a": 1,}}`
	got, err := ValidateJSONResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, ",}") {
		t.Errorf("expected trailing comma to be removed, got %q", got)
	}
}

func TestValidateJSONResponseThinkTags(t *testing.T) {
	input := "<think>thinking about this</think>{\"tool\": \"shell\"}"
	got, err := ValidateJSONResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(got, "think") {
		t.Errorf("expected think tags to be stripped, got %q", got)
	}
}

func TestValidateJSONResponseEmpty(t *testing.T) {
	_, err := ValidateJSONResponse("")
	if err == nil {
		t.Error("expected error for empty input")
	}
}

func TestValidateJSONResponseNoJSON(t *testing.T) {
	_, err := ValidateJSONResponse("just some plain text without any json")
	if err == nil {
		t.Error("expected error for non-JSON input")
	}
}

func TestCompressContextFitsInBudget(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "sys"},
		{Role: llm.RoleUser, Content: "hello"},
	}
	result := CompressContext(msgs, 10000)
	if len(result) != 2 {
		t.Errorf("expected 2 messages, got %d", len(result))
	}
}

func TestCompressContextTruncates(t *testing.T) {
	msgs := []llm.Message{
		{Role: llm.RoleSystem, Content: "system prompt"},
		{Role: llm.RoleUser, Content: "first"},
		{Role: llm.RoleAssistant, Content: strings.Repeat("long response ", 100)},
		{Role: llm.RoleUser, Content: "second"},
		{Role: llm.RoleAssistant, Content: strings.Repeat("another long response ", 100)},
		{Role: llm.RoleUser, Content: "latest question"},
	}
	result := CompressContext(msgs, 200)

	// Should keep system prompt and latest user message.
	if result[0].Role != llm.RoleSystem {
		t.Error("first message should be system prompt")
	}
	if result[len(result)-1].Content != "latest question" {
		t.Error("last message should be latest user question")
	}
}
