package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/store"
)

func setupProject(t *testing.T, titles ...string) (*store.SQLiteStore, string) {
	t.Helper()
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlite.Close() })
	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "Claim", Goal: "claim tests"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	for _, title := range titles {
		created, err := sqlite.CreateTask(ctx, domain.Task{ProjectID: project.ID, Title: title})
		if err != nil {
			t.Fatalf("create task %s: %v", title, err)
		}
		if _, err := sqlite.TransitionTask(ctx, created.ID, domain.TaskStatusReady, "ready for claim test"); err != nil {
			t.Fatalf("ready task %s: %v", title, err)
		}
	}
	return sqlite, project.ID
}

func TestClaimNextReadyTasksRespectsDependencies(t *testing.T) {
	sqlite, projectID := setupProject(t, "setup", "build")
	ctx := context.Background()
	tasks, _ := sqlite.ListTasksByProject(ctx, projectID)
	byTitle := map[string]domain.Task{}
	for _, task := range tasks {
		byTitle[task.Title] = task
	}
	if _, err := sqlite.CreateTaskDependency(ctx, domain.TaskDependency{
		TaskID:          byTitle["build"].ID,
		DependsOnTaskID: byTitle["setup"].ID,
	}); err != nil {
		t.Fatalf("create dependency: %v", err)
	}

	s := NewClaimingScheduler(sqlite, sqlite)
	claimed, err := s.ClaimNextReadyTasks(ctx, projectID, 5)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 1 || claimed[0].Title != "setup" {
		t.Fatalf("expected only 'setup' claimable, got %#v", claimed)
	}
	again, err := s.ClaimNextReadyTasks(ctx, projectID, 5)
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected ErrNoRows while dependency in progress, got %#v err=%v", again, err)
	}
}

func TestClaimTasksConcurrentWorkersGetDisjointSets(t *testing.T) {
	sqlite, projectID := setupProject(t, "t1", "t2", "t3", "t4", "t5", "t6")
	ctx := context.Background()
	tasks, _ := sqlite.ListTasksByProject(ctx, projectID)
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}

	s := NewClaimingScheduler(sqlite, sqlite)
	var (
		mu      sync.Mutex
		claimed = map[string]bool{}
		wg      sync.WaitGroup
	)
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				batch, err := s.ClaimNextReadyTasks(ctx, projectID, 2)
				if errors.Is(err, sql.ErrNoRows) {
					return
				}
				if err != nil {
					t.Errorf("claim: %v", err)
					return
				}
				mu.Lock()
				for _, task := range batch {
					if claimed[task.ID] {
						t.Errorf("task %s claimed twice", task.ID)
					}
					claimed[task.ID] = true
				}
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if len(claimed) != len(ids) {
		t.Fatalf("expected all %d tasks claimed exactly once, got %d", len(ids), len(claimed))
	}
}
