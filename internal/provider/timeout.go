package provider

import (
	"context"
	"time"
)

type TimeoutProvider struct {
	inner   LLMProvider
	timeout time.Duration
}

func NewTimeoutProvider(inner LLMProvider, timeout time.Duration) LLMProvider {
	if inner == nil || timeout <= 0 {
		return inner
	}
	return &TimeoutProvider{inner: inner, timeout: timeout}
}

func (p *TimeoutProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return ChatResponse{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	return p.inner.Chat(ctx, req)
}
