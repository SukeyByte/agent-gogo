package tester

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/tools"
)

type CommandStore interface {
	TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error)
	CreateTestResult(ctx context.Context, result domain.TestResult) (domain.TestResult, error)
}

type ToolCaller interface {
	Call(ctx context.Context, req tools.CallRequest) (tools.CallResponse, error)
}

type CommandTester struct {
	store   CommandStore
	tools   ToolCaller
	command string
}

func NewCommandTester(store CommandStore, tools ToolCaller, command string) *CommandTester {
	if strings.TrimSpace(command) == "" {
		command = "go test ./..."
	}
	return &CommandTester{store: store, tools: tools, command: command}
}

func (t *CommandTester) Test(ctx context.Context, task domain.Task, attempt domain.TaskAttempt) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if t.tools == nil {
		return Result{}, errors.New("tool runtime is required")
	}
	testingTask, err := t.store.TransitionTask(ctx, task.ID, domain.TaskStatusTesting, "command tester started")
	if err != nil {
		return Result{}, err
	}
	response, callErr := t.tools.Call(ctx, tools.CallRequest{
		AttemptID: attempt.ID,
		Name:      "test.run",
		Args:      map[string]any{"command": t.command},
	})
	output := commandOutput(response)
	if callErr != nil || !response.Result.Success {
		message := strings.TrimSpace(firstNonEmpty(response.Result.Error, errorString(callErr), "test command failed"))
		result, createErr := t.store.CreateTestResult(ctx, domain.TestResult{
			AttemptID:   attempt.ID,
			Name:        "command-test",
			Status:      domain.TestStatusFailed,
			Output:      firstNonEmpty(output, message),
			EvidenceRef: response.ToolCall.EvidenceRef,
		})
		if createErr != nil {
			return Result{}, createErr
		}
		failedTask, transitionErr := t.store.TransitionTask(ctx, testingTask.ID, domain.TaskStatusFailed, message)
		if transitionErr != nil {
			return Result{}, transitionErr
		}
		return Result{Task: failedTask, TestResult: result}, errors.New(message)
	}
	result, err := t.store.CreateTestResult(ctx, domain.TestResult{
		AttemptID:   attempt.ID,
		Name:        "command-test",
		Status:      domain.TestStatusPassed,
		Output:      output,
		EvidenceRef: response.ToolCall.EvidenceRef,
	})
	if err != nil {
		return Result{}, err
	}
	reviewingTask, err := t.store.TransitionTask(ctx, testingTask.ID, domain.TaskStatusReviewing, "command tester passed")
	if err != nil {
		return Result{}, err
	}
	return Result{Task: reviewingTask, TestResult: result}, nil
}

func commandOutput(response tools.CallResponse) string {
	if summary, ok := response.Result.Output["summary"].(string); ok {
		return summary
	}
	if len(response.Result.Output) == 0 {
		return ""
	}
	return fmt.Sprint(response.Result.Output)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
