package observer

import (
	"context"
	"testing"

	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/tools"
)

func TestInterpretToolCallClassifiesFileWrite(t *testing.T) {
	state := InterpretToolCall(domain.ToolCall{Name: "file.write", Status: domain.ToolCallStatusSucceeded}, tools.Result{
		Success: true,
		Output:  map[string]any{"path": "web/index.html"},
	})
	if state.Status != StateChanged {
		t.Fatalf("expected changed state, got %s", state.Status)
	}
	if state.Type != "state.file_changed" {
		t.Fatalf("expected file changed observation, got %s", state.Type)
	}
	if state.Signals["path"] != "web/index.html" {
		t.Fatalf("expected path signal, got %#v", state.Signals)
	}
}

func TestInterpreterPersistsObservationAndEvent(t *testing.T) {
	store := &recordingStore{}
	interpreter := NewInterpreter(store)
	state, err := interpreter.InterpretToolResult(context.Background(), ToolResultRequest{
		Task:    domain.Task{ID: "task-1"},
		Attempt: domain.TaskAttempt{ID: "attempt-1"},
		Response: tools.CallResponse{
			ToolCall: domain.ToolCall{ID: "tool-1", Name: "test.run", Status: domain.ToolCallStatusSucceeded},
			Result: tools.Result{
				Success:     true,
				Output:      map[string]any{"command": "go test ./...", "passed": true},
				EvidenceRef: "exec://test.run",
			},
		},
	})
	if err != nil {
		t.Fatalf("interpret: %v", err)
	}
	if state.Type != "state.tests_passed" {
		t.Fatalf("expected tests passed state, got %s", state.Type)
	}
	if len(store.observations) != 1 {
		t.Fatalf("expected observation, got %d", len(store.observations))
	}
	if len(store.events) != 1 {
		t.Fatalf("expected event, got %d", len(store.events))
	}
}

type recordingStore struct {
	observations []domain.Observation
	events       []domain.TaskEvent
}

func (s *recordingStore) CreateObservation(ctx context.Context, observation domain.Observation) (domain.Observation, error) {
	observation.ID = "obs-1"
	s.observations = append(s.observations, observation)
	return observation, nil
}

func (s *recordingStore) AddTaskEvent(ctx context.Context, event domain.TaskEvent) (domain.TaskEvent, error) {
	s.events = append(s.events, event)
	return event, nil
}
