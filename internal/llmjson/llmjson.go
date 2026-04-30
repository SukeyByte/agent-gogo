package llmjson

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/provider"
	"github.com/SukeyByte/agent-gogo/internal/textutil"
)

type Request struct {
	LLM         provider.LLMProvider
	Model       string
	System      string
	User        string
	SchemaName  string
	Schema      map[string]any
	Tools       []provider.ChatTool
	Metadata    map[string]string
	MaxRepairs  int
	Temperature *float64
}

func ChatObject(ctx context.Context, req Request, target any) error {
	if req.LLM == nil {
		return errors.New("llm provider is required")
	}
	req.SchemaName = strings.TrimSpace(req.SchemaName)
	if req.SchemaName == "" {
		req.SchemaName = "structured_output"
	}
	repairs := req.MaxRepairs
	if repairs < 0 {
		repairs = 0
	}
	format := jsonSchemaFormat(req.SchemaName, req.Schema)
	resp, err := chatWithFormat(ctx, req, format, req.Metadata)
	if err != nil && len(req.Schema) > 0 && isResponseFormatUnavailable(err) {
		format = &provider.ResponseFormat{Type: "json_object"}
		metadata := copyMetadata(req.Metadata)
		metadata["response_format_fallback"] = "json_object"
		resp, err = chatWithFormat(ctx, req, format, metadata)
	}
	if err != nil {
		return err
	}
	if err := textutil.DecodeJSONObject(resp.Text, target); err == nil {
		return nil
	} else if repairs == 0 {
		return fmt.Errorf("decode %s json: %w", req.SchemaName, err)
	} else {
		return repairObject(ctx, req, format, resp.Text, err, target)
	}
}

func chatWithFormat(ctx context.Context, req Request, format *provider.ResponseFormat, metadata map[string]string) (provider.ChatResponse, error) {
	return req.LLM.Chat(ctx, provider.ChatRequest{
		Model: req.Model,
		Messages: []provider.ChatMessage{
			{Role: "system", Content: withJSONContract(req.System, req.Schema)},
			{Role: "user", Content: req.User},
		},
		Metadata:       metadata,
		ResponseFormat: format,
		Tools:          req.Tools,
		Temperature:    req.Temperature,
	})
}

func repairObject(ctx context.Context, req Request, format *provider.ResponseFormat, badText string, decodeErr error, target any) error {
	payload, err := json.Marshal(map[string]any{
		"schema":         req.Schema,
		"decode_error":   decodeErr.Error(),
		"invalid_output": badText,
	})
	if err != nil {
		return err
	}
	metadata := copyMetadata(req.Metadata)
	metadata["repair"] = "json"
	resp, err := req.LLM.Chat(ctx, provider.ChatRequest{
		Model: req.Model,
		Messages: []provider.ChatMessage{
			{Role: "system", Content: "Repair the invalid model output into one valid JSON object matching the provided schema. Return JSON only."},
			{Role: "user", Content: string(payload)},
		},
		Metadata:       metadata,
		ResponseFormat: format,
		Tools:          req.Tools,
		Temperature:    req.Temperature,
	})
	if err != nil {
		return err
	}
	if err := textutil.DecodeJSONObject(resp.Text, target); err != nil {
		return fmt.Errorf("decode repaired %s json: %w", req.SchemaName, err)
	}
	return nil
}

func copyMetadata(metadata map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func isResponseFormatUnavailable(err error) bool {
	text := strings.ToLower(err.Error())
	if !strings.Contains(text, "response_format") {
		return false
	}
	for _, marker := range []string{"unavailable", "unsupported", "not support", "invalid_request"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func jsonSchemaFormat(name string, schema map[string]any) *provider.ResponseFormat {
	if len(schema) == 0 {
		return &provider.ResponseFormat{Type: "json_object"}
	}
	return &provider.ResponseFormat{
		Type: "json_schema",
		JSONSchema: &provider.JSONSchemaFormat{
			Name:   name,
			Schema: schema,
			Strict: true,
		},
	}
}

func withJSONContract(system string, schema map[string]any) string {
	system = strings.TrimSpace(system)
	var builder strings.Builder
	if system != "" {
		builder.WriteString(system)
		builder.WriteString("\n")
	}
	builder.WriteString("Return exactly one JSON object. Do not include markdown, prose, comments, or multiple JSON objects.")
	if len(schema) > 0 {
		if data, err := json.Marshal(schema); err == nil {
			builder.WriteString("\nJSON schema: ")
			builder.Write(data)
		}
	}
	return builder.String()
}
