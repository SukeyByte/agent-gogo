package chain

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

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
	resp, err := r.llm.Chat(ctx, provider.ChatRequest{
		Model: r.model,
		Messages: []provider.ChatMessage{
			{Role: "system", Content: routeSystemPrompt},
			{Role: "user", Content: string(payload)},
		},
	})
	if err != nil {
		return Decision{}, err
	}
	decision, err := decodeDecisionJSONObject(resp.Text)
	if err != nil {
		return Decision{}, err
	}
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
	var wire struct {
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
		RiskLevel   string `json:"risk_level"`
	}
	if err := decodeJSONObject(text, &wire); err != nil {
		return Decision{}, err
	}
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
		RiskLevel:   wire.RiskLevel,
	}, nil
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

const routeSystemPrompt = `You are the Chain Router for agent-gogo.
Return only one JSON object with:
level, reason, need_plan, need_tools, need_memory, need_review, need_browser, need_code, need_docs, persona_ids, skill_tags, tool_names, risk_level.
Use level L0 for direct answers, L1 for assisted single-step tasks, L2 for planned tasks, L3 for project agent tasks.
Do not include markdown.`
