package app

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/sukeke/agent-gogo/internal/browser"
	"github.com/sukeke/agent-gogo/internal/communication"
	appconfig "github.com/sukeke/agent-gogo/internal/config"
	"github.com/sukeke/agent-gogo/internal/contextbuilder"
	"github.com/sukeke/agent-gogo/internal/demo/webanswer"
	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/executor"
	"github.com/sukeke/agent-gogo/internal/function"
	"github.com/sukeke/agent-gogo/internal/memory"
	"github.com/sukeke/agent-gogo/internal/observability"
	"github.com/sukeke/agent-gogo/internal/persona"
	"github.com/sukeke/agent-gogo/internal/planner"
	"github.com/sukeke/agent-gogo/internal/provider"
	"github.com/sukeke/agent-gogo/internal/reviewer"
	appruntime "github.com/sukeke/agent-gogo/internal/runtime"
	"github.com/sukeke/agent-gogo/internal/scheduler"
	"github.com/sukeke/agent-gogo/internal/skill"
	"github.com/sukeke/agent-gogo/internal/store"
	"github.com/sukeke/agent-gogo/internal/tester"
	"github.com/sukeke/agent-gogo/internal/tools"
	"github.com/sukeke/agent-gogo/internal/validator"
)

const Version = "0.1.0-dev"

type Options struct {
	ConfigPath string
}

func Main(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stdout)
		return 0
	}
	ctx := context.Background()
	command, rest, opts, err := parseCommand(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		printUsage(stderr)
		return 2
	}
	switch command {
	case "plan":
		if len(rest) == 0 {
			_, _ = fmt.Fprintln(stderr, "plan requires a goal")
			return 2
		}
		if err := RunPlan(ctx, strings.Join(rest, " "), opts, stdout); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	case "answer-url", "run-web-answer":
		if len(rest) < 2 {
			_, _ = fmt.Fprintln(stderr, "answer-url requires a URL and a goal")
			return 2
		}
		if err := RunWebAnswer(ctx, rest[0], strings.Join(rest[1:], " "), opts, stdout); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	case "write-story", "story":
		if len(rest) == 0 {
			_, _ = fmt.Fprintln(stderr, "write-story requires a goal")
			return 2
		}
		if err := RunStory(ctx, strings.Join(rest, " "), opts, stdout); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
	default:
		_, _ = fmt.Fprintf(stderr, "unknown command %q\n", command)
		printUsage(stderr)
		return 2
	}
	return 0
}

func RunStory(ctx context.Context, goal string, opts Options, writer io.Writer) error {
	cfg, err := appconfig.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	logger, err := observability.NewFileLogger(cfg.Storage.LogPath, "story")
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
	toolRuntime := tools.NewBuiltinRuntime(sqlite, cfg.Storage.ArtifactPath)
	toolRuntime.UseLogger(logger)
	toolRuntime.UseSecurityPolicy(tools.SecurityPolicy{
		AllowShell:                cfg.Security.AllowShell,
		RequireConfirmationAtRisk: confirmationRisk(cfg),
	}, tools.AutoConfirmationGate{Approved: true})

	skillRegistry, err := skill.Discover(ctx, storySkillSources(cfg)...)
	if err != nil {
		return err
	}
	personaRegistry, err := persona.Discover(ctx, cfg.Storage.PersonaPath)
	if err != nil {
		return err
	}
	memoryIndex := memory.NewIndex()
	novelistPersona, err := generateNovelistPersona(ctx, loggedLLM, cfg.LLM.Model, goal, logger)
	if err != nil {
		return err
	}

	service := appruntime.NewServiceWithComponents(
		sqlite,
		planner.NewLLMPlanner(loggedLLM, cfg.LLM.Model),
		validator.NewMinimalTaskValidator(),
		scheduler.NewReadyScheduler(sqlite),
		executor.NewStoryExecutor(executor.StoryExecutorOptions{
			Store:  sqlite,
			Tools:  toolRuntime,
			LLM:    loggedLLM,
			Model:  cfg.LLM.Model,
			Logger: logger,
		}),
		tester.NewMinimalTester(sqlite),
		reviewer.NewLLMReviewer(sqlite, loggedLLM, cfg.LLM.Model),
	)
	service.UseLLM(loggedLLM, cfg.LLM.Model)
	service.UseCommunication(cfg.Communication.ChannelID, cfg.Communication.SessionID, commRuntime)
	service.UseContextAssets(function.NewCatalogRegistry(), skillRegistry, personaRegistry, memoryIndex, contextbuilder.NewSerializer(contextbuilder.SerializerOptions{}), logger)
	service.AddActivePersona(novelistPersona)

	if err := logger.Log(ctx, "input", map[string]any{
		"goal":          goal,
		"skill_sources": storySkillSources(cfg),
	}); err != nil {
		return err
	}
	project, err := service.CreateProject(ctx, appruntime.CreateProjectRequest{
		Name: "Short mystery story",
		Goal: goal,
	})
	if err != nil {
		return err
	}
	if _, err := service.PlanProject(ctx, project.ID); err != nil {
		return err
	}
	var story domain.Observation
	ranTasks := 0
	for {
		result, err := service.RunNextTask(ctx, project.ID)
		if errors.Is(err, sql.ErrNoRows) {
			break
		}
		if err != nil {
			return err
		}
		ranTasks++
		if candidate, obsErr := latestObservation(ctx, sqlite, result.Attempt.ID, "story.final"); obsErr == nil {
			story = candidate
		}
	}
	if ranTasks == 0 {
		return sql.ErrNoRows
	}
	if strings.TrimSpace(story.Summary) == "" {
		return errors.New("story.final observation not found")
	}
	logChannel(ctx, commRuntime, cfg, "output", story.Summary)
	logChannel(ctx, commRuntime, cfg, "logs", logger.Path())
	return logger.Log(ctx, "channel.output", map[string]any{
		"channel_id": cfg.Communication.ChannelID,
		"log_file":   logger.Path(),
	})
}

