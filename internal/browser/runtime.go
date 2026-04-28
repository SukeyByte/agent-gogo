package browser

import (
	"context"

	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/provider"
)

type Runtime struct {
	provider provider.BrowserProvider
}

type Snapshot struct {
	URL           string
	DOMSummary    string
	ScreenshotRef string
	Observation   domain.Observation
}

func NewRuntime(provider provider.BrowserProvider) *Runtime {
	return &Runtime{provider: provider}
}

func (r *Runtime) Open(ctx context.Context, url string) (Snapshot, error) {
	result, err := r.provider.Call(ctx, "open", map[string]any{"url": url})
	if err != nil {
		return Snapshot{}, err
	}
	return snapshotFromResult(result, "browser.open"), nil
}

func (r *Runtime) DOMSummary(ctx context.Context) (Snapshot, error) {
	result, err := r.provider.Call(ctx, "dom_summary", nil)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshotFromResult(result, "browser.dom_summary"), nil
}

func (r *Runtime) Click(ctx context.Context, text string) (Snapshot, error) {
	result, err := r.provider.Call(ctx, "click", map[string]any{"text": text})
	if err != nil {
		return Snapshot{}, err
	}
	return snapshotFromResult(result, "browser.click"), nil
}

func (r *Runtime) Screenshot(ctx context.Context) (Snapshot, error) {
	result, err := r.provider.Call(ctx, "screenshot", nil)
	if err != nil {
		return Snapshot{}, err
	}
	return snapshotFromResult(result, "browser.screenshot"), nil
}

func snapshotFromResult(result provider.BrowserProviderResult, observationType string) Snapshot {
	return Snapshot{
		URL:           result.URL,
		DOMSummary:    result.DOMSummary,
		ScreenshotRef: result.ScreenshotRef,
		Observation: domain.Observation{
			Type:        observationType,
			Summary:     result.DOMSummary,
			EvidenceRef: firstNonEmpty(result.ScreenshotRef, result.URL),
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
