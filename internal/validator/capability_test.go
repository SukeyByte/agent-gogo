package validator

import (
	"context"
	"strings"
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/capability"
	"github.com/SukeyByte/agent-gogo/internal/domain"
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

func TestCapabilityTaskValidatorUsesStructuredRequiredCapabilities(t *testing.T) {
	registry := capability.NewRegistry(capability.ToolSpec{Name: "file.read", RiskLevel: "low"})
	validator := NewCapabilityTaskValidator(NewMinimalTaskValidator(), registry, capability.Policy{})
	err := validator.ValidateTask(context.Background(), domain.Task{
		ProjectID:            "project",
		Title:                "Ambiguous task",
		Description:          "No browser wording here",
		AcceptanceCriteria:   []string{"accepted"},
		RequiredCapabilities: []string{"browser"},
	})
	if err == nil || !strings.Contains(err.Error(), "browser") {
		t.Fatalf("expected structured browser capability blocker, got %v", err)
	}
}

func TestCapabilityTaskValidatorAcceptsDocumentSummaryAliases(t *testing.T) {
	registry := capability.NewRegistry(capability.ToolSpec{Name: "file.read", RiskLevel: "low"})
	validator := NewCapabilityTaskValidator(NewMinimalTaskValidator(), registry, capability.Policy{})
	err := validator.ValidateTask(context.Background(), domain.Task{
		ProjectID:            "project",
		Title:                "Summarize README",
		Description:          "Read README.md and summarize it",
		AcceptanceCriteria:   []string{"summary produced"},
		RequiredCapabilities: []string{"document-understanding", "summarization", "read"},
	})
	if err != nil {
		t.Fatalf("expected document summary aliases to resolve to read capability, got %v", err)
	}
}

func TestCapabilityTaskValidatorPrunesReadOnlyOverDeclaredCapabilities(t *testing.T) {
	registry := capability.NewRegistry(
		capability.ToolSpec{Name: "file.read", RiskLevel: "low"},
		capability.ToolSpec{Name: "test.run", RiskLevel: "medium", RequiresShell: true},
	)
	validator := NewCapabilityTaskValidator(NewMinimalTaskValidator(), registry, capability.Policy{AllowShell: false})
	err := validator.ValidateTask(context.Background(), domain.Task{
		ProjectID:            "project",
		Title:                "Read go.mod",
		Description:          "读取当前仓库 go.mod，说明模块名和 Go 版本，不修改文件",
		AcceptanceCriteria:   []string{"模块名和 Go 版本已说明"},
		RequiredCapabilities: []string{"read", "execute", "verify"},
	})
	if err != nil {
		t.Fatalf("expected read-only task to ignore over-declared execute/verify, got %v", err)
	}
}
