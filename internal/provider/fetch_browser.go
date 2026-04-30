package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type FetchBrowserProviderConfig struct {
	HTTPClient       *http.Client
	MaxSummaryLength int
}

type FetchBrowserProvider struct {
	client           *http.Client
	maxSummaryLength int
	lastURL          string
	lastSummary      string
	lastStatus       string
}

func NewFetchBrowserProvider(config FetchBrowserProviderConfig) *FetchBrowserProvider {
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}
	maxSummaryLength := config.MaxSummaryLength
	if maxSummaryLength <= 0 {
		maxSummaryLength = 12000
	}
	return &FetchBrowserProvider{
		client:           client,
		maxSummaryLength: maxSummaryLength,
	}
}

func (p *FetchBrowserProvider) Call(ctx context.Context, action string, args map[string]any) (BrowserProviderResult, error) {
	if err := ctx.Err(); err != nil {
		return BrowserProviderResult{}, err
	}
	switch action {
	case "open":
		url, _ := args["url"].(string)
		return p.open(ctx, url)
	case "dom_summary":
		if p.lastURL == "" {
			return BrowserProviderResult{}, errors.New("browser page is not open")
		}
		return p.currentResult("dom_summary"), nil
	case "wait":
		if p.lastURL == "" {
			return BrowserProviderResult{}, errors.New("browser page is not open")
		}
		text, _ := args["text"].(string)
		if strings.TrimSpace(text) == "" || strings.Contains(strings.ToLower(p.lastSummary), strings.ToLower(strings.TrimSpace(text))) {
			return p.currentResult("wait"), nil
		}
		timeout := durationFromMS(args["timeout_ms"], 10000)
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return BrowserProviderResult{}, ctx.Err()
		case <-timer.C:
			return BrowserProviderResult{}, fmt.Errorf("timed out waiting for text %q", text)
		}
	case "extract":
		if p.lastURL == "" {
			return BrowserProviderResult{}, errors.New("browser page is not open")
		}
		query, _ := args["query"].(string)
		result := p.currentResult("extract")
		if strings.TrimSpace(query) != "" {
			result.Metadata["query"] = strings.TrimSpace(query)
		}
		return result, nil
	case "screenshot":
		if p.lastURL == "" {
			return BrowserProviderResult{}, errors.New("browser page is not open")
		}
		return BrowserProviderResult{
			URL:           p.lastURL,
			DOMSummary:    p.lastSummary,
			ScreenshotRef: p.lastURL,
			Metadata: map[string]string{
				"source": "http_fetch_no_bitmap",
			},
		}, nil
	default:
		return BrowserProviderResult{}, fmt.Errorf("unsupported browser action %q", action)
	}
}

func (p *FetchBrowserProvider) currentResult(action string) BrowserProviderResult {
	return BrowserProviderResult{
		URL:        p.lastURL,
		DOMSummary: p.lastSummary,
		Metadata: map[string]string{
			"status": p.lastStatus,
			"source": "http_fetch",
			"action": action,
		},
	}
}

func durationFromMS(value any, fallback int) time.Duration {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond
		}
	case float64:
		if typed > 0 {
			return time.Duration(typed) * time.Millisecond
		}
	}
	return time.Duration(fallback) * time.Millisecond
}

func (p *FetchBrowserProvider) open(ctx context.Context, url string) (BrowserProviderResult, error) {
	url = strings.TrimSpace(url)
	if url == "" {
		return BrowserProviderResult{}, errors.New("url is required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return BrowserProviderResult{}, err
	}
	req.Header.Set("User-Agent", "agent-gogo/0.1 (+https://github.com/SukeyByte/agent-gogo)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,text/plain;q=0.8,*/*;q=0.7")
	resp, err := p.client.Do(req)
	if err != nil {
		return BrowserProviderResult{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(p.maxSummaryLength*8)))
	if err != nil {
		return BrowserProviderResult{}, err
	}
	summary := summarizeHTML(string(body), p.maxSummaryLength)
	p.lastURL = resp.Request.URL.String()
	p.lastSummary = summary
	p.lastStatus = resp.Status
	return BrowserProviderResult{
		URL:        p.lastURL,
		DOMSummary: summary,
		Metadata: map[string]string{
			"status":       resp.Status,
			"content_type": resp.Header.Get("Content-Type"),
			"source":       "http_fetch",
		},
	}, nil
}

var (
	scriptBlockPattern = regexp.MustCompile(`(?is)<(script|style|noscript|svg)[^>]*>.*?</\s*(script|style|noscript|svg)\s*>`)
	tagPattern         = regexp.MustCompile(`(?is)<[^>]+>`)
	spacePattern       = regexp.MustCompile(`\s+`)
)

func summarizeHTML(html string, maxRunes int) string {
	text := scriptBlockPattern.ReplaceAllString(html, " ")
	text = tagPattern.ReplaceAllString(text, " ")
	text = htmlEntityCleanup(text)
	text = spacePattern.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	if maxRunes > 0 && utf8.RuneCountInString(text) > maxRunes {
		runes := []rune(text)
		text = string(runes[:maxRunes])
	}
	return text
}

func htmlEntityCleanup(text string) string {
	replacements := map[string]string{
		"&nbsp;":   " ",
		"&amp;":    "&",
		"&lt;":     "<",
		"&gt;":     ">",
		"&quot;":   `"`,
		"&#39;":    "'",
		"&apos;":   "'",
		"&hellip;": "...",
	}
	for from, to := range replacements {
		text = strings.ReplaceAll(text, from, to)
	}
	return text
}
