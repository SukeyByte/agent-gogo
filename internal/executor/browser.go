package executor

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/SukeyByte/agent-gogo/internal/browser"
	"github.com/SukeyByte/agent-gogo/internal/domain"
)

type BrowserActionStore interface {
	CreateToolCall(ctx context.Context, call domain.ToolCall) (domain.ToolCall, error)
	CreateObservation(ctx context.Context, observation domain.Observation) (domain.Observation, error)
	AddTaskEvent(ctx context.Context, event domain.TaskEvent) (domain.TaskEvent, error)
}

type BrowserExecutor struct {
	store   BrowserActionStore
	browser *browser.Runtime
}

func NewBrowserExecutor(store BrowserActionStore, browserRuntime *browser.Runtime) *BrowserExecutor {
	return &BrowserExecutor{store: store, browser: browserRuntime}
}

func (e *BrowserExecutor) Open(ctx context.Context, taskID string, attemptID string, url string) (browser.Snapshot, domain.ToolCall, error) {
	input := jsonString(map[string]any{"url": url})
	snapshot, err := e.browser.Open(ctx, url)
	if err != nil {
		call, createErr := e.store.CreateToolCall(ctx, domain.ToolCall{
			AttemptID: attemptID,
			Name:      "browser.open",
			InputJSON: input,
			Status:    domain.ToolCallStatusFailed,
			Error:     err.Error(),
		})
		if createErr != nil {
			return browser.Snapshot{}, domain.ToolCall{}, createErr
		}
		return browser.Snapshot{}, call, err
	}
	call, err := e.store.CreateToolCall(ctx, domain.ToolCall{
		AttemptID:   attemptID,
		Name:        "browser.open",
		InputJSON:   input,
		OutputJSON:  jsonString(snapshot),
		Status:      domain.ToolCallStatusSucceeded,
		EvidenceRef: snapshot.URL,
	})
	if err != nil {
		return browser.Snapshot{}, domain.ToolCall{}, err
	}
	if err := e.observe(ctx, taskID, attemptID, call.ID, "browser.open", snapshot); err != nil {
		return browser.Snapshot{}, domain.ToolCall{}, err
	}
	return snapshot, call, nil
}

func (e *BrowserExecutor) Click(ctx context.Context, taskID string, attemptID string, text string) (browser.Snapshot, domain.ToolCall, error) {
	input := jsonString(map[string]any{"text": text})
	snapshot, err := e.browser.Click(ctx, text)
	if err != nil {
		call, createErr := e.store.CreateToolCall(ctx, domain.ToolCall{
			AttemptID: attemptID,
			Name:      "browser.click",
			InputJSON: input,
			Status:    domain.ToolCallStatusFailed,
			Error:     err.Error(),
		})
		if createErr != nil {
			return browser.Snapshot{}, domain.ToolCall{}, createErr
		}
		return browser.Snapshot{}, call, err
	}
	call, err := e.store.CreateToolCall(ctx, domain.ToolCall{
		AttemptID:   attemptID,
		Name:        "browser.click",
		InputJSON:   input,
		OutputJSON:  jsonString(snapshot),
		Status:      domain.ToolCallStatusSucceeded,
		EvidenceRef: snapshot.URL,
	})
	if err != nil {
		return browser.Snapshot{}, domain.ToolCall{}, err
	}
	if err := e.observe(ctx, taskID, attemptID, call.ID, "browser.click", snapshot); err != nil {
		return browser.Snapshot{}, domain.ToolCall{}, err
	}
	return snapshot, call, nil
}

func (e *BrowserExecutor) AddEvent(ctx context.Context, taskID string, attemptID string, eventType string, message string) error {
	_, err := e.store.AddTaskEvent(ctx, domain.TaskEvent{
		TaskID:    taskID,
		AttemptID: attemptID,
		Type:      eventType,
		Message:   message,
	})
	return err
}

func (e *BrowserExecutor) observe(ctx context.Context, taskID string, attemptID string, toolCallID string, typ string, snapshot browser.Snapshot) error {
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   attemptID,
		ToolCallID:  toolCallID,
		Type:        typ,
		Summary:     snapshot.DOMSummary,
		EvidenceRef: snapshot.URL,
		Payload:     jsonString(snapshot),
	}); err != nil {
		return err
	}
	return e.AddEvent(ctx, taskID, attemptID, "executor."+typ, fmt.Sprintf("%s indexed %d chars from %s", typ, len([]rune(snapshot.DOMSummary)), snapshot.URL))
}

func jsonString(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
