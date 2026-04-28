package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type HTTPBrowserProviderConfig struct {
	BaseURL    string
	HTTPClient *http.Client
}

type HTTPBrowserProvider struct {
	baseURL string
	client  *http.Client
}

func NewHTTPBrowserProvider(config HTTPBrowserProviderConfig) (*HTTPBrowserProvider, error) {
	baseURL := strings.TrimRight(config.BaseURL, "/")
	if baseURL == "" {
		return nil, errors.New("browser provider base url is required")
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	return &HTTPBrowserProvider{baseURL: baseURL, client: client}, nil
}

func (p *HTTPBrowserProvider) Call(ctx context.Context, action string, args map[string]any) (BrowserProviderResult, error) {
	if args == nil {
		args = map[string]any{}
	}
	payload := map[string]any{
		"action": action,
		"args":   args,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return BrowserProviderResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/browser/call", bytes.NewReader(body))
	if err != nil {
		return BrowserProviderResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return BrowserProviderResult{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return BrowserProviderResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BrowserProviderResult{}, fmt.Errorf("browser provider request failed: status=%d body=%s", resp.StatusCode, string(responseBody))
	}
	var result BrowserProviderResult
	if err := json.Unmarshal(responseBody, &result); err != nil {
		return BrowserProviderResult{}, err
	}
	return result, nil
}
