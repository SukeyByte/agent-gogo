package tools

import (
	"context"
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/browser"
)

func TestRuntimeRegistersBrowserTools(t *testing.T) {
	ctx := context.Background()
	runtime := NewRuntime(nil)
	runtime.RegisterBrowserTools(fakeBrowserEngine{})

	response, err := runtime.Call(ctx, CallRequest{
		Name: "browser.open",
		Args: map[string]any{"url": "https://example.test"},
	})
	if err != nil {
		t.Fatalf("browser.open: %v", err)
	}
	if !response.Result.Success {
		t.Fatal("expected browser.open success")
	}
	if response.Result.Output["url"] != "https://example.test" {
		t.Fatalf("unexpected output %#v", response.Result.Output)
	}
	specNames := map[string]bool{}
	for _, spec := range runtime.ListSpecs() {
		specNames[spec.Name] = true
	}
	for _, name := range []string{"browser.open", "browser.click", "browser.type", "browser.input", "browser.wait", "browser.extract", "browser.dom_summary", "browser.screenshot"} {
		if !specNames[name] {
			t.Fatalf("expected browser spec %s", name)
		}
	}
}

type fakeBrowserEngine struct{}

func (e fakeBrowserEngine) Open(ctx context.Context, url string) (browser.Snapshot, error) {
	return browser.Snapshot{URL: url, DOMSummary: "Example Domain", ScreenshotRef: "screenshot://example"}, nil
}

func (e fakeBrowserEngine) Click(ctx context.Context, text string) (browser.Snapshot, error) {
	return browser.Snapshot{URL: "https://example.test/clicked", DOMSummary: "Clicked " + text}, nil
}

func (e fakeBrowserEngine) TypeText(ctx context.Context, text string) (browser.Snapshot, error) {
	return browser.Snapshot{URL: "https://example.test", DOMSummary: "Typed " + text}, nil
}

func (e fakeBrowserEngine) Input(ctx context.Context, selector string, value string) (browser.Snapshot, error) {
	return browser.Snapshot{URL: "https://example.test", DOMSummary: selector + "=" + value}, nil
}

func (e fakeBrowserEngine) Wait(ctx context.Context, text string, timeoutMS int) (browser.Snapshot, error) {
	return browser.Snapshot{URL: "https://example.test", DOMSummary: "Waited " + text}, nil
}

func (e fakeBrowserEngine) Extract(ctx context.Context, query string) (browser.Snapshot, error) {
	return browser.Snapshot{URL: "https://example.test", DOMSummary: "Extracted " + query}, nil
}

func (e fakeBrowserEngine) DOMSummary(ctx context.Context) (browser.Snapshot, error) {
	return browser.Snapshot{URL: "https://example.test", DOMSummary: "Example Domain"}, nil
}

func (e fakeBrowserEngine) Screenshot(ctx context.Context) (browser.Snapshot, error) {
	return browser.Snapshot{URL: "https://example.test", ScreenshotRef: "screenshot://example"}, nil
}
