package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/llmjson"
	"github.com/SukeyByte/agent-gogo/internal/observer"
	"github.com/SukeyByte/agent-gogo/internal/prompts"
	"github.com/SukeyByte/agent-gogo/internal/provider"
	"github.com/SukeyByte/agent-gogo/internal/tools"
)

type GenericStore interface {
	Store
	AddTaskEvent(ctx context.Context, event domain.TaskEvent) (domain.TaskEvent, error)
	CompleteTaskAttempt(ctx context.Context, attemptID string, status domain.AttemptStatus, message string) (domain.TaskAttempt, error)
}

type ToolRuntime interface {
	Call(ctx context.Context, req tools.CallRequest) (tools.CallResponse, error)
	ListSpecs() []tools.Spec
}

type GenericExecutorOptions struct {
	Store    GenericStore
	Tools    ToolRuntime
	LLM      provider.LLMProvider
	Model    string
	Observer *observer.Interpreter
	MaxSteps int
}

type GenericExecutor struct {
	store              GenericStore
	tools              ToolRuntime
	llm                provider.LLMProvider
	model              string
	observer           *observer.Interpreter
	maxSteps           int
	contextByProjectID map[string]string
}

type ExecutionError struct {
	Task    domain.Task
	Attempt domain.TaskAttempt
	Err     error
}

func (e *ExecutionError) Error() string {
	if e == nil || e.Err == nil {
		return "execution failed"
	}
	return e.Err.Error()
}

func (e *ExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewGenericExecutor(options GenericExecutorOptions) *GenericExecutor {
	maxSteps := options.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 12
	}
	interpreter := options.Observer
	if interpreter == nil {
		interpreter = observer.NewInterpreter(options.Store)
	}
	return &GenericExecutor{
		store:              options.Store,
		tools:              options.Tools,
		llm:                options.LLM,
		model:              options.Model,
		observer:           interpreter,
		maxSteps:           maxSteps,
		contextByProjectID: map[string]string{},
	}
}

func (e *GenericExecutor) UseRuntimeContext(projectID string, contextText string) {
	if e.contextByProjectID == nil {
		e.contextByProjectID = map[string]string{}
	}
	e.contextByProjectID[projectID] = contextText
}

func (e *GenericExecutor) Execute(ctx context.Context, task domain.Task) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if e.store == nil {
		return Result{}, errors.New("generic executor store is required")
	}
	inProgress, err := e.store.TransitionTask(ctx, task.ID, domain.TaskStatusInProgress, "generic executor started action loop")
	if err != nil {
		return Result{}, err
	}
	attempt, err := e.store.CreateTaskAttempt(ctx, task.ID)
	if err != nil {
		return Result{}, err
	}
	if e.llm == nil || e.tools == nil {
		return e.finishWithoutTools(ctx, inProgress, attempt, "generic executor completed without llm/tool runtime")
	}
	events := []actionEvent{}
	fingerprints := map[string]int{}
	for step := 1; step <= e.maxSteps; step++ {
		if summary, ok := autoFinishSummary(inProgress, events); ok {
			return e.finishWithoutTools(ctx, inProgress, attempt, summary)
		}
		action, err := e.nextAction(ctx, inProgress, attempt, step, events)
		if err != nil {
			return e.fail(ctx, inProgress, attempt, err)
		}
		action.Args = normalizeActionArgsForTask(inProgress, action.Tool, action.Args)
		fingerprint := actionFingerprint(action)
		if fingerprint != "" {
			fingerprints[fingerprint]++
			if fingerprints[fingerprint] > 2 {
				events = append(events, actionEvent{
					Step:    step,
					Action:  action.Action,
					Tool:    action.Tool,
					State:   "rejected",
					Summary: "repeated action blocked by progress guard",
					Error:   "same action and arguments repeated too many times",
				})
				continue
			}
		}
		if err := e.recordAction(ctx, inProgress.ID, attempt.ID, step, action); err != nil {
			return Result{}, err
		}
		switch action.Action {
		case "finish":
			summary := strings.TrimSpace(action.Summary)
			if summary == "" {
				summary = strings.TrimSpace(action.Reason)
			}
			if summary == "" {
				summary = "generic action loop finished"
			}
			if ok, reason := finishEvidenceReady(inProgress, events); !ok {
				events = append(events, actionEvent{
					Step:    step,
					Action:  "finish",
					State:   "rejected",
					Summary: summary,
					Error:   reason,
				})
				continue
			}
			return e.finishWithoutTools(ctx, inProgress, attempt, summary)
		case "ask_user":
			question := firstNonEmpty(action.Question, action.Reason, "user input required")
			needInput, transitionErr := e.store.TransitionTask(ctx, inProgress.ID, domain.TaskStatusNeedUserInput, question)
			if transitionErr != nil {
				return Result{}, transitionErr
			}
			if _, completeErr := e.store.CompleteTaskAttempt(ctx, attempt.ID, domain.AttemptStatusCancelled, question); completeErr != nil {
				return Result{}, completeErr
			}
			return Result{Task: needInput, Attempt: attempt}, &ExecutionError{Task: needInput, Attempt: attempt, Err: errors.New(question)}
		case "tool_call":
			if strings.TrimSpace(action.Tool) == "" {
				return e.fail(ctx, inProgress, attempt, errors.New("tool_call action missing tool"))
			}
			response, callErr := e.tools.Call(ctx, tools.CallRequest{
				AttemptID: attempt.ID,
				Name:      action.Tool,
				Args:      action.Args,
			})
			state, observeErr := e.observer.InterpretToolResult(ctx, observer.ToolResultRequest{
				Task:     inProgress,
				Attempt:  attempt,
				Response: response,
			})
			if observeErr != nil {
				return Result{}, observeErr
			}
			events = append(events, actionEvent{
				Step:        step,
				Action:      action.Action,
				Tool:        action.Tool,
				Summary:     state.Summary,
				State:       string(state.Status),
				Error:       state.FailureReason,
				EvidenceRef: state.EvidenceRef,
				Output:      compactToolOutput(response.Result.Output),
			})
			if callErr != nil {
				continue
			}
			if summary, ok := autoFinishSummary(inProgress, events); ok {
				return e.finishWithoutTools(ctx, inProgress, attempt, summary)
			}
		default:
			events = append(events, actionEvent{
				Step:    step,
				Action:  firstNonEmpty(action.Action, "unknown"),
				State:   "rejected",
				Summary: firstNonEmpty(action.Reason, action.Summary, "unsupported generic action"),
				Error:   fmt.Sprintf("unsupported generic action %q", action.Action),
			})
			continue
		}
	}
	return e.fail(ctx, inProgress, attempt, errors.New("generic executor reached max action steps without accepted finish"))
}

