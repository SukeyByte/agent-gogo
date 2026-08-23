package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/taskaware"
)

func (s *Service) createRepairTask(ctx context.Context, projectID string, failedTask domain.Task, attempt domain.TaskAttempt, eventType string, cause error) (domain.Task, error) {
	message := strings.TrimSpace(cause.Error())
	if message == "" {
		message = "runtime verification failed"
	}
	if latest, err := s.store.GetTask(ctx, failedTask.ID); err == nil {
		failedTask = latest
	}
	repairTargetID := failedTask.ID
	if rootID, err := s.rootRepairTargetID(ctx, failedTask.ID); err != nil {
		return domain.Task{}, err
	} else if rootID != "" {
		repairTargetID = rootID
	}
	if failedTask.Status != domain.TaskStatusFailed && failedTask.Status != domain.TaskStatusDone && failedTask.Status != domain.TaskStatusCancelled {
		if domain.CanTransitionTask(failedTask.Status, domain.TaskStatusFailed) {
			transitioned, err := s.store.TransitionTask(ctx, failedTask.ID, domain.TaskStatusFailed, message)
			if err != nil {
				return domain.Task{}, err
			}
			failedTask = transitioned
		}
	}
	if _, err := s.store.AddTaskEvent(ctx, domain.TaskEvent{
		TaskID:    failedTask.ID,
		AttemptID: attempt.ID,
		Type:      eventType,
		Message:   message,
		Payload:   fmt.Sprintf(`{"failed_task_id":%q}`, repairTargetID),
	}); err != nil {
		return domain.Task{}, err
	}
	if s.memories != nil {
		item := taskaware.FailureMemory(projectID, failedTask, attempt, eventType, message)
		s.memories.Add(item)
		if err := s.persistMemories(ctx); err != nil {
			return domain.Task{}, err
		}
		_ = s.log(ctx, "memory.extract", map[string]any{
			"project_id": projectID,
			"task_id":    failedTask.ID,
			"memory_id":  item.ID,
			"type":       item.Type,
		})
	}
	if depth := repairDepth(failedTask.Title); depth >= maxRepairDepth {
		limitErr := fmt.Errorf("repair limit reached for task %q after %d nested repair attempt(s)", failedTask.Title, depth)
		if _, err := s.store.AddTaskEvent(ctx, domain.TaskEvent{
			TaskID:    failedTask.ID,
			AttemptID: attempt.ID,
			Type:      "repair.limit_reached",
			Message:   limitErr.Error(),
			Payload:   fmt.Sprintf(`{"failed_task_id":%q,"max_depth":%d}`, repairTargetID, maxRepairDepth),
		}); err != nil {
			return domain.Task{}, err
		}
		// Repeated rejection means the plan is likely wrong, not the
		// execution: hand the failure history to the planner as a delta
		// re-plan instead of abandoning the project.
		gaps := []string{fmt.Sprintf("Task %q failed after %d repair attempts: %s", failedTask.Title, depth, message)}
		if len(failedTask.AcceptanceCriteria) > 0 {
			gaps[0] += " (acceptance: " + strings.Join(failedTask.AcceptanceCriteria, "; ") + ")"
		}
		if created, planErr := s.deltaReplan(ctx, projectID, gaps, "repair_limit_reached", nil); planErr == nil && len(created) > 0 {
			return domain.Task{}, limitErr
		} else if planErr != nil {
			_ = s.log(ctx, "project.replan.error", map[string]any{
				"project_id": projectID, "reason": "repair_limit_reached", "error": planErr.Error(),
			})
		}
		return domain.Task{}, limitErr
	}
	repair, err := s.store.CreateTask(ctx, domain.Task{
		ProjectID:   projectID,
		Title:       "Fix: " + failedTask.Title,
		Description: "Repair failed task after " + eventType + ": " + message + "\nOriginal task acceptance criteria: " + strings.Join(failedTask.AcceptanceCriteria, "; "),
		Status:      domain.TaskStatusDraft,
		AcceptanceCriteria: []string{
			"Failure evidence is understood or determined obsolete",
			"Original task acceptance criteria are now satisfied or a targeted fix is applied",
			"Original task can continue after repair",
		},
	})
	if err != nil {
		return domain.Task{}, err
	}
	if _, err := s.store.AddTaskEvent(ctx, domain.TaskEvent{
		TaskID:  repair.ID,
		Type:    "repair.linked",
		Message: "repair task linked to failed task",
		Payload: fmt.Sprintf(`{"failed_task_id":%q}`, repairTargetID),
	}); err != nil {
		return domain.Task{}, err
	}
	repair, err = s.store.TransitionTask(ctx, repair.ID, domain.TaskStatusReady, "repair task generated after failure")
	return repair, err
}

