// Package mcp implements a minimal Model Context Protocol client so the
// agent can consume tools from external MCP servers over stdio or HTTP.
//
// It speaks JSON-RPC 2.0 with the MCP lifecycle: initialize ->
// notifications/initialized -> tools/list -> tools/call. Only the client
// subset the agent needs is implemented; server-initiated requests are
// ignored.
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ProtocolVersion is the MCP protocol version this client negotiates.
const ProtocolVersion = "2025-06-18"

// Tool describes one tool exposed by an MCP server.
type Tool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// ContentBlock is one piece of tool result content (only text is consumed).
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// CallResult is the outcome of tools/call.
type CallResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError"`
}

// Text joins the text content blocks of the result.
func (r CallResult) Text() string {
	parts := make([]string, 0, len(r.Content))
	for _, block := range r.Content {
		if block.Type == "text" && strings.TrimSpace(block.Text) != "" {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// RPCError is a JSON-RPC level error returned by the server.
type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *RPCError) Error() string {
	return fmt.Sprintf("mcp error %d: %s", e.Code, e.Message)
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error"`
}

type initializeResult struct {
	ProtocolVersion string `json:"protocolVersion"`
	ServerInfo      struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	} `json:"serverInfo"`
}

// transport delivers one JSON-RPC request and returns the full JSON-RPC
// response object. waiter receives the response for async transports.
type transport interface {
	send(ctx context.Context, request []byte, id int64, waiter chan rpcResponse) (json.RawMessage, error)
	close() error
}

// Client is a connected MCP server. CallTool is safe for concurrent use.
type Client struct {
	name      string
	transport transport

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan rpcResponse
	tools   []Tool

	serverInfo string
}

// DialStdio launches an MCP server as a child process and connects over
// newline-delimited JSON on stdin/stdout.
func DialStdio(ctx context.Context, name string, command string, args []string) (*Client, error) {
	if strings.TrimSpace(command) == "" {
		return nil, errors.New("mcp stdio transport requires a command")
	}
	cmd := exec.CommandContext(ctx, command, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start mcp server %s: %w", name, err)
	}
	client := &Client{name: name, pending: map[int64]chan rpcResponse{}}
	client.transport = &stdioTransport{cmd: cmd, stdin: stdin, stdout: stdout, client: client}
	return client, nil
}

// DialHTTP connects to an MCP server over the streamable HTTP transport.
func DialHTTP(ctx context.Context, name string, url string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(url) == "" {
		return nil, errors.New("mcp http transport requires a url")
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 120 * time.Second}
	}
	return &Client{
		name:      name,
		transport: &httpTransport{url: strings.TrimSpace(url), client: httpClient},
		pending:   map[int64]chan rpcResponse{},
	}, nil
}

// Name returns the configured server name.
func (c *Client) Name() string { return c.name }

// ServerInfo returns the server identity reported during initialization.
func (c *Client) ServerInfo() string { return c.serverInfo }

// Close shuts the transport down.
func (c *Client) Close() error {
	if c == nil || c.transport == nil {
		return nil
	}
	return c.transport.close()
}

// register reserves a request id and its response channel.
func (c *Client) register() (int64, chan rpcResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextID++
	ch := make(chan rpcResponse, 1)
	c.pending[c.nextID] = ch
	return c.nextID, ch
}

// unregister drops a pending waiter unless it was already delivered.
func (c *Client) unregister(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, id)
}

// deliver routes a decoded response to the waiting caller.
func (c *Client) deliver(resp rpcResponse) {
	c.mu.Lock()
	ch, ok := c.pending[resp.ID]
	if ok {
		delete(c.pending, resp.ID)
	}
	c.mu.Unlock()
	if ok {
		ch <- resp
	}
}

func (c *Client) call(ctx context.Context, method string, params any, result any) error {
	id, waiter := c.register()
	defer c.unregister(id)
	req := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	raw, err := c.transport.send(ctx, body, id, waiter)
	if err != nil {
		return err
	}
	var resp rpcResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("mcp %s: decode response for %s: %w", c.name, method, err)
	}
	if resp.Error != nil {
		return &RPCError{Code: resp.Error.Code, Message: resp.Error.Message}
	}
	if result != nil && len(resp.Result) > 0 {
		if err := json.Unmarshal(resp.Result, result); err != nil {
			return fmt.Errorf("mcp %s: decode result for %s: %w", c.name, method, err)
		}
	}
	return nil
}

func (c *Client) notify(ctx context.Context, method string) error {
	notifier, ok := c.transport.(notificationTransport)
	if !ok {
		return nil
	}
	body, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method})
	if err != nil {
		return err
	}
	return notifier.notify(ctx, body)
}

type notificationTransport interface {
	notify(ctx context.Context, body []byte) error
}

