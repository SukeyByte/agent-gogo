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
	"strconv"
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

func (b *Bridge) open(ctx context.Context, rawURL string) (browserResult, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return browserResult{}, errors.New("url is required")
	}
	if err := b.ensureChrome(ctx); err != nil {
		return browserResult{}, err
	}
	created, err := b.newTarget(ctx)
	if err != nil {
		return browserResult{}, err
	}
	if created.WebSocketDebuggerURL == "" {
		return browserResult{}, errors.New("chrome target missing websocket url")
	}
	if err := b.navigate(ctx, created.WebSocketDebuggerURL, rawURL); err != nil {
		return browserResult{}, err
	}
	_ = b.bringPageToFront(ctx, created.WebSocketDebuggerURL)
	if err := b.waitReady(ctx, created.WebSocketDebuggerURL); err != nil {
		return browserResult{}, err
	}
	_ = b.bringPageToFront(ctx, created.WebSocketDebuggerURL)
	if err := b.waitReadableText(ctx, created.WebSocketDebuggerURL); err != nil {
		return browserResult{}, err
	}
	summary, currentURL, err := b.extractText(ctx, created.WebSocketDebuggerURL)
	if err != nil {
		return browserResult{}, err
	}
	created.URL = currentURL
	_ = b.bringChromeWindowToFront(ctx, currentURL)

	b.mu.Lock()
	b.last = created
	b.mu.Unlock()

	return browserResult{
		URL:        currentURL,
		DOMSummary: summary,
		Metadata: map[string]string{
			"provider": "chrome_mcp",
			"target":   created.ID,
		},
	}, nil
}

func (b *Bridge) domSummary(ctx context.Context) (browserResult, error) {
	last, err := b.lastTarget()
	if err != nil {
		return browserResult{}, err
	}
	summary, currentURL, err := b.extractText(ctx, last.WebSocketDebuggerURL)
	if err != nil {
		return browserResult{}, err
	}
	last.URL = currentURL
	b.mu.Lock()
	b.last = last
	b.mu.Unlock()
	return browserResult{
		URL:        currentURL,
		DOMSummary: summary,
		Metadata: map[string]string{
			"provider": "chrome_mcp",
			"target":   last.ID,
		},
	}, nil
}

func (b *Bridge) click(ctx context.Context, text string) (browserResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return browserResult{}, errors.New("click text is required")
	}
	last, err := b.lastTarget()
	if err != nil {
		return browserResult{}, err
	}
	before, _, _ := b.extractText(ctx, last.WebSocketDebuggerURL)
	session, err := dialCDP(ctx, last.WebSocketDebuggerURL)
	if err != nil {
		return browserResult{}, err
	}
	targetJSON, _ := json.Marshal(text)
	expression := fmt.Sprintf(`(() => {
const target = %s;
const all = Array.from(document.querySelectorAll('button,a,[role="button"],input,textarea,select,div,span'));
const visible = (el) => {
  const box = el.getBoundingClientRect();
  const style = window.getComputedStyle(el);
  return box.width > 0 && box.height > 0 && style.visibility !== 'hidden' && style.display !== 'none';
};
const labelOf = (el) => [el.innerText, el.textContent, el.value, el.getAttribute('aria-label'), el.getAttribute('title')].filter(Boolean).join(' ').replace(/\s+/g, ' ').trim();
const isInteractive = (el) => el.matches('button,a,[role="button"],input,textarea,select,[tabindex]');
const candidates = all
  .filter((el) => visible(el))
  .map((el) => {
    const label = labelOf(el);
    const clickable = isInteractive(el) ? el : (el.closest('button,a,[role="button"],[tabindex]') || el);
    const exact = label === target;
    const compact = label.length <= target.length + 24;
    const score = (exact ? 1000 : 0) + (compact ? 300 : 0) + (isInteractive(clickable) ? 150 : 0) - Math.min(label.length, 500);
    return {el, clickable, label, score};
  })
  .filter((item) => item.label.includes(target))
  .sort((a, b) => b.score - a.score);
const match = candidates[0];
if (!match) return {clicked:false, url:location.href, text:document.body ? document.body.innerText : ''};
match.clickable.scrollIntoView({block:'center', inline:'center'});
match.clickable.click();
return {clicked:true, clickedText:match.label, score:match.score, url:location.href, text:document.body ? document.body.innerText : ''};
})()`, string(targetJSON))
	response, err := session.Call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
	})
	_ = session.Close()
	if err != nil {
		return browserResult{}, err
	}
	result, _ := response["result"].(map[string]any)
	value, _ := result["value"].(map[string]any)
	clicked, _ := value["clicked"].(bool)
	if !clicked {
		return browserResult{}, fmt.Errorf("click target %q not found", text)
	}
	if err := b.waitTextChange(ctx, last.WebSocketDebuggerURL, before); err != nil {
		return browserResult{}, err
	}
	summary, currentURL, err := b.extractText(ctx, last.WebSocketDebuggerURL)
	if err != nil {
		return browserResult{}, err
	}
	last.URL = currentURL
	b.mu.Lock()
	b.last = last
	b.mu.Unlock()
	return browserResult{
		URL:        currentURL,
		DOMSummary: summary,
		Metadata: map[string]string{
			"provider": "chrome_mcp",
			"target":   last.ID,
			"action":   "click",
			"text":     text,
		},
	}, nil
}

