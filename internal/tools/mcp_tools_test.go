package tools

import (
	"context"
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/mcp"
)

type stubMCPServer struct {
	name  string
	tools []mcp.Tool
	calls []string
}

func (s *stubMCPServer) Name() string { return s.name }

func (s *stubMCPServer) Tools() []mcp.Tool { return s.tools }

func (s *stubMCPServer) CallTool(ctx context.Context, name string, args map[string]any) (mcp.CallResult, error) {
	s.calls = append(s.calls, name)
	return mcp.CallResult{Content: []mcp.ContentBlock{{Type: "text", Text: "ok:" + name}}}, nil
}

func TestRegisterMCPToolsNamespacesAndDispatches(t *testing.T) {
	runtime := NewBuiltinRuntime(nil, t.TempDir())
	server := &stubMCPServer{
		name: "calc",
		tools: []mcp.Tool{
			{
				Name:        "add",
				Description: "Add numbers.",
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{"a": map[string]any{"type": "number"}}},
			},
			{Name: "sub", Description: "", InputSchema: nil},
		},
	}
	if err := runtime.RegisterMCPTools(server); err != nil {
		t.Fatalf("register mcp tools: %v", err)
	}

	specs := runtime.ListSpecs()
	names := map[string]Spec{}
	for _, spec := range specs {
		names[spec.Name] = spec
	}
	add, ok := names["mcp.calc.add"]
	if !ok {
		t.Fatalf("expected mcp.calc.add registered, got %d specs", len(specs))
	}
	if add.RiskLevel != "medium" || add.InputSchema == nil {
		t.Fatalf("unexpected add spec: %#v", add)
	}
	sub, ok := names["mcp.calc.sub"]
	if !ok || sub.Description == "" || sub.InputSchema["type"] != "object" {
		t.Fatalf("expected defaulted sub spec: %#v", sub)
	}

	resp, err := runtime.Call(context.Background(), CallRequest{Name: "mcp.calc.add", Args: map[string]any{"a": 1}})
	if err != nil {
		t.Fatalf("call mcp tool: %v", err)
	}
	if !resp.Result.Success {
		t.Fatalf("expected success, got %#v", resp.Result)
	}
	if text, _ := resp.Result.Output["text"].(string); text != "ok:add" {
		t.Fatalf("unexpected output: %#v", resp.Result.Output)
	}
	if len(server.calls) != 1 || server.calls[0] != "add" {
		t.Fatalf("unexpected server calls: %v", server.calls)
	}
}

func TestRegisterMCPToolsRejectsEmptyServer(t *testing.T) {
	runtime := NewBuiltinRuntime(nil, t.TempDir())
	if err := runtime.RegisterMCPTools(&stubMCPServer{name: "x"}); err == nil {
		t.Fatal("expected error for server with no tools")
	}
	if err := runtime.RegisterMCPTools(nil); err != nil {
		t.Fatalf("nil server should be a no-op, got %v", err)
	}
}