// Initialize performs the MCP handshake and caches the tool list.
func (c *Client) Initialize(ctx context.Context) error {
	var init initializeResult
	err := c.call(ctx, "initialize", map[string]any{
		"protocolVersion": ProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "agent-gogo", "version": "0.1.0"},
	}, &init)
	if err != nil {
		return err
	}
	if init.ServerInfo.Name != "" {
		c.serverInfo = strings.TrimSpace(init.ServerInfo.Name + " " + init.ServerInfo.Version)
	}
	if err := c.notify(ctx, "notifications/initialized"); err != nil {
		return err
	}
	return c.refreshTools(ctx)
}

func (c *Client) refreshTools(ctx context.Context) error {
	var payload struct {
		Tools      []Tool `json:"tools"`
		NextCursor string `json:"nextCursor"`
	}
	if err := c.call(ctx, "tools/list", map[string]any{}, &payload); err != nil {
		return err
	}
	tools := payload.Tools
	cursor := payload.NextCursor
	for cursor != "" && len(tools) < 200 {
		var next struct {
			Tools      []Tool `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := c.call(ctx, "tools/list", map[string]any{"cursor": cursor}, &next); err != nil {
			break
		}
		tools = append(tools, next.Tools...)
		cursor = next.NextCursor
	}
	c.mu.Lock()
	c.tools = tools
	c.mu.Unlock()
	return nil
}

// Tools returns the tools discovered during initialization.
func (c *Client) Tools() []Tool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]Tool(nil), c.tools...)
}

// CallTool invokes one tool on the server.
func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (CallResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	var result CallResult
	err := c.call(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	}, &result)
	if err != nil {
		return CallResult{}, err
	}
	if result.IsError {
		return result, fmt.Errorf("mcp %s tool %s failed: %s", c.name, name, firstLine(result.Text()))
	}
	return result, nil
}

func firstLine(text string) string {
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		return text[:idx]
	}
	return text
}

// --- stdio transport ---

type stdioTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	writeMu sync.Mutex
	client  *Client
	started sync.Once
}

func (t *stdioTransport) send(ctx context.Context, request []byte, id int64, waiter chan rpcResponse) (json.RawMessage, error) {
	t.started.Do(func() { go t.readLoop() })
	t.writeMu.Lock()
	_, err := t.stdin.Write(append(request, '\n'))
	t.writeMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("write to mcp server: %w", err)
	}
	select {
	case resp := <-waiter:
		return json.Marshal(resp)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (t *stdioTransport) notify(ctx context.Context, body []byte) error {
	t.started.Do(func() { go t.readLoop() })
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	_, err := t.stdin.Write(append(body, '\n'))
	return err
}

func (t *stdioTransport) close() error {
	_ = t.stdin.Close()
	err := t.cmd.Wait()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return nil
	}
	return err
}

func (t *stdioTransport) readLoop() {
	scanner := bufio.NewScanner(t.stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var probe struct {
			ID     *int64          `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Error  *RPCError       `json:"error"`
		}
		if err := json.Unmarshal(line, &probe); err != nil {
			continue
		}
		if probe.Method != "" || probe.ID == nil {
			// Server notification or request; the agent does not act on these.
			continue
		}
		t.client.deliver(rpcResponse{ID: *probe.ID, Result: probe.Result, Error: probe.Error})
	}
}

// --- streamable HTTP transport ---

type httpTransport struct {
	url    string
	client *http.Client
}

func (t *httpTransport) send(ctx context.Context, request []byte, id int64, waiter chan rpcResponse) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(request))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := t.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("mcp http %s: status %d: %s", t.url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return decodeHTTPResponse(bufio.NewReader(resp.Body), id)
}

func (t *httpTransport) notify(ctx context.Context, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return nil
}

func (t *httpTransport) close() error { return nil }

// decodeHTTPResponse accepts either a plain JSON response or an SSE stream
// and returns the JSON-RPC response object matching id.
func decodeHTTPResponse(reader *bufio.Reader, id int64) (json.RawMessage, error) {
	// A payload starting with '{' is plain JSON; otherwise parse SSE lines.
	if head, _ := reader.Peek(1); len(head) > 0 && head[0] == '{' {
		raw, err := io.ReadAll(io.LimitReader(reader, 8*1024*1024))
		if err != nil {
			return nil, err
		}
		return json.RawMessage(bytes.TrimSpace(raw)), nil
	}
	for {
		line, readErr := reader.ReadString('\n')
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if data == "" {
				if readErr != nil {
					break
				}
				continue
			}
			var probe struct {
				ID     *int64 `json:"id"`
				Method string `json:"method"`
			}
			if json.Unmarshal([]byte(data), &probe) != nil {
				if readErr != nil {
					break
				}
				continue
			}
			if probe.Method == "" && probe.ID != nil && *probe.ID == id {
				return json.RawMessage(data), nil
			}
		}
		if readErr != nil {
			break
		}
	}
	return nil, fmt.Errorf("mcp http: stream ended without response for id %d", id)
}
