package tools

import "context"

// Tool defines the interface that all medical tools must implement.
// Each tool provides a JSON Schema for Claude's function calling and
// an Execute method that performs the actual computation.
type Tool interface {
	// Name returns the unique identifier for this tool.
	Name() string

	// Description returns a human-readable description of what the tool does.
	// This is included in the system prompt and tool definition for Claude.
	Description() string

	// Schema returns the JSON Schema for the tool's input parameters.
	// This is passed to Claude as part of the function definition.
	Schema() map[string]interface{}

	// Execute runs the tool with the given input and returns structured output.
	// The input map is the parsed JSON arguments from Claude's tool call.
	Execute(ctx context.Context, input map[string]interface{}) (*ToolResult, error)
}

// ToolResult wraps the output of a tool execution.
// Every result includes citations to enable evidence tracing.
type ToolResult struct {
	Success   bool                   `json:"success"`
	Data      map[string]interface{} `json:"data,omitempty"`
	Error     string                 `json:"error,omitempty"`
	Citations []CitationRef          `json:"citations,omitempty"`
}

// CitationRef is a lightweight citation reference included in tool results.
type CitationRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	DOI   string `json:"doi,omitempty"`
	PMID  string `json:"pmid,omitempty"`
	Level string `json:"level"`
	Year  int    `json:"year"`
}
