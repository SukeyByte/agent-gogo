package intent

import (
	"context"
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/chain"
	"github.com/SukeyByte/agent-gogo/internal/provider"
)

func TestLLMAnalyzerUsesProviderJSON(t *testing.T) {
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_schema" {
			t.Fatalf("expected structured response format, got %#v", req.ResponseFormat)
		}
		return provider.ChatResponse{
			Model: req.Model,
			Text: `{
				"task_type":"code",
				"complexity":"L3",
				"domains":["go","runtime","go"],
				"required_capabilities":["test.run","code.search"],
				"risk_level":"medium",
				"needs_user_confirmation":false,
				"grounding_requirement":"tests",
				"confidence":0.92
			}`,
		}, nil
	})
	analyzer := NewLLMAnalyzer(llm, "gpt-test")
	profile, err := analyzer.Analyze(context.Background(), Request{
		UserInput: "fix runtime",
		ChainDecision: chain.Decision{
			Level: chain.LevelProject,
		},
	})
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if profile.TaskType != "code" {
		t.Fatalf("expected code task, got %s", profile.TaskType)
	}
	if len(profile.Domains) != 2 {
		t.Fatalf("expected deduped domains, got %#v", profile.Domains)
	}
	if profile.ContextProfile().GroundingRequirement != "tests" {
		t.Fatalf("expected context profile grounding")
	}
}

func TestGroundingRequirementArrayIsJoinedWithoutGoSliceFormatting(t *testing.T) {
	var profile Profile
	err := decodeProfileJSONObject(`{
		"task_type":"code",
		"complexity":"medium",
		"domains":["go"],
		"required_capabilities":["file.read"],
		"risk_level":"low",
		"needs_user_confirmation":false,
		"grounding_requirement":["read_file","edit_file"],
		"confidence":0.8
	}`, &profile)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if profile.GroundingRequirement != "read_file, edit_file" {
		t.Fatalf("unexpected grounding requirement %q", profile.GroundingRequirement)
	}
}
