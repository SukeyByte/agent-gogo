package observer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/tools"
)

type StateStatus string

const (
	StateUnknown   StateStatus = "unknown"
	StateSucceeded StateStatus = "succeeded"
	StateFailed    StateStatus = "failed"
	StateNeedsUser StateStatus = "needs_user_input"
	StateChanged   StateStatus = "changed"
	StateVerified  StateStatus = "verified"
	StateObserved  StateStatus = "observed"
)

type Store interface {
	CreateObservation(ctx context.Context, observation domain.Observation) (domain.Observation, error)
	AddTaskEvent(ctx context.Context, event domain.TaskEvent) (domain.TaskEvent, error)
}

type State struct {
	Status        StateStatus
	Type          string
	Summary       string
	EvidenceRef   string
	FailureReason string
	Signals       map[string]string
}

type ToolResultRequest struct {
	Task     domain.Task
	Attempt  domain.TaskAttempt
	Response tools.CallResponse
}

type Interpreter struct {
	store Store
}

func NewInterpreter(store Store) *Interpreter {
	return &Interpreter{store: store}
}

func (i *Interpreter) InterpretToolResult(ctx context.Context, req ToolResultRequest) (State, error) {
	if err := ctx.Err(); err != nil {
		return State{}, err
	}
	state := InterpretToolCall(req.Response.ToolCall, req.Response.Result)
	if i.store == nil {
		return state, nil
	}
	payload, err := json.Marshal(map[string]any{
		"status":  state.Status,
		"signals": state.Signals,
		"tool":    req.Response.ToolCall.Name,
		"output":  req.Response.Result.Output,
		"error":   req.Response.Result.Error,
	})
	if err != nil {
		return State{}, err
	}
	observation, err := i.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   req.Attempt.ID,
		ToolCallID:  req.Response.ToolCall.ID,
		Type:        state.Type,
		Summary:     state.Summary,
		EvidenceRef: state.EvidenceRef,
		Payload:     string(payload),
	})
	if err != nil {
		return State{}, err
	}
	state.EvidenceRef = firstNonEmpty(state.EvidenceRef, observation.EvidenceRef)
	if _, err := i.store.AddTaskEvent(ctx, domain.TaskEvent{
		TaskID:    req.Task.ID,
		AttemptID: req.Attempt.ID,
		Type:      "observer.state_interpreted",
		Message:   state.Summary,
		Payload:   fmt.Sprintf(`{"state":%q,"observation_id":%q}`, state.Status, observation.ID),
	}); err != nil {
		return State{}, err
	}
	return state, nil
}

func InterpretToolCall(call domain.ToolCall, result tools.Result) State {
	state := State{
		Status:      StateSucceeded,
		Type:        "state.tool_succeeded",
		Summary:     fmt.Sprintf("%s completed", call.Name),
		EvidenceRef: firstNonEmpty(result.EvidenceRef, call.EvidenceRef),
		Signals:     map[string]string{"tool": call.Name},
	}
	if !result.Success || strings.TrimSpace(result.Error) != "" || call.Status == domain.ToolCallStatusFailed {
		state.Status = StateFailed
		state.Type = "state.tool_failed"
		state.FailureReason = firstNonEmpty(result.Error, call.Error, "tool failed")
		state.Summary = fmt.Sprintf("%s failed: %s", call.Name, state.FailureReason)
		switch call.Name {
		case "test.run":
			state.Type = "state.tests_failed"
			state.Summary = firstNonEmpty(stringOutput(result.Output, "summary"), state.Summary)
		case "shell.run":
			state.Type = "state.command_failed"
		}
		state.Signals["failure_reason"] = state.FailureReason
		return state
	}
	switch call.Name {
	case "file.write":
		state.Status = StateChanged
		state.Type = "state.file_changed"
		state.Summary = fmt.Sprintf("wrote %s", stringOutput(result.Output, "path"))
		state.Signals["path"] = stringOutput(result.Output, "path")
	case "file.patch":
		state.Status = StateChanged
		state.Type = "state.file_changed"
		state.Summary = fmt.Sprintf("patched %s", stringOutput(result.Output, "path"))
		state.Signals["path"] = stringOutput(result.Output, "path")
	case "artifact.write", "document.write":
		state.Status = StateChanged
		state.Type = "state.artifact_written"
		state.Summary = fmt.Sprintf("wrote artifact %s", firstNonEmpty(stringOutput(result.Output, "artifact_ref"), stringOutput(result.Output, "path")))
	case "memory.save":
		state.Status = StateChanged
		state.Type = "state.memory_persisted"
		state.Summary = fmt.Sprintf("persisted memory %s", stringOutput(result.Output, "memory_ref"))
	case "test.run":
		passed := boolOutput(result.Output, "passed")
		state.Status = StateVerified
		state.Type = "state.tests_passed"
		state.Summary = fmt.Sprintf("test command passed: %s", stringOutput(result.Output, "command"))
		if !passed {
			state.Status = StateFailed
			state.Type = "state.tests_failed"
			state.FailureReason = firstNonEmpty(result.Error, stringOutput(result.Output, "summary"), "test command failed")
			state.Summary = state.FailureReason
		}
	case "shell.run":
		passed := boolOutput(result.Output, "passed")
		state.Status = StateVerified
		state.Type = "state.command_passed"
		state.Summary = fmt.Sprintf("command passed: %s", stringOutput(result.Output, "command"))
		if !passed {
			state.Status = StateFailed
			state.Type = "state.command_failed"
			state.FailureReason = firstNonEmpty(result.Error, stringOutput(result.Output, "summary"), "command failed")
			state.Summary = state.FailureReason
		}
	case "git.status", "git.diff", "file.diff":
		state.Status = StateObserved
		state.Type = "state.repository_observed"
		state.Summary = call.Name + " captured repository evidence"
	case "browser.open", "browser.click", "browser.type", "browser.input", "browser.wait", "browser.extract", "browser.dom_summary", "browser.screenshot":
		state.Status = StateObserved
		state.Type = "state.browser_observed"
		state.Summary = call.Name + " captured browser state"
	}
	state.Summary = strings.TrimSpace(state.Summary)
	if state.Summary == "" {
		state.Summary = fmt.Sprintf("%s completed", call.Name)
	}
	return state
}

func stringOutput(output map[string]any, key string) string {
	if output == nil {
		return ""
	}
	value, _ := output[key].(string)
	return strings.TrimSpace(value)
}

func boolOutput(output map[string]any, key string) bool {
	if output == nil {
		return false
	}
	value, _ := output[key].(bool)
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