func RunPlan(ctx context.Context, goal string, opts Options, writer io.Writer) error {
	cfg, err := appconfig.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	llm, err := newLLMProvider(cfg)
	if err != nil {
		return err
	}
	sqlite, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer sqlite.Close()

	commRuntime := newCommunicationRuntime(cfg, writer)
	service := appruntime.NewService(sqlite)
	service.UseLLM(llm, cfg.LLM.Model)
	service.UseCommunication(cfg.Communication.ChannelID, cfg.Communication.SessionID, commRuntime)

	logChannel(ctx, commRuntime, cfg, "input", goal)
	project, err := service.CreateProject(ctx, appruntime.CreateProjectRequest{
		Name: "CLI plan",
		Goal: goal,
	})
	if err != nil {
		return err
	}
	logChannel(ctx, commRuntime, cfg, "index", "using Chain Router + Intent Analyzer + LLM Planner")
	tasks, err := service.PlanProject(ctx, project.ID)
	if err != nil {
		return err
	}
	logChannel(ctx, commRuntime, cfg, "plan", renderTasks(tasks))
	logChannel(ctx, commRuntime, cfg, "output", fmt.Sprintf("project_id=%s task_count=%d", project.ID, len(tasks)))
	return nil
}

func RunWebAnswer(ctx context.Context, url string, goal string, opts Options, writer io.Writer) error {
	cfg, err := appconfig.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	llm, err := newLLMProvider(cfg)
	if err != nil {
		return err
	}
	browserRuntime, closeBrowser, err := newBrowserRuntime(ctx, cfg)
	if err != nil {
		return err
	}
	defer closeBrowser()
	sqlite, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer sqlite.Close()

	commRuntime := newCommunicationRuntime(cfg, writer)
	webExecutor := webanswer.NewExecutor(sqlite, browserRuntime, llm, cfg.LLM.Model, url, goal)
	service := appruntime.NewServiceWithComponents(
		sqlite,
		planner.NewLLMPlanner(llm, cfg.LLM.Model),
		validator.NewMinimalTaskValidator(),
		scheduler.NewReadyScheduler(sqlite),
		webExecutor,
		tester.NewEvidenceTester(sqlite),
		reviewer.NewLLMReviewer(sqlite, llm, cfg.LLM.Model),
	)
	service.UseLLM(llm, cfg.LLM.Model)
	service.UseCommunication(cfg.Communication.ChannelID, cfg.Communication.SessionID, commRuntime)

	fullGoal := fmt.Sprintf("打开 %s，获取今日梅花易数的答案，并把答案结果发送到 channel 日志。用户目标：%s", url, goal)
	logChannel(ctx, commRuntime, cfg, "input", fullGoal)
	project, err := service.CreateProject(ctx, appruntime.CreateProjectRequest{
		Name: "Web answer",
		Goal: fullGoal,
	})
	if err != nil {
		return err
	}
	logChannel(ctx, commRuntime, cfg, "index", fmt.Sprintf("browser_provider=%s mcp_url=%s llm_provider=%s model=%s", cfg.Browser.Provider, cfg.Browser.MCPURL, cfg.LLM.Provider, cfg.LLM.Model))
	tasks, err := service.PlanProject(ctx, project.ID)
	if err != nil {
		return err
	}
	logChannel(ctx, commRuntime, cfg, "plan", renderTasks(tasks))
	logChannel(ctx, commRuntime, cfg, "execute", "RunNextTask starting with browser.open -> llm.answer -> evidence tester -> llm reviewer")
	result, err := service.RunNextTask(ctx, project.ID)
	if err != nil {
		return err
	}
	answer, err := latestAnswerObservation(ctx, sqlite, result.Attempt.ID)
	if err != nil {
		return err
	}
	logChannel(ctx, commRuntime, cfg, "output", answer.Summary)
	logChannel(ctx, commRuntime, cfg, "events", renderEvents(result.Events))
	return nil
}

