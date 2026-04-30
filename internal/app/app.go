package app

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	htmlstd "html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

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
	webhandlers "github.com/sukeke/agent-gogo/web/handlers"
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
	if isWebCommand(args[0]) {
		opts, addr, err := parseWebInput(args[1:])
		if err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			printUsage(stderr)
			return 2
		}
		if err := RunWebConsole(ctx, opts, addr, stdout); err != nil {
			_, _ = fmt.Fprintln(stderr, err)
			return 1
		}
		return 0
	}
	goal, opts, err := parseAgentInput(args)
	if err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		printUsage(stderr)
		return 2
	}
	if err := RunAgent(ctx, goal, opts, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

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
	handler := webhandlers.NewServer(sqlite, webhandlers.ConfigView{
		WorkspacePath: cfg.Storage.WorkspacePath,
		SQLitePath:    cfg.Storage.SQLitePath,
		ArtifactPath:  cfg.Storage.ArtifactPath,
		LogPath:       cfg.Storage.LogPath,
		SkillRoots:    append([]string(nil), cfg.Storage.SkillRoots...),
		PersonaPath:   cfg.Storage.PersonaPath,
		ChannelID:     cfg.Communication.ChannelID,
		SessionID:     cfg.Communication.SessionID,
	})
	_, _ = fmt.Fprintf(writer, "Web Console listening on http://%s\n", addr)
	server := &http.Server{Addr: addr, Handler: handler}
	return server.ListenAndServe()
}

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

	service := appruntime.NewServiceWithComponents(
		sqlite,
		planner.NewLLMPlanner(loggedLLM, cfg.LLM.Model),
		validator.NewMinimalTaskValidator(),
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
	service.UseMemoryPersistence(memoryPath)

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
		ShellAllowlist:            cfg.Security.ShellAllowlist,
		RequireConfirmationAtRisk: confirmationRisk(cfg),
	}, newConfirmationGate(writer))

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

