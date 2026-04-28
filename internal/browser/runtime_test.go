package browser

import (
	"context"
	"testing"

	"github.com/sukeke/agent-gogo/internal/provider"
)

func TestRuntimeCapturesBrowserEvidence(t *testing.T) {
	runtime := NewRuntime(testBrowserProvider{})
	snapshot, err := runtime.Open(context.Background(), "https://example.test")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if snapshot.URL != "https://example.test" {
		t.Fatalf("unexpected URL %q", snapshot.URL)
	}
	if snapshot.Observation.Type != "browser.open" {
		t.Fatalf("unexpected observation type %q", snapshot.Observation.Type)
	}
	if snapshot.Observation.Summary != "body > main Example" {
		t.Fatalf("unexpected observation summary %q", snapshot.Observation.Summary)
	}
}

type testBrowserProvider struct{}

func (p testBrowserProvider) Call(ctx context.Context, action string, args map[string]any) (provider.BrowserProviderResult, error) {
	if err := ctx.Err(); err != nil {
		return provider.BrowserProviderResult{}, err
	}
	return provider.BrowserProviderResult{
		URL:        "https://example.test",
		DOMSummary: "body > main Example",
		Metadata:   map[string]string{"action": action},
	}, nil
}