func parseCommand(args []string) (string, []string, Options, error) {
	flags := flag.NewFlagSet("agent-gogo", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "config path")
	if err := flags.Parse(args); err != nil {
		return "", nil, Options{}, err
	}
	rest := flags.Args()
	if len(rest) == 0 {
		return "", nil, Options{}, errors.New("missing command")
	}
	return rest[0], rest[1:], Options{ConfigPath: *configPath}, nil
}

func newLLMProvider(cfg appconfig.Config) (provider.LLMProvider, error) {
	if err := cfg.ValidateForLLM(); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: cfg.LLM.Timeout}
	switch strings.ToLower(cfg.LLM.Provider) {
	case "deepseek":
		thinking := cfg.LLM.ThinkingEnabled
		return provider.NewDeepSeekProvider(provider.DeepSeekConfig{
			APIKey:           cfg.LLM.APIKey,
			BaseURL:          cfg.LLM.BaseURL,
			ChatModel:        cfg.LLM.Model,
			ThinkingEnabled:  &thinking,
			ReasoningEffort:  cfg.LLM.ReasoningEffort,
			HTTPClient:       client,
			DefaultBaseURL:   provider.DefaultDeepSeekBaseURL,
			DefaultChatModel: provider.DefaultDeepSeekModel,
		})
	case "openai":
		return provider.NewOpenAIProvider(provider.OpenAIConfig{
			APIKey:     cfg.LLM.APIKey,
			BaseURL:    cfg.LLM.BaseURL,
			ChatModel:  cfg.LLM.Model,
			HTTPClient: client,
		})
	case "openai_compatible":
		return provider.NewOpenAICompatibleProvider(provider.OpenAICompatibleConfig{
			ProviderName:    "openai_compatible",
			APIKey:          cfg.LLM.APIKey,
			BaseURL:         cfg.LLM.BaseURL,
			ChatModel:       cfg.LLM.Model,
			ReasoningEffort: cfg.LLM.ReasoningEffort,
			HTTPClient:      client,
		})
	default:
		return nil, fmt.Errorf("unsupported llm provider %q", cfg.LLM.Provider)
	}
}

func newBrowserRuntime(ctx context.Context, cfg appconfig.Config) (*browser.Runtime, func() error, error) {
	client := &http.Client{Timeout: cfg.Browser.Timeout}
	switch strings.ToLower(cfg.Browser.Provider) {
	case "chrome_mcp":
		browserProvider, err := provider.NewManagedChromeMCPBrowserProvider(ctx, provider.ChromeMCPBrowserProviderConfig{
			MCPURL:           cfg.Browser.MCPURL,
			HTTPClient:       client,
			AutoStart:        cfg.Browser.AutoStartMCP,
			DebugPort:        cfg.Browser.DebugPort,
			ChromePath:       cfg.Browser.ChromePath,
			UserDataDir:      cfg.Browser.UserDataDir,
			Headless:         cfg.Browser.Headless,
			MaxSummaryLength: cfg.Browser.MaxSummaryLength,
		})
		if err != nil {
			return nil, nil, err
		}
		return browser.NewRuntime(browserProvider), browserProvider.Close, nil
	case "http_fetch":
		return browser.NewRuntime(provider.NewFetchBrowserProvider(provider.FetchBrowserProviderConfig{
			HTTPClient:       client,
			MaxSummaryLength: cfg.Browser.MaxSummaryLength,
		})), func() error { return nil }, nil
	default:
		return nil, nil, fmt.Errorf("unsupported browser provider %q", cfg.Browser.Provider)
	}
}

