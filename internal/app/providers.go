package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SukeyByte/agent-gogo/internal/browser"
	"github.com/SukeyByte/agent-gogo/internal/communication"
	appconfig "github.com/SukeyByte/agent-gogo/internal/config"
	"github.com/SukeyByte/agent-gogo/internal/mcp"
	"github.com/SukeyByte/agent-gogo/internal/provider"
	"github.com/SukeyByte/agent-gogo/internal/store"
	"github.com/SukeyByte/agent-gogo/internal/tools"
)

func newLLMProvider(cfg appconfig.Config) (provider.LLMProvider, error) {
	if err := cfg.ValidateForLLM(); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: cfg.LLM.Timeout}
	thinking := cfg.LLM.ThinkingEnabled
	llm, err := provider.NewRegisteredLLMProvider(cfg.LLM.Provider, provider.OpenAICompatibleConfig{
		APIKey:          cfg.LLM.APIKey,
		BaseURL:         cfg.LLM.BaseURL,
		ChatModel:       cfg.LLM.Model,
		ThinkingEnabled: &thinking,
		ReasoningEffort: cfg.LLM.ReasoningEffort,
		HTTPClient:      client,
	})
	if err != nil {
		return nil, err
	}
	return provider.NewTimeoutProvider(llm, cfg.LLM.Timeout), nil
}

// NewLLMFromConfig creates a new LLM provider from an LLMConfig (for hot-swap).
func newLLMFromConfig(cfg appconfig.LLMConfig) (provider.LLMProvider, error) {
	if cfg.Provider == "" || cfg.APIKey == "" || cfg.Model == "" {
		return nil, fmt.Errorf("llm provider, api_key, and model are required")
	}
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 120 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	thinking := cfg.ThinkingEnabled
	llm, err := provider.NewRegisteredLLMProvider(cfg.Provider, provider.OpenAICompatibleConfig{
		APIKey:          cfg.APIKey,
		BaseURL:         cfg.BaseURL,
		ChatModel:       cfg.Model,
		ThinkingEnabled: &thinking,
		ReasoningEffort: cfg.ReasoningEffort,
		HTTPClient:      client,
	})
	if err != nil {
		return nil, err
	}
	return provider.NewTimeoutProvider(llm, timeout), nil
}

func newBrowserRuntime(ctx context.Context, cfg appconfig.Config) (*browser.Runtime, func() error, error) {
	client := &http.Client{Timeout: cfg.Browser.Timeout}
	switch strings.ToLower(cfg.Browser.Provider) {
	case "chrome_mcp":
		browserProvider, err := provider.NewManagedChromeMCPBrowserProvider(ctx, provider.ChromeMCPBrowserProviderConfig{
			MCPURL:           cfg.Browser.MCPURL,
			HTTPClient:       client,
			AutoStart:        cfg.Browser.AutoStartMCP,
			DebugPort:        cfg.Browser.DebugPort,
			ChromePath:       cfg.Browser.ChromePath,
			UserDataDir:      cfg.Browser.UserDataDir,
			Headless:         cfg.Browser.Headless,
			MaxSummaryLength: cfg.Browser.MaxSummaryLength,
		})
		if err != nil {
			return nil, nil, err
		}
		return browser.NewRuntime(browserProvider), browserProvider.Close, nil
	case "http_fetch":
		return browser.NewRuntime(provider.NewFetchBrowserProvider(provider.FetchBrowserProviderConfig{
			HTTPClient:       client,
			MaxSummaryLength: cfg.Browser.MaxSummaryLength,
		})), func() error { return nil }, nil
	default:
		return nil, nil, fmt.Errorf("unsupported browser provider %q", cfg.Browser.Provider)
	}
}

type lazyBrowserEngine struct {
	cfg     appconfig.Config
	mu      sync.Mutex
	runtime *browser.Runtime
	close   func() error
}

func newLazyBrowserEngine(cfg appconfig.Config) *lazyBrowserEngine {
	return &lazyBrowserEngine{cfg: cfg}
}

func (e *lazyBrowserEngine) init(ctx context.Context) (*browser.Runtime, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runtime != nil {
		return e.runtime, nil
	}
	runtime, closeBrowser, err := newBrowserRuntime(ctx, e.cfg)
	if err != nil {
		return nil, err
	}
	e.runtime = runtime
	e.close = closeBrowser
	return runtime, nil
}

