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

func TestRuntimeSupportsInteractiveBrowserActions(t *testing.T) {
	runtime := NewRuntime(testBrowserProvider{})
	cases := []struct {
		name string
		run  func(context.Context) (Snapshot, error)
		want string
	}{
		{name: "type", run: func(ctx context.Context) (Snapshot, error) { return runtime.TypeText(ctx, "hello") }, want: "browser.type"},
		{name: "input", run: func(ctx context.Context) (Snapshot, error) { return runtime.Input(ctx, "#q", "hello") }, want: "browser.input"},
		{name: "wait", run: func(ctx context.Context) (Snapshot, error) { return runtime.Wait(ctx, "Example", 100) }, want: "browser.wait"},
		{name: "extract", run: func(ctx context.Context) (Snapshot, error) { return runtime.Extract(ctx, "main") }, want: "browser.extract"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := tc.run(context.Background())
			if err != nil {
				t.Fatalf("%s: %v", tc.name, err)
			}
			if snapshot.Observation.Type != tc.want {
				t.Fatalf("expected %s, got %s", tc.want, snapshot.Observation.Type)
			}
		})
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