func openStore(ctx context.Context, cfg appconfig.Config) (*store.SQLiteStore, error) {
	if cfg.Storage.SQLitePath != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(cfg.Storage.SQLitePath), 0o755); err != nil {
			return nil, err
		}
	}
	return store.OpenSQLite(ctx, cfg.Storage.SQLitePath)
}

func newCommunicationRuntime(cfg appconfig.Config, writer io.Writer) *communication.Runtime {
	outbox := communication.NewMemoryOutbox()
	runtime := communication.NewRuntime(outbox, communication.NewRenderer())
	runtime.RegisterChannel(cfg.Communication.ChannelID, communication.NewCLIAdapter(cfg.Communication.ChannelID, writer))
	return runtime
}

func logChannel(ctx context.Context, runtime *communication.Runtime, cfg appconfig.Config, stage string, text string) {
	_, _ = runtime.Dispatch(ctx, communication.NewMessageIntent(cfg.Communication.ChannelID, fmt.Sprintf("[%s] %s", stage, text)))
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

func renderEvents(events []domain.TaskEvent) string {
	var builder strings.Builder
	for i, event := range events {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(fmt.Sprintf("%s %s -> %s %s", event.Type, event.FromState, event.ToState, event.Message))
	}
	return builder.String()
}

func latestAnswerObservation(ctx context.Context, sqlite *store.SQLiteStore, attemptID string) (domain.Observation, error) {
	return latestObservation(ctx, sqlite, attemptID, "llm.answer")
}

func latestObservation(ctx context.Context, sqlite *store.SQLiteStore, attemptID string, typ string) (domain.Observation, error) {
	observations, err := sqlite.ListObservationsByAttempt(ctx, attemptID)
	if err != nil {
		return domain.Observation{}, err
	}
	for i := len(observations) - 1; i >= 0; i-- {
		if observations[i].Type == typ {
			return observations[i], nil
		}
	}
	return domain.Observation{}, fmt.Errorf("%s observation not found", typ)
}

func generateNovelistPersona(ctx context.Context, llm provider.LLMProvider, model string, goal string, logger observability.Logger) (contextbuilder.Persona, error) {
	resp, err := llm.Chat(ctx, provider.ChatRequest{
		Model: model,
		Messages: []provider.ChatMessage{
			{Role: "system", Content: "你是 agent-gogo 的 Persona Composer。请为当前任务生成一个临时小说家分人格，只输出可放进系统上下文的简洁角色指令，不要输出 JSON，不要包含 API key。"},
			{Role: "user", Content: goal},
		},
		Metadata: map[string]string{"stage": "persona.generate", "persona_id": "ephemeral-novelist"},
	})
	if err != nil {
		return contextbuilder.Persona{}, err
	}
	instructions := strings.TrimSpace(resp.Text)
	if instructions == "" {
		return contextbuilder.Persona{}, errors.New("generated novelist persona is empty")
	}
	persona := contextbuilder.Persona{
		ID:           "ephemeral-novelist",
		Name:         "小说家分人格",
		VersionHash:  "runtime-generated-v1",
		Instructions: instructions,
	}
	if logger != nil {
		_ = logger.Log(ctx, "persona.generate", persona)
	}
	return persona, nil
}

func storySkillSources(cfg appconfig.Config) []string {
	return append([]string(nil), cfg.Storage.SkillRoots...)
}

func confirmationRisk(cfg appconfig.Config) string {
	if cfg.Security.RequireConfirmHighRisk {
		return "high"
	}
	return ""
}

func printUsage(writer io.Writer) {
	_, _ = fmt.Fprintf(writer, `agent-gogo %s

Usage:
  agent-gogo plan "目标"
  agent-gogo answer-url https://example.com "问题"
  agent-gogo write-story "我希望完成一个短篇推理小说的编写"

Config:
  DEEPSEEK_API_KEY is required for the default DeepSeek provider.
  AGENT_GOGO_BROWSER_MCP_URL overrides the Chrome MCP HTTP bridge URL.
`, Version)
}
