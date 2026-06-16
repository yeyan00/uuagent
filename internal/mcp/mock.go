package mcp

import (
	"context"
	"encoding/json"
	"fmt"
)

// Tool is a minimal MCP-like tool descriptor used by P0 tests. It intentionally
// avoids a full MCP dependency while preserving the integration seam.
type Tool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Client is the tiny interface the agent needs from MCP providers.
type Client interface {
	ListTools(ctx context.Context) ([]Tool, error)
	CallTool(ctx context.Context, name string, args map[string]any) (string, error)
}

// MockClient is a deterministic MCP simulator for tests.
type MockClient struct{}

// NewMockClient returns a deterministic MCP simulator.
func NewMockClient() *MockClient { return &MockClient{} }

// ListTools exposes one mock tool.
func (m *MockClient) ListTools(ctx context.Context) ([]Tool, error) {
	return []Tool{{Name: "mcp_echo", Description: "Echo arguments through the mock MCP client."}}, nil
}

// CallTool executes a mock MCP tool.
func (m *MockClient) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	if name != "mcp_echo" {
		return "", fmt.Errorf("unknown mock MCP tool %q", name)
	}
	data, _ := json.Marshal(args)
	return "mock-mcp:" + string(data), nil
}
