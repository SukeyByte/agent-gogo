package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/provider"
	"github.com/SukeyByte/agent-gogo/internal/store"
	"github.com/SukeyByte/agent-gogo/internal/tools"
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
		if len(req.Tools) != 0 {
			t.Fatalf("generic executor should expose tools in JSON context, not provider tool_calls with dotted names: %#v", req.Tools)
		}
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

func TestGenericExecutorNormalizesCommonToolAliases(t *testing.T) {
	toolRuntime := tools.NewRuntime(nil)
	handler := func(ctx context.Context, args map[string]any) (tools.Result, error) {
		return tools.Result{Success: true}, nil
	}
	toolRuntime.Register(tools.Spec{Name: "file.read", Description: "read file", RiskLevel: "low"}, handler)
	toolRuntime.Register(tools.Spec{Name: "file.write", Description: "write file", RiskLevel: "medium"}, handler)
	toolRuntime.Register(tools.Spec{Name: "file.patch", Description: "patch file", RiskLevel: "medium"}, handler)
	toolRuntime.Register(tools.Spec{Name: "test.run", Description: "run tests", RiskLevel: "medium"}, handler)
	toolRuntime.Register(tools.Spec{Name: "code.search", Description: "search code", RiskLevel: "low"}, handler)

	exec := NewGenericExecutor(GenericExecutorOptions{Tools: toolRuntime})
	cases := map[string]string{
		"read_file":   "file.read",
		"write_file":  "file.write",
		"edit_file":   "file.patch",
		"run_tests":   "test.run",
		"search_code": "code.search",
	}
	for input, want := range cases {
		if got := exec.normalizeToolName(input); got != want {
			t.Fatalf("normalizeToolName(%q) = %q, want %q", input, got, want)
		}
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

func TestGenericExecutorRejectsWebSummaryFinishWithoutBrowserEvidence(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "generic", Goal: "summarize web"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "总结网页",
		Description:        "打开 https://example.com 并总结页面内容",
		AcceptanceCriteria: []string{"总结基于页面可见文本"},
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
		return tools.Result{Success: true, Output: map[string]any{"path": "README.md", "content": "local file"}, EvidenceRef: "README.md"}, nil
	})
	step := 0
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		step++
		switch step {
		case 1:
			return provider.ChatResponse{Text: `{"action":"tool_call","tool":"file.read","args":{"path":"README.md"},"reason":"wrong evidence","summary":"","question":""}`}, nil
		case 2:
			return provider.ChatResponse{Text: `{"action":"finish","tool":"","args":{},"reason":"summary done","summary":"Example summary without browser evidence","question":""}`}, nil
		default:
			return provider.ChatResponse{Text: `{"action":"finish","tool":"","args":{},"reason":"still done","summary":"Still no browser evidence","question":""}`}, nil
		}
	})
	exec := NewGenericExecutor(GenericExecutorOptions{
		Store:    sqlite,
		Tools:    toolRuntime,
		LLM:      llm,
		Model:    "test-model",
		MaxSteps: 3,
	})
	_, err = exec.Execute(ctx, ready)
	if err == nil || !strings.Contains(err.Error(), "max action steps") {
		t.Fatalf("expected executor to reject finish without browser evidence, got %v", err)
	}
}

func TestGenericExecutorAllowsResearchFinishWithBrowserEvidence(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "generic", Goal: "research web"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "研究上下文与可用资料",
		Description:        "先读取、搜索或浏览必要资料，确认任务事实、约束、现有实现和可用工具",
		AcceptanceCriteria: []string{"已用可用工具收集完成任务所需的事实和上下文"},
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
		return tools.Result{Success: true, Output: map[string]any{"url": args["url"], "dom_summary": "Example Domain"}, EvidenceRef: "https://example.com"}, nil
	})
	step := 0
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		step++
		if step == 1 {
			return provider.ChatResponse{Text: `{"action":"tool_call","tool":"browser.open","args":{"url":"https://example.com"},"reason":"collect web evidence","summary":"","question":""}`}, nil
		}
		return provider.ChatResponse{Text: `{"action":"finish","tool":"","args":{},"reason":"research complete","summary":"browser evidence captured","question":""}`}, nil
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

