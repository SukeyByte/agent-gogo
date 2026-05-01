package app

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/capability"
	"github.com/SukeyByte/agent-gogo/internal/channels/webconsole"
	appconfig "github.com/SukeyByte/agent-gogo/internal/config"
	"github.com/SukeyByte/agent-gogo/internal/provider"
	"github.com/SukeyByte/agent-gogo/internal/store"
)

func TestParseAgentInputTreatsAllWordsAsGoal(t *testing.T) {
	goal, opts, err := parseAgentInput([]string{"--config", "configs/test.yaml", "为苏柯宇写一个个人网页并完成部署"})
	if err != nil {
		t.Fatalf("parse input: %v", err)
	}
	if goal != "为苏柯宇写一个个人网页并完成部署" {
		t.Fatalf("unexpected goal %q", goal)
	}
	if opts.ConfigPath != "configs/test.yaml" {
		t.Fatalf("unexpected config path %q", opts.ConfigPath)
	}
}

func TestParseWebInput(t *testing.T) {
	opts, addr, err := parseWebInput([]string{"--config", "configs/test.yaml", "--addr", "127.0.0.1:9090"})
	if err != nil {
		t.Fatalf("parse web input: %v", err)
	}
	if opts.ConfigPath != "configs/test.yaml" {
		t.Fatalf("unexpected config path %q", opts.ConfigPath)
	}
	if addr != "127.0.0.1:9090" {
		t.Fatalf("unexpected addr %q", addr)
	}
	if !isWebCommand("console") {
		t.Fatal("expected console to be a web command")
	}
}

func TestInitWebRuntimeRegistersBrowserCapability(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	root := t.TempDir()
	cfg := appconfig.Default()
	cfg.Storage.WorkspacePath = root
	cfg.Storage.ArtifactPath = filepath.Join(root, "artifacts")
	cfg.Storage.SkillRoots = []string{filepath.Join(root, "skills")}
	cfg.Storage.PersonaPath = filepath.Join(root, "personas")

	assets, err := loadWebAssets(ctx, cfg)
	if err != nil {
		t.Fatalf("load web assets: %v", err)
	}
	bridge, err := initWebRuntime(ctx, cfg, sqlite, provider.ChatFunc(func(context.Context, provider.ChatRequest) (provider.ChatResponse, error) {
		return provider.ChatResponse{Text: "{}"}, nil
	}), webconsole.NewSSEHub(1), "web", "session", assets)
	if err != nil {
		t.Fatalf("init web runtime: %v", err)
	}
	defer bridge.Close()

	availability, err := bridge.toolRuntime.CapabilityRegistry().CheckAvailability(ctx, capability.AvailabilityRequest{
		RequiredCapabilities: []string{"browser"},
		Policy:               bridge.toolRuntime.CapabilityPolicy(),
	})
	if err != nil {
		t.Fatalf("check browser availability: %v", err)
	}
	if !availability.Available {
		t.Fatalf("expected browser capability available, missing=%v blocked=%v", availability.MissingCapabilities, availability.BlockedCapabilities)
	}
}

func TestExtractPersonalSiteName(t *testing.T) {
	if got := extractPersonalSiteName("为张三写一个个人网页并完成部署"); got != "张三" {
		t.Fatalf("expected 张三, got %q", got)
	}
}
