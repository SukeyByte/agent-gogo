package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/llmjson"
	"github.com/sukeke/agent-gogo/internal/observer"
	"github.com/sukeke/agent-gogo/internal/provider"
	"github.com/sukeke/agent-gogo/internal/tools"
)

type GenericStore interface {
	Store
	AddTaskEvent(ctx context.Context, event domain.TaskEvent) (domain.TaskEvent, error)
	CompleteTaskAttempt(ctx context.Context, attemptID string, status domain.AttemptStatus, message string) (domain.TaskAttempt, error)
}

type ToolRuntime interface {
	Call(ctx context.Context, req tools.CallRequest) (tools.CallResponse, error)
	ListSpecs() []tools.Spec
}

type GenericExecutorOptions struct {
	Store    GenericStore
	Tools    ToolRuntime
	LLM      provider.LLMProvider
	Model    string
	Observer *observer.Interpreter
	MaxSteps int
}

type GenericExecutor struct {
	store              GenericStore
	tools              ToolRuntime
	llm                provider.LLMProvider
	model              string
	observer           *observer.Interpreter
	maxSteps           int
	contextByProjectID map[string]string
}

type ExecutionError struct {
	Task    domain.Task
	Attempt domain.TaskAttempt
	Err     error
}

func (e *ExecutionError) Error() string {
	if e == nil || e.Err == nil {
		return "execution failed"
	}
	return e.Err.Error()
}

