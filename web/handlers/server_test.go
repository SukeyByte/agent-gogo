package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/store"
)

func TestDashboardRendersProjects(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()
	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "Console project", Goal: "render dashboard"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	if _, err := sqlite.CreateTask(ctx, domain.Task{ProjectID: project.ID, Title: "Task", AcceptanceCriteria: []string{"visible"}}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	server := NewServer(sqlite, ConfigView{WorkspacePath: ".", ChannelID: "web"})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, expected := range []string{"Dashboard", "Console project", "Projects", "Tasks"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected body to contain %q:\n%s", expected, body)
		}
	}
}

func TestTaskDetailRendersEventsAndObservations(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()
	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "Console project", Goal: "render task"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{ProjectID: project.ID, Title: "Observed task", AcceptanceCriteria: []string{"visible"}})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ready, err := sqlite.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "ready")
	if err != nil {
		t.Fatalf("ready task: %v", err)
	}
	attempt, err := sqlite.CreateTaskAttempt(ctx, ready.ID)
	if err != nil {
		t.Fatalf("create attempt: %v", err)
	}
	if _, err := sqlite.CreateObservation(ctx, domain.Observation{AttemptID: attempt.ID, Type: "state.file_changed", Summary: "wrote file"}); err != nil {
		t.Fatalf("create observation: %v", err)
	}

	server := NewServer(sqlite, ConfigView{WorkspacePath: ".", ChannelID: "web"})
	req := httptest.NewRequest(http.MethodGet, "/tasks/"+task.ID, nil)
	rec := httptest.NewRecorder()
	server.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, expected := range []string{"Task Detail", "Observed task", "state.file_changed", "task_attempt.created"} {
		if !strings.Contains(body, expected) {
			t.Fatalf("expected body to contain %q:\n%s", expected, body)
		}
	}
}
