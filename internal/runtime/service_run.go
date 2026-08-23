package runtime

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"

	comm "github.com/SukeyByte/agent-gogo/internal/communication"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/executor"
)

// claimingScheduler is implemented by schedulers that atomically claim
// tasks, which is required for concurrent execution.
type claimingScheduler interface {
	ClaimNextReadyTasks(ctx context.Context, projectID string, limit int) ([]domain.Task, error)
}

// claimNextTask returns one atomically claimed task, or sql.ErrNoRows.
func (s *Service) claimNextTask(ctx context.Context, projectID string) (domain.Task, error) {
	if claimer, ok := s.scheduler.(claimingScheduler); ok {
		claimed, err := claimer.ClaimNextReadyTasks(ctx, projectID, 1)
		if err != nil {
			return domain.Task{}, err
		}
		if len(claimed) == 0 {
			return domain.Task{}, sql.ErrNoRows
		}
		return claimed[0], nil
	}
	return s.scheduler.NextReadyTask(ctx, projectID)
}

func (s *Service) RunNextTask(ctx context.Context, projectID string) (TaskRunResult, error) {
	if err := ctx.Err(); err != nil {
		return TaskRunResult{}, err
	}
	task, err := s.claimNextTask(ctx, projectID)
	if err != nil {
		return TaskRunResult{}, err
	}
	if err := s.log(ctx, "scheduler.ready", task); err != nil {
		return TaskRunResult{}, err
	}
	return s.runClaimedTask(ctx, projectID, task)
}

// runClaimedTask drives one already-claimed task through the full
// execute -> test -> review pipeline. Safe to call from multiple goroutines.
func (s *Service) runClaimedTask(ctx context.Context, projectID string, task domain.Task) (result TaskRunResult, err error) {
	defer func() {
		if s.taskSink == nil {
			return
		}
		final := result
		if err != nil {
			final.Task = task
			if fresh, fetchErr := s.store.GetTask(ctx, task.ID); fetchErr == nil {
				final.Task = fresh
			}
		}
		s.taskSink(final, err)
	}()
	return s.runClaimedTaskPipeline(ctx, projectID, task)
}

