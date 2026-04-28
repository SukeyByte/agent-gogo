package provider

import "context"

type ChatMessage struct {
	Role    string
	Content string
}

type ChatRequest struct {
	Model    string
	Messages []ChatMessage
	Metadata map[string]string
}

type ChatResponse struct {
	Model string
	Text  string
	Usage map[string]int
}

type LLMProvider interface {
	Chat(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

type ChatFunc func(ctx context.Context, req ChatRequest) (ChatResponse, error)

func (f ChatFunc) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	return f(ctx, req)
}

type BrowserProviderResult struct {
	URL           string
	DOMSummary    string
	ScreenshotRef string
	Metadata      map[string]string
}

type BrowserProvider interface {
	Call(ctx context.Context, action string, args map[string]any) (BrowserProviderResult, error)
}

type StorageProvider interface {
	Put(ctx context.Context, key string, value []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
}
