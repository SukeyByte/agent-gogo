package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	appconfig "github.com/SukeyByte/agent-gogo/internal/config"
	"github.com/SukeyByte/agent-gogo/internal/contextbuilder"
	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/executor"
	"github.com/SukeyByte/agent-gogo/internal/memory"
	"github.com/SukeyByte/agent-gogo/internal/observability"
	"github.com/SukeyByte/agent-gogo/internal/planner"
	"github.com/SukeyByte/agent-gogo/internal/reviewer"
	appruntime "github.com/SukeyByte/agent-gogo/internal/runtime"
	"github.com/SukeyByte/agent-gogo/internal/scheduler"
	"github.com/SukeyByte/agent-gogo/internal/tester"
	"github.com/SukeyByte/agent-gogo/internal/tools"
	"github.com/SukeyByte/agent-gogo/internal/validator"
)

func RunTaskAwarenessDemo(ctx context.Context, goal string, opts Options, writer io.Writer) error {
	cfg, err := appconfig.Load(opts.ConfigPath)
	if err != nil {
		return err
	}
	logger, err := observability.NewFileLogger(cfg.Storage.LogPath, "w9-demo")
	if err != nil {
		return err
	}
	sqlite, err := openStore(ctx, cfg)
	if err != nil {
		return err
	}
	defer sqlite.Close()

	commRuntime := newCommunicationRuntime(cfg, writer)
	recorder := &taskAwarenessDemoExecutor{store: sqlite}
	service := appruntime.NewServiceWithComponents(
		sqlite,
		taskAwarenessDemoPlanner{},
		validator.NewMinimalTaskValidator(),
		scheduler.NewReadyScheduler(sqlite),
		recorder,
		tester.NewMinimalTester(sqlite),
		reviewer.NewMinimalReviewer(sqlite),
	)
	service.UseCommunication(cfg.Communication.ChannelID, cfg.Communication.SessionID, commRuntime)
	service.UseContextAssets(nil, nil, nil, memory.NewIndex(), contextbuilder.NewSerializer(contextbuilder.SerializerOptions{}), logger)

	if err := logger.Log(ctx, "input", map[string]any{"goal": goal, "demo": "w9-task-awareness"}); err != nil {
		return err
	}
	project, err := service.CreateProject(ctx, appruntime.CreateProjectRequest{
		Name: "W9 task awareness demo",
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

	first, err := service.RunNextTask(ctx, project.ID)
	if err != nil {
		return err
	}
	second, err := service.RunNextTask(ctx, project.ID)
	if err != nil {
		return err
	}
	secondContext := recorder.contextForTask(second.Task.ID)
	if strings.TrimSpace(secondContext) == "" {
		return fmt.Errorf("w9 demo context for second task was empty")
	}

	toolRuntime := tools.NewBuiltinRuntime(sqlite, cfg.Storage.ArtifactPath)
	toolRuntime.UseLogger(logger)
	toolRuntime.UseSecurityPolicy(tools.SecurityPolicy{
		AllowShell:                false,
		RequireConfirmationAtRisk: confirmationRisk(cfg),
	}, newConfirmationGate(writer))
	promptPath := filepath.ToSlash(filepath.Join("w9", project.ID+"-second-task-context.txt"))
	promptArtifact, err := toolRuntime.Call(ctx, tools.CallRequest{
		AttemptID: second.Attempt.ID,
		Name:      "artifact.write",
		Args: map[string]any{
			"path":    promptPath,
			"content": secondContext,
			"summary": "W9 second task runtime ContextPack prompt",
		},
	})
	if err != nil {
		return err
	}
	report := renderW9DemoReport(project, tasks, first, second, secondContext, promptArtifact.Result.EvidenceRef, logger.Path())
	reportPath := filepath.ToSlash(filepath.Join("w9", project.ID+"-result.md"))
	reportArtifact, err := toolRuntime.Call(ctx, tools.CallRequest{
		AttemptID: second.Attempt.ID,
		Name:      "artifact.write",
		Args: map[string]any{
			"path":    reportPath,
			"content": report,
			"summary": "W9 task awareness demo result",
		},
	})
	if err != nil {
		return err
	}

	logChannel(ctx, commRuntime, cfg, "output", fmt.Sprintf("project=%s first=%s:%s second=%s:%s", project.ID, first.Task.Title, first.Task.Status, second.Task.Title, second.Task.Status))
	logChannel(ctx, commRuntime, cfg, "prompt", promptExcerpt(secondContext))
	logChannel(ctx, commRuntime, cfg, "artifacts", fmt.Sprintf("prompt=%s result=%s", promptArtifact.Result.EvidenceRef, reportArtifact.Result.EvidenceRef))
	logChannel(ctx, commRuntime, cfg, "logs", logger.Path())
	return logger.Log(ctx, "channel.output", map[string]any{
		"channel_id":      cfg.Communication.ChannelID,
		"project_id":      project.ID,
		"first_task":      first.Task.Title,
		"second_task":     second.Task.Title,
		"prompt_artifact": promptArtifact.Result.EvidenceRef,
		"result_artifact": reportArtifact.Result.EvidenceRef,
		"log_file":        logger.Path(),
	})
}

type taskAwarenessDemoPlanner struct{}

func (p taskAwarenessDemoPlanner) PlanProject(ctx context.Context, req planner.PlanRequest) ([]domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []domain.Task{
		{
			ProjectID:          req.Project.ID,
			Title:              "Discover project state",
			Description:        "Produce a durable observation that later tasks must reuse.",
			Status:             domain.TaskStatusDraft,
			AcceptanceCriteria: []string{"project baseline is observed", "observation has evidence"},
		},
		{
			ProjectID:          req.Project.ID,
			Title:              "Use project memory",
			Description:        "Read the Project Digest and RelevantMemories created from the first task.",
			Status:             domain.TaskStatusDraft,
			AcceptanceCriteria: []string{"context includes dependency state", "context includes extracted memory"},
			DependsOn:          []string{"Discover project state"},
		},
	}, nil
}

type taskAwarenessDemoExecutor struct {
	store    executor.Store
	contexts map[string]string
}

func (e *taskAwarenessDemoExecutor) UseRuntimeContext(projectID string, contextText string) {
	if e.contexts == nil {
		e.contexts = map[string]string{}
	}
	e.contexts[projectID] = contextText
}

func (e *taskAwarenessDemoExecutor) contextForTask(taskID string) string {
	if e.contexts == nil {
		return ""
	}
	return e.contexts[taskID]
}

func (e *taskAwarenessDemoExecutor) Execute(ctx context.Context, task domain.Task) (executor.Result, error) {
	if err := ctx.Err(); err != nil {
		return executor.Result{}, err
	}
	if e.contexts == nil {
		e.contexts = map[string]string{}
	}
	e.contexts[task.ID] = e.contexts[task.ProjectID]
	inProgress, err := e.store.TransitionTask(ctx, task.ID, domain.TaskStatusInProgress, "w9 demo executor started task")
	if err != nil {
		return executor.Result{}, err
	}
	attempt, err := e.store.CreateTaskAttempt(ctx, task.ID)
	if err != nil {
		return executor.Result{}, err
	}
	summary := "Discovered project baseline: W9 should carry this observation into the next task through Project Digest and RelevantMemories."
	if strings.Contains(strings.ToLower(task.Title), "memory") {
		summary = "Second task received Project Digest, dependency state, and extracted memory from the first task."
	}
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   attempt.ID,
		Type:        "w9.demo",
		Summary:     summary,
		EvidenceRef: "context://" + task.ID,
		Payload:     fmt.Sprintf(`{"runtime_context_present":%t}`, strings.TrimSpace(e.contexts[task.ID]) != ""),
	}); err != nil {
		return executor.Result{}, err
	}
	implemented, err := e.store.TransitionTask(ctx, inProgress.ID, domain.TaskStatusImplemented, "w9 demo executor wrote observation")
	if err != nil {
		return executor.Result{}, err
	}
	return executor.Result{Task: implemented, Attempt: attempt}, nil
}

