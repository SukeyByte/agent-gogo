package llmjson

import (
	"context"
	"errors"
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/provider"
)

func TestChatObjectRequestsJSONSchemaAndRepairsInvalidOutput(t *testing.T) {
	ctx := context.Background()
	calls := 0
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		calls++
		if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_schema" {
			t.Fatalf("expected json_schema response format, got %#v", req.ResponseFormat)
		}
		if req.ResponseFormat.JSONSchema == nil || !req.ResponseFormat.JSONSchema.Strict {
			t.Fatalf("expected strict schema, got %#v", req.ResponseFormat.JSONSchema)
		}
		if calls == 1 {
			return provider.ChatResponse{Text: "not json"}, nil
		}
		if req.Metadata["repair"] != "json" {
			t.Fatalf("expected repair metadata, got %#v", req.Metadata)
		}
		return provider.ChatResponse{Text: `{"value":"ok"}`}, nil
	})

	var out struct {
		Value string `json:"value"`
	}
	err := ChatObject(ctx, Request{
		LLM:        llm,
		Model:      "test",
		System:     "Return value.",
		User:       "{}",
		SchemaName: "test_schema",
		Schema: map[string]any{
			"type":       "object",
			"required":   []string{"value"},
			"properties": map[string]any{"value": map[string]any{"type": "string"}},
		},
		Metadata:   map[string]string{"stage": "test"},
		MaxRepairs: 1,
	}, &out)
	if err != nil {
		t.Fatalf("chat object: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected two calls, got %d", calls)
	}
	if out.Value != "ok" {
		t.Fatalf("unexpected value %q", out.Value)
	}
}

func TestChatObjectFallsBackToJSONObjectWhenJSONSchemaUnavailable(t *testing.T) {
	ctx := context.Background()
	calls := 0
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		calls++
		switch calls {
		case 1:
			if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_schema" {
				t.Fatalf("expected first request to use json_schema, got %#v", req.ResponseFormat)
			}
			return provider.ChatResponse{}, errors.New(`provider request failed: status=400 body={"error":{"message":"This response_format type is unavailable now","type":"invalid_request_error"}}`)
		case 2:
			if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_object" {
				t.Fatalf("expected fallback request to use json_object, got %#v", req.ResponseFormat)
			}
			if req.Metadata["response_format_fallback"] != "json_object" {
				t.Fatalf("expected fallback metadata, got %#v", req.Metadata)
			}
			return provider.ChatResponse{Text: `{"value":"ok"}`}, nil
		default:
			t.Fatalf("unexpected call %d", calls)
			return provider.ChatResponse{}, nil
		}
	})

	var out struct {
		Value string `json:"value"`
	}
	err := ChatObject(ctx, Request{
		LLM:        llm,
		Model:      "test",
		System:     "Return value.",
		User:       "{}",
		SchemaName: "test_schema",
		Schema: map[string]any{
			"type":       "object",
			"required":   []string{"value"},
			"properties": map[string]any{"value": map[string]any{"type": "string"}},
		},
		Metadata: map[string]string{"stage": "test"},
	}, &out)
	if err != nil {
		t.Fatalf("chat object: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected two calls, got %d", calls)
	}
	if out.Value != "ok" {
		t.Fatalf("unexpected value %q", out.Value)
	}
}
