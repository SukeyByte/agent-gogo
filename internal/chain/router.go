package chain

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/sukeke/agent-gogo/internal/llmjson"
	"github.com/sukeke/agent-gogo/internal/prompts"
	"github.com/sukeke/agent-gogo/internal/provider"
)

type Level string

const (
	LevelDirect  Level = "L0"
	LevelAssist  Level = "L1"
	LevelPlanned Level = "L2"
	LevelProject Level = "L3"
)

type Decision struct {
	Level       Level    `json:"level"`
	Reason      string   `json:"reason"`
	NeedPlan    bool     `json:"need_plan"`
	NeedTools   bool     `json:"need_tools"`
	NeedMemory  bool     `json:"need_memory"`
	NeedReview  bool     `json:"need_review"`
	NeedBrowser bool     `json:"need_browser"`
	NeedCode    bool     `json:"need_code"`
	NeedDocs    bool     `json:"need_docs"`
	PersonaIDs  []string `json:"persona_ids"`
	SkillTags   []string `json:"skill_tags"`
	ToolNames   []string `json:"tool_names"`
	RiskLevel   string   `json:"risk_level"`
}

type Request struct {
	UserInput string
	ProjectID string
	Channel   string
}

type Router interface {
	Route(ctx context.Context, req Request) (Decision, error)
}

type LLMRouter struct {
	llm   provider.LLMProvider
	model string
}

func NewLLMRouter(llm provider.LLMProvider, model string) *LLMRouter {
	return &LLMRouter{llm: llm, model: model}
}

func (r *LLMRouter) Route(ctx context.Context, req Request) (Decision, error) {
	if r.llm == nil {
		return Decision{}, errors.New("llm provider is required")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return Decision{}, err
	}
	var wire routeDecisionWire
	err = llmjson.ChatObject(ctx, llmjson.Request{
		LLM:        r.llm,
		Model:      r.model,
		System:     routeSystemPrompt,
		User:       string(payload),
		SchemaName: "chain_router_decision",
		Schema:     routeDecisionSchema(),
		Metadata:   map[string]string{"stage": "chain.router"},
		MaxRepairs: 1,
	}, &wire)
	if err != nil {
		return Decision{}, err
	}
	decision := decisionFromWire(wire)
	if decision.Level == "" {
		return Decision{}, errors.New("chain decision level is required")
	}
	return normalizeDecision(decision), nil
}

func normalizeDecision(decision Decision) Decision {
	decision.PersonaIDs = sortedUnique(decision.PersonaIDs)
	decision.SkillTags = sortedUnique(decision.SkillTags)
	decision.ToolNames = sortedUnique(decision.ToolNames)
	decision.RiskLevel = strings.ToLower(decision.RiskLevel)
	return decision
}

func decodeDecisionJSONObject(text string) (Decision, error) {
	var wire routeDecisionWire
	if err := decodeJSONObject(text, &wire); err != nil {
		return Decision{}, err
	}
	return decisionFromWire(wire), nil
}

type routeDecisionWire struct {
	Level       Level  `json:"level"`
	Reason      string `json:"reason"`
	NeedPlan    any    `json:"need_plan"`
	NeedTools   any    `json:"need_tools"`
	NeedMemory  any    `json:"need_memory"`
	NeedReview  any    `json:"need_review"`
	NeedBrowser any    `json:"need_browser"`
	NeedCode    any    `json:"need_code"`
	NeedDocs    any    `json:"need_docs"`
	PersonaIDs  any    `json:"persona_ids"`
	SkillTags   any    `json:"skill_tags"`
	ToolNames   any    `json:"tool_names"`
	RiskLevel   any    `json:"risk_level"`
}

func decisionFromWire(wire routeDecisionWire) Decision {
	toolNames := stringList(wire.ToolNames)
	if names := stringList(wire.NeedTools); len(toolNames) == 0 && len(names) > 0 {
		toolNames = names
	}
	return Decision{
		Level:       wire.Level,
		Reason:      wire.Reason,
		NeedPlan:    boolish(wire.NeedPlan),
		NeedTools:   boolish(wire.NeedTools),
		NeedMemory:  boolish(wire.NeedMemory),
		NeedReview:  boolish(wire.NeedReview),
		NeedBrowser: boolish(wire.NeedBrowser),
		NeedCode:    boolish(wire.NeedCode),
		NeedDocs:    boolish(wire.NeedDocs),
		PersonaIDs:  stringList(wire.PersonaIDs),
		SkillTags:   stringList(wire.SkillTags),
		ToolNames:   toolNames,
		RiskLevel:   riskLevelValue(wire.RiskLevel),
	}
}

func boolish(value any) bool {
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
	case []string:
		return len(typed) > 0
	case float64:
		return typed != 0
	default:
		return false
	}
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, strings.TrimSpace(text))
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

func riskLevelValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		switch {
		case typed <= 1:
			return "low"
		case typed <= 2:
			return "medium"
		case typed <= 3:
			return "high"
		default:
			return "critical"
		}
	case bool:
		if typed {
			return "medium"
		}
		return "low"
	default:
		return ""
	}
}

var routeSystemPrompt = prompts.Text("chain_router")

func routeDecisionSchema() map[string]any {
	return objectSchema([]string{"level", "reason", "need_plan", "need_tools", "need_memory", "need_review", "need_browser", "need_code", "need_docs", "persona_ids", "skill_tags", "tool_names", "risk_level"}, map[string]any{
		"level":        map[string]any{"type": "string", "enum": []string{"L0", "L1", "L2", "L3"}},
		"reason":       map[string]any{"type": "string"},
		"need_plan":    map[string]any{"type": "boolean"},
		"need_tools":   map[string]any{"type": "boolean"},
		"need_memory":  map[string]any{"type": "boolean"},
		"need_review":  map[string]any{"type": "boolean"},
		"need_browser": map[string]any{"type": "boolean"},
		"need_code":    map[string]any{"type": "boolean"},
		"need_docs":    map[string]any{"type": "boolean"},
		"persona_ids":  arraySchema("string"),
		"skill_tags":   arraySchema("string"),
		"tool_names":   arraySchema("string"),
		"risk_level":   map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "critical"}},
	})
}

func objectSchema(required []string, properties map[string]any) map[string]any {
	return map[string]any{
		"type":                 "object",
		"required":             required,
		"properties":           properties,
		"additionalProperties": false,
	}
}

func arraySchema(itemType string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": itemType}}
}
