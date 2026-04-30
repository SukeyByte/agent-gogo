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
	"sync"

	"github.com/sukeke/agent-gogo/internal/browser"
	"github.com/sukeke/agent-gogo/internal/communication"
	appconfig "github.com/sukeke/agent-gogo/internal/config"
	"github.com/sukeke/agent-gogo/internal/contextbuilder"
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
	"github.com/sukeke/agent-gogo/internal/session"
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

	channelID := cfg.Communication.ChannelID
	if channelID == "" {
		channelID = "web"
	}
	sessionID := cfg.Communication.SessionID
	if sessionID == "" {
		sessionID = "local"
	}

	// SSE hub for real-time push to browser
	hub := webhandlers.NewSSEHub(50)

	// Try to init full runtime (LLM may not be configured)
	var sender webhandlers.ChannelEventSender
	llm, llmErr := newLLMProvider(cfg)
	if llmErr == nil {
		sender, err = initWebRuntime(ctx, cfg, sqlite, llm, hub, channelID, sessionID)
		if err != nil {
			_, _ = fmt.Fprintf(writer, "Warning: runtime init failed (%v), running in read-only mode\n", err)
			sender = nil
		}
	} else {
		_, _ = fmt.Fprintf(writer, "Warning: LLM provider not configured (%v), running in read-only mode\n", llmErr)
	}

	distDir := findDistDir()
	apiServer := webhandlers.NewAPIServer(sqlite, sender, hub, webhandlers.ConfigView{
		WorkspacePath: cfg.Storage.WorkspacePath,
		SQLitePath:    cfg.Storage.SQLitePath,
		ArtifactPath:  cfg.Storage.ArtifactPath,
		LogPath:       cfg.Storage.LogPath,
		SkillRoots:    append([]string(nil), cfg.Storage.SkillRoots...),
		PersonaPath:   cfg.Storage.PersonaPath,
		ChannelID:     channelID,
		SessionID:     sessionID,
	}, channelID, sessionID, distDir)

	mode := "read-only"
	if sender != nil {
		mode = "full runtime"
	}
	_, _ = fmt.Fprintf(writer, "Web Console (%s) listening on http://%s\n", mode, addr)
	server := &http.Server{Addr: addr, Handler: apiServer}
	return server.ListenAndServe()
}

func initWebRuntime(ctx context.Context, cfg appconfig.Config, sqlite *store.SQLiteStore, llm provider.LLMProvider, hub *webhandlers.SSEHub, channelID, sessionID string) (*runtimeServiceBridge, error) {
	logger := observability.NoopLogger{}
	loggedLLM := observability.NewLoggingLLMProvider(llm, logger)

	commRuntime := communication.NewRuntime(communication.NewMemoryOutbox(), communication.NewRenderer())
	commRuntime.RegisterChannel(channelID, webhandlers.NewWebConsoleAdapter(channelID, hub))

	toolRuntime := tools.NewBuiltinRuntime(sqlite, cfg.Storage.WorkspacePath)
	toolRuntime.UseLogger(logger)
	toolRuntime.UseSecurityPolicy(tools.SecurityPolicy{
		AllowShell:                cfg.Security.AllowShell,
		ShellAllowlist:            cfg.Security.ShellAllowlist,
		RequireConfirmationAtRisk: confirmationRisk(cfg),
	}, tools.AutoConfirmationGate{})

	skillRegistry, err := skill.Discover(ctx, cfg.Storage.SkillRoots...)
	if err != nil {
		return nil, fmt.Errorf("skill discover: %w", err)
	}
	personaRegistry, err := persona.Discover(ctx, cfg.Storage.PersonaPath)
	if err != nil {
		return nil, fmt.Errorf("persona discover: %w", err)
	}
	memoryPath := filepath.Join(cfg.Storage.ArtifactPath, "memories.jsonl")
	memoryIndex, err := memory.NewPersistentIndex(ctx, memoryPath)
	if err != nil {
		return nil, fmt.Errorf("memory index: %w", err)
	}

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
	service.UseContextAssets(function.NewCatalogRegistry(), skillRegistry, personaRegistry, memoryIndex, contextbuilder.NewSerializer(contextbuilder.SerializerOptions{}), logger)
	service.UseContextBudget(cfg.Runtime.ContextMaxChars)
	service.UseMemoryPersistence(memoryPath)

	return &runtimeServiceBridge{service: service}, nil
}

type runtimeServiceBridge struct {
	service *appruntime.Service
}

func (b *runtimeServiceBridge) HandleChannelEvent(ctx context.Context, event webhandlers.InboundEvent) error {
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

func (b *runtimeServiceBridge) HandleUserConfirmation(ctx context.Context, confirmation webhandlers.InboundConfirmation) error {
	return b.service.HandleUserConfirmation(ctx, appruntime.UserConfirmation{
		ProjectID: confirmation.ProjectID,
		TaskID:    confirmation.TaskID,
		AttemptID: confirmation.AttemptID,
		ActionID:  confirmation.ActionID,
		Approved:  confirmation.Approved,
		Message:   confirmation.Message,
	})
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
	llm, err := provider.NewRegisteredLLMProvider(cfg.LLM.Provider, provider.OpenAICompatibleConfig{
		APIKey:          cfg.LLM.APIKey,
		BaseURL:         cfg.LLM.BaseURL,
		ChatModel:       cfg.LLM.Model,
		ThinkingEnabled: &thinking,
		ReasoningEffort: cfg.LLM.ReasoningEffort,
		HTTPClient:      client,
	})
	if err != nil {
		return nil, err
	}
	return provider.NewTimeoutProvider(llm, cfg.LLM.Timeout), nil
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