func renderW9DemoReport(project domain.Project, tasks []domain.Task, first appruntime.TaskRunResult, second appruntime.TaskRunResult, prompt string, promptArtifact string, logPath string) string {
	var b strings.Builder
	b.WriteString("# W9 Task Awareness Demo Run\n\n")
	b.WriteString("Project: ")
	b.WriteString(project.ID)
	b.WriteString("\nGoal: ")
	b.WriteString(project.Goal)
	b.WriteString("\n\n## Planned Tasks\n\n")
	b.WriteString(renderTasks(tasks))
	b.WriteString("\n\n## Results\n\n")
	b.WriteString("- First task: ")
	b.WriteString(first.Task.Title)
	b.WriteString(" -> ")
	b.WriteString(string(first.Task.Status))
	b.WriteString("\n- Second task: ")
	b.WriteString(second.Task.Title)
	b.WriteString(" -> ")
	b.WriteString(string(second.Task.Status))
	b.WriteString("\n- Prompt artifact: ")
	b.WriteString(promptArtifact)
	b.WriteString("\n- Log file: ")
	b.WriteString(logPath)
	b.WriteString("\n\n## Second Task Prompt Excerpt\n\n```text\n")
	b.WriteString(promptExcerpt(prompt))
	b.WriteString("\n```\n")
	return b.String()
}

func promptExcerpt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}
	needles := []string{"\"project_state\"", "\"task_state\"", "\"relevant_memories\"", "\"depends_on\"", "Task completed"}
	lines := strings.Split(prompt, "\n")
	var selected []string
	for _, line := range lines {
		for _, needle := range needles {
			if strings.Contains(line, needle) {
				selected = append(selected, line)
				break
			}
		}
	}
	if len(selected) == 0 {
		return truncateText(prompt, 1400)
	}
	return truncateText(strings.Join(selected, "\n"), 2200)
}

func truncateText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	if limit <= 3 {
		return value[:limit]
	}
	return value[:limit-3] + "..."
}
