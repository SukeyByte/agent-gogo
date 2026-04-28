package planner

import (
	"context"
	"strings"

	"github.com/sukeke/agent-gogo/internal/chain"
	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/intent"
)

type PlanRequest struct {
	Project       domain.Project
	UserInput     string
	ChainDecision chain.Decision
	IntentProfile intent.Profile
	ContextText   string
}

type Planner interface {
	PlanProject(ctx context.Context, req PlanRequest) ([]domain.Task, error)
}

type FixedPlanner struct{}

func NewFixedPlanner() *FixedPlanner {
	return &FixedPlanner{}
}

func (p *FixedPlanner) PlanProject(ctx context.Context, req PlanRequest) ([]domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	goal := strings.TrimSpace(req.Project.Goal)
	if goal == "" {
		goal = "Run the minimal runtime task"
	}

	return []domain.Task{
		{
			ProjectID:   req.Project.ID,
			Title:       "Run minimal runtime task",
			Description: "Fixed M3 planner task for validating project planning, scheduling, execution, testing, and review.",
			Status:      domain.TaskStatusDraft,
			AcceptanceCriteria: []string{
				"task attempt is created",
				"task reaches DONE",
				"task events record the lifecycle",
				goal,
			},
		},
	}, nil
}
