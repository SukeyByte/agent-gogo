package tools

import (
	"context"
	"strings"
	"testing"
)

func TestCLIConfirmationGateRequiresExplicitYes(t *testing.T) {
	gate := NewCLIConfirmationGate(strings.NewReader("no\n"), nil)
	approved, err := gate.Confirm(context.Background(), ConfirmationRequest{ToolName: "git.commit", RiskLevel: "high"})
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if approved {
		t.Fatal("expected no to reject")
	}
	gate = NewCLIConfirmationGate(strings.NewReader("yes\n"), nil)
	approved, err = gate.Confirm(context.Background(), ConfirmationRequest{ToolName: "git.commit", RiskLevel: "high"})
	if err != nil {
		t.Fatalf("confirm yes: %v", err)
	}
	if !approved {
		t.Fatal("expected yes to approve")
	}
}