func (e *ExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewGenericExecutor(options GenericExecutorOptions) *GenericExecutor {
	maxSteps := options.MaxSteps
	if maxSteps <= 0 {
		maxSteps = 12
	}
	interpreter := options.Observer
	if interpreter == nil {
		interpreter = observer.NewInterpreter(options.Store)
	}
	return &GenericExecutor{
		store:              options.Store,
		tools:              options.Tools,
		llm:                options.LLM,
		model:              options.Model,
		observer:           interpreter,
		maxSteps:           maxSteps,
		contextByProjectID: map[string]string{},
	}
}

func (e *GenericExecutor) UseRuntimeContext(projectID string, contextText string) {
	if e.contextByProjectID == nil {
		e.contextByProjectID = map[string]string{}
	}
	e.contextByProjectID[projectID] = contextText
}

func (e *GenericExecutor) Execute(ctx context.Context, task domain.Task) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if e.store == nil {
		return Result{}, errors.New("generic executor store is required")
	}
	inProgress, err := e.store.TransitionTask(ctx, task.ID, domain.TaskStatusInProgress, "generic executor started action loop")
	if err != nil {
		return Result{}, err
	}
	attempt, err := e.store.CreateTaskAttempt(ctx, task.ID)
	if err != nil {
		return Result{}, err
	}
	if e.llm == nil || e.tools == nil {
		return e.finishWithoutTools(ctx, inProgress, attempt, "generic executor completed without llm/tool runtime")
	}
	events := []actionEvent{}
	fingerprints := map[string]int{}
	for step := 1; step <= e.maxSteps; step++ {
		if summary, ok := autoFinishSummary(inProgress, events); ok {
			return e.finishWithoutTools(ctx, inProgress, attempt, summary)
		}
		action, err := e.nextAction(ctx, inProgress, attempt, step, events)
		if err != nil {
			return e.fail(ctx, inProgress, attempt, err)
		}
		fingerprint := actionFingerprint(action)
		if fingerprint != "" {
			fingerprints[fingerprint]++
			if fingerprints[fingerprint] > 2 {
				events = append(events, actionEvent{
					Step:    step,
					Action:  action.Action,
					Tool:    action.Tool,
					State:   "rejected",
					Summary: "repeated action blocked by progress guard",
					Error:   "same action and arguments repeated too many times",
				})
				continue
			}
		}
		if err := e.recordAction(ctx, inProgress.ID, attempt.ID, step, action); err != nil {
			return Result{}, err
		}
		switch action.Action {
		case "finish":
			summary := strings.TrimSpace(action.Summary)
			if summary == "" {
				summary = strings.TrimSpace(action.Reason)
			}
			if summary == "" {
				summary = "generic action loop finished"
			}
			if ok, reason := finishEvidenceReady(inProgress, events); !ok {
				events = append(events, actionEvent{
					Step:    step,
					Action:  "finish",
					State:   "rejected",
					Summary: summary,
					Error:   reason,
				})
				continue
			}
			return e.finishWithoutTools(ctx, inProgress, attempt, summary)
		case "ask_user":
			question := firstNonEmpty(action.Question, action.Reason, "user input required")
			needInput, transitionErr := e.store.TransitionTask(ctx, inProgress.ID, domain.TaskStatusNeedUserInput, question)
			if transitionErr != nil {
				return Result{}, transitionErr
			}
			if _, completeErr := e.store.CompleteTaskAttempt(ctx, attempt.ID, domain.AttemptStatusCancelled, question); completeErr != nil {
				return Result{}, completeErr
			}
			return Result{Task: needInput, Attempt: attempt}, &ExecutionError{Task: needInput, Attempt: attempt, Err: errors.New(question)}
		case "tool_call":
			if strings.TrimSpace(action.Tool) == "" {
				return e.fail(ctx, inProgress, attempt, errors.New("tool_call action missing tool"))
			}
			response, callErr := e.tools.Call(ctx, tools.CallRequest{
				AttemptID: attempt.ID,
				Name:      action.Tool,
				Args:      action.Args,
			})
			state, observeErr := e.observer.InterpretToolResult(ctx, observer.ToolResultRequest{
				Task:     inProgress,
				Attempt:  attempt,
				Response: response,
			})
			if observeErr != nil {
				return Result{}, observeErr
			}
			events = append(events, actionEvent{
				Step:        step,
				Action:      action.Action,
				Tool:        action.Tool,
				Summary:     state.Summary,
				State:       string(state.Status),
				Error:       state.FailureReason,
				EvidenceRef: state.EvidenceRef,
				Output:      compactToolOutput(response.Result.Output),
			})
			if callErr != nil {
				continue
			}
			if summary, ok := autoFinishSummary(inProgress, events); ok {
				return e.finishWithoutTools(ctx, inProgress, attempt, summary)
			}
		default:
			events = append(events, actionEvent{
				Step:    step,
				Action:  firstNonEmpty(action.Action, "unknown"),
				State:   "rejected",
				Summary: firstNonEmpty(action.Reason, action.Summary, "unsupported generic action"),
				Error:   fmt.Sprintf("unsupported generic action %q", action.Action),
			})
			continue
		}
	}
	return e.fail(ctx, inProgress, attempt, errors.New("generic executor reached max action steps without accepted finish"))
}

func (e *GenericExecutor) nextAction(ctx context.Context, task domain.Task, attempt domain.TaskAttempt, step int, events []actionEvent) (agentAction, error) {
	payload, err := json.Marshal(map[string]any{
		"task": map[string]any{
			"id":                  task.ID,
			"title":               task.Title,
			"description":         task.Description,
			"acceptance_criteria": task.AcceptanceCriteria,
		},
		"attempt_id":      attempt.ID,
		"step":            step,
		"runtime_context": e.contextByProjectID[task.ProjectID],
		"available_tools": toolSchemas(e.tools.ListSpecs()),
		"prior_events":    events,
	})
	if err != nil {
		return agentAction{}, err
	}
	specs := e.tools.ListSpecs()
	var action agentAction
	if err := llmjson.ChatObject(ctx, llmjson.Request{
		LLM:        e.llm,
		Model:      e.model,
		System:     genericExecutorPrompt,
		User:       string(payload),
		SchemaName: "generic_agent_action",
		Schema:     agentActionSchema(specs),
		Tools:      providerTools(specs),
		Metadata: map[string]string{
			"stage":      "executor.generic.action",
			"task_id":    task.ID,
			"attempt_id": attempt.ID,
			"step":       fmt.Sprint(step),
		},
		MaxRepairs: 1,
	}, &action); err != nil {
		return agentAction{}, err
	}
	action.Action = strings.TrimSpace(action.Action)
	action.Tool = e.normalizeToolName(action.Tool)
	if action.Args == nil {
		action.Args = map[string]any{}
	}
	if toolName := e.normalizeToolName(action.Action); e.isAvailableTool(toolName) {
		action.Tool = toolName
		action.Action = "tool_call"
	}
	return action, nil
}

