package observability

import (
	"context"

	"github.com/SukeyByte/agent-gogo/internal/provider"
)

type LoggingLLMProvider struct {
	inner  provider.LLMProvider
	logger Logger
}

func NewLoggingLLMProvider(inner provider.LLMProvider, logger Logger) *LoggingLLMProvider {
	return &LoggingLLMProvider{inner: inner, logger: logger}
}

func (p *LoggingLLMProvider) Chat(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
	if p.logger != nil {
		_ = p.logger.Log(ctx, "llm.request", map[string]any{
			"model":    req.Model,
			"messages": req.Messages,
			"metadata": req.Metadata,
		})
	}
	resp, err := p.inner.Chat(ctx, req)
	if p.logger != nil {
		payload := map[string]any{
			"model": resp.Model,
			"text":  resp.Text,
			"usage": resp.Usage,
		}
		if err != nil {
			payload["error"] = err.Error()
		}
		_ = p.logger.Log(ctx, "llm.response", payload)
	}
	return resp, err
}
