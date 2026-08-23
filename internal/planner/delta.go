package planner

import (
	"context"
	"errors"
	"fmt"

	"github.com/SukeyByte/agent-gogo/internal/domain"
)

// PlanFeedback drives delta re-planning: the planner sees what is already
// done and the gaps to close, and proposes ONLY new work for those gaps.
type PlanFeedback struct {
	// CompletedTasks summarizes finished work as "title — summary" lines.
	CompletedTasks []string `json:"completed_tasks,omitempty"`
	// Gaps describe what is missing or broken, in priority order.
	Gaps []string `json:"gaps"`
	// Reason names the trigger ("project_final_review" or
	// "repair_limit_reached") so the planner can weigh the feedback.
	Reason string `json:"reason"`
}

// PlanDelta produces the additional tasks for a feedback-driven re-plan.
// Implementations must not re-propose completed work.
type DeltaPlanner interface {
	PlanDelta(ctx context.Context, req PlanRequest) ([]domain.Task, error)
}

// FixedDeltaPlanner turns each feedback gap into one concrete task. It is
// the deterministic fallback used by tests and when no LLM is configured.
type FixedDeltaPlanner struct{}

func (FixedDeltaPlanner) PlanDelta(ctx context.Context, req PlanRequest) ([]domain.Task, error) {
	if req.Feedback == nil || len(req.Feedback.Gaps) == 0 {
		return nil, errors.New("delta plan requires feedback gaps")
	}
	tasks := make([]domain.Task, 0, len(req.Feedback.Gaps))
	for i, gap := range req.Feedback.Gaps {
		gap = trimSpace(gap)
		if gap == "" {
			continue
		}
		tasks = append(tasks, domain.Task{
			Title:       fmt.Sprintf("Delta %d: %s", i+1, truncate(gap, 80)),
			Description: "Close the gap identified by " + req.Feedback.Reason + ": " + gap,
			AcceptanceCriteria: []string{
				"The gap is demonstrably closed",
			},
		})
	}
	if len(tasks) == 0 {
		return nil, errors.New("feedback contained no usable gaps")
	}
	return tasks, nil
}

func trimSpace(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t' || s[0] == '\n') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t' || s[len(s)-1] == '\n') {
		s = s[:len(s)-1]
	}
	return s
}

func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max-1]) + "…"
}