func (e *GenericExecutor) normalizeToolName(name string) string {
	name = strings.TrimSpace(name)
	if e.isAvailableTool(name) {
		return name
	}
	aliases := map[string]string{
		"read_file":       "file.read",
		"file_read":       "file.read",
		"write_file":      "file.write",
		"file_write":      "file.write",
		"edit_file":       "file.patch",
		"file_edit":       "file.patch",
		"patch_file":      "file.patch",
		"file_patch":      "file.patch",
		"run_tests":       "test.run",
		"run_test":        "test.run",
		"go_test":         "test.run",
		"run_command":     "shell.run",
		"shell_command":   "shell.run",
		"search_code":     "code.search",
		"code_search":     "code.search",
		"index_code":      "code.index",
		"code_index":      "code.index",
		"git_diff":        "git.diff",
		"git_status":      "git.status",
		"browser_open":    "browser.open",
		"open_url":        "browser.open",
		"browser_extract": "browser.extract",
	}
	if mapped := aliases[strings.ToLower(name)]; e.isAvailableTool(mapped) {
		return mapped
	}
	return name
}

func (e *GenericExecutor) isAvailableTool(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || e.tools == nil {
		return false
	}
	for _, spec := range e.tools.ListSpecs() {
		if spec.Name == name {
			return true
		}
	}
	return false
}

func (e *GenericExecutor) recordAction(ctx context.Context, taskID string, attemptID string, step int, action agentAction) error {
	payload, err := json.Marshal(action)
	if err != nil {
		return err
	}
	_, err = e.store.AddTaskEvent(ctx, domain.TaskEvent{
		TaskID:    taskID,
		AttemptID: attemptID,
		Type:      "agent.action_selected",
		Message:   fmt.Sprintf("step %d selected %s %s", step, action.Action, action.Tool),
		Payload:   string(payload),
	})
	return err
}

func (e *GenericExecutor) finishWithoutTools(ctx context.Context, task domain.Task, attempt domain.TaskAttempt, summary string) (Result, error) {
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   attempt.ID,
		Type:        "agent.finish",
		Summary:     summary,
		EvidenceRef: "agent://finish",
	}); err != nil {
		return Result{}, err
	}
	implemented, err := e.store.TransitionTask(ctx, task.ID, domain.TaskStatusImplemented, summary)
	if err != nil {
		return Result{}, err
	}
	return Result{Task: implemented, Attempt: attempt}, nil
}

func (e *GenericExecutor) fail(ctx context.Context, task domain.Task, attempt domain.TaskAttempt, cause error) (Result, error) {
	message := strings.TrimSpace(cause.Error())
	if message == "" {
		message = "generic action loop failed"
	}
	failed, err := e.store.TransitionTask(ctx, task.ID, domain.TaskStatusFailed, message)
	if err != nil {
		return Result{}, err
	}
	completedAttempt, err := e.store.CompleteTaskAttempt(ctx, attempt.ID, domain.AttemptStatusFailed, message)
	if err != nil {
		return Result{}, err
	}
	return Result{}, &ExecutionError{Task: failed, Attempt: completedAttempt, Err: cause}
}

type agentAction struct {
	Action   string         `json:"action"`
	Tool     string         `json:"tool"`
	Args     map[string]any `json:"args"`
	Reason   string         `json:"reason"`
	Summary  string         `json:"summary"`
	Question string         `json:"question"`
}

