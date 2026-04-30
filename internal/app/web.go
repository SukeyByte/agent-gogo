package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/communication"
	appconfig "github.com/SukeyByte/agent-gogo/internal/config"
	"github.com/SukeyByte/agent-gogo/internal/contextbuilder"
	"github.com/SukeyByte/agent-gogo/internal/discovery"
	"github.com/SukeyByte/agent-gogo/internal/executor"
	"github.com/SukeyByte/agent-gogo/internal/function"
	"github.com/SukeyByte/agent-gogo/internal/memory"
	"github.com/SukeyByte/agent-gogo/internal/observability"
	"github.com/SukeyByte/agent-gogo/internal/persona"
	"github.com/SukeyByte/agent-gogo/internal/planner"
	"github.com/SukeyByte/agent-gogo/internal/provider"
	"github.com/SukeyByte/agent-gogo/internal/reviewer"
	appruntime "github.com/SukeyByte/agent-gogo/internal/runtime"
	"github.com/SukeyByte/agent-gogo/internal/scheduler"
	"github.com/SukeyByte/agent-gogo/internal/session"
	"github.com/SukeyByte/agent-gogo/internal/skill"
	"github.com/SukeyByte/agent-gogo/internal/store"
	"github.com/SukeyByte/agent-gogo/internal/tester"
	"github.com/SukeyByte/agent-gogo/internal/tools"
	"github.com/SukeyByte/agent-gogo/internal/validator"
	webhandlers "github.com/SukeyByte/agent-gogo/web/handlers"
)

func RunWebConsole(ctx context.Context, opts Options, addr string, writer io.Writer) error {
	cfg, err := appconfig.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	sqlite, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer sqlite.Close()

	if strings.TrimSpace(addr) == "" {
		addr = firstNonEmpty(os.Getenv("AGENT_GOGO_WEB_ADDR"), "127.0.0.1:8080")
	}

	channelID := cfg.Communication.ChannelID
	if channelID == "" {
		channelID = "web"
	}
	sessionID := cfg.Communication.SessionID
	if sessionID == "" {
		sessionID = "local"
	}

	hub := webhandlers.NewSSEHub(50)

	assets, assetErr := loadWebAssets(ctx, cfg)
	if assetErr != nil {
		_, _ = fmt.Fprintf(writer, "Warning: asset discovery failed (%v), management APIs may be partial\n", assetErr)
	}

	var sender webhandlers.ChannelEventSender
	llm, llmErr := newLLMProvider(cfg)
	if llmErr == nil {
		sender, err = initWebRuntime(ctx, cfg, sqlite, llm, hub, channelID, sessionID, assets)
		if err != nil {
			_, _ = fmt.Fprintf(writer, "Warning: runtime init failed (%v), running in read-only mode\n", err)
			sender = nil
		}
	} else {
		_, _ = fmt.Fprintf(writer, "Warning: LLM provider not configured (%v), running in read-only mode\n", llmErr)
	}

	distDir := findDistDir()
	apiServer := webhandlers.NewAPIServer(sqlite, sender, hub, webhandlers.ConfigView{
		WorkspacePath:          cfg.Storage.WorkspacePath,
		SQLitePath:             cfg.Storage.SQLitePath,
		ArtifactPath:           cfg.Storage.ArtifactPath,
		LogPath:                cfg.Storage.LogPath,
		SkillRoots:             append([]string(nil), cfg.Storage.SkillRoots...),
		PersonaPath:            cfg.Storage.PersonaPath,
		ChannelID:              channelID,
		SessionID:              sessionID,
		ContextMaxChars:        cfg.Runtime.ContextMaxChars,
		MaxTasksPerProject:     cfg.Runtime.MaxTasksPerProject,
		RequireConfirmHighRisk: cfg.Security.RequireConfirmHighRisk,
		AllowShell:             cfg.Security.AllowShell,
		ShellAllowlist:         append([]string(nil), cfg.Security.ShellAllowlist...),
		LLMTimeoutSeconds:      int(cfg.LLM.Timeout.Seconds()),
		BrowserTimeoutSeconds:  int(cfg.Browser.Timeout.Seconds()),
	}, channelID, sessionID, distDir)
	apiServer.UseSessionStore(sqlite)
	apiServer.UseAssets(assets.skills, assets.personas, assets.memories)

	mode := "read-only"
	if sender != nil {
		mode = "full runtime"
	}
	_, _ = fmt.Fprintf(writer, "Web Console (%s) listening on http://%s\n", mode, addr)
	server := &http.Server{Addr: addr, Handler: apiServer}
	return server.ListenAndServe()
}