func (e *GenericExecutor) nextAction(ctx context.Context, task domain.Task, attempt domain.TaskAttempt, step int, events []actionEvent) (agentAction, error) {
	payload, err := json.Marshal(map[string]any{
		"task": map[string]any{
			"id":                  task.ID,
			"title":               task.Title,
			"description":         task.Description,
			"acceptance_criteria": task.AcceptanceCriteria,
		},
		"attempt_id":      attempt.ID,
		"step":            step,
		"runtime_context": e.contextByProjectID[task.ProjectID],
		"available_tools": toolSchemas(e.tools.ListSpecs()),
		"prior_events":    events,
	})
	if err != nil {
		return agentAction{}, err
	}
	specs := e.tools.ListSpecs()
	var action agentAction
	if err := llmjson.ChatObject(ctx, llmjson.Request{
		LLM:        e.llm,
		Model:      e.model,
		System:     genericExecutorPrompt,
		User:       string(payload),
		SchemaName: "generic_agent_action",
		Schema:     agentActionSchema(specs),
		Metadata: map[string]string{
			"stage":      "executor.generic.action",
			"task_id":    task.ID,
			"attempt_id": attempt.ID,
			"step":       fmt.Sprint(step),
		},
		MaxRepairs: 1,
	}, &action); err != nil {
		return agentAction{}, err
	}
	action.Action = strings.TrimSpace(action.Action)
	action.Tool = e.normalizeToolName(action.Tool)
	if action.Args == nil {
		action.Args = map[string]any{}
	}
	if toolName := e.normalizeToolName(action.Action); e.isAvailableTool(toolName) {
		action.Tool = toolName
		action.Action = "tool_call"
	}
	return action, nil
}

func (e *GenericExecutor) recordAction(ctx context.Context, taskID string, attemptID string, step int, action agentAction) error {
	payload, err := json.Marshal(action)
	if err != nil {
		return err
	}
	_, err = e.store.AddTaskEvent(ctx, domain.TaskEvent{
		TaskID:    taskID,
		AttemptID: attemptID,
		Type:      "agent.action_selected",
		Message:   fmt.Sprintf("step %d selected %s %s", step, action.Action, action.Tool),
		Payload:   string(payload),
	})
	return err
}

func (e *GenericExecutor) finishWithoutTools(ctx context.Context, task domain.Task, attempt domain.TaskAttempt, summary string) (Result, error) {
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   attempt.ID,
		Type:        "agent.finish",
		Summary:     summary,
		EvidenceRef: "agent://finish",
	}); err != nil {
		return Result{}, err
	}
	implemented, err := e.store.TransitionTask(ctx, task.ID, domain.TaskStatusImplemented, summary)
	if err != nil {
		return Result{}, err
	}
	return Result{Task: implemented, Attempt: attempt}, nil
}

func (e *GenericExecutor) fail(ctx context.Context, task domain.Task, attempt domain.TaskAttempt, cause error) (Result, error) {
	message := strings.TrimSpace(cause.Error())
	if message == "" {
		message = "generic action loop failed"
	}
	failed, err := e.store.TransitionTask(ctx, task.ID, domain.TaskStatusFailed, message)
	if err != nil {
		return Result{}, err
	}
	completedAttempt, err := e.store.CompleteTaskAttempt(ctx, attempt.ID, domain.AttemptStatusFailed, message)
	if err != nil {
		return Result{}, err
	}
	return Result{}, &ExecutionError{Task: failed, Attempt: completedAttempt, Err: cause}
}

var genericExecutorPrompt = prompts.Text("generic_executor")
