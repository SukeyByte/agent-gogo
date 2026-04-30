package executor

import (
	"context"
	"testing"

	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/provider"
	"github.com/sukeke/agent-gogo/internal/store"
	"github.com/sukeke/agent-gogo/internal/tools"
)

func TestGenericExecutorRunsToolActionLoop(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "generic", Goal: "write a file"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "Write artifact",
		Description:        "Write a file through tool runtime",
		AcceptanceCriteria: []string{"file write tool is called"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ready, err := sqlite.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "ready")
	if err != nil {
		t.Fatalf("ready task: %v", err)
	}

	toolRuntime := tools.NewRuntime(sqlite)
	toolRuntime.Register(tools.Spec{Name: "file.write", Description: "write file", RiskLevel: "medium"}, func(ctx context.Context, args map[string]any) (tools.Result, error) {
		return tools.Result{
			Success:     true,
			Output:      map[string]any{"path": args["path"]},
			EvidenceRef: "file://" + args["path"].(string),
		}, nil
	})
	step := 0
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		step++
		if step == 1 {
			return provider.ChatResponse{Text: `{"action":"tool_call","tool":"file.write","args":{"path":"artifact.txt","content":"ok"},"reason":"write requested file"}`}, nil
		}
		return provider.ChatResponse{Text: `{"action":"finish","summary":"artifact written","reason":"tool evidence exists"}`}, nil
	})
	exec := NewGenericExecutor(GenericExecutorOptions{
		Store:    sqlite,
		Tools:    toolRuntime,
		LLM:      llm,
		Model:    "test-model",
		MaxSteps: 3,
	})
	result, err := exec.Execute(ctx, ready)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Task.Status != domain.TaskStatusImplemented {
		t.Fatalf("expected implemented task, got %s", result.Task.Status)
	}
	observations, err := sqlite.ListObservationsByAttempt(ctx, result.Attempt.ID)
	if err != nil {
		t.Fatalf("list observations: %v", err)
	}
	types := map[string]bool{}
	for _, observation := range observations {
		types[observation.Type] = true
	}
	for _, typ := range []string{"state.file_changed", "agent.finish"} {
		if !types[typ] {
			t.Fatalf("expected observation %s in %#v", typ, observations)
		}
	}
}

func TestGenericExecutorContinuesAfterFailedToolCall(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "generic", Goal: "recover"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "Recover from bad action",
		Description:        "Retry with a valid tool call after one bad action",
		AcceptanceCriteria: []string{"file write tool is called"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ready, err := sqlite.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "ready")
	if err != nil {
		t.Fatalf("ready task: %v", err)
	}

	toolRuntime := tools.NewRuntime(sqlite)
	toolRuntime.Register(tools.Spec{Name: "file.read", Description: "read file", RiskLevel: "low"}, func(ctx context.Context, args map[string]any) (tools.Result, error) {
		return tools.Result{Success: false, Error: "not found"}, tools.ErrToolNotFound
	})
	toolRuntime.Register(tools.Spec{Name: "file.write", Description: "write file", RiskLevel: "medium"}, func(ctx context.Context, args map[string]any) (tools.Result, error) {
		return tools.Result{
			Success:     true,
			Output:      map[string]any{"path": args["path"]},
			EvidenceRef: "file://" + args["path"].(string),
		}, nil
	})
	step := 0
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		step++
		switch step {
		case 1:
			return provider.ChatResponse{Text: `{"action":"tool_call","tool":"file.read","args":{"path":"missing.txt"},"reason":"try reading"}`}, nil
		case 2:
			return provider.ChatResponse{Text: `{"action":"tool_call","tool":"file.write","args":{"path":"artifact.txt","content":"ok"},"reason":"recover with write"}`}, nil
		default:
			return provider.ChatResponse{Text: `{"action":"finish","summary":"artifact written","reason":"tool evidence exists"}`}, nil
		}
	})
	exec := NewGenericExecutor(GenericExecutorOptions{
		Store:    sqlite,
		Tools:    toolRuntime,
		LLM:      llm,
		Model:    "test-model",
		MaxSteps: 4,
	})
	result, err := exec.Execute(ctx, ready)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Task.Status != domain.TaskStatusImplemented {
		t.Fatalf("expected implemented task, got %s", result.Task.Status)
	}
}