type webAssets struct {
	skills   *skill.Registry
	personas *persona.Registry
	memories *memory.Index
	path     string
}

func loadWebAssets(ctx context.Context, cfg appconfig.Config) (webAssets, error) {
	var out webAssets
	var errs []error
	skillRegistry, err := skill.Discover(ctx, cfg.Storage.SkillRoots...)
	if err != nil {
		errs = append(errs, fmt.Errorf("skill discover: %w", err))
	} else {
		out.skills = skillRegistry
	}
	personaRegistry, err := persona.Discover(ctx, cfg.Storage.PersonaPath)
	if err != nil {
		errs = append(errs, fmt.Errorf("persona discover: %w", err))
	} else {
		out.personas = personaRegistry
	}
	out.path = filepath.Join(cfg.Storage.ArtifactPath, "memories.jsonl")
	memoryIndex, err := memory.NewPersistentIndex(ctx, out.path)
	if err != nil {
		errs = append(errs, fmt.Errorf("memory index: %w", err))
	} else {
		out.memories = memoryIndex
	}
	if len(errs) > 0 {
		return out, errors.Join(errs...)
	}
	return out, nil
}

func initWebRuntime(ctx context.Context, cfg appconfig.Config, sqlite *store.SQLiteStore, llm provider.LLMProvider, hub *webhandlers.SSEHub, channelID, sessionID string, assets webAssets) (*runtimeServiceBridge, error) {
	logger := observability.NoopLogger{}
	loggedLLM := observability.NewLoggingLLMProvider(llm, logger)

	commRuntime := communication.NewRuntime(communication.NewMemoryOutbox(), communication.NewRenderer())
	commRuntime.RegisterChannel(channelID, webhandlers.NewWebConsoleAdapter(channelID, hub))

	toolRuntime := tools.NewBuiltinRuntime(sqlite, cfg.Storage.WorkspacePath)
	toolRuntime.UseLogger(logger)
	policy := tools.SecurityPolicy{
		AllowShell:                cfg.Security.AllowShell,
		ShellAllowlist:            cfg.Security.ShellAllowlist,
		RequireConfirmationAtRisk: confirmationRisk(cfg),
	}
	toolRuntime.UseSecurityPolicy(policy, tools.AutoConfirmationGate{})

	if assets.skills == nil || assets.personas == nil || assets.memories == nil {
		loaded, err := loadWebAssets(ctx, cfg)
		if err != nil {
			return nil, err
		}
		assets = loaded
	}

	sessionSvc := session.NewService(sqlite, session.Config{MaxIdle: cfg.Session.MaxIdle})

	service := appruntime.NewServiceWithComponents(
		sqlite,
		planner.NewLLMPlanner(loggedLLM, cfg.LLM.Model),
		validator.NewCapabilityTaskValidator(validator.NewMinimalTaskValidator(), toolRuntime.CapabilityRegistry(), toolRuntime.CapabilityPolicy()),
		scheduler.NewReadyScheduler(sqlite),
		executor.NewGenericExecutor(executor.GenericExecutorOptions{
			Store:    sqlite,
			Tools:    toolRuntime,
			LLM:      loggedLLM,
			Model:    cfg.LLM.Model,
			MaxSteps: 12,
		}),
		tester.NewGenericEvidenceTester(sqlite),
		reviewer.NewLLMReviewer(sqlite, loggedLLM, cfg.LLM.Model),
	)
	service.UseLLM(loggedLLM, cfg.LLM.Model)
	service.UseCommunication(channelID, sessionID, commRuntime)
	service.UseContextAssets(function.NewCatalogRegistry(), assets.skills, assets.personas, assets.memories, contextbuilder.NewSerializer(contextbuilder.SerializerOptions{}), logger)
	service.UseContextBudget(cfg.Runtime.ContextMaxChars)
	service.UseMemoryPersistence(assets.path)
	service.UseDiscoveryLoop(discovery.NewToolLoop(tools.NewBuiltinRuntime(nil, cfg.Storage.WorkspacePath)).UseMemory(assets.memories))

	return &runtimeServiceBridge{
		service:     service,
		sessions:    sessionSvc,
		toolRuntime: toolRuntime,
		policy:      policy,
	}, nil
}

