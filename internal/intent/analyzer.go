package intent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/chain"
	"github.com/SukeyByte/agent-gogo/internal/contextbuilder"
	"github.com/SukeyByte/agent-gogo/internal/llmjson"
	"github.com/SukeyByte/agent-gogo/internal/prompts"
	"github.com/SukeyByte/agent-gogo/internal/provider"
	"github.com/SukeyByte/agent-gogo/internal/textutil"
)

type Profile struct {
	TaskType              string   `json:"task_type"`
	Complexity            string   `json:"complexity"`
	Domains               []string `json:"domains"`
	RequiredCapabilities  []string `json:"required_capabilities"`
	RiskLevel             string   `json:"risk_level"`
	NeedsUserConfirmation bool     `json:"needs_user_confirmation"`
	GroundingRequirement  string   `json:"grounding_requirement"`
	Confidence            float64  `json:"confidence"`
}

type Request struct {
	UserInput     string         `json:"user_input"`
	ChainDecision chain.Decision `json:"chain_decision"`
}

type Analyzer interface {
	Analyze(ctx context.Context, req Request) (Profile, error)
}

type LLMAnalyzer struct {
	llm   provider.LLMProvider
	model string
}

func NewLLMAnalyzer(llm provider.LLMProvider, model string) *LLMAnalyzer {
	return &LLMAnalyzer{llm: llm, model: model}
}

func (a *LLMAnalyzer) Analyze(ctx context.Context, req Request) (Profile, error) {
	if a.llm == nil {
		return Profile{}, errors.New("llm provider is required")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return Profile{}, err
	}
	var wire intentProfileWire
	if err := llmjson.ChatObject(ctx, llmjson.Request{
		LLM:        a.llm,
		Model:      a.model,
		System:     intentSystemPrompt,
		User:       string(payload),
		SchemaName: "intent_profile",
		Schema:     intentProfileSchema(),
		Metadata:   map[string]string{"stage": "intent.analyze"},
		MaxRepairs: 1,
	}, &wire); err != nil {
		return Profile{}, err
	}
	profile := profileFromWire(wire)
	if profile.TaskType == "" {
		return Profile{}, errors.New("intent task_type is required")
	}
	return normalizeProfile(profile), nil
}

func (p Profile) ContextProfile() contextbuilder.IntentProfile {
	return contextbuilder.IntentProfile{
		TaskType:              p.TaskType,
		Complexity:            p.Complexity,
		Domains:               append([]string(nil), p.Domains...),
		RequiredCapabilities:  append([]string(nil), p.RequiredCapabilities...),
		RiskLevel:             p.RiskLevel,
		NeedsUserConfirmation: p.NeedsUserConfirmation,
		GroundingRequirement:  p.GroundingRequirement,
		Confidence:            p.Confidence,
	}
}

func normalizeProfile(profile Profile) Profile {
	profile.Domains = textutil.SortedUniqueStrings(profile.Domains)
	profile.RequiredCapabilities = textutil.SortedUniqueStrings(profile.RequiredCapabilities)
	profile.RiskLevel = strings.ToLower(profile.RiskLevel)
	return profile
}

var intentSystemPrompt = prompts.Text("intent_analyzer")

func decodeProfileJSONObject(text string, target *Profile) error {
	var wire intentProfileWire
	if err := textutil.DecodeJSONObject(text, &wire); err != nil {
		return err
	}
	*target = profileFromWire(wire)
	return nil
}

type intentProfileWire struct {
	TaskType              any `json:"task_type"`
	Complexity            any `json:"complexity"`
	Domains               any `json:"domains"`
	RequiredCapabilities  any `json:"required_capabilities"`
	RiskLevel             any `json:"risk_level"`
	NeedsUserConfirmation any `json:"needs_user_confirmation"`
	GroundingRequirement  any `json:"grounding_requirement"`
	Confidence            any `json:"confidence"`
}

func profileFromWire(wire intentProfileWire) Profile {
	return Profile{
		TaskType:              textValue(wire.TaskType),
		Complexity:            textValue(wire.Complexity),
		Domains:               stringList(wire.Domains),
		RequiredCapabilities:  stringList(wire.RequiredCapabilities),
		RiskLevel:             textValue(wire.RiskLevel),
		NeedsUserConfirmation: boolValue(wire.NeedsUserConfirmation),
		GroundingRequirement:  groundingRequirementString(wire.GroundingRequirement),
		Confidence:            floatValue(wire.Confidence),
	}
}

func groundingRequirementString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		if typed {
			return "required"
		}
		return "none"
	case nil:
		return ""
	case []any, []string:
		return strings.Join(stringList(typed), ", ")
	default:
		return strings.TrimSpace(strings.Trim(fmt.Sprint(typed), `"`))
	}
}

func textValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case nil:
		return ""
	default:
		return strings.TrimSpace(strings.Trim(fmt.Sprint(typed), `"`))
	}
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := textValue(item); text != "" {
				result = append(result, text)
			}
		}
		return result
	case string:
		if strings.TrimSpace(typed) == "" {
			return nil
		}
		return []string{strings.TrimSpace(typed)}
	default:
		return nil
	}
}

func boolValue(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		switch strings.ToLower(strings.TrimSpace(typed)) {
		case "true", "yes", "required", "needed", "1":
			return true
		default:
			return false
		}
	case []any:
		return len(typed) > 0
	case float64:
		return typed != 0
	default:
		return false
	}
}

func floatValue(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case string:
		var parsed float64
		if _, err := fmt.Sscanf(strings.TrimSpace(typed), "%f", &parsed); err == nil {
			return parsed
		}
		return 0
	default:
		return 0
	}
}

func intentProfileSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"required": []string{
			"task_type",
			"complexity",
			"domains",
			"required_capabilities",
			"risk_level",
			"needs_user_confirmation",
			"grounding_requirement",
			"confidence",
		},
		"additionalProperties": false,
		"properties": map[string]any{
			"task_type":               map[string]any{"type": "string"},
			"complexity":              map[string]any{"type": "string"},
			"domains":                 arraySchema("string"),
			"required_capabilities":   arraySchema("string"),
			"risk_level":              map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "critical"}},
			"needs_user_confirmation": map[string]any{"type": "boolean"},
			"grounding_requirement":   map[string]any{"type": "string"},
			"confidence":              map[string]any{"type": "number"},
		},
	}
}

func arraySchema(itemType string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": itemType}}
}
