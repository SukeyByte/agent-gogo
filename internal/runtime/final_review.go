package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	comm "github.com/SukeyByte/agent-gogo/internal/communication"
	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/planner"
	"github.com/SukeyByte/agent-gogo/internal/reviewer"
)

// maxProjectReplans bounds feedback-driven re-plans per project so a
// persistent disagreement between reviewer and planner cannot loop forever.
const maxProjectReplans = 2

// finalReviewOutcome tells the run loops what happened at finalization.
type finalReviewOutcome int

const (
	finalReviewPassed finalReviewOutcome = iota
	finalReviewResumed
	finalReviewExhausted
)

// finalizeProjectRun applies the shared end-of-run logic: block dependents
// of blocked tasks, pause on incomplete work, run the project-level final
// review (when configured), and only then mark the project completed.
func (s *Service) finalizeProjectRun(ctx context.Context, projectID string, ranTasks int) (int, bool, error) {
	if blocked, blockErr := s.blockTasksWaitingOnBlockedDependencies(ctx, projectID); blockErr != nil {
		return ranTasks, false, blockErr
	} else if blocked > 0 {
		return ranTasks, false, nil
	}
	summary, incomplete, summaryErr := s.projectIncompleteSummary(ctx, projectID)
	if summaryErr != nil {
		return ranTasks, false, summaryErr
	}
	if incomplete {
		s.emitProjectBlocked(ctx, projectID, fmt.Sprintf("Project run paused after %d task(s): %s", ranTasks, summary))
		return ranTasks, false, nil
	}

	if s.projectReviewer != nil {
		outcome, reviewErr := s.runProjectFinalReview(ctx, projectID, ranTasks)
		if reviewErr != nil {
			return ranTasks, false, reviewErr
		}
		switch outcome {
		case finalReviewResumed:
			return ranTasks, true, nil
		case finalReviewExhausted:
			// Re-plan budget exhausted: the project stays unfinished and the
			// final review verdict is visible instead of a silent pass.
			return ranTasks, false, nil
		}
	}

	if _, err := s.store.UpdateProjectStatus(ctx, projectID, domain.ProjectStatusCompleted); err != nil {
		return ranTasks, false, err
	}
	if err := s.emit(ctx, comm.NewDoneIntent(s.communicationChannel, fmt.Sprintf("Project run finished: %d task(s) completed", ranTasks)), projectID); err != nil {
		return ranTasks, false, err
	}
	return ranTasks, false, nil
}

// runProjectFinalReview judges the assembled delivery against the goal and,
// on rejection, queues delta tasks for the identified gaps.
func (s *Service) runProjectFinalReview(ctx context.Context, projectID string, ranTasks int) (finalReviewOutcome, error) {
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return finalReviewPassed, err
	}
	tasks, err := s.store.ListTasksByProject(ctx, projectID)
	if err != nil {
		return finalReviewPassed, err
	}
	reviews, err := s.collectProjectReviews(ctx, tasks)
	if err != nil {
		return finalReviewPassed, err
	}
	artifacts, err := s.store.ListArtifactsByProject(ctx, projectID)
	if err != nil {
		return finalReviewPassed, err
	}
	round := s.projectReplanCount(ctx, projectID)
	review, reviewErr := s.projectReviewer.ReviewProject(ctx, reviewer.ProjectReviewInput{
		Goal:        project.Goal,
		Tasks:       tasks,
		Reviews:     reviews,
		Artifacts:   artifacts,
		ReplanRound: round,
	})
	if reviewErr != nil {
		// A broken reviewer must not wedge the project: log and pass through.
		_ = s.log(ctx, "project.final_review.error", map[string]any{
			"project_id": projectID, "error": reviewErr.Error(),
		})
		return finalReviewPassed, nil
	}
	_ = s.log(ctx, "project.final_review", map[string]any{
		"project_id": projectID,
		"approved":   review.Approved,
		"summary":    review.Summary,
		"gaps":       review.Gaps,
		"round":      round,
	})
	if review.Approved {
		_ = s.emit(ctx, comm.NewProgressIntent(s.communicationChannel, "Project final review approved: "+firstLine(review.Summary), nil), projectID)
		return finalReviewPassed, nil
	}

	if round >= maxProjectReplans {
		_ = s.log(ctx, "project.final_review.exhausted", map[string]any{
			"project_id": projectID,
			"round":      round,
			"max":        maxProjectReplans,
			"gaps":       review.Gaps,
		})
		s.emitProjectBlocked(ctx, projectID, fmt.Sprintf(
			"Project final review rejected after %d re-plan round(s): %s; unresolved gaps: %s",
			round, firstLine(review.Summary), strings.Join(review.Gaps, "; ")))
		return finalReviewExhausted, nil
	}

	created, planErr := s.deltaReplan(ctx, projectID, review.Gaps, "project_final_review", completedTaskSummary(tasks, reviews))
	if planErr != nil || len(created) == 0 {
		_ = s.log(ctx, "project.replan.error", map[string]any{
			"project_id": projectID, "error": fmt.Sprint(planErr), "created": len(created),
		})
		s.emitProjectBlocked(ctx, projectID, "Project final review rejected and re-planning produced no tasks: "+firstLine(review.Summary))
		return finalReviewExhausted, nil
	}
	return finalReviewResumed, nil
}