func (b *Bridge) typeText(ctx context.Context, text string) (browserResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return browserResult{}, errors.New("type text is required")
	}
	last, err := b.lastTarget()
	if err != nil {
		return browserResult{}, err
	}
	session, err := dialCDP(ctx, last.WebSocketDebuggerURL)
	if err != nil {
		return browserResult{}, err
	}
	textJSON, _ := json.Marshal(text)
	expression := fmt.Sprintf(`(() => {
const text = %s;
const writable = (el) => el && (el.isContentEditable || el.matches('input,textarea,[role="textbox"]'));
const candidates = [document.activeElement, ...Array.from(document.querySelectorAll('input,textarea,[contenteditable="true"],[role="textbox"]'))];
const el = candidates.find(writable);
if (!el) return {ok:false, reason:'no writable field', url:location.href};
el.focus();
if (el.isContentEditable) {
  document.execCommand('insertText', false, text);
} else {
  const value = String(el.value || '');
  const start = Number.isInteger(el.selectionStart) ? el.selectionStart : value.length;
  const end = Number.isInteger(el.selectionEnd) ? el.selectionEnd : value.length;
  el.value = value.slice(0, start) + text + value.slice(end);
  el.dispatchEvent(new InputEvent('input', {bubbles:true, inputType:'insertText', data:text}));
  el.dispatchEvent(new Event('change', {bubbles:true}));
}
return {ok:true, url:location.href, text:document.body ? document.body.innerText : ''};
})()`, string(textJSON))
	response, err := session.Call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
	})
	_ = session.Close()
	if err != nil {
		return browserResult{}, err
	}
	result, _ := response["result"].(map[string]any)
	value, _ := result["value"].(map[string]any)
	ok, _ := value["ok"].(bool)
	if !ok {
		reason, _ := value["reason"].(string)
		return browserResult{}, fmt.Errorf("type failed: %s", firstNonEmpty(reason, "no writable field"))
	}
	summary, currentURL, err := b.extractText(ctx, last.WebSocketDebuggerURL)
	if err != nil {
		return browserResult{}, err
	}
	last.URL = currentURL
	b.mu.Lock()
	b.last = last
	b.mu.Unlock()
	return browserResult{
		URL:        currentURL,
		DOMSummary: summary,
		Metadata: map[string]string{
			"provider": "chrome_mcp",
			"target":   last.ID,
			"action":   "type",
		},
	}, nil
}