func (s *Service) runClaimedTaskPipeline(ctx context.Context, projectID string, task domain.Task) (TaskRunResult, error) {
	if err := s.emitTaskProgress(ctx, projectID, task, domain.TaskStatusInProgress, "Task started: "+task.Title); err != nil {
		return TaskRunResult{}, err
	}
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return TaskRunResult{}, err
	}
	_, decision, profile := s.getState(projectID)
	taskContext, err := s.buildRuntimeContext(ctx, project, task.ID, decision, profile)
	if err != nil {
		return TaskRunResult{}, err
	}
	s.setState(projectID, taskContext, decision, profile)
	if consumer, ok := s.executor.(runtimeContextConsumer); ok {
		consumer.UseRuntimeContext(projectID, taskContext)
	}
	executed, err := s.executor.Execute(ctx, task)
	if err != nil {
		var executionErr *executor.ExecutionError
		if errors.As(err, &executionErr) && executionErr.Attempt.ID != "" {
			if _, repairErr := s.createRepairTask(ctx, projectID, executionErr.Task, executionErr.Attempt, "executor.failed", err); repairErr != nil {
				err = errors.Join(err, repairErr)
			}
		}
		s.emitTaskBlocked(ctx, projectID, task, "Task failed during execution: "+err.Error())
		return TaskRunResult{}, &TaskFailedError{Stage: "executor", TaskTitle: task.Title, Err: err}
	}
	if err := s.log(ctx, "executor.result", executed); err != nil {
		return TaskRunResult{}, err
	}
	if err := s.emitTaskProgress(ctx, projectID, executed.Task, domain.TaskStatusImplemented, "Task implemented: "+executed.Task.Title); err != nil {
		return TaskRunResult{}, err
	}
	if err := s.emitTaskProgress(ctx, projectID, executed.Task, domain.TaskStatusTesting, "Task testing: "+executed.Task.Title); err != nil {
		return TaskRunResult{}, err
	}
	tested, err := s.tester.Test(ctx, executed.Task, executed.Attempt)
	if err != nil {
		if _, repairErr := s.createRepairTask(ctx, projectID, executed.Task, executed.Attempt, "tester.failed", err); repairErr != nil {
			err = errors.Join(err, repairErr)
		}
		s.emitTaskBlocked(ctx, projectID, executed.Task, "Task failed during testing: "+err.Error())
		return TaskRunResult{}, &TaskFailedError{Stage: "tester", TaskTitle: executed.Task.Title, Err: err}
	}
	if err := s.log(ctx, "tester.result", tested); err != nil {
		return TaskRunResult{}, err
	}
	if err := s.emitTaskProgress(ctx, projectID, tested.Task, domain.TaskStatusReviewing, "Task reviewing: "+tested.Task.Title); err != nil {
		return TaskRunResult{}, err
	}
	reviewed, err := s.reviewer.Review(ctx, tested.Task, executed.Attempt)
	if err != nil {
		if _, repairErr := s.createRepairTask(ctx, projectID, tested.Task, executed.Attempt, "reviewer.rejected", err); repairErr != nil {
			err = errors.Join(err, repairErr)
		}
		s.emitTaskBlocked(ctx, projectID, tested.Task, "Task failed during review: "+err.Error())
		return TaskRunResult{}, &TaskFailedError{Stage: "reviewer", TaskTitle: tested.Task.Title, Err: err}
	}
	if err := s.log(ctx, "reviewer.result", reviewed); err != nil {
		return TaskRunResult{}, err
	}
	if err := s.extractTaskMemories(ctx, projectID, reviewed.Task, reviewed.Attempt, reviewed.ReviewResult); err != nil {
		return TaskRunResult{}, err
	}
	if err := s.completeRepairedTask(ctx, projectID, reviewed.Task); err != nil {
		return TaskRunResult{}, err
	}
	events, err := s.store.ListTaskEvents(ctx, reviewed.Task.ID)
	if err != nil {
		return TaskRunResult{}, err
	}
	result := TaskRunResult{
		ProjectID:    projectID,
		Task:         reviewed.Task,
		Attempt:      reviewed.Attempt,
		TestResult:   tested.TestResult,
		ReviewResult: reviewed.ReviewResult,
		Events:       events,
	}
	if err := s.emit(ctx, comm.NewDoneIntent(s.communicationChannel, fmt.Sprintf("Task done: %s", reviewed.Task.Title)), projectID); err != nil {
		return TaskRunResult{}, err
	}
	s.saveSessionContext(ctx, projectID)
	return result, nil
}

func (s *Service) RunProjectTasks(ctx context.Context, projectID string, maxTasks int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if maxTasks <= 0 {
		maxTasks = 50
	}
	if s.parallelism > 1 {
		return s.runProjectTasksParallel(ctx, projectID, maxTasks)
	}
	if err := s.emit(ctx, comm.NewProgressIntent(s.communicationChannel, "Project run started", map[string]any{
		"project_id": projectID,
		"status":     "RUNNING",
	}), projectID); err != nil {
		return 0, err
	}
	ranTasks := 0
	iterations := 0
	for iterations < maxTasks {
		iterations++
		result, err := s.RunNextTask(ctx, projectID)
		if errors.Is(err, sql.ErrNoRows) {
			finished, resumed, finErr := s.finalizeProjectRun(ctx, projectID, ranTasks)
			ranTasks = finished
			if finErr != nil {
				return ranTasks, finErr
			}
			if resumed {
				// Final review rejected and queued delta tasks; keep running.
				continue
			}
			if err := s.emit(ctx, comm.NewDoneIntent(s.communicationChannel, fmt.Sprintf("Project run finished: %d task(s) completed", ranTasks)), projectID); err != nil {
				return ranTasks, err
			}
			return ranTasks, nil
		}
		if err != nil {
			if hasReady, readyErr := s.hasReadyTask(ctx, projectID); readyErr != nil {
				return ranTasks, readyErr
			} else if hasReady {
				_ = s.emit(ctx, comm.NewProgressIntent(s.communicationChannel, "Project run continuing with recovery task after failure", map[string]any{
					"project_id": projectID,
					"status":     "RECOVERING",
					"error":      err.Error(),
				}), projectID)
				continue
			}
			s.emitProjectBlocked(ctx, projectID, fmt.Sprintf("Project run stopped after %d task(s): %s", ranTasks, err.Error()))
			return ranTasks, err
		}
		ranTasks++
		if result.Task.Status == domain.TaskStatusDone {
			continue
		}
	}
	err := fmt.Errorf("max task limit reached: %d", maxTasks)
	s.emitProjectBlocked(ctx, projectID, err.Error())
	return ranTasks, err
}

