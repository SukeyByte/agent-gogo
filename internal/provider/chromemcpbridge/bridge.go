package chromemcpbridge

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Config struct {
	DebugPort        int
	ChromePath       string
	UserDataDir      string
	Headless         bool
	MaxSummaryLength int
}

type Bridge struct {
	config Config
	client *http.Client
	mu     sync.Mutex
	cmd    *exec.Cmd
	last   target
}

func New(config Config) *Bridge {
	if config.DebugPort == 0 {
		config.DebugPort = 9223
	}
	if config.ChromePath == "" {
		config.ChromePath = defaultChromePath()
	}
	if config.UserDataDir == "" {
		config.UserDataDir = filepath.Join(os.TempDir(), "agent-gogo-chrome-mcp-profile")
	}
	if config.MaxSummaryLength <= 0 {
		config.MaxSummaryLength = 12000
	}
	return &Bridge{
		config: config,
		client: &http.Client{Timeout: 20 * time.Second},
	}
}

func (b *Bridge) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/browser/call", b.handleCall)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return mux
}

func (b *Bridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cmd == nil || b.cmd.Process == nil {
		return nil
	}
	return b.cmd.Process.Kill()
}

func (b *Bridge) handleCall(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req callRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	result, err := b.call(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (b *Bridge) call(ctx context.Context, req callRequest) (browserResult, error) {
	switch req.Action {
	case "open":
		rawURL, _ := req.Args["url"].(string)
		return b.open(ctx, rawURL)
	case "click":
		text, _ := req.Args["text"].(string)
		return b.click(ctx, text)
	case "type":
		text, _ := req.Args["text"].(string)
		return b.typeText(ctx, text)
	case "input":
		selector, _ := req.Args["selector"].(string)
		value, _ := req.Args["value"].(string)
		return b.input(ctx, selector, value)
	case "wait":
		text, _ := req.Args["text"].(string)
		return b.wait(ctx, text, timeoutMSFromArg(req.Args["timeout_ms"], 10000))
	case "extract":
		query, _ := req.Args["query"].(string)
		return b.extract(ctx, query)
	case "dom_summary":
		return b.domSummary(ctx)
	case "screenshot":
		return b.screenshot(ctx)
	default:
		return browserResult{}, fmt.Errorf("unsupported browser action %q", req.Action)
	}
}

func (b *Bridge) chromeRunningLocked() bool {
	if b.cmd == nil || b.cmd.Process == nil {
		return false
	}
	if b.cmd.ProcessState != nil && b.cmd.ProcessState.Exited() {
		return false
	}
	return b.cmd.Process.Signal(syscall.Signal(0)) == nil
}

func (b *Bridge) version(ctx context.Context) (map[string]any, error) {
	var out map[string]any
	if err := b.getJSON(ctx, "/json/version", &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (b *Bridge) newTarget(ctx context.Context) (target, error) {
	var created target
	path := "/json/new?about:blank"
	if err := b.requestJSON(ctx, http.MethodPut, path, &created); err != nil {
		if fallbackErr := b.requestJSON(ctx, http.MethodGet, path, &created); fallbackErr != nil {
			return target{}, err
		}
	}
	return created, nil
}

func (b *Bridge) navigate(ctx context.Context, websocketURL string, rawURL string) error {
	session, err := dialCDP(ctx, websocketURL)
	if err != nil {
		return err
	}
	defer session.Close()
	if _, err := session.Call(ctx, "Page.enable", nil); err != nil {
		return err
	}
	_, _ = session.Call(ctx, "Page.bringToFront", nil)
	_, err = session.Call(ctx, "Page.navigate", map[string]any{"url": rawURL})
	return err
}

func (b *Bridge) bringPageToFront(ctx context.Context, websocketURL string) error {
	session, err := dialCDP(ctx, websocketURL)
	if err != nil {
		return err
	}
	defer session.Close()
	_, err = session.Call(ctx, "Page.bringToFront", nil)
	return err
}

func (b *Bridge) bringChromeWindowToFront(ctx context.Context, targetURL string) error {
	if b.config.Headless || runtime.GOOS != "darwin" || strings.TrimSpace(targetURL) == "" {
		return nil
	}
	script := `
on run argv
  set targetURL to item 1 of argv
  tell application "Google Chrome"
    activate
    repeat with w in windows
      set tabCount to count of tabs of w
      repeat with i from 1 to tabCount
        if URL of tab i of w is targetURL then
          set active tab index of w to i
          set index of w to 1
          return
        end if
      end repeat
    end repeat
  end tell
end run`
	return exec.CommandContext(ctx, "osascript", "-e", script, targetURL).Run()
}

func (b *Bridge) waitReady(ctx context.Context, websocketURL string) error {
	deadline := time.Now().Add(15 * time.Second)
	for {
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for page load")
		}
		session, err := dialCDP(ctx, websocketURL)
		if err != nil {
			return err
		}
		response, err := session.Call(ctx, "Runtime.evaluate", map[string]any{
			"expression":    "document.readyState",
			"returnByValue": true,
		})
		_ = session.Close()
		if err == nil {
			if result, _ := response["result"].(map[string]any); result != nil {
				if value, _ := result["value"].(string); value == "complete" || value == "interactive" {
					return nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func (b *Bridge) requestJSON(ctx context.Context, method string, path string, out any) error {
	endpoint := fmt.Sprintf("http://127.0.0.1:%d%s", b.config.DebugPort, path)
	req, err := http.NewRequestWithContext(ctx, method, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("chrome devtools request failed: status=%d body=%s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, out)
}

type callRequest struct {
	Action string         `json:"action"`
	Args   map[string]any `json:"args"`
}

type browserResult struct {
	URL           string            `json:"URL"`
	DOMSummary    string            `json:"DOMSummary"`
	ScreenshotRef string            `json:"ScreenshotRef"`
	Metadata      map[string]string `json:"Metadata"`
}

type target struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	URL                  string `json:"url"`
	WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
}

type cdpSession struct {
	conn net.Conn
	rw   *bufio.ReadWriter
	next int
	mu   sync.Mutex
}

func dialCDP(ctx context.Context, rawURL string) (*cdpSession, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	if parsed.Scheme != "ws" {
		return nil, fmt.Errorf("unsupported websocket scheme %q", parsed.Scheme)
	}
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", parsed.Host)
	if err != nil {
		return nil, err
	}
	keyBytes := make([]byte, 16)
	if _, err := rand.Read(keyBytes); err != nil {
		_ = conn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyBytes)
	path := parsed.RequestURI()
	if path == "" {
		path = "/"
	}
	request := fmt.Sprintf("GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\n\r\n", path, parsed.Host, key)
	if _, err := conn.Write([]byte(request)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	status, err := rw.ReadString('\n')
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	if !strings.Contains(status, "101") {
		_ = conn.Close()
		return nil, fmt.Errorf("websocket upgrade failed: %s", strings.TrimSpace(status))
	}
	for {
		line, err := rw.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		if strings.TrimSpace(line) == "" {
			break
		}
	}
	return &cdpSession{conn: conn, rw: rw}, nil
}

func (s *cdpSession) Close() error {
	return s.conn.Close()
}

func (s *cdpSession) Call(ctx context.Context, method string, params map[string]any) (map[string]any, error) {
	s.mu.Lock()
	s.next++
	id := s.next
	s.mu.Unlock()
	payload := map[string]any{"id": id, "method": method}
	if params != nil {
		payload["params"] = params
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = s.conn.SetDeadline(deadline)
	} else {
		_ = s.conn.SetDeadline(time.Now().Add(20 * time.Second))
	}
	if err := writeWSFrame(s.rw, data); err != nil {
		return nil, err
	}
	for {
		message, err := readWSFrame(s.rw)
		if err != nil {
			return nil, err
		}
		var response map[string]any
		if err := json.Unmarshal(message, &response); err != nil {
			return nil, err
		}
		responseID, _ := response["id"].(float64)
		if int(responseID) != id {
			continue
		}
		if rawError, ok := response["error"]; ok {
			return nil, fmt.Errorf("cdp %s failed: %v", method, rawError)
		}
		result, _ := response["result"].(map[string]any)
		return result, nil
	}
}

func writeWSFrame(rw *bufio.ReadWriter, payload []byte) error {
	var frame bytes.Buffer
	frame.WriteByte(0x81)
	length := len(payload)
	switch {
	case length < 126:
		frame.WriteByte(byte(0x80 | length))
	case length <= 65535:
		frame.WriteByte(0x80 | 126)
		_ = binary.Write(&frame, binary.BigEndian, uint16(length))
	default:
		frame.WriteByte(0x80 | 127)
		_ = binary.Write(&frame, binary.BigEndian, uint64(length))
	}
	mask := make([]byte, 4)
	if _, err := rand.Read(mask); err != nil {
		return err
	}
	frame.Write(mask)
	for i, b := range payload {
		frame.WriteByte(b ^ mask[i%4])
	}
	if _, err := rw.Write(frame.Bytes()); err != nil {
		return err
	}
	return rw.Flush()
}

func timeoutMSFromArg(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func readWSFrame(rw *bufio.ReadWriter) ([]byte, error) {
	header := make([]byte, 2)
	if _, err := io.ReadFull(rw, header); err != nil {
		return nil, err
	}
	opcode := header[0] & 0x0f
	length := uint64(header[1] & 0x7f)
	switch length {
	case 126:
		var n uint16
		if err := binary.Read(rw, binary.BigEndian, &n); err != nil {
			return nil, err
		}
		length = uint64(n)
	case 127:
		if err := binary.Read(rw, binary.BigEndian, &length); err != nil {
			return nil, err
		}
	}
	masked := header[1]&0x80 != 0
	var mask []byte
	if masked {
		mask = make([]byte, 4)
		if _, err := io.ReadFull(rw, mask); err != nil {
			return nil, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(rw, payload); err != nil {
		return nil, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	switch opcode {
	case 0x1:
		return payload, nil
	case 0x8:
		return nil, errors.New("websocket closed")
	case 0x9:
		return readWSFrame(rw)
	default:
		return payload, nil
	}
}

func waitUntil(ctx context.Context, timeout time.Duration, fn func() bool) error {
	deadline := time.Now().Add(timeout)
	for {
		if fn() {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for chrome")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func defaultChromePath() string {
	for _, candidate := range []string{
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		"google-chrome",
		"chromium",
	} {
		if _, err := os.Stat(candidate); err == nil || strings.Contains(candidate, "chrome") || strings.Contains(candidate, "chromium") {
			return candidate
		}
	}
	sum := sha1.Sum([]byte("chrome"))
	return fmt.Sprintf("chrome-%x", sum[:4])
}