func (b *Bridge) input(ctx context.Context, selector string, value string) (browserResult, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return browserResult{}, errors.New("input selector is required")
	}
	last, err := b.lastTarget()
	if err != nil {
		return browserResult{}, err
	}
	session, err := dialCDP(ctx, last.WebSocketDebuggerURL)
	if err != nil {
		return browserResult{}, err
	}
	selectorJSON, _ := json.Marshal(selector)
	valueJSON, _ := json.Marshal(value)
	expression := fmt.Sprintf(`(() => {
const selector = %s;
const value = %s;
const norm = (text) => String(text || '').replace(/\s+/g, ' ').trim().toLowerCase();
const candidates = Array.from(document.querySelectorAll('input,textarea,[contenteditable="true"],[role="textbox"]'));
const labelText = (el) => {
  const labels = [];
  if (el.id) {
    labels.push(...Array.from(document.querySelectorAll('label[for="' + CSS.escape(el.id) + '"]')).map((label) => label.innerText || label.textContent));
  }
  const parentLabel = el.closest('label');
  if (parentLabel) labels.push(parentLabel.innerText || parentLabel.textContent);
  labels.push(el.getAttribute('aria-label'), el.getAttribute('name'), el.getAttribute('id'), el.getAttribute('placeholder'), el.getAttribute('title'));
  return labels.filter(Boolean).join(' ');
};
const selectorHints = [selector];
const attrMatch = selector.match(/\[(?:name|aria-label|placeholder|id)=["']?([^"'\]]+)["']?\]/i);
if (attrMatch) selectorHints.push(attrMatch[1]);
if (/^[#.][\w-]+$/.test(selector)) selectorHints.push(selector.slice(1));
let el = null;
try {
  el = document.querySelector(selector);
} catch (_) {}
if (!el) {
  const hints = selectorHints.map(norm).filter(Boolean);
  const scored = candidates
    .map((candidate, index) => {
      const label = norm(labelText(candidate));
      const exact = hints.some((hint) => label === hint || norm(candidate.getAttribute('name')) === hint || norm(candidate.id) === hint);
      const contains = hints.some((hint) => hint && label.includes(hint));
      return {candidate, score: (exact ? 1000 : 0) + (contains ? 400 : 0) - index};
    })
    .sort((a, b) => b.score - a.score);
  if (scored.length === 1 || (scored[0] && scored[0].score > 0)) {
    el = scored[0].candidate;
  }
}
if (!el) return {ok:false, reason:'selector or field label not found', url:location.href};
el.scrollIntoView({block:'center', inline:'center'});
el.focus();
if (el.isContentEditable) {
  el.textContent = value;
} else if ('value' in el) {
  el.value = value;
} else {
  el.setAttribute('value', value);
}
try {
  el.dispatchEvent(new InputEvent('input', {bubbles:true, inputType:'insertReplacementText', data:value}));
} catch (_) {
  el.dispatchEvent(new Event('input', {bubbles:true}));
}
el.dispatchEvent(new Event('change', {bubbles:true}));
return {ok:true, url:location.href, text:document.body ? document.body.innerText : ''};
})()`, string(selectorJSON), string(valueJSON))
	response, err := session.Call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
	})
	_ = session.Close()
	if err != nil {
		return browserResult{}, err
	}
	result, _ := response["result"].(map[string]any)
	jsValue, _ := result["value"].(map[string]any)
	ok, _ := jsValue["ok"].(bool)
	if !ok {
		reason, _ := jsValue["reason"].(string)
		return browserResult{}, fmt.Errorf("input failed for selector %q: %s", selector, firstNonEmpty(reason, "not found"))
	}
	summary, currentURL, err := b.extractText(ctx, last.WebSocketDebuggerURL)
	if err != nil {
		return browserResult{}, err
	}
	last.URL = currentURL
	b.mu.Lock()
	b.last = last
	b.mu.Unlock()
	return browserResult{
		URL:        currentURL,
		DOMSummary: summary,
		Metadata: map[string]string{
			"provider": "chrome_mcp",
			"target":   last.ID,
			"action":   "input",
			"selector": selector,
		},
	}, nil
}

