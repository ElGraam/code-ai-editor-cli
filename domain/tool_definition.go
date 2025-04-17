package domain

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
)

// ToolDefinition represents a tool that can be used by the agent.
// It includes the tool's name, a description of what it does,
// the schema for the input it expects, and the function to execute
// when the tool is called.
type ToolDefinition struct {
	Name        string                         `json:"name"`
	Description string                         `json:"description"`
	InputSchema anthropic.ToolInputSchemaParam `json:"input_schema"`
	Function    func(input json.RawMessage) (string, error)
}

// ToolRepository defines the interface for interacting with tools.
// It provides methods for retrieving, finding, and executing tools.
type ToolRepository interface {
	GetAllTools() []ToolDefinition

	FindToolByName(name string) (ToolDefinition, bool)

	ExecuteTool(id, name string, input json.RawMessage) anthropic.ContentBlockParamUnion
}
