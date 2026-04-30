package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	appconfig "github.com/SukeyByte/agent-gogo/internal/config"
	"github.com/SukeyByte/agent-gogo/internal/contextbuilder"
	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/observability"
	"github.com/SukeyByte/agent-gogo/internal/provider"
	"github.com/SukeyByte/agent-gogo/internal/store"
)

func extractPersonalSiteName(goal string) string {
	for _, marker := range []string{"为", "给"} {
		start := strings.Index(goal, marker)
		if start == -1 {
			continue
		}
		candidate := goal[start+len(marker):]
		stop := len(candidate)
		for _, sep := range []string{"写", "做", "创建", "生成", "建立", "设计", "搭建", "的"} {
			if index := strings.Index(candidate, sep); index >= 0 && index < stop {
				stop = index
			}
		}
		name := strings.Trim(candidate[:stop], " \t\r\n，。,.!?！？")
		if name != "" && len([]rune(name)) <= 12 {
			return name
		}
	}
	if strings.Contains(goal, "苏柯宇") {
		return "苏柯宇"
	}
	return "苏柯宇"
}

func renderEvents(events []domain.TaskEvent) string {
	var builder strings.Builder
	for i, event := range events {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(fmt.Sprintf("%s %s -> %s %s", event.Type, event.FromState, event.ToState, event.Message))
	}
	return builder.String()
}

func latestAnswerObservation(ctx context.Context, sqlite *store.SQLiteStore, attemptID string) (domain.Observation, error) {
	return latestObservation(ctx, sqlite, attemptID, "llm.answer")
}

func latestObservation(ctx context.Context, sqlite *store.SQLiteStore, attemptID string, typ string) (domain.Observation, error) {
	observations, err := sqlite.ListObservationsByAttempt(ctx, attemptID)
	if err != nil {
		return domain.Observation{}, err
	}
	for i := len(observations) - 1; i >= 0; i-- {
		if observations[i].Type == typ {
			return observations[i], nil
		}
	}
	return domain.Observation{}, fmt.Errorf("%s observation not found", typ)
}

func generateNovelistPersona(ctx context.Context, llm provider.LLMProvider, model string, goal string, logger observability.Logger) (contextbuilder.Persona, error) {
	resp, err := llm.Chat(ctx, provider.ChatRequest{
		Model: model,
		Messages: []provider.ChatMessage{
			{Role: "system", Content: "你是 agent-gogo 的 Persona Composer。请为当前任务生成一个临时小说家分人格，只输出可放进系统上下文的简洁角色指令，不要输出 JSON，不要包含 API key。"},
			{Role: "user", Content: goal},
		},
		Metadata: map[string]string{"stage": "persona.generate", "persona_id": "ephemeral-novelist"},
	})
	if err != nil {
		return contextbuilder.Persona{}, err
	}
	instructions := strings.TrimSpace(resp.Text)
	if instructions == "" {
		return contextbuilder.Persona{}, errors.New("generated novelist persona is empty")
	}
	persona := contextbuilder.Persona{
		ID:           "ephemeral-novelist",
		Name:         "小说家分人格",
		VersionHash:  "runtime-generated-v1",
		Instructions: instructions,
	}
	if logger != nil {
		_ = logger.Log(ctx, "persona.generate", persona)
	}
	return persona, nil
}

func storySkillSources(cfg appconfig.Config) []string {
	return append([]string(nil), cfg.Storage.SkillRoots...)
}
