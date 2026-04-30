package validator

import (
	"context"
	"strings"
	"testing"

	"github.com/sukeke/agent-gogo/internal/capability"
	"github.com/sukeke/agent-gogo/internal/domain"
)

func TestCapabilityTaskValidatorBlocksUnavailableCapability(t *testing.T) {
	registry := capability.NewRegistry(capability.ToolSpec{Name: "file.read", RiskLevel: "low"})
	validator := NewCapabilityTaskValidator(NewMinimalTaskValidator(), registry, capability.Policy{})
	err := validator.ValidateTask(context.Background(), domain.Task{
		ProjectID:          "project",
		Title:              "打开网页并总结",
		Description:        "打开 https://example.com 并总结页面内容",
		AcceptanceCriteria: []string{"网页内容已读取"},
	})
	if err == nil || !strings.Contains(err.Error(), "browser") {
		t.Fatalf("expected missing browser capability, got %v", err)
	}
}

func TestCapabilityTaskValidatorHonorsShellPolicy(t *testing.T) {
	registry := capability.NewRegistry(capability.ToolSpec{Name: "test.run", RiskLevel: "medium", RequiresShell: true})
	validator := NewCapabilityTaskValidator(NewMinimalTaskValidator(), registry, capability.Policy{AllowShell: false})
	err := validator.ValidateTask(context.Background(), domain.Task{
		ProjectID:          "project",
		Title:              "Run tests",
		Description:        "Run go test ./... and verify tests pass",
		AcceptanceCriteria: []string{"go test ./... passes"},
	})
	if err == nil || !strings.Contains(err.Error(), "shell is disabled") {
		t.Fatalf("expected shell policy blocker, got %v", err)
	}
}