func RunPersonalSite(ctx context.Context, name string, opts Options, writer io.Writer) error {
	cfg, err := appconfig.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	logger, err := observability.NewFileLogger(cfg.Storage.LogPath, "personal-site")
	if err != nil {
		return err
	}
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
	if err := logger.Log(ctx, "input", map[string]any{
		"goal":         "为" + name + "写一个个人网页并完成部署",
		"workspace":    cfg.Storage.WorkspacePath,
		"test_command": cfg.Runtime.TestCommand,
	}); err != nil {
		return err
	}

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "M8 personal site", Goal: "Build and deploy a personal website for " + name})
	if err != nil {
		return err
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "Build and deploy personal website",
		Description:        "Use M8 engineering tools to create a static personal site and deploy it to web/dist.",
		AcceptanceCriteria: []string{"Code index is readable", "Source files are written", "Deployment files are written", "Tests pass", "Git diff is available"},
	})
	if err != nil {
		return err
	}
	ready, err := sqlite.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "M8 acceptance task ready")
	if err != nil {
		return err
	}
	inProgress, err := sqlite.TransitionTask(ctx, ready.ID, domain.TaskStatusInProgress, "personal site build started")
	if err != nil {
		return err
	}
	attempt, err := sqlite.CreateTaskAttempt(ctx, inProgress.ID)
	if err != nil {
		return err
	}
	if _, err := toolRuntime.Call(ctx, tools.CallRequest{AttemptID: attempt.ID, Name: "code.index", Args: map[string]any{"limit": 40, "symbol_limit": 40}}); err != nil {
		return err
	}
	if _, err := toolRuntime.Call(ctx, tools.CallRequest{AttemptID: attempt.ID, Name: "code.symbols", Args: map[string]any{"query": "RunPersonalSite", "limit": 20}}); err != nil {
		return err
	}

	html := personalSiteHTML(name)
	css := personalSiteCSS()
	files := []struct {
		path    string
		content string
	}{
		{path: "web/static/sukeyu/index.html", content: html},
		{path: "web/static/sukeyu/styles.css", content: css},
		{path: "web/dist/sukeyu/index.html", content: html},
		{path: "web/dist/sukeyu/styles.css", content: css},
	}
	for _, file := range files {
		if _, err := toolRuntime.Call(ctx, tools.CallRequest{
			AttemptID: attempt.ID,
			Name:      "file.write",
			Args:      map[string]any{"path": file.path, "content": file.content},
		}); err != nil {
			return err
		}
	}
	if _, err := toolRuntime.Call(ctx, tools.CallRequest{AttemptID: attempt.ID, Name: "git.status", Args: map[string]any{}}); err != nil {
		return err
	}
	if _, err := toolRuntime.Call(ctx, tools.CallRequest{AttemptID: attempt.ID, Name: "file.diff", Args: map[string]any{"path": "web/static/sukeyu/index.html"}}); err != nil {
		return err
	}
	implemented, err := sqlite.TransitionTask(ctx, inProgress.ID, domain.TaskStatusImplemented, "personal site source and deploy files written")
	if err != nil {
		return err
	}
	testResult, err := tester.NewCommandTester(sqlite, toolRuntime, cfg.Runtime.TestCommand).Test(ctx, implemented, attempt)
	if err != nil {
		return err
	}
	reviewResult, err := reviewer.NewMinimalReviewer(sqlite).Review(ctx, testResult.Task, attempt)
	if err != nil {
		return err
	}
	logChannel(ctx, commRuntime, cfg, "output", fmt.Sprintf("personal_site=%s deployed=%s task=%s", "web/static/sukeyu/index.html", "web/dist/sukeyu/index.html", reviewResult.Task.Status))
	logChannel(ctx, commRuntime, cfg, "logs", logger.Path())
	return logger.Log(ctx, "channel.output", map[string]any{
		"channel_id":  cfg.Communication.ChannelID,
		"log_file":    logger.Path(),
		"task_status": reviewResult.Task.Status,
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

func parseAgentInput(args []string) (string, Options, error) {
	flags := flag.NewFlagSet("agent-gogo", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "config path")
	if err := flags.Parse(args); err != nil {
		return "", Options{}, err
	}
	rest := flags.Args()
	if len(rest) == 0 {
		return "", Options{}, errors.New("missing agent goal")
	}
	return strings.TrimSpace(strings.Join(rest, " ")), Options{ConfigPath: *configPath}, nil
}

func parseWebInput(args []string) (Options, string, error) {
	flags := flag.NewFlagSet("agent-gogo web", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	configPath := flags.String("config", "", "config path")
	addr := flags.String("addr", "", "listen address")
	if err := flags.Parse(args); err != nil {
		return Options{}, "", err
	}
	return Options{ConfigPath: *configPath}, strings.TrimSpace(*addr), nil
}

func isWebCommand(arg string) bool {
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "web", "console", "serve":
		return true
	default:
		return false
	}
}

func extractPersonalSiteName(goal string) string {
	for _, marker := range []string{"为", "给"} {
		start := strings.Index(goal, marker)
		if start == -1 {
			continue
		}
		candidate := goal[start+len(marker):]
		stop := len(candidate)
		for _, sep := range []string{"写", "做", "创建", "生成", "建立", "设计", "搭建", "的"} {
			if index := strings.Index(candidate, sep); index >= 0 && index < stop {
				stop = index
			}
		}
		name := strings.Trim(candidate[:stop], " \t\r\n，。,.!?！？")
		if name != "" && len([]rune(name)) <= 12 {
			return name
		}
	}
	if strings.Contains(goal, "苏柯宇") {
		return "苏柯宇"
	}
	return "苏柯宇"
}

func newLLMProvider(cfg appconfig.Config) (provider.LLMProvider, error) {
	if err := cfg.ValidateForLLM(); err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: cfg.LLM.Timeout}
	thinking := cfg.LLM.ThinkingEnabled
	return provider.NewRegisteredLLMProvider(cfg.LLM.Provider, provider.OpenAICompatibleConfig{
		APIKey:          cfg.LLM.APIKey,
		BaseURL:         cfg.LLM.BaseURL,
		ChatModel:       cfg.LLM.Model,
		ThinkingEnabled: &thinking,
		ReasoningEffort: cfg.LLM.ReasoningEffort,
		HTTPClient:      client,
	})
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

type lazyBrowserEngine struct {
	cfg     appconfig.Config
	mu      sync.Mutex
	runtime *browser.Runtime
	close   func() error
}

func newLazyBrowserEngine(cfg appconfig.Config) *lazyBrowserEngine {
	return &lazyBrowserEngine{cfg: cfg}
}

func (e *lazyBrowserEngine) init(ctx context.Context) (*browser.Runtime, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.runtime != nil {
		return e.runtime, nil
	}
	runtime, closeBrowser, err := newBrowserRuntime(ctx, e.cfg)
	if err != nil {
		return nil, err
	}
	e.runtime = runtime
	e.close = closeBrowser
	return runtime, nil
}

func (e *lazyBrowserEngine) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.close == nil {
		return nil
	}
	return e.close()
}

func (e *lazyBrowserEngine) Open(ctx context.Context, url string) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.Open(ctx, url)
}

