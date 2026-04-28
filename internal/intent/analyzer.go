package intent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/sukeke/agent-gogo/internal/chain"
	"github.com/sukeke/agent-gogo/internal/contextbuilder"
	"github.com/sukeke/agent-gogo/internal/provider"
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
	resp, err := a.llm.Chat(ctx, provider.ChatRequest{
		Model: a.model,
		Messages: []provider.ChatMessage{
			{Role: "system", Content: intentSystemPrompt},
			{Role: "user", Content: string(payload)},
		},
	})
	if err != nil {
		return Profile{}, err
	}
	var profile Profile
	if err := decodeProfileJSONObject(resp.Text, &profile); err != nil {
		return Profile{}, err
	}
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
	profile.Domains = sortedUnique(profile.Domains)
	profile.RequiredCapabilities = sortedUnique(profile.RequiredCapabilities)
	profile.RiskLevel = strings.ToLower(profile.RiskLevel)
	return profile
}

const intentSystemPrompt = `You are the Intent Analyzer for agent-gogo.
Return only one JSON object with:
task_type, complexity, domains, required_capabilities, risk_level, needs_user_confirmation, grounding_requirement, confidence.
Keep the answer small and do not include tool schemas. Do not include markdown.`

func decodeJSONObject(text string, target any) error {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start >= 0 && end >= start {
		text = text[start : end+1]
	}
	return json.Unmarshal([]byte(text), target)
}

func decodeProfileJSONObject(text string, target *Profile) error {
	var wire struct {
		TaskType              any `json:"task_type"`
		Complexity            any `json:"complexity"`
		Domains               any `json:"domains"`
		RequiredCapabilities  any `json:"required_capabilities"`
		RiskLevel             any `json:"risk_level"`
		NeedsUserConfirmation any `json:"needs_user_confirmation"`
		GroundingRequirement  any `json:"grounding_requirement"`
		Confidence            any `json:"confidence"`
	}
	if err := decodeJSONObject(text, &wire); err != nil {
		return err
	}
	*target = Profile{
		TaskType:              textValue(wire.TaskType),
		Complexity:            textValue(wire.Complexity),
		Domains:               stringList(wire.Domains),
		RequiredCapabilities:  stringList(wire.RequiredCapabilities),
		RiskLevel:             textValue(wire.RiskLevel),
		NeedsUserConfirmation: boolValue(wire.NeedsUserConfirmation),
		GroundingRequirement:  groundingRequirementString(wire.GroundingRequirement),
		Confidence:            floatValue(wire.Confidence),
	}
	return nil
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

func sortedUnique(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	out := result[:0]
	var previous string
	for i, value := range result {
		if i > 0 && value == previous {
			continue
		}
		out = append(out, value)
		previous = value
	}
	if out == nil {
		return []string{}
	}
	return out
}
