package planner

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/provider"
)

type LLMPlanner struct {
	llm   provider.LLMProvider
	model string
}

func NewLLMPlanner(llm provider.LLMProvider, model string) *LLMPlanner {
	return &LLMPlanner{llm: llm, model: model}
}

func (p *LLMPlanner) PlanProject(ctx context.Context, req PlanRequest) ([]domain.Task, error) {
	if p.llm == nil {
		return nil, errors.New("llm provider is required")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	resp, err := p.llm.Chat(ctx, provider.ChatRequest{
		Model: p.model,
		Messages: []provider.ChatMessage{
			{Role: "system", Content: plannerSystemPrompt},
			{Role: "user", Content: string(payload)},
		},
	})
	if err != nil {
		return nil, err
	}
	var output plannerOutput
	if err := decodeJSONObject(resp.Text, &output); err != nil {
		return nil, err
	}
	if len(output.Tasks) == 0 {
		return nil, errors.New("planner returned no tasks")
	}

	tasks := make([]domain.Task, 0, len(output.Tasks))
	for _, planned := range output.Tasks {
		title := strings.TrimSpace(planned.Title)
		if title == "" {
			return nil, errors.New("planner task title is required")
		}
		criteria := sortedUnique(planned.Acceptance)
		if len(criteria) == 0 {
			return nil, errors.New("planner task acceptance criteria are required")
		}
		description := strings.TrimSpace(planned.Goal)
		if description == "" {
			description = strings.TrimSpace(planned.Description)
		}
		tasks = append(tasks, domain.Task{
			ProjectID:          req.Project.ID,
			Title:              title,
			Description:        description,
			Status:             domain.TaskStatusDraft,
			AcceptanceCriteria: criteria,
		})
	}
	return tasks, nil
}

type plannerOutput struct {
	Tasks []plannedTask `json:"tasks"`
}

type plannedTask struct {
	Title       string   `json:"title"`
	Goal        string   `json:"goal"`
	Description string   `json:"description"`
	Type        string   `json:"type"`
	DependsOn   []string `json:"depends_on"`
	Acceptance  []string `json:"acceptance"`
}

const plannerSystemPrompt = `You are the Planner for agent-gogo.
Return only JSON with this shape:
{"tasks":[{"title":"...","goal":"...","type":"code|browser|document|runtime|general","depends_on":[],"acceptance":["..."]}]}
Rules:
- Planner only creates DRAFT task content.
- Each task must have a clear title, goal, type, dependencies by title, and acceptance criteria.
- Do not combine execution, testing, and review into one acceptance-free task.
- For browser tasks, require visible page text, DOM summary, user-facing content, and evidence URL; do not require raw HTML or HTTP status unless the user explicitly asks for them.
- Do not include markdown.`

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
