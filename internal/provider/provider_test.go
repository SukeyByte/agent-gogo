package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeepSeekProviderUsesChatCompletionsHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			if r.Header.Get("Authorization") != "Bearer test-key" {
				t.Fatalf("missing bearer auth: %s", r.Header.Get("Authorization"))
			}
			var req chatCompletionRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode chat completion request: %v", err)
			}
			if req.Model != "deepseek-test" {
				t.Fatalf("unexpected model %s", req.Model)
			}
			_, _ = w.Write([]byte(`{
				"model":"deepseek-test",
				"choices":[{"message":{"content":"real provider path"}}],
				"usage":{"prompt_tokens":3,"completion_tokens":4,"total_tokens":7}
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	provider, err := NewDeepSeekProvider(DeepSeekConfig{
		APIKey:           "test-key",
		BaseURL:          server.URL,
		ChatModel:        "deepseek-test",
		HTTPClient:       server.Client(),
		DefaultBaseURL:   DefaultDeepSeekBaseURL,
		DefaultChatModel: DefaultDeepSeekModel,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	ctx := context.Background()
	chat, err := provider.Chat(ctx, ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if chat.Text != "real provider path" {
		t.Fatalf("unexpected chat text %q", chat.Text)
	}
	if chat.Usage["total_tokens"] != 7 {
		t.Fatalf("unexpected usage %#v", chat.Usage)
	}
}

func TestOpenAIProviderUsesSameChatCompletionsCore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var req chatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode chat completion request: %v", err)
		}
		if req.Model != "gpt-test" {
			t.Fatalf("unexpected model %s", req.Model)
		}
		_, _ = w.Write([]byte(`{
			"model":"gpt-test",
			"choices":[{"message":{"content":"shared compatible provider"}}],
			"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}
		}`))
	}))
	defer server.Close()

	provider, err := NewOpenAIProvider(OpenAIConfig{
		APIKey:     "test-key",
		BaseURL:    server.URL,
		ChatModel:  "gpt-test",
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	chat, err := provider.Chat(context.Background(), ChatRequest{
		Messages: []ChatMessage{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if chat.Text != "shared compatible provider" {
		t.Fatalf("unexpected chat text %q", chat.Text)
	}
}

func TestHTTPBrowserProviderUsesConfiguredEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/browser/call" {
			http.NotFound(w, r)
			return
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode browser request: %v", err)
		}
		if req["action"] != "open" {
			t.Fatalf("unexpected action %#v", req["action"])
		}
		_, _ = w.Write([]byte(`{"URL":"https://example.test","DOMSummary":"Example DOM","ScreenshotRef":"","Metadata":{"action":"open"}}`))
	}))
	defer server.Close()

	browser, err := NewHTTPBrowserProvider(HTTPBrowserProviderConfig{
		BaseURL:    server.URL,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("new browser provider: %v", err)
	}
	result, err := browser.Call(context.Background(), "open", map[string]any{"url": "https://example.test"})
	if err != nil {
		t.Fatalf("browser call: %v", err)
	}
	if result.DOMSummary != "Example DOM" {
		t.Fatalf("unexpected DOM summary %q", result.DOMSummary)
	}
}

func TestChromeMCPBrowserProviderUsesConfiguredEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/browser/call" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"URL":"https://example.test","DOMSummary":"Chrome MCP DOM","ScreenshotRef":"","Metadata":{"source":"chrome_mcp"}}`))
	}))
	defer server.Close()

	browser, err := NewChromeMCPBrowserProvider(ChromeMCPBrowserProviderConfig{
		MCPURL:     server.URL,
		HTTPClient: server.Client(),
	})
	if err != nil {
		t.Fatalf("new chrome mcp provider: %v", err)
	}
	result, err := browser.Call(context.Background(), "open", map[string]any{"url": "https://example.test"})
	if err != nil {
		t.Fatalf("browser call: %v", err)
	}
	if result.DOMSummary != "Chrome MCP DOM" {
		t.Fatalf("unexpected DOM summary %q", result.DOMSummary)
	}
}

func TestFetchBrowserProviderExtractsAfterOpen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><body><main>Example Domain</main></body></html>`))
	}))
	defer server.Close()

	browser := NewFetchBrowserProvider(FetchBrowserProviderConfig{
		HTTPClient: server.Client(),
	})
	if _, err := browser.Call(context.Background(), "open", map[string]any{"url": server.URL}); err != nil {
		t.Fatalf("open: %v", err)
	}
	result, err := browser.Call(context.Background(), "extract", map[string]any{"query": "main"})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if result.DOMSummary != "Example Domain" {
		t.Fatalf("unexpected DOM summary %q", result.DOMSummary)
	}
	if result.Metadata["action"] != "extract" {
		t.Fatalf("unexpected metadata %#v", result.Metadata)
	}
}

func TestMemoryStorageProvider(t *testing.T) {
	storage := NewMemoryStorageProvider()
	ctx := context.Background()
	if err := storage.Put(ctx, "key", []byte("value")); err != nil {
		t.Fatalf("put: %v", err)
	}
	value, err := storage.Get(ctx, "key")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if string(value) != "value" {
		t.Fatalf("unexpected value %q", string(value))
	}
}
