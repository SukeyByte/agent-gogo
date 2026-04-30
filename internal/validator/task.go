package validator

import (
	"context"
	"errors"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/domain"
)

type TaskValidator interface {
	ValidateTask(ctx context.Context, task domain.Task) error
}

type MinimalTaskValidator struct{}

func NewMinimalTaskValidator() *MinimalTaskValidator {
	return &MinimalTaskValidator{}
}

func (v *MinimalTaskValidator) ValidateTask(ctx context.Context, task domain.Task) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(task.ProjectID) == "" {
		return errors.New("task project id is required")
	}
	if strings.TrimSpace(task.Title) == "" {
		return errors.New("task title is required")
	}
	if len(task.AcceptanceCriteria) == 0 {
		return errors.New("task acceptance criteria are required")
	}
	return nil
}
