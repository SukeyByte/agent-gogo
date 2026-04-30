package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	appconfig "github.com/sukeke/agent-gogo/internal/config"
	"github.com/sukeke/agent-gogo/internal/contextbuilder"
	"github.com/sukeke/agent-gogo/internal/discovery"
	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/executor"
	"github.com/sukeke/agent-gogo/internal/function"
	"github.com/sukeke/agent-gogo/internal/memory"
	"github.com/sukeke/agent-gogo/internal/observability"
	"github.com/sukeke/agent-gogo/internal/persona"
	"github.com/sukeke/agent-gogo/internal/planner"
	"github.com/sukeke/agent-gogo/internal/reviewer"
	appruntime "github.com/sukeke/agent-gogo/internal/runtime"
	"github.com/sukeke/agent-gogo/internal/scheduler"
	"github.com/sukeke/agent-gogo/internal/session"
	"github.com/sukeke/agent-gogo/internal/skill"
	"github.com/sukeke/agent-gogo/internal/tester"
	"github.com/sukeke/agent-gogo/internal/tools"
	"github.com/sukeke/agent-gogo/internal/validator"
)

func RunAgent(ctx context.Context, goal string, opts Options, writer io.Writer) error {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return errors.New("agent goal is required")
	}
	return RunGeneric(ctx, goal, opts, writer)
}

func RunGeneric(ctx context.Context, goal string, opts Options, writer io.Writer) error {
	cfg, err := appconfig.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	logger, err := observability.NewFileLogger(cfg.Storage.LogPath, "generic")
	if err != nil {
		return err
	}
	llm, err := newLLMProvider(cfg)
	if err != nil {
		return err
	}
	loggedLLM := observability.NewLoggingLLMProvider(llm, logger)
	sqlite, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer sqlite.Close()

	commRuntime := newCommunicationRuntime(cfg, writer)
	toolRuntime := tools.NewBuiltinRuntime(sqlite, cfg.Storage.WorkspacePath)
	toolRuntime.UseLogger(logger)
	toolRuntime.UseSecurityPolicy(tools.SecurityPolicy{
		AllowShell:                cfg.Security.AllowShell,
		ShellAllowlist:            cfg.Security.ShellAllowlist,
		RequireConfirmationAtRisk: confirmationRisk(cfg),
	}, newConfirmationGate(writer))
	browserEngine := newLazyBrowserEngine(cfg)
	defer browserEngine.Close()
	toolRuntime.RegisterBrowserTools(browserEngine)

	skillRegistry, err := skill.Discover(ctx, cfg.Storage.SkillRoots...)
	if err != nil {
		return err
	}
	personaRegistry, err := persona.Discover(ctx, cfg.Storage.PersonaPath)
	if err != nil {
		return err
	}
	memoryPath := filepath.Join(cfg.Storage.ArtifactPath, "memories.jsonl")
	memoryIndex, err := memory.NewPersistentIndex(ctx, memoryPath)
	if err != nil {
		return err
	}
	testRunner := tester.Tester(tester.NewGenericEvidenceTester(sqlite))

	sessionSvc := session.NewService(sqlite, session.Config{
		MaxIdle: cfg.Session.MaxIdle,
	})
	// CLI 每次运行创建新 session；Web/Telegram 等持久通道通过 FindOrCreate 复用
	sess, err := sessionSvc.FindOrCreate(ctx, session.FindOrCreateRequest{
		ChannelType: cfg.Communication.ChannelID,
		ChannelID:   cfg.Communication.SessionID,
		UserID:      cfg.Session.UserID,
		Goal:        goal,
	})
	if err != nil {
		return err
	}
	defer func() { _ = sessionSvc.Complete(context.Background(), sess.ID) }()

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
		testRunner,
		reviewer.NewLLMReviewer(sqlite, loggedLLM, cfg.LLM.Model),
	)
	service.UseLLM(loggedLLM, cfg.LLM.Model)
	service.UseCommunication(cfg.Communication.ChannelID, cfg.Communication.SessionID, commRuntime)
	service.UseContextAssets(function.NewCatalogRegistry(), skillRegistry, personaRegistry, memoryIndex, contextbuilder.NewSerializer(contextbuilder.SerializerOptions{}), logger)
	service.UseContextBudget(cfg.Runtime.ContextMaxChars)
	service.UseMemoryPersistence(memoryPath)
	service.UseSession(sessionSvc, sess.ID)
	service.UseDiscoveryLoop(discovery.NewToolLoop(tools.NewBuiltinRuntime(nil, cfg.Storage.WorkspacePath)).UseMemory(memoryIndex))

	if err := logger.Log(ctx, "input", map[string]any{
		"goal":          goal,
		"workspace":     cfg.Storage.WorkspacePath,
		"memory_path":   memoryPath,
		"skill_sources": cfg.Storage.SkillRoots,
	}); err != nil {
		return err
	}
	logChannel(ctx, commRuntime, cfg, "input", goal)
	project, err := service.CreateProject(ctx, appruntime.CreateProjectRequest{
		Name: "Agent goal",
		Goal: goal,
	})
	if err != nil {
		return err
	}
	if _, err := sessionSvc.BindProject(ctx, sess.ID, project.ID, goal); err != nil {
		return err
	}
	tasks, err := service.PlanProject(ctx, project.ID)
	if err != nil {
		return err
	}
	logChannel(ctx, commRuntime, cfg, "plan", renderTasks(tasks))
	limit := cfg.Runtime.MaxTasksPerProject
	if limit <= 0 {
		limit = 50
	}
	ranTasks := 0
	for ranTasks < limit {
		result, err := service.RunNextTask(ctx, project.ID)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		ranTasks++
		logChannel(ctx, commRuntime, cfg, "task", fmt.Sprintf("%s -> %s", result.Task.Title, result.Task.Status))
	}
	if ranTasks == limit {
		return fmt.Errorf("max task limit reached: %d", limit)
	}
	logChannel(ctx, commRuntime, cfg, "output", fmt.Sprintf("project_id=%s completed_tasks=%d log_file=%s", project.ID, ranTasks, logger.Path()))
	return logger.Log(ctx, "channel.output", map[string]any{
		"channel_id":      cfg.Communication.ChannelID,
		"log_file":        logger.Path(),
		"completed_tasks": ranTasks,
	})
}

func RunStory(ctx context.Context, goal string, opts Options, writer io.Writer) error {
	return RunGeneric(ctx, goal, opts, writer)
}

func RunPersonalSite(ctx context.Context, name string, opts Options, writer io.Writer) error {
	name = firstNonEmpty(name, "苏柯宇")
	return RunGeneric(ctx, "为"+name+"写一个个人网页并完成部署", opts, writer)
}

func RunPlan(ctx context.Context, goal string, opts Options, writer io.Writer) error {
	return RunGeneric(ctx, goal, opts, writer)
}

func RunWebAnswer(ctx context.Context, url string, goal string, opts Options, writer io.Writer) error {
	fullGoal := fmt.Sprintf("打开 %s，获取今日梅花易数的答案，并把答案结果发送到 channel 日志。用户目标：%s", url, goal)
	return RunGeneric(ctx, fullGoal, opts, writer)
}

func renderTasks(tasks []domain.Task) string {
	var builder strings.Builder
	for i, task := range tasks {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(fmt.Sprintf("%d. %s | %s | %s", i+1, task.Title, task.Status, task.Description))
	}
	return builder.String()
}
