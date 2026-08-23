package reviewer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/llmjson"
	"github.com/SukeyByte/agent-gogo/internal/provider"
)

// ProjectReview is the verdict of a project-level final review: does the
// combined delivery actually achieve the project goal?
type ProjectReview struct {
	Approved bool
	Summary  string
	Gaps     []string
}

// ProjectReviewInput carries everything the reviewer needs to judge the
// project as a whole, not task by task.
type ProjectReviewInput struct {
	Goal        string
	Tasks       []domain.Task
	Reviews     []domain.ReviewResult
	Artifacts   []domain.Artifact
	ReplanRound int
}

const llmProjectReviewerSystemPrompt = `You are agent-gogo's project final reviewer.
You see the project goal, every task with its status, the per-task review summaries, and the artifact list.
Task-level reviews already passed; your job is the INTEGRATION question: assembled together, do these deliverables actually achieve the stated goal?
Look for cross-task gaps: disconnected pieces, missing glue work, unstated implicit requirements, or deliverables that individually pass but jointly fall short.
Return JSON only: {"approved":true|false,"summary":"...","gaps":["..."]}.
Approve generously when the goal is reasonably met; reject with concrete, actionable gaps when it is not.`

type projectReviewDecision struct {
	Approved bool     `json:"approved"`
	Summary  string   `json:"summary"`
	Gaps     []string `json:"gaps"`
}

// ProjectReviewerIface is the project-level final review contract shared
// with the runtime package.
type ProjectReviewerIface interface {
	ReviewProject(ctx context.Context, input ProjectReviewInput) (ProjectReview, error)
}

// LLMProjectReviewer performs the project-level final review in one LLM call.
type LLMProjectReviewer struct {
	llm   provider.LLMProvider
	model string
}

func NewLLMProjectReviewer(llm provider.LLMProvider, model string) *LLMProjectReviewer {
	return &LLMProjectReviewer{llm: llm, model: model}
}

func (r *LLMProjectReviewer) ReviewProject(ctx context.Context, input ProjectReviewInput) (ProjectReview, error) {
	if r.llm == nil {
		return ProjectReview{}, errors.New("llm provider is required")
	}
	payload, err := json.Marshal(input)
	if err != nil {
		return ProjectReview{}, err
	}
	var decision projectReviewDecision
	if err := llmjson.ChatObject(ctx, llmjson.Request{
		LLM:        r.llm,
		Model:      r.model,
		System:     llmProjectReviewerSystemPrompt,
		User:       string(payload),
		SchemaName: "project_final_review",
		Schema: map[string]any{
			"type": "object",
			"required": []string{
				"approved", "summary", "gaps",
			},
			"additionalProperties": false,
			"properties": map[string]any{
				"approved": map[string]any{"type": "boolean"},
				"summary":  map[string]any{"type": "string"},
				"gaps":     map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			},
		},
		Metadata:   map[string]string{"stage": "reviewer.project.final"},
		MaxRepairs: 1,
	}, &decision); err != nil {
		return ProjectReview{}, err
	}
	gaps := make([]string, 0, len(decision.Gaps))
	for _, gap := range decision.Gaps {
		if strings.TrimSpace(gap) != "" {
			gaps = append(gaps, strings.TrimSpace(gap))
		}
	}
	return ProjectReview{Approved: decision.Approved, Summary: decision.Summary, Gaps: gaps}, nil
}