func (b *Bridge) wait(ctx context.Context, text string, timeoutMS int) (browserResult, error) {
	last, err := b.lastTarget()
	if err != nil {
		return browserResult{}, err
	}
	target := strings.ToLower(strings.TrimSpace(text))
	timeout := time.Duration(timeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)
	for {
		summary, currentURL, err := b.extractText(ctx, last.WebSocketDebuggerURL)
		if err == nil && (target == "" || strings.Contains(strings.ToLower(summary), target)) {
			last.URL = currentURL
			b.mu.Lock()
			b.last = last
			b.mu.Unlock()
			return browserResult{
				URL:        currentURL,
				DOMSummary: summary,
				Metadata: map[string]string{
					"provider": "chrome_mcp",
					"target":   last.ID,
					"action":   "wait",
					"text":     text,
				},
			}, nil
		}
		if time.Now().After(deadline) {
			return browserResult{}, fmt.Errorf("timed out waiting for text %q", text)
		}
		select {
		case <-ctx.Done():
			return browserResult{}, ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
}

func (b *Bridge) extract(ctx context.Context, query string) (browserResult, error) {
	last, err := b.lastTarget()
	if err != nil {
		return browserResult{}, err
	}
	query = strings.TrimSpace(query)
	summary, currentURL, err := b.extractQueryText(ctx, last.WebSocketDebuggerURL, query)
	if err != nil {
		return browserResult{}, err
	}
	last.URL = currentURL
	b.mu.Lock()
	b.last = last
	b.mu.Unlock()
	metadata := map[string]string{
		"provider": "chrome_mcp",
		"target":   last.ID,
		"action":   "extract",
	}
	if query != "" {
		metadata["query"] = query
	}
	return browserResult{
		URL:        currentURL,
		DOMSummary: summary,
		Metadata:   metadata,
	}, nil
}

func (b *Bridge) screenshot(ctx context.Context) (browserResult, error) {
	last, err := b.lastTarget()
	if err != nil {
		return browserResult{}, err
	}
	session, err := dialCDP(ctx, last.WebSocketDebuggerURL)
	if err != nil {
		return browserResult{}, err
	}
	defer session.Close()
	response, err := session.Call(ctx, "Page.captureScreenshot", map[string]any{"format": "png"})
	if err != nil {
		return browserResult{}, err
	}
	data, _ := response["data"].(string)
	return browserResult{
		URL:           last.URL,
		DOMSummary:    "screenshot captured",
		ScreenshotRef: "data:image/png;base64," + data,
		Metadata: map[string]string{
			"provider": "chrome_mcp",
			"target":   last.ID,
		},
	}, nil
}

func (b *Bridge) ensureChrome(ctx context.Context) error {
	if _, err := b.version(ctx); err == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.chromeRunningLocked() {
		return waitUntil(ctx, 5*time.Second, func() bool {
			_, err := b.version(ctx)
			return err == nil
		})
	}
	b.cmd = nil
	if err := os.MkdirAll(b.config.UserDataDir, 0o755); err != nil {
		return err
	}
	args := []string{
		"--remote-debugging-address=127.0.0.1",
		"--remote-debugging-port=" + strconv.Itoa(b.config.DebugPort),
		"--user-data-dir=" + b.config.UserDataDir,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-background-networking",
	}
	if b.config.Headless {
		args = append(args, "--headless=new", "--disable-gpu")
	}
	args = append(args, "about:blank")
	cmd := exec.Command(b.config.ChromePath, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return err
	}
	b.cmd = cmd
	go func() { _ = cmd.Wait() }()
	return waitUntil(ctx, 10*time.Second, func() bool {
		_, err := b.version(ctx)
		return err == nil
	})
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

func (b *Bridge) waitReadableText(ctx context.Context, websocketURL string) error {
	deadline := time.Now().Add(12 * time.Second)
	for {
		session, err := dialCDP(ctx, websocketURL)
		if err != nil {
			return err
		}
		response, err := session.Call(ctx, "Runtime.evaluate", map[string]any{
			"expression":    `document.body ? document.body.innerText.trim().length : 0`,
			"returnByValue": true,
		})
		_ = session.Close()
		if err == nil {
			if result, _ := response["result"].(map[string]any); result != nil {
				if value, ok := result["value"].(float64); ok && value > 0 {
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

func (b *Bridge) waitTextChange(ctx context.Context, websocketURL string, before string) error {
	deadline := time.Now().Add(45 * time.Second)
	previous := strings.TrimSpace(before)
	var firstChanged time.Time
	var lastChanged time.Time
	var lastText string
	for {
		text, _, err := b.extractText(ctx, websocketURL)
		trimmed := strings.TrimSpace(text)
		if err == nil && trimmed != "" && trimmed != previous && !isTransientText(trimmed) {
			now := time.Now()
			if firstChanged.IsZero() {
				firstChanged = now
				lastChanged = now
				lastText = trimmed
			}
			if trimmed != lastText {
				lastText = trimmed
				lastChanged = now
			}
			if isStableEnough(previous, trimmed, firstChanged, lastChanged, now) {
				return nil
			}
		}
		if time.Now().After(deadline) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(750 * time.Millisecond):
		}
	}
}

func isTransientText(text string) bool {
	return strings.Contains(text, "正在为您揭晓") || strings.Contains(text, "加载中")
}

func isStableEnough(previous string, current string, firstChanged time.Time, lastChanged time.Time, now time.Time) bool {
	if strings.Contains(current, "你的答案") || strings.Contains(strings.ToLower(current), "your answer") {
		if len([]rune(current)) < len([]rune(previous))+120 && now.Sub(firstChanged) < 18*time.Second {
			return false
		}
	}
	return now.Sub(lastChanged) >= 2*time.Second
}

func (b *Bridge) extractText(ctx context.Context, websocketURL string) (string, string, error) {
	session, err := dialCDP(ctx, websocketURL)
	if err != nil {
		return "", "", err
	}
	defer session.Close()
	expression := fmt.Sprintf(`(() => {
const text = document.body ? document.body.innerText : document.documentElement.innerText;
return {url: location.href, text: (text || "").replace(/\s+/g, " ").trim().slice(0, %d)};
})()`, b.config.MaxSummaryLength)
	response, err := session.Call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
	})
	if err != nil {
		return "", "", err
	}
	result, _ := response["result"].(map[string]any)
	value, _ := result["value"].(map[string]any)
	text, _ := value["text"].(string)
	currentURL, _ := value["url"].(string)
	return text, currentURL, nil
}

func (b *Bridge) extractQueryText(ctx context.Context, websocketURL string, query string) (string, string, error) {
	if strings.TrimSpace(query) == "" {
		return b.extractText(ctx, websocketURL)
	}
	session, err := dialCDP(ctx, websocketURL)
	if err != nil {
		return "", "", err
	}
	defer session.Close()
	queryJSON, _ := json.Marshal(query)
	expression := fmt.Sprintf(`(() => {
const query = %s;
let text = "";
try {
  const nodes = Array.from(document.querySelectorAll(query));
  if (nodes.length > 0) {
    text = nodes.map((node) => node.innerText || node.textContent || "").join(" ");
  }
} catch (error) {}
if (!text) {
  text = document.body ? document.body.innerText : document.documentElement.innerText;
}
return {url: location.href, text: (text || "").replace(/\s+/g, " ").trim().slice(0, %d)};
})()`, string(queryJSON), b.config.MaxSummaryLength)
	response, err := session.Call(ctx, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
	})
	if err != nil {
		return "", "", err
	}
	result, _ := response["result"].(map[string]any)
	value, _ := result["value"].(map[string]any)
	text, _ := value["text"].(string)
	currentURL, _ := value["url"].(string)
	return text, currentURL, nil
}

func (b *Bridge) lastTarget() (target, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.last.WebSocketDebuggerURL == "" {
		return target{}, errors.New("browser page is not open")
	}
	return b.last, nil
}

func (b *Bridge) getJSON(ctx context.Context, path string, out any) error {
	return b.requestJSON(ctx, http.MethodGet, path, out)
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
