package chain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/llmjson"
	"github.com/SukeyByte/agent-gogo/internal/prompts"
	"github.com/SukeyByte/agent-gogo/internal/provider"
)

type Level string

const (
	LevelDirect  Level = "L0"
	LevelAssist  Level = "L1"
	LevelPlanned Level = "L2"
	LevelProject Level = "L3"
)

type Decision struct {
	Level          Level    `json:"level"`
	Reason         string   `json:"reason"`
	NeedPlan       bool     `json:"need_plan"`
	NeedTools      bool     `json:"need_tools"`
	NeedMemory     bool     `json:"need_memory"`
	NeedReview     bool     `json:"need_review"`
	NeedBrowser    bool     `json:"need_browser"`
	NeedCode       bool     `json:"need_code"`
	NeedDocs       bool     `json:"need_docs"`
	RequiresDAG    bool     `json:"requires_dag"`
	EstimatedSteps int      `json:"estimated_steps"`
	PersonaIDs     []string `json:"persona_ids"`
	SkillTags      []string `json:"skill_tags"`
	ToolNames      []string `json:"tool_names"`
	RiskLevel      string   `json:"risk_level"`
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
	if isBrowserOnlyActionSequence(decision) && decision.Level == LevelProject {
		decision.Level = LevelPlanned
	}
	if decision.RequiresDAG || (decision.EstimatedSteps >= 4 && !isBrowserOnlyActionSequence(decision)) {
		decision.Level = LevelProject
		decision.NeedPlan = true
		decision.NeedTools = true
		decision.NeedReview = true
	}
	if decision.Level == LevelProject {
		decision.NeedPlan = true
		if decision.EstimatedSteps < 4 {
			decision.EstimatedSteps = 4
		}
	}
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
	return normalizeDecision(decisionFromWire(wire)), nil
}

type routeDecisionWire struct {
	Level          Level  `json:"level"`
	Reason         string `json:"reason"`
	NeedPlan       any    `json:"need_plan"`
	NeedTools      any    `json:"need_tools"`
	NeedMemory     any    `json:"need_memory"`
	NeedReview     any    `json:"need_review"`
	NeedBrowser    any    `json:"need_browser"`
	NeedCode       any    `json:"need_code"`
	NeedDocs       any    `json:"need_docs"`
	RequiresDAG    any    `json:"requires_dag"`
	EstimatedSteps any    `json:"estimated_steps"`
	PersonaIDs     any    `json:"persona_ids"`
	SkillTags      any    `json:"skill_tags"`
	ToolNames      any    `json:"tool_names"`
	RiskLevel      any    `json:"risk_level"`
}

func decisionFromWire(wire routeDecisionWire) Decision {
	toolNames := stringList(wire.ToolNames)
	if names := stringList(wire.NeedTools); len(toolNames) == 0 && len(names) > 0 {
		toolNames = names
	}
	return Decision{
		Level:          wire.Level,
		Reason:         wire.Reason,
		NeedPlan:       boolish(wire.NeedPlan),
		NeedTools:      boolish(wire.NeedTools),
		NeedMemory:     boolish(wire.NeedMemory),
		NeedReview:     boolish(wire.NeedReview),
		NeedBrowser:    boolish(wire.NeedBrowser),
		NeedCode:       boolish(wire.NeedCode),
		NeedDocs:       boolish(wire.NeedDocs),
		RequiresDAG:    boolish(wire.RequiresDAG),
		EstimatedSteps: intish(wire.EstimatedSteps),
		PersonaIDs:     stringList(wire.PersonaIDs),
		SkillTags:      stringList(wire.SkillTags),
		ToolNames:      toolNames,
		RiskLevel:      riskLevelValue(wire.RiskLevel),
	}
}

func IsProjectScale(decision Decision) bool {
	if isBrowserOnlyActionSequence(decision) {
		return false
	}
	return decision.Level == LevelProject || decision.RequiresDAG || decision.EstimatedSteps >= 4
}

func isBrowserOnlyActionSequence(decision Decision) bool {
	return decision.NeedBrowser && !decision.NeedCode && !decision.NeedDocs && !decision.RequiresDAG && decision.EstimatedSteps <= 6
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

func intish(value any) int {
	switch typed := value.(type) {
	case int:
		if typed < 0 {
			return 0
		}
		return typed
	case float64:
		if typed < 0 {
			return 0
		}
		return int(typed)
	case string:
		text := strings.TrimSpace(typed)
		if text == "" {
			return 0
		}
		var parsed int
		if _, err := fmt.Sscanf(text, "%d", &parsed); err == nil && parsed > 0 {
			return parsed
		}
		return 0
	default:
		return 0
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
	return objectSchema([]string{"level", "reason", "need_plan", "need_tools", "need_memory", "need_review", "need_browser", "need_code", "need_docs", "requires_dag", "estimated_steps", "persona_ids", "skill_tags", "tool_names", "risk_level"}, map[string]any{
		"level":           map[string]any{"type": "string", "enum": []string{"L0", "L1", "L2", "L3"}},
		"reason":          map[string]any{"type": "string"},
		"need_plan":       map[string]any{"type": "boolean"},
		"need_tools":      map[string]any{"type": "boolean"},
		"need_memory":     map[string]any{"type": "boolean"},
		"need_review":     map[string]any{"type": "boolean"},
		"need_browser":    map[string]any{"type": "boolean"},
		"need_code":       map[string]any{"type": "boolean"},
		"need_docs":       map[string]any{"type": "boolean"},
		"requires_dag":    map[string]any{"type": "boolean"},
		"estimated_steps": map[string]any{"type": "integer", "minimum": 0},
		"persona_ids":     arraySchema("string"),
		"skill_tags":      arraySchema("string"),
		"tool_names":      arraySchema("string"),
		"risk_level":      map[string]any{"type": "string", "enum": []string{"low", "medium", "high", "critical"}},
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