type runtimeServiceBridge struct {
	service     *appruntime.Service
	sessions    *session.Service
	toolRuntime *tools.Runtime
	policy      tools.SecurityPolicy
}

func (b *runtimeServiceBridge) HandleChannelEvent(ctx context.Context, event webhandlers.InboundEvent) error {
	switch strings.TrimSpace(event.Type) {
	case "config.update":
		return b.updateRuntimeConfig(ctx, event.Payload)
	case "session.resume":
		return b.resumeSession(ctx, event.SessionID, event.ProjectID)
	default:
		return b.service.HandleChannelEvent(ctx, appruntime.ChannelEvent{
			Type:      event.Type,
			ChannelID: event.ChannelID,
			SessionID: event.SessionID,
			ProjectID: event.ProjectID,
			TaskID:    event.TaskID,
			Text:      event.Text,
			Payload:   event.Payload,
		})
	}
}

func (b *runtimeServiceBridge) HandleUserConfirmation(ctx context.Context, confirmation webhandlers.InboundConfirmation) error {
	return b.service.HandleUserConfirmation(ctx, appruntime.UserConfirmation{
		ConfirmationID: confirmation.ConfirmationID,
		ProjectID:      confirmation.ProjectID,
		TaskID:         confirmation.TaskID,
		AttemptID:      confirmation.AttemptID,
		ActionID:       confirmation.ActionID,
		Approved:       confirmation.Approved,
		Message:        confirmation.Message,
	})
}

func (b *runtimeServiceBridge) updateRuntimeConfig(ctx context.Context, payload map[string]string) error {
	if b.service == nil {
		return errors.New("runtime service is unavailable")
	}
	if raw := strings.TrimSpace(payload["context_max_chars"]); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			return fmt.Errorf("invalid context_max_chars: %w", err)
		}
		b.service.UseContextBudget(value)
	}
	if raw := strings.TrimSpace(payload["allow_shell"]); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("invalid allow_shell: %w", err)
		}
		b.policy.AllowShell = value
	}
	if raw := strings.TrimSpace(payload["require_confirm_high_risk"]); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("invalid require_confirm_high_risk: %w", err)
		}
		if value {
			b.policy.RequireConfirmationAtRisk = "high"
		} else {
			b.policy.RequireConfirmationAtRisk = ""
		}
	}
	if raw := strings.TrimSpace(payload["shell_allowlist"]); raw != "" {
		b.policy.ShellAllowlist = splitConfigList(raw)
	}
	if b.toolRuntime != nil {
		b.toolRuntime.UseSecurityPolicy(b.policy, tools.AutoConfirmationGate{})
	}
	return b.service.HandleChannelEvent(ctx, appruntime.ChannelEvent{
		Type:    "config.update",
		Payload: payload,
	})
}

func (b *runtimeServiceBridge) resumeSession(ctx context.Context, sessionID string, projectID string) error {
	if b.sessions == nil {
		return errors.New("session service is unavailable")
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return errors.New("session_id is required")
	}
	sess, err := b.sessions.Resume(ctx, sessionID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(projectID) == "" {
		projectID = sess.ProjectID
	}
	if strings.TrimSpace(projectID) == "" {
		if sctx, err := b.sessions.GetRuntimeContext(ctx, sessionID); err == nil {
			projectID = sctx.ProjectID
		}
	}
	if strings.TrimSpace(projectID) == "" {
		return nil
	}
	go b.runReadyTasks(context.Background(), projectID)
	return nil
}

func (b *runtimeServiceBridge) runReadyTasks(ctx context.Context, projectID string) {
	for i := 0; i < 50; i++ {
		_, err := b.service.RunNextTask(ctx, projectID)
		if errors.Is(err, sql.ErrNoRows) {
			return
		}
		if err != nil {
			return
		}
	}
}

func findDistDir() string {
	candidates := []string{
		"web/frontend/dist",
		"../web/frontend/dist",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	return ""
}

func splitConfigList(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(raw), &values); err == nil {
		return values
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}
