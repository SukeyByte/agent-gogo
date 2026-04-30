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

	"github.com/sukeke/agent-gogo/internal/browser"
	"github.com/sukeke/agent-gogo/internal/communication"
	appconfig "github.com/sukeke/agent-gogo/internal/config"
	"github.com/sukeke/agent-gogo/internal/provider"
	"github.com/sukeke/agent-gogo/internal/store"
	"github.com/sukeke/agent-gogo/internal/tools"
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
