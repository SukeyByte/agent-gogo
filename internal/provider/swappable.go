package provider

import (
	"context"
	"sync"
)

// SwappableLLMProvider wraps an LLMProvider with a mutex so the underlying
// provider can be hot-swapped at runtime without restarting the process.
// All components share the same pointer, so Swap() is visible to everyone.
type SwappableLLMProvider struct {
	mu    sync.RWMutex
	inner LLMProvider
	model string
}

func NewSwappableLLMProvider(initial LLMProvider) *SwappableLLMProvider {
	return &SwappableLLMProvider{inner: initial}
}

func (s *SwappableLLMProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.inner.Chat(ctx, req)
}

func (s *SwappableLLMProvider) Swap(newProvider LLMProvider) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.inner = newProvider
}

func (s *SwappableLLMProvider) SetModel(model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.model = model
}

func (s *SwappableLLMProvider) Model() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.model
}
