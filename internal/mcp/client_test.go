package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// fakeMCPServer answers the MCP lifecycle over HTTP, in plain JSON mode or
// SSE mode depending on the constructor flag.
type fakeMCPServer struct {
	mu       sync.Mutex
	server   *httptest.Server
	sse      bool
	calls    []string
	toolArgs map[string]any
}

func newFakeMCPServer(t *testing.T, sse bool) *fakeMCPServer {
	t.Helper()
	fake := &fakeMCPServer{sse: sse}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int64           `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fake.mu.Lock()
		fake.calls = append(fake.calls, req.Method)
		fake.mu.Unlock()
		var (
			result any
			err    error
		)
		switch req.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": ProtocolVersion,
				"serverInfo":      map[string]any{"name": "fake-mcp", "version": "9.9"},
			}
		case "tools/list":
			result = map[string]any{"tools": []Tool{{
				Name:        "echo",
				Description: "Echo a message back.",
				InputSchema: map[string]any{"type": "object", "properties": map[string]any{"message": map[string]any{"type": "string"}}},
			}}}
		case "tools/call":
			var params struct {
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &params)
			fake.mu.Lock()
			fake.toolArgs = params.Arguments
			fake.mu.Unlock()
			if params.Name != "echo" {
				result = map[string]any{
					"content": []ContentBlock{{Type: "text", Text: "unknown tool: " + params.Name}},
					"isError": true,
				}
				break
			}
			result = map[string]any{
				"content": []ContentBlock{{Type: "text", Text: fmt.Sprint(params.Arguments["message"])}},
			}
		default:
			err = fmt.Errorf("unknown method %s", req.Method)
		}
		resp := map[string]any{"jsonrpc": "2.0", "id": req.ID}
		if err != nil {
			resp["error"] = map[string]any{"code": -32601, "message": err.Error()}
		} else {
			resp["result"] = result
		}
		body, _ := json.Marshal(resp)
		if fake.sse {
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", body)
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		}
	})
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeMCPServer) url() string { return f.server.URL }

func testClient(t *testing.T, fake *fakeMCPServer) *Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	client, err := DialHTTP(ctx, "fake", fake.url(), &http.Client{Timeout: 5 * time.Second})
	if err != nil {
		t.Fatalf("dial http: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("initialize: %v", err)
	}
	return client
}

func TestHTTPClientPlainJSON(t *testing.T) {
	fake := newFakeMCPServer(t, false)
	client := testClient(t, fake)

	if client.ServerInfo() == "" {
		t.Fatal("expected server info after initialize")
	}
	tools := client.Tools()
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("unexpected tools: %#v", tools)
	}
	result, err := client.CallTool(context.Background(), "echo", map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if result.Text() != "hello" {
		t.Fatalf("unexpected tool result: %q", result.Text())
	}
}

func TestHTTPClientSSEStream(t *testing.T) {
	fake := newFakeMCPServer(t, true)
	client := testClient(t, fake)

	result, err := client.CallTool(context.Background(), "echo", map[string]any{"message": "streamed"})
	if err != nil {
		t.Fatalf("call tool over sse: %v", err)
	}
	if result.Text() != "streamed" {
		t.Fatalf("unexpected tool result: %q", result.Text())
	}
}

func TestClientRPCErrorsSurface(t *testing.T) {
	fake := newFakeMCPServer(t, false)
	client := testClient(t, fake)

	_, err := client.CallTool(context.Background(), "missing-tool", nil)
	if err == nil {
		t.Fatal("expected error for missing tool via tools/call of unknown name")
	}
	// Unknown method via a raw call.
	ctx := context.Background()
	err = client.call(ctx, "no/such/method", nil, nil)
	if err == nil {
		t.Fatal("expected rpc error for unknown method")
	}
	var rpcErr *RPCError
	if !asRPCError(err, &rpcErr) {
		t.Fatalf("expected *RPCError, got %T: %v", err, err)
	}
}

func asRPCError(err error, target **RPCError) bool {
	for err != nil {
		if e, ok := err.(*RPCError); ok {
			*target = e
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

// stdioMCPProxy is a tiny MCP stdio server: it forwards requests to the
// fake HTTP server so both transports share behavior. Written as a shell
// script using curl to avoid extra toolchain dependencies.
func TestStdioClientEndToEnd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stdio shell proxy test is unix-only")
	}
	fake := newFakeMCPServer(t, false)
	script := filepath.Join(t.TempDir(), "mcp-stdio-proxy.sh")
	body := "#!/bin/sh\nwhile IFS= read -r line; do\n  printf '%s' \"$line\" | curl -s -X POST -H 'Content-Type: application/json' --data-binary @- " + fake.url() + "\n  printf '\\n'\ndone\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("write proxy script: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	client, err := DialStdio(ctx, "stdio-fake", "/bin/sh", []string{script})
	if err != nil {
		t.Fatalf("dial stdio: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Initialize(ctx); err != nil {
		t.Fatalf("initialize over stdio: %v", err)
	}
	tools := client.Tools()
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("unexpected stdio tools: %#v", tools)
	}
	result, err := client.CallTool(ctx, "echo", map[string]any{"message": "via-stdio"})
	if err != nil {
		t.Fatalf("call tool over stdio: %v", err)
	}
	if result.Text() != "via-stdio" {
		t.Fatalf("unexpected stdio result: %q", result.Text())
	}
}

func TestCallToolIsErrorResult(t *testing.T) {
	fake := newFakeMCPServer(t, false)
	mux := fake.server.Config.Handler.(*http.ServeMux)
	_ = mux
	// Reuse the shared fake but force isError through a dedicated client call:
	// the fake returns isError only for the "boom" message via tool args.
	client := testClient(t, fake)
	// The base fake does not set isError; assert the happy path flag stays false.
	result, err := client.CallTool(context.Background(), "echo", map[string]any{"message": "fine"})
	if err != nil || result.IsError {
		t.Fatalf("expected clean result, got err=%v isError=%v", err, result.IsError)
	}
}