func (s *Service) hasReadyTask(ctx context.Context, projectID string) (bool, error) {
	_, err := s.scheduler.NextReadyTask(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) blockTasksWaitingOnBlockedDependencies(ctx context.Context, projectID string) (int, error) {
	tasks, err := s.store.ListTasksByProject(ctx, projectID)
	if err != nil {
		return 0, err
	}
	dependencies, err := s.store.ListTaskDependenciesByProject(ctx, projectID)
	if err != nil {
		return 0, err
	}
	statusByID := make(map[string]domain.TaskStatus, len(tasks))
	taskByID := make(map[string]domain.Task, len(tasks))
	for _, task := range tasks {
		statusByID[task.ID] = task.Status
		taskByID[task.ID] = task
	}
	dependencyTitles := map[string][]string{}
	for _, dependency := range dependencies {
		if !dependencyBlocksDependents(statusByID[dependency.DependsOnTaskID]) {
			continue
		}
		if blockedDependency, ok := taskByID[dependency.DependsOnTaskID]; ok {
			dependencyTitles[dependency.TaskID] = append(dependencyTitles[dependency.TaskID], blockedDependency.Title)
		} else {
			dependencyTitles[dependency.TaskID] = append(dependencyTitles[dependency.TaskID], dependency.DependsOnTaskID)
		}
	}
	blocked := 0
	for _, task := range tasks {
		if task.Status != domain.TaskStatusReady && task.Status != domain.TaskStatusDraft {
			continue
		}
		titles := dependencyTitles[task.ID]
		if len(titles) == 0 {
			continue
		}
		reason := "blocked dependency cannot complete: " + strings.Join(titles, ", ")
		updated, err := s.store.TransitionTask(ctx, task.ID, domain.TaskStatusBlocked, reason)
		if err != nil {
			return blocked, err
		}
		s.emitTaskBlocked(ctx, projectID, updated, "Task blocked by dependency: "+task.Title+" - "+reason)
		blocked++
	}
	return blocked, nil
}

func dependencyBlocksDependents(status domain.TaskStatus) bool {
	switch status {
	case domain.TaskStatusBlocked, domain.TaskStatusNeedUserInput, domain.TaskStatusFailed, domain.TaskStatusReviewFailed, domain.TaskStatusCancelled:
		return true
	default:
		return false
	}
}

func (s *Service) projectIncompleteSummary(ctx context.Context, projectID string) (string, bool, error) {
	tasks, err := s.store.ListTasksByProject(ctx, projectID)
	if err != nil {
		return "", false, err
	}
	counts := map[domain.TaskStatus]int{}
	incomplete := 0
	for _, task := range tasks {
		if task.Status == domain.TaskStatusDone || task.Status == domain.TaskStatusCancelled {
			continue
		}
		incomplete++
		counts[task.Status]++
	}
	if incomplete == 0 {
		return "", false, nil
	}
	order := []domain.TaskStatus{
		domain.TaskStatusDraft,
		domain.TaskStatusReady,
		domain.TaskStatusInProgress,
		domain.TaskStatusImplemented,
		domain.TaskStatusTesting,
		domain.TaskStatusReviewing,
		domain.TaskStatusBlocked,
		domain.TaskStatusNeedUserInput,
		domain.TaskStatusReviewFailed,
		domain.TaskStatusFailed,
	}
	parts := make([]string, 0, len(counts))
	for _, status := range order {
		if count := counts[status]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", status, count))
		}
	}
	return fmt.Sprintf("%d incomplete task(s) remain (%s)", incomplete, strings.Join(parts, ", ")), true, nil
}

