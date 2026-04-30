package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/SukeyByte/agent-gogo/internal/provider/chromemcpbridge"
)

type ChromeMCPBrowserProviderConfig struct {
	MCPURL           string
	HTTPClient       *http.Client
	AutoStart        bool
	DebugPort        int
	ChromePath       string
	UserDataDir      string
	Headless         bool
	MaxSummaryLength int
}

type ChromeMCPBrowserProvider struct {
	*HTTPBrowserProvider
}

type ManagedChromeMCPBrowserProvider struct {
	*ChromeMCPBrowserProvider
	server *http.Server
	bridge *chromemcpbridge.Bridge
}

func NewChromeMCPBrowserProvider(config ChromeMCPBrowserProviderConfig) (*ChromeMCPBrowserProvider, error) {
	httpProvider, err := NewHTTPBrowserProvider(HTTPBrowserProviderConfig{
		BaseURL:    config.MCPURL,
		HTTPClient: config.HTTPClient,
	})
	if err != nil {
		return nil, err
	}
	return &ChromeMCPBrowserProvider{HTTPBrowserProvider: httpProvider}, nil
}

func NewManagedChromeMCPBrowserProvider(ctx context.Context, config ChromeMCPBrowserProviderConfig) (*ManagedChromeMCPBrowserProvider, error) {
	if config.HTTPClient == nil {
		config.HTTPClient = &http.Client{Timeout: 60 * time.Second}
	}
	if !config.AutoStart {
		provider, err := NewChromeMCPBrowserProvider(config)
		if err != nil {
			return nil, err
		}
		return &ManagedChromeMCPBrowserProvider{ChromeMCPBrowserProvider: provider}, nil
	}
	if err := checkChromeMCPHealth(ctx, config); err != nil {
		bridge, server, startErr := startChromeMCPBridge(ctx, config)
		if startErr != nil {
			return nil, fmt.Errorf("chrome mcp unavailable (%v) and auto-start failed: %w", err, startErr)
		}
		provider, providerErr := NewChromeMCPBrowserProvider(config)
		if providerErr != nil {
			_ = server.Shutdown(context.Background())
			_ = bridge.Close()
			return nil, providerErr
		}
		return &ManagedChromeMCPBrowserProvider{ChromeMCPBrowserProvider: provider, server: server, bridge: bridge}, nil
	}
	provider, err := NewChromeMCPBrowserProvider(config)
	if err != nil {
		return nil, err
	}
	return &ManagedChromeMCPBrowserProvider{ChromeMCPBrowserProvider: provider}, nil
}

func (p *ManagedChromeMCPBrowserProvider) Close() error {
	if p == nil {
		return nil
	}
	var err error
	if p.server != nil {
		err = p.server.Shutdown(context.Background())
	}
	if p.bridge != nil {
		if bridgeErr := p.bridge.Close(); err == nil {
			err = bridgeErr
		}
	}
	return err
}

func checkChromeMCPHealth(ctx context.Context, config ChromeMCPBrowserProviderConfig) error {
	endpoint := strings.TrimRight(config.MCPURL, "/") + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := config.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health check returned %d", resp.StatusCode)
	}
	return nil
}

func startChromeMCPBridge(ctx context.Context, config ChromeMCPBrowserProviderConfig) (*chromemcpbridge.Bridge, *http.Server, error) {
	parsed, err := url.Parse(config.MCPURL)
	if err != nil {
		return nil, nil, err
	}
	if parsed.Host == "" {
		return nil, nil, fmt.Errorf("chrome mcp url must include host: %s", config.MCPURL)
	}
	bridge := chromemcpbridge.New(chromemcpbridge.Config{
		DebugPort:        config.DebugPort,
		ChromePath:       config.ChromePath,
		UserDataDir:      config.UserDataDir,
		Headless:         config.Headless,
		MaxSummaryLength: config.MaxSummaryLength,
	})
	server := &http.Server{
		Addr:    parsed.Host,
		Handler: bridge.Handler(),
	}
	go func() {
		_ = server.ListenAndServe()
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := checkChromeMCPHealth(ctx, config); err == nil {
			return bridge, server, nil
		}
		if time.Now().After(deadline) {
			_ = server.Shutdown(context.Background())
			_ = bridge.Close()
			return nil, nil, fmt.Errorf("timed out waiting for chrome mcp bridge at %s", config.MCPURL)
		}
		select {
		case <-ctx.Done():
			_ = server.Shutdown(context.Background())
			_ = bridge.Close()
			return nil, nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