type actionEvent struct {
	Step        int    `json:"step"`
	Action      string `json:"action"`
	Tool        string `json:"tool,omitempty"`
	State       string `json:"state,omitempty"`
	Summary     string `json:"summary,omitempty"`
	Error       string `json:"error,omitempty"`
	EvidenceRef string `json:"evidence_ref,omitempty"`
	Output      string `json:"output,omitempty"`
}

func toolSchemas(specs []tools.Spec) []map[string]any {
	result := make([]map[string]any, 0, len(specs))
	for _, spec := range specs {
		result = append(result, map[string]any{
			"name":           spec.Name,
			"description":    spec.Description,
			"risk_level":     spec.RiskLevel,
			"requires_shell": spec.RequiresShell,
			"input_schema":   spec.InputSchema,
			"output_schema":  spec.OutputSchema,
		})
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func compactToolOutput(output map[string]any) string {
	if len(output) == 0 {
		return ""
	}
	data, err := json.Marshal(output)
	if err != nil {
		return fmt.Sprint(output)
	}
	text := string(data)
	const maxOutput = 2400
	if len(text) <= maxOutput {
		return text
	}
	return text[:maxOutput] + "...[truncated]"
}

func actionFingerprint(action agentAction) string {
	switch action.Action {
	case "tool_call":
		data, err := json.Marshal(action.Args)
		if err != nil {
			data = []byte(fmt.Sprint(action.Args))
		}
		return "tool:" + strings.TrimSpace(action.Tool) + ":" + string(data)
	case "finish":
		return "finish:" + strings.TrimSpace(action.Summary)
	case "ask_user":
		return "ask_user:" + strings.TrimSpace(action.Question)
	default:
		return "action:" + strings.TrimSpace(action.Action)
	}
}

func finishEvidenceReady(task domain.Task, events []actionEvent) (bool, string) {
	hasSuccessfulToolEvidence := false
	hasSourceRead := false
	hasBrowserRead := false
	hasTestRun := false
	hasPassingTest := false
	for _, event := range events {
		switch event.State {
		case "succeeded", "changed", "verified", "observed":
			if event.Tool != "" {
				hasSuccessfulToolEvidence = true
			}
		}
		if event.Tool == "test.run" {
			hasTestRun = true
			if event.State == "verified" {
				hasPassingTest = true
			}
			if event.State == "failed" && taskNeedsDiagnosticTestRun(task) {
				hasSuccessfulToolEvidence = true
			}
		}
		if event.Tool == "file.read" {
			hasSourceRead = true
		}
		if strings.HasPrefix(event.Tool, "browser.") && event.State == "observed" {
			hasBrowserRead = true
		}
	}
	if !hasSuccessfulToolEvidence {
		return false, "finish requires at least one successful interpreted tool result"
	}
	if taskNeedsPassingTest(task) && !hasPassingTest {
		return false, "finish requires passing test.run evidence for this task"
	}
	if taskNeedsDiagnosticTestRun(task) && !hasTestRun {
		return false, "finish requires test.run evidence for this task"
	}
	if taskNeedsBrowserRead(task) && !taskNeedsGeneratedText(task) && !hasBrowserRead {
		return false, "finish requires browser evidence for this task"
	}
	if taskNeedsSourceRead(task) && !hasSourceRead {
		return false, "finish requires file.read evidence for this task"
	}
	return true, ""
}

func autoFinishSummary(task domain.Task, events []actionEvent) (string, bool) {
	if len(events) == 0 {
		return "", false
	}
	hasSourceRead := false
	hasBrowserRead := false
	for _, event := range events {
		if event.Tool == "file.read" && event.State == "succeeded" {
			hasSourceRead = true
		}
		if strings.HasPrefix(event.Tool, "browser.") && event.State == "observed" {
			hasBrowserRead = true
		}
	}
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if taskNeedsBrowserRead(task) && !taskNeedsGeneratedText(task) && hasBrowserRead && strings.HasPrefix(event.Tool, "browser.") && event.State == "observed" {
			return "browser evidence captured for this web-reading task: " + firstNonEmpty(event.Summary, event.Output), true
		}
		if taskNeedsReadOnly(task) && event.Tool == "file.read" && event.State == "succeeded" {
			return "file content captured for this read-only task: " + firstNonEmpty(event.Summary, event.Output), true
		}
		if taskNeedsGeneratedText(task) && (event.Tool == "file.write" || event.Tool == "document.write" || event.Tool == "artifact.write") && event.State == "changed" {
			return "generated text written for this task: " + firstNonEmpty(event.Summary, event.Output), true
		}
		if taskNeedsCodeChange(task) && !taskNeedsPassingTest(task) && !taskNeedsMechanicalVerification(task) && (event.Tool == "file.patch" || event.Tool == "file.write") && event.State == "changed" {
			return "workspace file changed and satisfies this code-change task: " + firstNonEmpty(event.Summary, event.Output), true
		}
		if event.Tool != "test.run" {
			continue
		}
		if taskNeedsPassingTest(task) && event.State == "verified" {
			return "test.run passed and satisfies this verification task", true
		}
		if taskNeedsDiagnosticTestRun(task) && (!taskNeedsSourceRead(task) || hasSourceRead) {
			return "test.run output captured for diagnostic task: " + firstNonEmpty(event.Summary, event.Output), true
		}
	}
	return "", false
}

func taskNeedsPassingTest(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	if strings.Contains(text, "go test") && (strings.Contains(text, "pass") || strings.Contains(text, "ok") || strings.Contains(text, "通过") || strings.Contains(text, "退出码0") || strings.Contains(text, "status 0")) {
		return true
	}
	for _, marker := range []string{
		"tests pass",
		"tests passed",
		"all tests pass",
		"all tests passed",
		"test passed",
		"passing tests",
		"go test ./... returns ok",
		"returns ok",
		"no fail",
		"验证所有测试",
		"驗證所有測試",
		"所有测试通过",
		"所有測試通過",
		"验证修改成功",
		"確認所有測試",
		"确认所有测试",
		"均通过",
		"全部通过",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func taskNeedsDiagnosticTestRun(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	if taskNeedsPassingTest(task) {
		return false
	}
	for _, marker := range []string{
		"go test",
		"run tests",
		"test output",
		"failing test",
		"failure output",
		"运行失败测试",
		"获取失败",
		"捕获失败",
		"失败输出",
		"执行 go test",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	for _, field := range strings.FieldsFunc(text, func(r rune) bool {
		return !(r >= 'a' && r <= 'z')
	}) {
		if field == "tests" || field == "testing" {
			return true
		}
	}
	return false
}

func taskNeedsSourceRead(task domain.Task) bool {
	if taskNeedsBrowserRead(task) {
		return false
	}
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	for _, marker := range []string{
		"source file",
		"source files",
		"implementation file",
		"failing function",
		"relevant source",
		"read relevant",
		"源文件",
		"实现文件",
		"實現文件",
		"读取",
		"閱讀",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func taskNeedsBrowserRead(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	for _, marker := range []string{
		"http://",
		"https://",
		"url",
		"web page",
		"webpage",
		"browser",
		"visible text",
		"dom",
		"网页",
		"網頁",
		"页面",
		"頁面",
		"浏览器",
		"瀏覽器",
		"可见文本",
		"可見文本",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func taskNeedsGeneratedText(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	for _, marker := range []string{
		"summarize",
		"summary",
		"write a summary",
		"draft",
		"compose",
		"总结",
		"總結",
		"概括",
		"撰写",
		"撰寫",
		"编写",
		"編寫",
		"用中文",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func taskNeedsReadOnly(task domain.Task) bool {
	if taskNeedsPassingTest(task) || taskNeedsDiagnosticTestRun(task) || taskNeedsCodeChange(task) || taskNeedsMechanicalVerification(task) || taskNeedsGeneratedText(task) || taskNeedsBrowserRead(task) {
		return false
	}
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	for _, marker := range []string{
		"read file",
		"read the file",
		"inspect file",
		"show file",
		"file content",
		"读取文件",
		"查看文件",
		"文件内容",
		"閱讀文件",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func taskNeedsMechanicalVerification(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	for _, marker := range []string{
		"gofmt",
		"format",
		"formatted",
		"compile",
		"build",
		"syntax",
		"lint",
		"signature",
		"格式化",
		"编译",
		"編譯",
		"构建",
		"構建",
		"语法",
		"語法",
		"签名",
		"簽名",
		"机械验收",
		"機械驗收",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func taskNeedsCodeChange(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	for _, marker := range []string{
		"modify",
		"patch",
		"fix failing",
		"code change",
		"minimal code",
		"apply minimal",
		"修改代码",
		"修改源代码",
		"应用",
		"修复",
		"最小修改",
		"最小化",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

const genericExecutorPrompt = `You are the GenericExecutor for agent-gogo.
Choose exactly one next action and return only JSON.
Allowed JSON shapes:
{"action":"tool_call","tool":"file.write","args":{"path":"...","content":"..."},"reason":"...","summary":"","question":""}
{"action":"finish","tool":"","args":{},"reason":"...","summary":"...","question":""}
{"action":"ask_user","tool":"","args":{},"reason":"...","summary":"","question":"..."}
Rules:
- Use only tools listed in available_tools.
- Prefer small reversible tool calls.
- Prefer code.index or code.search to discover repository structure.
- Prefer file.read to inspect file contents.
- Prefer file.patch for small source edits.
- Prefer test.run, not shell.run, when validating tests.
- shell.run is exec-style, not a real shell: do not use pipes, redirects, semicolons, glob wildcards, command substitution, environment assignments, or chained commands.
- Treat prior_events.output as the concrete result of previous tool calls; do not repeat a discovery/read command when prior_events already contains the needed files, content, or test output.
- If the task asks for a summary, draft, explanation, or other generated text, put the actual generated text in finish.summary after grounding it in prior tool output.
- Continue calling tools until task acceptance criteria have concrete evidence.
- Finish only when the task is implemented and enough evidence exists for tester/reviewer.
- Do not ask the user whether to continue to a later planned task; finish the current task when its acceptance criteria are met and the runtime scheduler will run the next task.
- Do not ask the user for permission to read, patch, or test workspace files; tool runtime handles policy and confirmation.
- Do not ask the user to inspect a file you wrote; use file.read when inspection is necessary.
- Use ask_user only when required information is absent from the workspace/tools or when the task cannot continue without external human input.
- Do not include markdown or prose outside JSON.`

func agentActionSchema(specs []tools.Spec) map[string]any {
	toolNames := make([]string, 0, len(specs))
	for _, spec := range specs {
		toolNames = append(toolNames, spec.Name)
	}
	return map[string]any{
		"type": "object",
		"required": []string{
			"action",
			"tool",
			"args",
			"reason",
			"summary",
			"question",
		},
		"additionalProperties": false,
		"properties": map[string]any{
			"action":   map[string]any{"type": "string"},
			"tool":     map[string]any{"type": "string", "enum": append([]string{""}, toolNames...)},
			"args":     map[string]any{"type": "object"},
			"reason":   map[string]any{"type": "string"},
			"summary":  map[string]any{"type": "string"},
			"question": map[string]any{"type": "string"},
		},
	}
}

func providerTools(specs []tools.Spec) []provider.ChatTool {
	out := make([]provider.ChatTool, 0, len(specs))
	for _, spec := range specs {
		out = append(out, provider.ChatTool{
			Name:        spec.Name,
			Description: spec.Description,
			InputSchema: spec.InputSchema,
		})
	}
	return out
}