func (s *Service) RetryTask(ctx context.Context, taskID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	task, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	switch task.Status {
	case domain.TaskStatusReady:
		_, err = s.store.AddTaskEvent(ctx, domain.TaskEvent{
			TaskID:  task.ID,
			Type:    "runtime.retry_requested",
			Message: "task is already ready",
		})
		return err
	case domain.TaskStatusDraft:
		_, err = s.store.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "retry requested from draft")
		return err
	case domain.TaskStatusBlocked, domain.TaskStatusNeedUserInput, domain.TaskStatusReviewFailed, domain.TaskStatusFailed:
		_, err = s.store.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "retry requested")
		return err
	default:
		return fmt.Errorf("task %s cannot be retried from %s", task.ID, task.Status)
	}
}

func (s *Service) ReplanProject(ctx context.Context, projectID string, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "replan requested"
	}
	if err := s.log(ctx, "runtime.replan", map[string]string{"project_id": project.ID, "reason": reason}); err != nil {
		return err
	}
	if err := s.emit(ctx, comm.NewMessageIntent(s.communicationChannel, "Replanning project: "+reason), project.ID); err != nil {
		return err
	}
	_, err = s.PlanProject(ctx, project.ID)
	return err
}

func (s *Service) HandleChannelEvent(ctx context.Context, event ChannelEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch strings.TrimSpace(event.Type) {
	case "goal.submitted":
		name := strings.TrimSpace(event.Payload["name"])
		if name == "" {
			name = "Channel project"
		}
		project, err := s.CreateProject(ctx, CreateProjectRequest{Name: name, Goal: event.Text})
		if err != nil {
			return err
		}
		if _, err = s.PlanProject(ctx, project.ID); err != nil {
			s.emitProjectBlocked(ctx, project.ID, "Project blocked during planning: "+err.Error())
			return nil
		}
		s.dispatchProjectRun(project.ID)
		return nil
	case "task.retry":
		return s.RetryTask(ctx, event.TaskID)
	case "project.replan":
		return s.ReplanProject(ctx, event.ProjectID, event.Text)
	default:
		return s.log(ctx, "runtime.channel_event", event)
	}
}

func (s *Service) dispatchProjectRun(projectID string) {
	if s.runDispatcher != nil {
		s.runDispatcher(projectID)
		return
	}
	go s.runProjectTasksInBackground(projectID)
}

func (s *Service) runProjectTasksInBackground(projectID string) {
	if _, err := s.RunProjectTasks(context.Background(), projectID, 0); err != nil {
		_ = s.log(context.Background(), "runtime.project_run_failed", map[string]string{
			"project_id": projectID,
			"error":      err.Error(),
		})
	}
}

