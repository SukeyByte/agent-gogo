package capability

import (
	"context"
	"strings"
	"testing"
)

func TestRegistryResolvesToolPolicy(t *testing.T) {
	registry := NewRegistry(ToolSpec{Name: "test.run", RiskLevel: "medium", RequiresShell: true})
	resolution, err := registry.ResolveTool(context.Background(), ToolRequest{
		ToolName: "test.run",
		Args:     map[string]any{"command": "go test ./..."},
		Policy: Policy{
			AllowShell:                true,
			ShellAllowlist:            []string{"go test"},
			RequireConfirmationAtRisk: "medium",
		},
	})
	if err != nil {
		t.Fatalf("resolve tool: %v", err)
	}
	if !resolution.RequiresConfirmation {
		t.Fatal("expected medium risk tool to require confirmation")
	}
	if len(resolution.Capabilities) == 0 {
		t.Fatal("expected inferred capabilities")
	}
}

func TestRegistryBlocksUnavailableCapability(t *testing.T) {
	registry := NewRegistry(ToolSpec{Name: "shell.run", RiskLevel: "medium", RequiresShell: true})
	availability, err := registry.CheckAvailability(context.Background(), AvailabilityRequest{
		RequiredCapabilities: []string{"execute"},
		Policy:               Policy{AllowShell: false},
	})
	if err != nil {
		t.Fatalf("check availability: %v", err)
	}
	if availability.Available {
		t.Fatal("expected execute capability to be blocked")
	}
	if len(availability.BlockedCapabilities) == 0 || !strings.Contains(availability.BlockedCapabilities[0], "shell is disabled") {
		t.Fatalf("expected shell blocker, got %#v", availability.BlockedCapabilities)
	}
}

func TestRegistryMapsCapabilityToTools(t *testing.T) {
	registry := NewRegistry(ToolSpec{Name: "file.write", RiskLevel: "medium"})
	tools := registry.ToolsForCapability("write")
	if len(tools) != 1 {
		t.Fatalf("expected one write tool, got %d", len(tools))
	}
	if tools[0].Name != "file.write" {
		t.Fatalf("expected file.write, got %s", tools[0].Name)
	}
}

func TestRegistryNormalizesSemanticReadCapabilities(t *testing.T) {
	registry := NewRegistry(ToolSpec{Name: "file.read", RiskLevel: "low"})
	availability, err := registry.CheckAvailability(context.Background(), AvailabilityRequest{
		RequiredCapabilities: []string{"document-understanding", "summarization"},
		Policy:               Policy{},
	})
	if err != nil {
		t.Fatalf("check availability: %v", err)
	}
	if !availability.Available {
		t.Fatalf("expected semantic read aliases to be available, missing=%v blocked=%v", availability.MissingCapabilities, availability.BlockedCapabilities)
	}
}