func TestGenericExecutorNormalizesToolNameAction(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "generic", Goal: "normalize"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "Write artifact",
		Description:        "Write a file through tool runtime",
		AcceptanceCriteria: []string{"file write tool is called"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ready, err := sqlite.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "ready")
	if err != nil {
		t.Fatalf("ready task: %v", err)
	}

	toolRuntime := tools.NewRuntime(sqlite)
	toolRuntime.Register(tools.Spec{Name: "file.write", Description: "write file", RiskLevel: "medium"}, func(ctx context.Context, args map[string]any) (tools.Result, error) {
		return tools.Result{Success: true, Output: map[string]any{"path": args["path"]}, EvidenceRef: "file://" + args["path"].(string)}, nil
	})
	step := 0
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		step++
		if step == 1 {
			return provider.ChatResponse{Text: `{"action":"file.write","args":{"path":"artifact.txt","content":"ok"},"reason":"write requested file"}`}, nil
		}
		return provider.ChatResponse{Text: `{"action":"finish","summary":"artifact written","reason":"tool evidence exists"}`}, nil
	})
	exec := NewGenericExecutor(GenericExecutorOptions{
		Store:    sqlite,
		Tools:    toolRuntime,
		LLM:      llm,
		Model:    "test-model",
		MaxSteps: 3,
	})
	result, err := exec.Execute(ctx, ready)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Task.Status != domain.TaskStatusImplemented {
		t.Fatalf("expected implemented task, got %s", result.Task.Status)
	}
}

func TestGenericExecutorAutoFinishesDiagnosticTestRun(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "generic", Goal: "diagnose"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "运行 go test 获取失败详情",
		Description:        "执行 go test ./... 命令，获取失败的测试名称和错误信息。",
		AcceptanceCriteria: []string{"已运行 go test ./... 并获取到失败测试的详细输出，包括测试名称和错误信息。"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ready, err := sqlite.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "ready")
	if err != nil {
		t.Fatalf("ready task: %v", err)
	}

	toolRuntime := tools.NewRuntime(sqlite)
	toolRuntime.Register(tools.Spec{Name: "test.run", Description: "run tests", RiskLevel: "medium"}, func(ctx context.Context, args map[string]any) (tools.Result, error) {
		return tools.Result{
			Success:     false,
			Error:       "exit status 1",
			Output:      map[string]any{"command": "go test ./...", "passed": false, "summary": "FAIL TestAdd"},
			EvidenceRef: "exec://test.run",
		}, nil
	})
	step := 0
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		step++
		if step > 1 {
			t.Fatalf("expected executor to auto-finish after test.run evidence")
		}
		return provider.ChatResponse{Text: `{"action":"tool_call","tool":"test.run","args":{"command":"go test ./..."},"reason":"capture failing test output"}`}, nil
	})
	exec := NewGenericExecutor(GenericExecutorOptions{
		Store:    sqlite,
		Tools:    toolRuntime,
		LLM:      llm,
		Model:    "test-model",
		MaxSteps: 3,
	})
	result, err := exec.Execute(ctx, ready)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Task.Status != domain.TaskStatusImplemented {
		t.Fatalf("expected implemented task, got %s", result.Task.Status)
	}
}

func TestGenericExecutorAutoFinishesBrowserReadTask(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "generic", Goal: "read web"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "读取网页内容",
		Description:        "读取 https://example.com 的页面内容，包括可见文本和页面结构",
		AcceptanceCriteria: []string{"可见文本内容不空", "页面成功加载无错误"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ready, err := sqlite.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "ready")
	if err != nil {
		t.Fatalf("ready task: %v", err)
	}

	toolRuntime := tools.NewRuntime(sqlite)
	toolRuntime.Register(tools.Spec{Name: "browser.open", Description: "open page", RiskLevel: "low"}, func(ctx context.Context, args map[string]any) (tools.Result, error) {
		return tools.Result{
			Success:     true,
			Output:      map[string]any{"url": args["url"], "dom_summary": "Example Domain"},
			EvidenceRef: "https://example.com",
		}, nil
	})
	step := 0
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		step++
		if step > 1 {
			t.Fatalf("expected executor to auto-finish after browser evidence")
		}
		return provider.ChatResponse{Text: `{"action":"tool_call","tool":"browser.open","args":{"url":"https://example.com"},"reason":"read web page"}`}, nil
	})
	exec := NewGenericExecutor(GenericExecutorOptions{
		Store:    sqlite,
		Tools:    toolRuntime,
		LLM:      llm,
		Model:    "test-model",
		MaxSteps: 3,
	})
	result, err := exec.Execute(ctx, ready)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Task.Status != domain.TaskStatusImplemented {
		t.Fatalf("expected implemented task, got %s", result.Task.Status)
	}
}

