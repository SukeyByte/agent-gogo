package reviewer

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/llmjson"
	"github.com/sukeke/agent-gogo/internal/provider"
)

type LLMReviewStore interface {
	TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error)
	CreateReviewResult(ctx context.Context, result domain.ReviewResult) (domain.ReviewResult, error)
	CompleteTaskAttempt(ctx context.Context, attemptID string, status domain.AttemptStatus, message string) (domain.TaskAttempt, error)
	ListObservationsByAttempt(ctx context.Context, attemptID string) ([]domain.Observation, error)
	ListToolCallsByAttempt(ctx context.Context, attemptID string) ([]domain.ToolCall, error)
}

type LLMReviewer struct {
	store LLMReviewStore
	llm   provider.LLMProvider
	model string
}

func NewLLMReviewer(store LLMReviewStore, llm provider.LLMProvider, model string) *LLMReviewer {
	return &LLMReviewer{store: store, llm: llm, model: model}
}

func (r *LLMReviewer) Review(ctx context.Context, task domain.Task, attempt domain.TaskAttempt) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if r.llm == nil {
		return Result{}, errors.New("llm provider is required")
	}
	observations, err := r.store.ListObservationsByAttempt(ctx, attempt.ID)
	if err != nil {
		return Result{}, err
	}
	toolCalls, err := r.store.ListToolCallsByAttempt(ctx, attempt.ID)
	if err != nil {
		return Result{}, err
	}
	payload := map[string]any{
		"task":         task,
		"observations": observations,
		"tool_calls":   toolCalls,
		"instruction":  "Return JSON only: {\"approved\":true|false,\"summary\":\"...\"}. Approve when the task acceptance criteria are satisfied by observation summaries or payloads. Do not require a separate report/file/console output unless the task explicitly asks to create one.",
	}
	var decision llmReviewDecision
	if err := llmjson.ChatObject(ctx, llmjson.Request{
		LLM:        r.llm,
		Model:      r.model,
		System:     llmReviewerSystemPrompt,
		User:       mustJSON(payload),
		SchemaName: "review_decision",
		Schema:     reviewDecisionSchema(),
		Metadata:   map[string]string{"stage": "reviewer.llm"},
		MaxRepairs: 1,
	}, &decision); err != nil {
		return Result{}, err
	}
	summary := strings.TrimSpace(decision.Summary)
	if summary == "" {
		summary = "llm reviewer returned an empty summary"
	}
	if !decision.Approved {
		result, createErr := r.store.CreateReviewResult(ctx, domain.ReviewResult{
			AttemptID: attempt.ID,
			Status:    domain.ReviewStatusRejected,
			Summary:   summary,
		})
		if createErr != nil {
			return Result{}, createErr
		}
		failedTask, transitionErr := r.store.TransitionTask(ctx, task.ID, domain.TaskStatusReviewFailed, summary)
		if transitionErr != nil {
			return Result{}, transitionErr
		}
		completedAttempt, completeErr := r.store.CompleteTaskAttempt(ctx, attempt.ID, domain.AttemptStatusFailed, summary)
		if completeErr != nil {
			return Result{}, completeErr
		}
		return Result{Task: failedTask, Attempt: completedAttempt, ReviewResult: result}, errors.New(summary)
	}
	result, err := r.store.CreateReviewResult(ctx, domain.ReviewResult{
		AttemptID: attempt.ID,
		Status:    domain.ReviewStatusApproved,
		Summary:   summary,
	})
	if err != nil {
		return Result{}, err
	}
	doneTask, err := r.store.TransitionTask(ctx, task.ID, domain.TaskStatusDone, "llm reviewer approved task completion")
	if err != nil {
		return Result{}, err
	}
	completedAttempt, err := r.store.CompleteTaskAttempt(ctx, attempt.ID, domain.AttemptStatusSucceeded, "llm reviewer approved task completion")
	if err != nil {
		return Result{}, err
	}
	return Result{Task: doneTask, Attempt: completedAttempt, ReviewResult: result}, nil
}

type llmReviewDecision struct {
	Approved bool   `json:"approved"`
	Summary  string `json:"summary"`
}

const llmReviewerSystemPrompt = `You are agent-gogo's reviewer.
Return JSON only with fields approved and summary.
Reject empty, ungrounded, or unverifiable task outputs.
Judge only the task's stated acceptance criteria. Do not invent extra requirements.
Observation summaries and observation payloads are the task output evidence.
Tool calls, including their input_json and output_json, are also task evidence.
Approve if the acceptance criteria are satisfied anywhere in the observations, including tool output payloads and agent.finish summaries.
For file.patch tasks, the old/new text in tool_calls.input_json is valid evidence of what changed.
Do not require go build, go test, lint, or compile evidence unless the task acceptance criteria explicitly ask for that verification.
Do not reject solely because earlier tool calls failed when later observations contain enough successful evidence.
Do not require a separate structured report, document, or console output unless the task explicitly asks to create that artifact.
For browser tasks, visible DOM text plus evidence URL is valid evidence; do not require raw HTML or HTTP status unless the user explicitly requested raw HTML or status codes.`

func reviewDecisionSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"required":             []string{"approved", "summary"},
		"additionalProperties": false,
		"properties": map[string]any{
			"approved": map[string]any{"type": "boolean"},
			"summary":  map[string]any{"type": "string"},
		},
	}
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