func TestAutoFinishDoesNotAcceptPatchWhenMechanicalVerificationRequired(t *testing.T) {
	task := domain.Task{
		Title:              "修改代码并保持 gofmt",
		Description:        "修改最少代码，并确认 gofmt 后语法正确。",
		AcceptanceCriteria: []string{"代码已经 gofmt 格式化", "语法正确"},
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

func TestAutoFinishAcceptsReadOnlyFileTask(t *testing.T) {
	task := domain.Task{
		Title:              "读取文件内容",
		Description:        "读取文件 README.md 的内容",
		AcceptanceCriteria: []string{"文件内容已读取"},
	}
	summary, ok := autoFinishSummary(task, []actionEvent{{
		Step:    1,
		Action:  "tool_call",
		Tool:    "file.read",
		State:   "succeeded",
		Summary: "file.read completed",
		Output:  `{"path":"README.md","content":"hello"}`,
	}})
	if !ok {
		t.Fatal("expected read-only file task to auto finish")
	}
	if !strings.Contains(summary, "file content captured") {
		t.Fatalf("unexpected summary %q", summary)
	}
}

func TestAutoFinishDoesNotAcceptBrowserOpenOnlyForInteractionTask(t *testing.T) {
	task := domain.Task{
		Title:              "Open URL and interact with page",
		Description:        "Open the URL, type 'hello-browser' in the Message input, click Go, and wait for 'done:hello-browser' to appear.",
		AcceptanceCriteria: []string{"done:hello-browser appears on the page"},
	}
	summary, ok := autoFinishSummary(task, []actionEvent{{
		Step:    1,
		Action:  "tool_call",
		Tool:    "browser.open",
		State:   "observed",
		Summary: "browser.open captured browser state",
		Output:  `{"dom_summary":"Agent Browser Test MessageGo"}`,
	}})
	if ok {
		t.Fatalf("unexpected auto finish: %s", summary)
	}
}

func TestAutoFinishAcceptsCompletedBrowserInteractionTask(t *testing.T) {
	task := domain.Task{
		Title:              "Open URL and interact with page",
		Description:        "Open the URL, type 'hello-browser' in the Message input, click Go, and wait for 'done:hello-browser' to appear.",
		AcceptanceCriteria: []string{"done:hello-browser appears on the page"},
	}
	summary, ok := autoFinishSummary(task, []actionEvent{
		{Step: 1, Action: "tool_call", Tool: "browser.open", State: "observed", Output: `{"dom_summary":"Agent Browser Test MessageGo"}`},
		{Step: 2, Action: "tool_call", Tool: "browser.input", State: "observed", Output: `{"dom_summary":"Agent Browser Test MessageGo"}`},
		{Step: 3, Action: "tool_call", Tool: "browser.click", State: "observed", Output: `{"dom_summary":"Agent Browser Test MessageGo done:hello-browser"}`},
		{Step: 4, Action: "tool_call", Tool: "browser.extract", State: "observed", Summary: "done text captured", Output: `{"dom_summary":"Agent Browser Test MessageGo done:hello-browser"}`},
	})
	if !ok {
		t.Fatal("expected completed browser interaction task to auto finish")
	}
	if !strings.Contains(summary, "browser evidence captured") {
		t.Fatalf("unexpected summary %q", summary)
	}
}

func TestGenericExecutorBlocksRepeatedIdenticalToolCalls(t *testing.T) {
	ctx := context.Background()
	sqlite, err := store.OpenSQLite(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer sqlite.Close()

	project, err := sqlite.CreateProject(ctx, domain.Project{Name: "generic", Goal: "guard"})
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	task, err := sqlite.CreateTask(ctx, domain.Task{
		ProjectID:          project.ID,
		Title:              "Write artifact after context",
		Description:        "Write a file through tool runtime after reading context",
		AcceptanceCriteria: []string{"file write tool is called"},
	})
	if err != nil {
		t.Fatalf("create task: %v", err)
	}
	ready, err := sqlite.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "ready")
	if err != nil {
		t.Fatalf("ready task: %v", err)
	}

	readCalls := 0
	toolRuntime := tools.NewRuntime(sqlite)
	toolRuntime.Register(tools.Spec{Name: "file.read", Description: "read file", RiskLevel: "low"}, func(ctx context.Context, args map[string]any) (tools.Result, error) {
		readCalls++
		return tools.Result{Success: true, Output: map[string]any{"path": args["path"], "content": "hello"}, EvidenceRef: "README.md"}, nil
	})
	toolRuntime.Register(tools.Spec{Name: "file.write", Description: "write file", RiskLevel: "medium"}, func(ctx context.Context, args map[string]any) (tools.Result, error) {
		return tools.Result{Success: true, Output: map[string]any{"path": args["path"]}, EvidenceRef: "file://" + args["path"].(string)}, nil
	})
	step := 0
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		step++
		if step <= 3 {
			return provider.ChatResponse{Text: `{"action":"tool_call","tool":"file.read","args":{"path":"README.md"},"reason":"read context"}`}, nil
		}
		if step == 4 {
			return provider.ChatResponse{Text: `{"action":"tool_call","tool":"file.write","args":{"path":"artifact.txt","content":"ok"},"reason":"recover with progress"}`}, nil
		}
		return provider.ChatResponse{Text: `{"action":"finish","summary":"artifact written","reason":"tool evidence exists"}`}, nil
	})
	exec := NewGenericExecutor(GenericExecutorOptions{
		Store:    sqlite,
		Tools:    toolRuntime,
		LLM:      llm,
		Model:    "test-model",
		MaxSteps: 5,
	})
	result, err := exec.Execute(ctx, ready)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.Task.Status != domain.TaskStatusImplemented {
		t.Fatalf("expected implemented task, got %s", result.Task.Status)
	}
	if readCalls != 2 {
		t.Fatalf("expected third identical read to be blocked, got %d read calls", readCalls)
	}
}
