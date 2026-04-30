package executor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/observability"
	"github.com/SukeyByte/agent-gogo/internal/provider"
	"github.com/SukeyByte/agent-gogo/internal/textutil"
	"github.com/SukeyByte/agent-gogo/internal/tools"
)

type StoryStore interface {
	TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error)
	CreateTaskAttempt(ctx context.Context, taskID string) (domain.TaskAttempt, error)
	CreateObservation(ctx context.Context, observation domain.Observation) (domain.Observation, error)
	AddTaskEvent(ctx context.Context, event domain.TaskEvent) (domain.TaskEvent, error)
}

type ToolCaller interface {
	Call(ctx context.Context, req tools.CallRequest) (tools.CallResponse, error)
}

type StoryExecutor struct {
	store              StoryStore
	tools              ToolCaller
	llm                provider.LLMProvider
	model              string
	logger             observability.Logger
	contextByProjectID map[string]string
}

type StoryExecutorOptions struct {
	Store  StoryStore
	Tools  ToolCaller
	LLM    provider.LLMProvider
	Model  string
	Logger observability.Logger
}

func NewStoryExecutor(options StoryExecutorOptions) *StoryExecutor {
	return &StoryExecutor{
		store:              options.Store,
		tools:              options.Tools,
		llm:                options.LLM,
		model:              options.Model,
		logger:             options.Logger,
		contextByProjectID: map[string]string{},
	}
}

func (e *StoryExecutor) UseRuntimeContext(projectID string, contextText string) {
	if e.contextByProjectID == nil {
		e.contextByProjectID = map[string]string{}
	}
	e.contextByProjectID[projectID] = contextText
}

func (e *StoryExecutor) Execute(ctx context.Context, task domain.Task) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if e.llm == nil {
		return Result{}, errors.New("llm provider is required")
	}
	if e.tools == nil {
		return Result{}, errors.New("tool runtime is required")
	}
	inProgress, err := e.store.TransitionTask(ctx, task.ID, domain.TaskStatusInProgress, "story executor started")
	if err != nil {
		return Result{}, err
	}
	attempt, err := e.store.CreateTaskAttempt(ctx, task.ID)
	if err != nil {
		return Result{}, err
	}
	if err := e.addEvent(ctx, task.ID, attempt.ID, "story.persona.active", "using runtime-generated novelist persona from ContextPack"); err != nil {
		return Result{}, err
	}

	draft, err := e.generateStory(ctx, task)
	if err != nil {
		return Result{}, err
	}
	docPath := fmt.Sprintf("stories/%s.md", task.ID)
	doc, err := e.tools.Call(ctx, tools.CallRequest{
		AttemptID: attempt.ID,
		Name:      "document.write",
		Args: map[string]any{
			"path":    docPath,
			"content": draft.Story,
			"summary": draft.Title,
		},
	})
	if err != nil {
		return Result{}, err
	}
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   attempt.ID,
		ToolCallID:  doc.ToolCall.ID,
		Type:        "document.write",
		Summary:     fmt.Sprintf("story document written: %s", doc.Result.Output["artifact_ref"]),
		EvidenceRef: doc.Result.EvidenceRef,
		Payload:     jsonString(doc.Result.Output),
	}); err != nil {
		return Result{}, err
	}
	mem, err := e.tools.Call(ctx, tools.CallRequest{
		AttemptID: attempt.ID,
		Name:      "memory.save",
		Args: map[string]any{
			"key":     task.ID + "-story-key-points",
			"scope":   "project",
			"summary": "短篇推理小说关键记忆: " + draft.Title,
			"body":    draft.Memory,
			"tags":    []any{"story", "mystery", "continuity"},
		},
	})
	if err != nil {
		return Result{}, err
	}
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   attempt.ID,
		ToolCallID:  mem.ToolCall.ID,
		Type:        "memory.save",
		Summary:     fmt.Sprintf("story memory saved: %s", mem.Result.Output["memory_ref"]),
		EvidenceRef: mem.Result.EvidenceRef,
		Payload:     jsonString(mem.Result.Output),
	}); err != nil {
		return Result{}, err
	}
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   attempt.ID,
		Type:        "story.final",
		Summary:     draft.Story,
		EvidenceRef: doc.Result.EvidenceRef,
		Payload:     jsonString(draft),
	}); err != nil {
		return Result{}, err
	}
	implemented, err := e.store.TransitionTask(ctx, inProgress.ID, domain.TaskStatusImplemented, "story executor wrote document and saved memory")
	if err != nil {
		return Result{}, err
	}
	return Result{Task: implemented, Attempt: attempt}, nil
}

type storyDraft struct {
	Title  string `json:"title"`
	Story  string `json:"story"`
	Memory string `json:"memory"`
}

func (e *StoryExecutor) generateStory(ctx context.Context, task domain.Task) (storyDraft, error) {
	req := provider.ChatRequest{
		Model: e.model,
		Messages: []provider.ChatMessage{
			{Role: "system", Content: storySystemPrompt},
			{Role: "user", Content: jsonString(map[string]any{
				"task":                task,
				"acceptance_criteria": task.AcceptanceCriteria,
				"runtime_context":     e.runtimeContext(task.ProjectID),
				"instruction":         "请写一篇完整中文短篇推理小说，并提取后续可复用的 memory。必须返回 JSON。",
			})},
		},
		Metadata: map[string]string{"executor": "story"},
	}
	resp, err := e.llm.Chat(ctx, req)
	if err != nil {
		return storyDraft{}, err
	}
	var draft storyDraft
	if err := textutil.DecodeJSONObject(resp.Text, &draft); err != nil {
		return storyDraft{}, err
	}
	draft.Title = strings.TrimSpace(draft.Title)
	draft.Story = strings.TrimSpace(draft.Story)
	draft.Memory = strings.TrimSpace(draft.Memory)
	if draft.Title == "" || draft.Story == "" || draft.Memory == "" {
		return storyDraft{}, errors.New("story response requires title, story, and memory")
	}
	if e.logger != nil {
		_ = e.logger.Log(ctx, "story.generated", draft)
	}
	return draft, nil
}

func (e *StoryExecutor) runtimeContext(projectID string) string {
	if e.contextByProjectID == nil {
		return ""
	}
	return strings.TrimSpace(e.contextByProjectID[projectID])
}

func (e *StoryExecutor) addEvent(ctx context.Context, taskID string, attemptID string, eventType string, message string) error {
	_, err := e.store.AddTaskEvent(ctx, domain.TaskEvent{
		TaskID:    taskID,
		AttemptID: attemptID,
		Type:      eventType,
		Message:   message,
	})
	return err
}

const storySystemPrompt = `You are agent-gogo's story executor.
Write a complete Chinese short mystery story grounded in the active ContextPack, persona, and story skills.
Return only JSON:
{"title":"...","story":"...","memory":"..."}
Rules:
- The story must be finished prose, not only an outline.
- It must include a fair clue, a misleading clue, a contradiction, and a reveal.
- memory must preserve protagonist, suspects, setting, clue logic, culprit motive, and continuity constraints.
- Do not include API keys or private runtime data.`