func (e *lazyBrowserEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.close == nil {
		return nil
	}
	return e.close()
}

func (e *lazyBrowserEngine) Open(ctx context.Context, url string) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.Open(ctx, url)
}

func (e *lazyBrowserEngine) Click(ctx context.Context, text string) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.Click(ctx, text)
}

func (e *lazyBrowserEngine) TypeText(ctx context.Context, text string) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.TypeText(ctx, text)
}

func (e *lazyBrowserEngine) Input(ctx context.Context, selector string, value string) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.Input(ctx, selector, value)
}

func (e *lazyBrowserEngine) Wait(ctx context.Context, text string, timeoutMS int) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.Wait(ctx, text, timeoutMS)
}

func (e *lazyBrowserEngine) Extract(ctx context.Context, query string) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.Extract(ctx, query)
}

func (e *lazyBrowserEngine) DOMSummary(ctx context.Context) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.DOMSummary(ctx)
}

func (e *lazyBrowserEngine) Screenshot(ctx context.Context) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.Screenshot(ctx)
}

func openStore(ctx context.Context, cfg appconfig.Config) (*store.SQLiteStore, error) {
	if cfg.Storage.SQLitePath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(cfg.Storage.SQLitePath), 0o755); err != nil {
			return nil, err
		}
	}
	return store.OpenSQLite(ctx, cfg.Storage.SQLitePath)
}

func newCommunicationRuntime(cfg appconfig.Config, writer io.Writer) *communication.Runtime {
	outbox := communication.NewMemoryOutbox()
	runtime := communication.NewRuntime(outbox, communication.NewRenderer())
	runtime.RegisterChannel(cfg.Communication.ChannelID, communication.NewCLIAdapter(cfg.Communication.ChannelID, writer))
	return runtime
}

func logChannel(ctx context.Context, runtime *communication.Runtime, cfg appconfig.Config, stage string, text string) {
	_, _ = runtime.Dispatch(ctx, communication.NewMessageIntent(cfg.Communication.ChannelID, fmt.Sprintf("[%s] %s", stage, text)))
}

func confirmationRisk(cfg appconfig.Config) string {
	if cfg.Security.RequireConfirmHighRisk {
		return "high"
	}
	return ""
}

func newConfirmationGate(writer io.Writer) tools.ConfirmationGate {
	return tools.NewCLIConfirmationGate(os.Stdin, writer)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

// connectMCPServers dials every enabled MCP server from the config and
// registers its tools into the tool runtime. Servers that fail to start are
// logged and skipped so one broken server cannot take the agent down. The
// returned closer shuts all clients down.
func connectMCPServers(ctx context.Context, cfg appconfig.Config, toolRuntime *tools.Runtime, warn func(format string, args ...any)) func() {
	clients := make([]*mcp.Client, 0, len(cfg.MCPServers))
	for _, server := range cfg.MCPServers {
		if !server.Enabled {
			continue
		}
		var (
			client *mcp.Client
			err    error
		)
		switch {
		case strings.TrimSpace(server.Command) != "":
			client, err = mcp.DialStdio(ctx, server.Name, server.Command, server.Args)
		case strings.TrimSpace(server.URL) != "":
			client, err = mcp.DialHTTP(ctx, server.Name, server.URL, nil)
		default:
			err = fmt.Errorf("mcp server %s has neither command nor url", server.Name)
		}
		if err == nil {
			err = client.Initialize(ctx)
		}
		if err != nil {
			if warn != nil {
				warn("Warning: MCP server %s unavailable (%v), skipping its tools\n", server.Name, err)
			}
			_ = client.Close()
			continue
		}
		if err := toolRuntime.RegisterMCPTools(client); err != nil {
			if warn != nil {
				warn("Warning: MCP server %s registered no tools (%v)\n", server.Name, err)
			}
			_ = client.Close()
			continue
		}
		clients = append(clients, client)
	}
	return func() {
		for _, client := range clients {
			_ = client.Close()
		}
	}
}

// shellTimeout returns the per-command execution bound for shell tools.
func shellTimeout(cfg appconfig.Config) time.Duration {
	if cfg.Security.ShellTimeout > 0 {
		return cfg.Security.ShellTimeout
	}
	return 120 * time.Second
}
