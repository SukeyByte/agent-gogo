package provider

import (
	"fmt"
	"strings"
	"sync"
)

type LLMFactory func(OpenAICompatibleConfig) (LLMProvider, error)

var (
	llmRegistryMu sync.RWMutex
	llmRegistry   = map[string]LLMFactory{}
)

func init() {
	RegisterLLMProvider("deepseek", func(config OpenAICompatibleConfig) (LLMProvider, error) {
		config.DefaultBaseURL = DefaultDeepSeekBaseURL
		config.DefaultChatModel = DefaultDeepSeekModel
		return NewDeepSeekProvider(config)
	})
	RegisterLLMProvider("openai", func(config OpenAICompatibleConfig) (LLMProvider, error) {
		config.DefaultBaseURL = DefaultOpenAIBaseURL
		return NewOpenAIProvider(config)
	})
	RegisterLLMProvider("openai_compatible", func(config OpenAICompatibleConfig) (LLMProvider, error) {
		if config.ProviderName == "" {
			config.ProviderName = "openai_compatible"
		}
		return NewOpenAICompatibleProvider(config)
	})
}

func RegisterLLMProvider(name string, factory LLMFactory) {
	name = normalizeProviderName(name)
	if name == "" || factory == nil {
		return
	}
	llmRegistryMu.Lock()
	defer llmRegistryMu.Unlock()
	llmRegistry[name] = factory
}

func NewRegisteredLLMProvider(name string, config OpenAICompatibleConfig) (LLMProvider, error) {
	name = normalizeProviderName(name)
	llmRegistryMu.RLock()
	factory := llmRegistry[name]
	llmRegistryMu.RUnlock()
	if factory == nil {
		return nil, fmt.Errorf("unsupported llm provider %q", name)
	}
	if config.ProviderName == "" {
		config.ProviderName = name
	}
	return factory(config)
}

func RegisteredLLMProviders() []string {
	llmRegistryMu.RLock()
	defer llmRegistryMu.RUnlock()
	names := make([]string, 0, len(llmRegistry))
	for name := range llmRegistry {
		names = append(names, name)
	}
	return names
}

func normalizeProviderName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
