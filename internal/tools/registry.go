package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"

	"github.com/doctor-agent/internal/llm"
)

// Registry manages the collection of available medical tools.
// It generates ToolParam definitions for Claude's function calling
// and dispatches tool calls to the appropriate handler.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
	order []string // preserved insertion order for determinism
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
		order: make([]string, 0),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := tool.Name()
	if _, ok := r.tools[name]; !ok {
		r.order = append(r.order, name)
	}
	r.tools[name] = tool
}

// GetToolDefinitions returns Anthropic-compatible tool definitions
// for all registered tools.
func (r *Registry) GetToolDefinitions() []anthropic.ToolUnionParam {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]anthropic.ToolUnionParam, 0, len(r.order))
	for _, name := range r.order {
		tool := r.tools[name]
		schema := tool.Schema()

		toolParam := anthropic.ToolParam{
			Name:        tool.Name(),
			Description: param.Opt[string]{Value: tool.Description()},
			InputSchema: anthropic.ToolInputSchemaParam{
				Type:       constant.Object("object"),
				Properties: schema["properties"],
				Required:   toStringSlice(schema["required"]),
			},
		}

		defs = append(defs, anthropic.ToolUnionParam{
			OfTool: &toolParam,
		})
	}
	return defs
}

// Dispatch routes a tool call to the appropriate handler and returns the result.
func (r *Registry) Dispatch(ctx context.Context, name string, input map[string]any) (*ToolResult, error) {
	r.mu.RLock()
	tool, ok := r.tools[name]
	r.mu.RUnlock()

	if !ok {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("unknown tool: %s", name),
		}, fmt.Errorf("unknown tool: %s", name)
	}

	return tool.Execute(ctx, input)
}

// GetGenericToolDefinitions returns provider-agnostic tool definitions
// compatible with the LLMProvider interface.
func (r *Registry) GetGenericToolDefinitions() []llm.ToolDefinition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	defs := make([]llm.ToolDefinition, 0, len(r.order))
	for _, name := range r.order {
		tool := r.tools[name]
		schema := tool.Schema()

		required := toStringSlice(schema["required"])

		props, _ := schema["properties"].(map[string]any)

		defs = append(defs, llm.ToolDefinition{
			Name:        name,
			Description: tool.Description(),
			Parameters:  props,
			Required:    required,
		})
	}
	return defs
}

// GetTool returns a registered tool by name.
func (r *Registry) GetTool(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	return tool, ok
}

// ListTools returns the names of all registered tools.
func (r *Registry) ListTools() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, len(r.order))
	copy(names, r.order)
	return names
}

// GetToolDescriptions returns human-readable descriptions for all tools.
func (r *Registry) GetToolDescriptions() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	descs := make([]string, 0, len(r.order))
	for _, name := range r.order {
		tool := r.tools[name]
		descs = append(descs, fmt.Sprintf("**%s**: %s", tool.Name(), tool.Description()))
	}
	return descs
}

// toStringSlice converts an interface{} that should be a []string to []string.
func toStringSlice(v any) []string {
	if v == nil {
		return nil
	}
	switch items := v.(type) {
	case []string:
		return items
	case []any:
		result := make([]string, 0, len(items))
		for _, item := range items {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	default:
		return nil
	}
}
