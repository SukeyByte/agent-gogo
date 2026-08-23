package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	appconfig "github.com/SukeyByte/agent-gogo/internal/config"
	"github.com/SukeyByte/agent-gogo/internal/tools"
)

func TestConnectMCPServersSkipsBrokenServers(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix paths in test")
	}
	dir := t.TempDir()
	// A working echo MCP server script: responds to initialize and tools/list.
	script := filepath.Join(dir, "echo-mcp.sh")
	body := `#!/bin/sh
while IFS= read -r line; do
  case "$line" in
    *initialize*)
      printf '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-06-18","serverInfo":{"name":"echo","version":"1.0"}}}\n' ;;
    *tools/list*)
      printf '{"jsonrpc":"2.0","id":2,"result":{"tools":[{"name":"ping","description":"ping"}]}}\n' ;;
    *initialized*)
      : ;;
    *ping*)
      printf '{"jsonrpc":"2.0","id":3,"result":{"content":[{"type":"text","text":"pong"}]}}\n' ;;
  esac
done
`
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	cfg := appconfig.Config{}
	cfg.MCPServers = []appconfig.MCPServerConfig{
		{Name: "broken", Command: "/nonexistent/mcp-server", Enabled: true},
		{Name: "off", URL: "http://127.0.0.1:1/mcp", Enabled: false},
		{Name: "echo", Command: script, Enabled: true},
	}
	runtimeTools := tools.NewBuiltinRuntime(nil, dir)
	var warnings []string
	closer := connectMCPServers(context.Background(), cfg, runtimeTools, func(format string, args ...any) {
		warnings = append(warnings, strings.TrimSpace(fmt.Sprintf(format, args...)))
	})
	defer closer()

	if len(warnings) != 1 || !strings.Contains(warnings[0], "broken") {
		t.Fatalf("expected exactly one warning about 'broken', got %#v", warnings)
	}
	specs := runtimeTools.ListSpecs()
	found := false
	for _, spec := range specs {
		if spec.Name == "mcp.echo.ping" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected mcp.echo.ping registered; specs: %d", len(specs))
	}
	resp, err := runtimeTools.Call(context.Background(), tools.CallRequest{Name: "mcp.echo.ping"})
	if err != nil {
		t.Fatalf("call mcp.echo.ping: %v", err)
	}
	if text, _ := resp.Result.Output["text"].(string); text != "pong" {
		t.Fatalf("unexpected ping output: %#v", resp.Result.Output)
	}
}
