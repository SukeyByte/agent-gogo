package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/mcp"
)

// MCPClients is the interface the tool runtime needs from an MCP client.
// internal/mcp.Client satisfies it; tests can stub it.
type MCPClients interface {
	Name() string
	Tools() []mcp.Tool
	CallTool(ctx context.Context, name string, args map[string]any) (mcp.CallResult, error)
}

// MCPToolPrefix namespaces MCP tools in the registry so they cannot collide
// with builtin tools.
const MCPToolPrefix = "mcp."

// RegisterMCPTools registers every tool exposed by the MCP server as a
// medium-risk callable tool. Tool names are prefixed with "mcp.<server>.".
func (r *Runtime) RegisterMCPTools(server MCPClients) error {
	if server == nil {
		return nil
	}
	serverName := strings.TrimSpace(server.Name())
	if serverName == "" {
		return fmt.Errorf("mcp server name is required")
	}
	tools := server.Tools()
	if len(tools) == 0 {
		return fmt.Errorf("mcp server %s exposes no tools", serverName)
	}
	for _, tool := range tools {
		toolName := tool.Name
		if strings.TrimSpace(toolName) == "" {
			continue
		}
		schema := tool.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		description := strings.TrimSpace(tool.Description)
		if description == "" {
			description = "Tool " + toolName + " from MCP server " + serverName + "."
		}
		// Servers often namespace their own tools ("notes.add"); avoid
		// doubling the server prefix into "mcp.notes.notes.add".
		registeredName := MCPToolPrefix + serverName + "." + toolName
		if prefix := serverName + "."; strings.HasPrefix(toolName, prefix) {
			registeredName = MCPToolPrefix + toolName
		}
		r.Register(Spec{
			Name:        registeredName,
			Description: description,
			RiskLevel:   "medium",
			InputSchema: schema,
		}, func(ctx context.Context, args map[string]any) (Result, error) {
			result, err := server.CallTool(ctx, toolName, args)
			if err != nil {
				return Result{Success: false, Error: err.Error()}, err
			}
			return Result{
				Success: !result.IsError,
				Output:  map[string]any{"text": result.Text()},
			}, nil
		})
	}
	return nil
}
