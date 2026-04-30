package tools

import (
	"context"
	"errors"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/browser"
)

type BrowserEngine interface {
	Open(ctx context.Context, url string) (browser.Snapshot, error)
	Click(ctx context.Context, text string) (browser.Snapshot, error)
	TypeText(ctx context.Context, text string) (browser.Snapshot, error)
	Input(ctx context.Context, selector string, value string) (browser.Snapshot, error)
	Wait(ctx context.Context, text string, timeoutMS int) (browser.Snapshot, error)
	Extract(ctx context.Context, query string) (browser.Snapshot, error)
	DOMSummary(ctx context.Context) (browser.Snapshot, error)
	Screenshot(ctx context.Context) (browser.Snapshot, error)
}

func (r *Runtime) RegisterBrowserTools(engine BrowserEngine) {
	r.Register(Spec{
		Name:        "browser.open",
		Description: "Open a URL in the browser runtime and capture visible page state.",
		RiskLevel:   "low",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"url"},
			"properties": map[string]any{
				"url": map[string]any{"type": "string"},
			},
		},
		OutputSchema: browserOutputSchema(),
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		if engine == nil {
			return Result{Success: false, Error: "browser engine is not configured"}, errors.New("browser engine is not configured")
		}
		url, _ := args["url"].(string)
		if strings.TrimSpace(url) == "" {
			return Result{Success: false, Error: "url is required"}, errors.New("url is required")
		}
		snapshot, err := engine.Open(ctx, url)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return browserResult(snapshot), nil
	})
	r.Register(Spec{
		Name:        "browser.click",
		Description: "Click visible text in the current browser page and capture the resulting page state.",
		RiskLevel:   "low",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"text"},
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
		},
		OutputSchema: browserOutputSchema(),
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		if engine == nil {
			return Result{Success: false, Error: "browser engine is not configured"}, errors.New("browser engine is not configured")
		}
		text, _ := args["text"].(string)
		if strings.TrimSpace(text) == "" {
			return Result{Success: false, Error: "text is required"}, errors.New("text is required")
		}
		snapshot, err := engine.Click(ctx, text)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return browserResult(snapshot), nil
	})
	r.Register(Spec{
		Name:        "browser.type",
		Description: "Type text into the focused field or the first writable field on the current page.",
		RiskLevel:   "low",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"text"},
			"properties": map[string]any{
				"text": map[string]any{"type": "string"},
			},
		},
		OutputSchema: browserOutputSchema(),
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		if engine == nil {
			return Result{Success: false, Error: "browser engine is not configured"}, errors.New("browser engine is not configured")
		}
		text, _ := args["text"].(string)
		if strings.TrimSpace(text) == "" {
			return Result{Success: false, Error: "text is required"}, errors.New("text is required")
		}
		snapshot, err := engine.TypeText(ctx, text)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return browserResult(snapshot), nil
	})
	r.Register(Spec{
		Name:        "browser.input",
		Description: "Set the value of a field matching a CSS selector and capture the resulting page state.",
		RiskLevel:   "low",
		InputSchema: map[string]any{
			"type":     "object",
			"required": []string{"selector", "value"},
			"properties": map[string]any{
				"selector": map[string]any{"type": "string"},
				"value":    map[string]any{"type": "string"},
			},
		},
		OutputSchema: browserOutputSchema(),
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		if engine == nil {
			return Result{Success: false, Error: "browser engine is not configured"}, errors.New("browser engine is not configured")
		}
		selector, _ := args["selector"].(string)
		value, _ := args["value"].(string)
		if strings.TrimSpace(selector) == "" {
			return Result{Success: false, Error: "selector is required"}, errors.New("selector is required")
		}
		snapshot, err := engine.Input(ctx, selector, value)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return browserResult(snapshot), nil
	})
	r.Register(Spec{
		Name:        "browser.wait",
		Description: "Wait for visible page text, then capture page state.",
		RiskLevel:   "low",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"text":       map[string]any{"type": "string"},
				"timeout_ms": map[string]any{"type": "number"},
			},
		},
		OutputSchema: browserOutputSchema(),
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		if engine == nil {
			return Result{Success: false, Error: "browser engine is not configured"}, errors.New("browser engine is not configured")
		}
		text, _ := args["text"].(string)
		timeoutMS := intArg(args["timeout_ms"], 10000)
		snapshot, err := engine.Wait(ctx, text, timeoutMS)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return browserResult(snapshot), nil
	})
	r.Register(Spec{
		Name:        "browser.extract",
		Description: "Extract visible page text, optionally narrowed by a CSS selector or search query.",
		RiskLevel:   "low",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string"},
			},
		},
		OutputSchema: browserOutputSchema(),
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		if engine == nil {
			return Result{Success: false, Error: "browser engine is not configured"}, errors.New("browser engine is not configured")
		}
		query, _ := args["query"].(string)
		snapshot, err := engine.Extract(ctx, query)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return browserResult(snapshot), nil
	})
	r.Register(Spec{
		Name:         "browser.dom_summary",
		Description:  "Capture the current browser page URL and DOM summary.",
		RiskLevel:    "low",
		InputSchema:  map[string]any{"type": "object"},
		OutputSchema: browserOutputSchema(),
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		if engine == nil {
			return Result{Success: false, Error: "browser engine is not configured"}, errors.New("browser engine is not configured")
		}
		snapshot, err := engine.DOMSummary(ctx)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return browserResult(snapshot), nil
	})
	r.Register(Spec{
		Name:         "browser.screenshot",
		Description:  "Capture a screenshot reference for the current browser page.",
		RiskLevel:    "low",
		InputSchema:  map[string]any{"type": "object"},
		OutputSchema: browserOutputSchema(),
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		if engine == nil {
			return Result{Success: false, Error: "browser engine is not configured"}, errors.New("browser engine is not configured")
		}
		snapshot, err := engine.Screenshot(ctx)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return browserResult(snapshot), nil
	})
}

func browserResult(snapshot browser.Snapshot) Result {
	return Result{
		Success: true,
		Output: map[string]any{
			"url":            snapshot.URL,
			"dom_summary":    snapshot.DOMSummary,
			"screenshot_ref": snapshot.ScreenshotRef,
		},
		EvidenceRef: firstBrowserValue(snapshot.ScreenshotRef, snapshot.URL),
		Metadata:    map[string]string{"url": snapshot.URL},
	}
}

func browserOutputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"url":            map[string]any{"type": "string"},
			"dom_summary":    map[string]any{"type": "string"},
			"screenshot_ref": map[string]any{"type": "string"},
		},
	}
}

func firstBrowserValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
