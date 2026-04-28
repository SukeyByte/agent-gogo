package domain

import "testing"

func TestValidateTaskTransitionHappyPath(t *testing.T) {
	path := []TaskStatus{
		TaskStatusDraft,
		TaskStatusReady,
		TaskStatusInProgress,
		TaskStatusImplemented,
		TaskStatusTesting,
		TaskStatusReviewing,
		TaskStatusDone,
	}

	for i := 0; i < len(path)-1; i++ {
		if err := ValidateTaskTransition(path[i], path[i+1]); err != nil {
			t.Fatalf("expected %s -> %s to be valid: %v", path[i], path[i+1], err)
		}
	}
}

func TestValidateTaskTransitionRejectsInvalidJump(t *testing.T) {
	if err := ValidateTaskTransition(TaskStatusDraft, TaskStatusDone); err == nil {
		t.Fatal("expected DRAFT -> DONE to be invalid")
	}
}

func TestValidateTaskTransitionAllowsRepairPath(t *testing.T) {
	if err := ValidateTaskTransition(TaskStatusReviewFailed, TaskStatusReady); err != nil {
		t.Fatalf("expected REVIEW_FAILED -> READY to be valid: %v", err)
	}
}