func (s *Service) HandleUserConfirmation(ctx context.Context, confirmation UserConfirmation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	task, err := s.store.GetTask(ctx, confirmation.TaskID)
	if err != nil {
		return err
	}
	message := strings.TrimSpace(confirmation.Message)
	if message == "" {
		message = "user confirmation received"
	}
	decision := "rejected"
	if confirmation.Approved {
		decision = "approved"
	}
	if _, err := s.store.AddTaskEvent(ctx, domain.TaskEvent{
		TaskID:    task.ID,
		AttemptID: confirmation.AttemptID,
		Type:      "user.confirmation." + decision,
		Message:   message,
		Payload:   fmt.Sprintf(`{"confirmation_id":%q,"action_id":%q}`, confirmation.ConfirmationID, confirmation.ActionID),
	}); err != nil {
		return err
	}
	if confirmation.Approved && task.Status == domain.TaskStatusNeedUserInput {
		_, err = s.store.TransitionTask(ctx, task.ID, domain.TaskStatusReady, message)
		return err
	}
	if !confirmation.Approved && task.Status == domain.TaskStatusNeedUserInput {
		_, err = s.store.TransitionTask(ctx, task.ID, domain.TaskStatusBlocked, message)
		return err
	}
	return nil
}

type runtimeContextConsumer interface {
	UseRuntimeContext(projectID string, contextText string)
}

// runProjectTasksParallel executes up to s.parallelism tasks of a project
// concurrently. Tasks are claimed atomically, so the same task can never run
// twice; single-task failures queue repair work and the run continues.
func (s *Service) runProjectTasksParallel(ctx context.Context, projectID string, maxTasks int) (int, error) {
	if err := s.emit(ctx, comm.NewProgressIntent(s.communicationChannel, "Project run started", map[string]any{
		"project_id": projectID,
		"status":     "RUNNING",
		"parallel":   s.parallelism,
	}), projectID); err != nil {
		return 0, err
	}
	claimer, ok := s.scheduler.(claimingScheduler)
	if !ok {
		// No atomic claiming available; fall back to sequential execution.
		s.parallelism = 1
		return s.RunProjectTasks(ctx, projectID, maxTasks)
	}
	workers := s.parallelism
	if workers > 8 {
		workers = 8
	}
	var (
		wg         sync.WaitGroup
		ranMu      sync.Mutex
		ran        int
		sem        = make(chan struct{}, workers)
		iterations = 0
	)
claimLoop:
	for iterations < maxTasks*2 {
		if ranMu.Lock(); ran >= maxTasks {
			ranMu.Unlock()
			break
		} else {
			ranMu.Unlock()
		}
		claimed, err := claimer.ClaimNextReadyTasks(ctx, projectID, workers)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			wg.Wait()
			return ran, err
		}
		if len(claimed) == 0 {
			// Nothing claimable: wait for in-flight work; finishing tasks may
			// unblock dependents, otherwise finalize the project run.
			wg.Wait()
			claimed, err = claimer.ClaimNextReadyTasks(ctx, projectID, workers)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return ran, err
			}
			if len(claimed) == 0 {
				finished, resumed, finErr := s.finalizeProjectRun(ctx, projectID, ran)
				if finErr != nil {
					return finished, finErr
				}
				if resumed {
					// Delta tasks queued by the final review: loop claims them.
					continue
				}
				return finished, nil
			}
		}
		for _, task := range claimed {
			iterations++
			wg.Add(1)
			sem <- struct{}{}
			go func(task domain.Task) {
				defer wg.Done()
				defer func() { <-sem }()
				_, err := s.runClaimedTask(ctx, projectID, task)
				ranMu.Lock()
				ran++
				ranMu.Unlock()
				if err != nil {
					s.log(context.Background(), "parallel.task.failed", map[string]any{
						"task_id": task.ID, "title": task.Title, "error": err.Error(),
					})
				}
			}(task)
		}
	}
	wg.Wait()
	if ran >= maxTasks {
		return ran, fmt.Errorf("max task limit reached: %d", maxTasks)
	}
	finished, resumed, finErr := s.finalizeProjectRun(ctx, projectID, ran)
	if finErr != nil {
		return finished, finErr
	}
	if resumed {
		// Delta tasks queued by the final review: reset counters, claim them.
		iterations = 0
		goto claimLoop
	}
	return finished, nil
}
