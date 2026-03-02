package tool

import "context"

// Tool is the interface every GOAgent tool must implement.
type Tool interface {
	// Name returns the unique tool identifier.
	Name() string

	// Description returns a human-readable description for the LLM.
	Description() string

	// Schema returns the JSON Schema for the tool's parameters.
	// This gets passed to the LLM as the function calling schema.
	Schema() map[string]interface{}

	// Execute runs the tool with the given arguments and returns the result.
	Execute(ctx context.Context, args map[string]interface{}) (string, error)
}