func repairDepth(title string) int {
	depth := 0
	title = strings.TrimSpace(title)
	for strings.HasPrefix(title, "Fix: ") {
		depth++
		title = strings.TrimSpace(strings.TrimPrefix(title, "Fix: "))
	}
	return depth
}

func (s *Service) completeRepairedTask(ctx context.Context, projectID string, repairTask domain.Task) error {
	current := repairTask
	seen := map[string]bool{}
	for i := 0; i < 16; i++ {
		if current.ID == "" || seen[current.ID] {
			return nil
		}
		seen[current.ID] = true
		failedTaskID, err := s.repairTargetID(ctx, current.ID)
		if err != nil {
			return err
		}
		if failedTaskID == "" {
			return nil
		}
		failedTask, err := s.store.GetTask(ctx, failedTaskID)
		if err != nil {
			return err
		}
		switch failedTask.Status {
		case domain.TaskStatusFailed, domain.TaskStatusReviewFailed:
			updated, err := s.store.TransitionTask(ctx, failedTask.ID, domain.TaskStatusDone, "completed by repair task: "+repairTask.Title)
			if err != nil {
				return err
			}
			if err := s.emitTaskProgress(ctx, projectID, updated, domain.TaskStatusDone, "Task repaired and marked done: "+updated.Title); err != nil {
				return err
			}
			if err := s.completeSupersededRepairTasks(ctx, projectID, updated.ID, repairTask.ID); err != nil {
				return err
			}
			current = updated
		default:
			current = failedTask
		}
	}
	return nil
}

func (s *Service) completeSupersededRepairTasks(ctx context.Context, projectID string, repairedTaskID string, successfulRepairTaskID string) error {
	tasks, err := s.store.ListTasksByProject(ctx, projectID)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.ID == successfulRepairTaskID {
			continue
		}
		switch task.Status {
		case domain.TaskStatusFailed, domain.TaskStatusReviewFailed:
		default:
			continue
		}
		targetID, err := s.repairTargetID(ctx, task.ID)
		if err != nil {
			return err
		}
		if targetID != repairedTaskID {
			continue
		}
		updated, err := s.store.TransitionTask(ctx, task.ID, domain.TaskStatusDone, "superseded by successful repair task")
		if err != nil {
			return err
		}
		if err := s.emitTaskProgress(ctx, projectID, updated, domain.TaskStatusDone, "Superseded repair task marked done: "+updated.Title); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) repairTargetID(ctx context.Context, repairTaskID string) (string, error) {
	events, err := s.store.ListTaskEvents(ctx, repairTaskID)
	if err != nil {
		return "", err
	}
	for _, event := range events {
		if event.Type != "repair.linked" {
			continue
		}
		var payload struct {
			FailedTaskID string `json:"failed_task_id"`
		}
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			return "", err
		}
		return strings.TrimSpace(payload.FailedTaskID), nil
	}
	return "", nil
}

func (s *Service) rootRepairTargetID(ctx context.Context, taskID string) (string, error) {
	currentID := strings.TrimSpace(taskID)
	seen := map[string]bool{}
	for i := 0; i < 16; i++ {
		if currentID == "" || seen[currentID] {
			return currentID, nil
		}
		seen[currentID] = true
		nextID, err := s.repairTargetID(ctx, currentID)
		if err != nil {
			return "", err
		}
		if nextID == "" {
			return currentID, nil
		}
		currentID = nextID
	}
	return currentID, nil
}
