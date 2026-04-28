package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAICompatibleConfig struct {
	ProviderName     string
	APIKey           string
	BaseURL          string
	ChatModel        string
	ThinkingEnabled  *bool
	ReasoningEffort  string
	HTTPClient       *http.Client
	DefaultBaseURL   string
	DefaultChatModel string
}

type OpenAICompatibleProvider struct {
	providerName    string
	apiKey          string
	baseURL         string
	chatModel       string
	thinkingEnabled *bool
	reasoningEffort string
	client          *http.Client
}

func NewOpenAICompatibleProvider(config OpenAICompatibleConfig) (*OpenAICompatibleProvider, error) {
	providerName := strings.TrimSpace(config.ProviderName)
	if providerName == "" {
		providerName = "openai_compatible"
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("%s api key is required", providerName)
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = strings.TrimRight(strings.TrimSpace(config.DefaultBaseURL), "/")
	}
	if baseURL == "" {
		return nil, fmt.Errorf("%s base url is required", providerName)
	}
	model := strings.TrimSpace(config.ChatModel)
	if model == "" {
		model = strings.TrimSpace(config.DefaultChatModel)
	}
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 120 * time.Second}
	}
	return &OpenAICompatibleProvider{
		providerName:    providerName,
		apiKey:          config.APIKey,
		baseURL:         baseURL,
		chatModel:       model,
		thinkingEnabled: config.ThinkingEnabled,
		reasoningEffort: strings.TrimSpace(config.ReasoningEffort),
		client:          client,
	}, nil
}

func (p *OpenAICompatibleProvider) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = p.chatModel
	}
	if model == "" {
		return ChatResponse{}, fmt.Errorf("%s chat model is required", p.providerName)
	}

	payload := chatCompletionRequest{
		Model:    model,
		Messages: make([]chatCompletionMessage, 0, len(req.Messages)),
		Stream:   false,
	}
	if p.thinkingEnabled != nil {
		payload.Thinking = &thinkingConfig{Type: thinkingType(*p.thinkingEnabled)}
	}
	if p.reasoningEffort != "" {
		payload.ReasoningEffort = p.reasoningEffort
	}
	for _, message := range req.Messages {
		payload.Messages = append(payload.Messages, chatCompletionMessage{
			Role:    message.Role,
			Content: message.Content,
		})
	}

	var response chatCompletionResponse
	if err := p.post(ctx, "/chat/completions", payload, &response); err != nil {
		return ChatResponse{}, err
	}
	text := response.text()
	if strings.TrimSpace(text) == "" {
		return ChatResponse{}, fmt.Errorf("%s returned an empty message", p.providerName)
	}
	return ChatResponse{
		Model: firstNonEmptyString(response.Model, model),
		Text:  text,
		Usage: response.usageMap(),
	}, nil
}

func (p *OpenAICompatibleProvider) post(ctx context.Context, path string, payload any, target any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	responseBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return err
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return fmt.Errorf("%s request failed: status=%d body=%s", p.providerName, httpResp.StatusCode, string(responseBody))
	}
	if err := json.Unmarshal(responseBody, target); err != nil {
		return err
	}
	return nil
}

type chatCompletionRequest struct {
	Model           string                  `json:"model"`
	Messages        []chatCompletionMessage `json:"messages"`
	Thinking        *thinkingConfig         `json:"thinking,omitempty"`
	ReasoningEffort string                  `json:"reasoning_effort,omitempty"`
	Stream          bool                    `json:"stream"`
}

type thinkingConfig struct {
	Type string `json:"type"`
}

type chatCompletionMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content          string `json:"content"`
			ReasoningContent string `json:"reasoning_content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

func (r chatCompletionResponse) text() string {
	for _, choice := range r.Choices {
		if strings.TrimSpace(choice.Message.Content) != "" {
			return choice.Message.Content
		}
	}
	return ""
}

func (r chatCompletionResponse) usageMap() map[string]int {
	return map[string]int{
		"input_tokens":  r.Usage.PromptTokens,
		"output_tokens": r.Usage.CompletionTokens,
		"total_tokens":  r.Usage.TotalTokens,
	}
}

func thinkingType(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