func (e *lazyBrowserEngine) Click(ctx context.Context, text string) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.Click(ctx, text)
}

func (e *lazyBrowserEngine) TypeText(ctx context.Context, text string) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.TypeText(ctx, text)
}

func (e *lazyBrowserEngine) Input(ctx context.Context, selector string, value string) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.Input(ctx, selector, value)
}

func (e *lazyBrowserEngine) Wait(ctx context.Context, text string, timeoutMS int) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.Wait(ctx, text, timeoutMS)
}

func (e *lazyBrowserEngine) Extract(ctx context.Context, query string) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.Extract(ctx, query)
}

func (e *lazyBrowserEngine) DOMSummary(ctx context.Context) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.DOMSummary(ctx)
}

func (e *lazyBrowserEngine) Screenshot(ctx context.Context) (browser.Snapshot, error) {
	runtime, err := e.init(ctx)
	if err != nil {
		return browser.Snapshot{}, err
	}
	return runtime.Screenshot(ctx)
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

func personalSiteHTML(name string) string {
	escapedName := htmlstd.EscapeString(strings.TrimSpace(name))
	if escapedName == "" {
		escapedName = "苏柯宇"
	}
	return `<!doctype html>
<html lang="zh-CN">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>` + escapedName + `</title>
  <meta name="description" content="` + escapedName + ` 的个人网页">
  <link rel="stylesheet" href="./styles.css">
</head>
<body>
  <main>
    <section class="intro" aria-labelledby="page-title">
      <div class="identity-mark" aria-hidden="true">
        <span>SKY</span>
      </div>
      <div class="intro-copy">
        <p class="eyebrow">Personal Website</p>
        <h1 id="page-title">` + escapedName + `</h1>
        <p class="lead">把复杂想法整理成清楚的产品、代码和表达。</p>
        <div class="actions">
          <a href="#work">查看方向</a>
          <a href="#contact">联系</a>
        </div>
      </div>
    </section>

    <section id="work" class="section">
      <h2>关注方向</h2>
      <div class="grid">
        <article>
          <span class="number">01</span>
          <h3>AI Runtime</h3>
          <p>关注可恢复执行、工具安全边界、上下文工程和可观测日志。</p>
        </article>
        <article>
          <span class="number">02</span>
          <h3>Product Engineering</h3>
          <p>从真实使用场景出发，把原型、后端、前端和验证闭环连起来。</p>
        </article>
        <article>
          <span class="number">03</span>
          <h3>Creative Systems</h3>
          <p>把写作、网页、自动化和工程工具做成可以长期迭代的系统。</p>
        </article>
      </div>
    </section>

    <section class="section profile">
      <h2>工作方式</h2>
      <p>先弄清问题，再压缩路径；先做可运行的版本，再用测试和反馈把它磨稳。</p>
      <ul>
        <li>偏好明确的状态机和小步验证。</li>
        <li>重视代码可读性、日志、回滚点和交付体验。</li>
        <li>喜欢把抽象能力落到一个能被打开、运行、检查的产物里。</li>
      </ul>
    </section>

    <section id="contact" class="section contact">
      <h2>联系</h2>
      <p>适合讨论 AI 工具、个人自动化、产品原型和工程系统。</p>
      <a href="mailto:hello@example.com">hello@example.com</a>
    </section>
  </main>
</body>
</html>
`
}

func personalSiteCSS() string {
	return `:root {
  color-scheme: light;
  --ink: #1e2528;
  --muted: #667175;
  --paper: #f7f4ee;
  --panel: #ffffff;
  --line: #d9d2c7;
  --accent: #206a5d;
  --accent-2: #b94f32;
  --shadow: rgba(37, 45, 48, 0.12);
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}

* {
  box-sizing: border-box;
}

body {
  margin: 0;
  background: var(--paper);
  color: var(--ink);
}

a {
  color: inherit;
}

main {
  width: min(1120px, calc(100% - 40px));
  margin: 0 auto;
}

.intro {
  min-height: 78vh;
  display: grid;
  grid-template-columns: minmax(260px, 0.9fr) minmax(320px, 1.1fr);
  gap: 56px;
  align-items: center;
  padding: 48px 0 40px;
  border-bottom: 1px solid var(--line);
}

.identity-mark {
  aspect-ratio: 4 / 5;
  display: grid;
  place-items: center;
  background:
    radial-gradient(circle at 26% 22%, rgba(255,255,255,0.88), transparent 26%),
    linear-gradient(145deg, #174d45 0%, #206a5d 48%, #c36246 100%);
  box-shadow: 0 24px 70px var(--shadow);
  border: 1px solid rgba(255,255,255,0.65);
}

.identity-mark span {
  color: #fffaf2;
  font-size: clamp(3rem, 8vw, 7rem);
  font-weight: 800;
  letter-spacing: 0;
}

.eyebrow {
  margin: 0 0 14px;
  color: var(--accent);
  font-size: 0.78rem;
  font-weight: 700;
  text-transform: uppercase;
}

h1 {
  margin: 0;
  font-size: clamp(3.2rem, 8vw, 7.5rem);
  line-height: 0.95;
  letter-spacing: 0;
}

.lead {
  max-width: 560px;
  margin: 24px 0 0;
  color: var(--muted);
  font-size: 1.25rem;
  line-height: 1.8;
}

.actions {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-top: 30px;
}

.actions a,
.contact a {
  display: inline-flex;
  align-items: center;
  min-height: 44px;
  padding: 0 18px;
  border: 1px solid var(--ink);
  text-decoration: none;
  font-weight: 700;
}

.actions a:first-child {
  background: var(--ink);
  color: #fff;
}

.section {
  padding: 56px 0;
  border-bottom: 1px solid var(--line);
}

h2 {
  margin: 0 0 24px;
  font-size: 1.6rem;
  letter-spacing: 0;
}

.grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 18px;
}

article {
  min-height: 220px;
  padding: 22px;
  background: var(--panel);
  border: 1px solid var(--line);
  box-shadow: 0 14px 35px var(--shadow);
}

.number {
  display: block;
  color: var(--accent-2);
  font-weight: 800;
  margin-bottom: 38px;
}

h3 {
  margin: 0 0 12px;
  font-size: 1.18rem;
}

p,
li {
  color: var(--muted);
  line-height: 1.8;
}

.profile p {
  max-width: 720px;
  font-size: 1.1rem;
}

.profile ul {
  margin: 22px 0 0;
  padding-left: 22px;
}

.contact {
  padding-bottom: 80px;
}

@media (max-width: 820px) {
  main {
    width: min(100% - 28px, 680px);
  }

  .intro {
    min-height: auto;
    grid-template-columns: 1fr;
    gap: 32px;
    padding-top: 28px;
  }

  .identity-mark {
    aspect-ratio: 16 / 10;
  }

  .grid {
    grid-template-columns: 1fr;
  }
}
`
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

func newConfirmationGate(writer io.Writer) tools.ConfirmationGate {
	return tools.NewCLIConfirmationGate(os.Stdin, writer)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func printUsage(writer io.Writer) {
	_, _ = fmt.Fprintf(writer, `agent-gogo %s

Usage:
  agent-gogo web --addr 127.0.0.1:8080
  agent-gogo "实现一个可测试的 Web Console Dashboard"
  agent-gogo "打开 https://example.com，提取页面信息并总结"
  agent-gogo "为当前仓库规划并执行下一步 runtime 改造"

Config:
  DEEPSEEK_API_KEY is required for the default DeepSeek provider.
  AGENT_GOGO_BROWSER_MCP_URL overrides the Chrome MCP HTTP bridge URL.
  AGENT_GOGO_ALLOW_SHELL=true enables allowlisted engineering commands.
`, Version)
}