func TestGenericExecutorContinuesAfterUnsupportedAction(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "generic", Goal: "recover action"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "Write artifact",
		Description:        "Write a file through tool runtime",
		AcceptanceCriteria: []string{"file write tool is called"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ready, err := sqlite.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "ready")
	if err != nil {
		t.Fatalf("ready task: %v", err)
	}

	toolRuntime := tools.NewRuntime(sqlite)
	toolRuntime.Register(tools.Spec{Name: "file.write", Description: "write file", RiskLevel: "medium"}, func(ctx context.Context, args map[string]any) (tools.Result, error) {
		return tools.Result{Success: true, Output: map[string]any{"path": args["path"]}, EvidenceRef: "file://" + args["path"].(string)}, nil
	})
	step := 0
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		step++
		switch step {
		case 1:
			return provider.ChatResponse{Text: `{"action":"burn","reason":"bad model output"}`}, nil
		case 2:
			return provider.ChatResponse{Text: `{"action":"tool_call","tool":"file.write","args":{"path":"artifact.txt","content":"ok"},"reason":"recover"}`}, nil
		default:
			return provider.ChatResponse{Text: `{"action":"finish","summary":"artifact written"}`}, nil
		}
	})
	exec := NewGenericExecutor(GenericExecutorOptions{
		Store:    sqlite,
		Tools:    toolRuntime,
		LLM:      llm,
		Model:    "test-model",
		MaxSteps: 4,
	})
	result, err := exec.Execute(ctx, ready)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Task.Status != domain.TaskStatusImplemented {
		t.Fatalf("expected implemented task, got %s", result.Task.Status)
	}
}

func TestGenericExecutorAutoFinishesGeneratedTextWriteOnLastStep(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "generic", Goal: "summarize"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "中文总结网页主要内容",
		Description:        "生成一段简要的中文总结文字",
		AcceptanceCriteria: []string{"总结为中文", "总结简洁明了"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ready, err := sqlite.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "ready")
	if err != nil {
		t.Fatalf("ready task: %v", err)
	}

	toolRuntime := tools.NewRuntime(sqlite)
	toolRuntime.Register(tools.Spec{Name: "file.write", Description: "write file", RiskLevel: "medium"}, func(ctx context.Context, args map[string]any) (tools.Result, error) {
		return tools.Result{Success: true, Output: map[string]any{"path": args["path"]}, EvidenceRef: "file://" + args["path"].(string)}, nil
	})
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		return provider.ChatResponse{Text: `{"action":"tool_call","tool":"file.write","args":{"path":"summary.md","content":"这是中文总结。"},"reason":"write summary"}`}, nil
	})
	exec := NewGenericExecutor(GenericExecutorOptions{
		Store:    sqlite,
		Tools:    toolRuntime,
		LLM:      llm,
		Model:    "test-model",
		MaxSteps: 1,
	})
	result, err := exec.Execute(ctx, ready)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Task.Status != domain.TaskStatusImplemented {
		t.Fatalf("expected implemented task, got %s", result.Task.Status)
	}
}

func TestAutoFinishDoesNotAcceptPatchWhenPassingTestsRequired(t *testing.T) {
	task := domain.Task{
		Title:              "修改代码以修复失败测试",
		Description:        "修改最少代码，并运行 go test ./... 确认通过。",
		AcceptanceCriteria: []string{"修改后 go test ./... 全部通过（0失败）"},
	}
	summary, ok := autoFinishSummary(task, []actionEvent{{
		Step:    1,
		Action:  "tool_call",
		Tool:    "file.patch",
		State:   "changed",
		Summary: "patched add.go",
	}})
	if ok {
		t.Fatalf("unexpected auto finish: %s", summary)
	}
}
