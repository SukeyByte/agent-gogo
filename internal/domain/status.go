package domain

import "fmt"

type TaskStatus string

const (
	TaskStatusDraft         TaskStatus = "DRAFT"
	TaskStatusReady         TaskStatus = "READY"
	TaskStatusInProgress    TaskStatus = "IN_PROGRESS"
	TaskStatusImplemented   TaskStatus = "IMPLEMENTED"
	TaskStatusTesting       TaskStatus = "TESTING"
	TaskStatusReviewing     TaskStatus = "REVIEWING"
	TaskStatusDone          TaskStatus = "DONE"
	TaskStatusBlocked       TaskStatus = "BLOCKED"
	TaskStatusNeedUserInput TaskStatus = "NEED_USER_INPUT"
	TaskStatusReviewFailed  TaskStatus = "REVIEW_FAILED"
	TaskStatusFailed        TaskStatus = "FAILED"
	TaskStatusCancelled     TaskStatus = "CANCELLED"
)

var taskTransitions = map[TaskStatus]map[TaskStatus]struct{}{
	TaskStatusDraft: {
		TaskStatusReady:     {},
		TaskStatusBlocked:   {},
		TaskStatusCancelled: {},
	},
	TaskStatusReady: {
		TaskStatusInProgress: {},
		TaskStatusBlocked:    {},
		TaskStatusCancelled:  {},
	},
	TaskStatusInProgress: {
		TaskStatusImplemented:   {},
		TaskStatusBlocked:       {},
		TaskStatusNeedUserInput: {},
		TaskStatusFailed:        {},
		TaskStatusCancelled:     {},
	},
	TaskStatusImplemented: {
		TaskStatusTesting:   {},
		TaskStatusFailed:    {},
		TaskStatusCancelled: {},
	},
	TaskStatusTesting: {
		TaskStatusReviewing:    {},
		TaskStatusReviewFailed: {},
		TaskStatusFailed:       {},
		TaskStatusCancelled:    {},
	},
	TaskStatusReviewing: {
		TaskStatusDone:         {},
		TaskStatusReviewFailed: {},
		TaskStatusFailed:       {},
		TaskStatusCancelled:    {},
	},
	TaskStatusBlocked: {
		TaskStatusReady:     {},
		TaskStatusCancelled: {},
	},
	TaskStatusNeedUserInput: {
		TaskStatusReady:     {},
		TaskStatusBlocked:   {},
		TaskStatusCancelled: {},
	},
	TaskStatusReviewFailed: {
		TaskStatusReady:     {},
		TaskStatusCancelled: {},
	},
	TaskStatusFailed: {
		TaskStatusReady:     {},
		TaskStatusCancelled: {},
	},
}

func CanTransitionTask(from, to TaskStatus) bool {
	if from == to {
		return true
	}
	allowed, ok := taskTransitions[from]
	if !ok {
		return false
	}
	_, ok = allowed[to]
	return ok
}

func ValidateTaskTransition(from, to TaskStatus) error {
	if CanTransitionTask(from, to) {
		return nil
	}
	return fmt.Errorf("invalid task status transition: %s -> %s", from, to)
}