// deltaReplan plans only the work needed to close the given gaps and queues
// it as READY "Delta:" tasks.
func (s *Service) deltaReplan(ctx context.Context, projectID string, gaps []string, reason string, completed []string) ([]domain.Task, error) {
	plannerInstance := s.deltaPlanner
	if plannerInstance == nil {
		if dp, ok := s.planner.(planner.DeltaPlanner); ok {
			plannerInstance = dp
		} else {
			plannerInstance = planner.FixedDeltaPlanner{}
		}
	}
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	contextText, decision, profile := s.getState(projectID)
	req := planner.PlanRequest{
		Project:       project,
		UserInput:     project.Goal,
		ChainDecision: decision,
		IntentProfile: profile,
		ContextText:   contextText,
		Feedback: &planner.PlanFeedback{
			CompletedTasks: completed,
			Gaps:           gaps,
			Reason:         reason,
		},
	}
	planned, err := plannerInstance.PlanDelta(ctx, req)
	if err != nil {
		return nil, err
	}
	created := make([]domain.Task, 0, len(planned))
	for _, task := range planned {
		task.ProjectID = projectID
		if task.Status == "" {
			task.Status = domain.TaskStatusDraft
		}
		stored, err := s.store.CreateTask(ctx, task)
		if err != nil {
			return created, err
		}
		ready, err := s.store.TransitionTask(ctx, stored.ID, domain.TaskStatusReady, "delta re-plan task queued ("+reason+")")
		if err != nil {
			return created, err
		}
		created = append(created, ready)
	}
	s.incrementProjectReplan(ctx, projectID)
	_ = s.log(ctx, "project.replanned", map[string]any{
		"project_id": projectID,
		"reason":     reason,
		"gaps":       gaps,
		"new_tasks":  len(created),
	})
	_ = s.emit(ctx, comm.NewProgressIntent(s.communicationChannel, fmt.Sprintf("Project re-planned (%s): %d delta task(s) queued", reason, len(created)),
		map[string]any{"gaps": gaps}), projectID)
	return created, nil
}

// collectProjectReviews gathers the latest review result per task.
func (s *Service) collectProjectReviews(ctx context.Context, tasks []domain.Task) ([]domain.ReviewResult, error) {
	var reviews []domain.ReviewResult
	for _, task := range tasks {
		attempts, err := s.store.ListTaskAttemptsByTask(ctx, task.ID)
		if err != nil {
			return nil, err
		}
		for _, attempt := range attempts {
			results, err := s.store.ListReviewResultsByAttempt(ctx, attempt.ID)
			if err != nil {
				return nil, err
			}
			reviews = append(reviews, results...)
		}
	}
	return reviews, nil
}

func completedTaskSummary(tasks []domain.Task, reviews []domain.ReviewResult) []string {
	if len(reviews) == 0 {
		completed := make([]string, 0, len(tasks))
		for _, task := range tasks {
			if task.Status == domain.TaskStatusDone {
				completed = append(completed, task.Title)
			}
		}
		return completed
	}
	// Reviews are already scoped to this project's tasks; join them into
	// one line per review so the planner sees what each approval covered.
	completed := make([]string, 0, len(reviews))
	for _, review := range reviews {
		line := firstLine(review.Summary)
		if line == "" {
			line = "review approved"
		}
		completed = append(completed, line)
	}
	return completed
}

func (s *Service) projectReplanCount(ctx context.Context, projectID string) int {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.replansByProject[projectID]
}

func (s *Service) incrementProjectReplan(ctx context.Context, projectID string) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.replansByProject == nil {
		s.replansByProject = map[string]int{}
	}
	s.replansByProject[projectID]++
}

func firstLine(text string) string {
	if idx := strings.IndexByte(text, '\n'); idx >= 0 {
		return strings.TrimSpace(text[:idx])
	}
	return strings.TrimSpace(text)
}

var _ = errors.New
