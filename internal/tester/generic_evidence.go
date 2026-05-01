package tester

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/domain"
)

type GenericEvidenceStore interface {
	TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error)
	CreateTestResult(ctx context.Context, result domain.TestResult) (domain.TestResult, error)
	ListObservationsByAttempt(ctx context.Context, attemptID string) ([]domain.Observation, error)
	ListToolCallsByAttempt(ctx context.Context, attemptID string) ([]domain.ToolCall, error)
}

type GenericEvidenceTester struct {
	store GenericEvidenceStore
}

func NewGenericEvidenceTester(store GenericEvidenceStore) *GenericEvidenceTester {
	return &GenericEvidenceTester{store: store}
}

func (t *GenericEvidenceTester) Test(ctx context.Context, task domain.Task, attempt domain.TaskAttempt) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	testingTask, err := t.store.TransitionTask(ctx, task.ID, domain.TaskStatusTesting, "generic evidence tester started")
	if err != nil {
		return Result{}, err
	}
	observations, err := t.store.ListObservationsByAttempt(ctx, attempt.ID)
	if err != nil {
		return Result{}, err
	}
	calls, err := t.store.ListToolCallsByAttempt(ctx, attempt.ID)
	if err != nil {
		return Result{}, err
	}
	if reason := genericEvidenceFailure(task, observations, calls); reason != "" {
		result, createErr := t.store.CreateTestResult(ctx, domain.TestResult{
			AttemptID: attempt.ID,
			Name:      "generic-evidence",
			Status:    domain.TestStatusFailed,
			Output:    reason,
		})
		if createErr != nil {
			return Result{}, createErr
		}
		failedTask, transitionErr := t.store.TransitionTask(ctx, testingTask.ID, domain.TaskStatusFailed, reason)
		if transitionErr != nil {
			return Result{}, transitionErr
		}
		return Result{Task: failedTask, TestResult: result}, errors.New(reason)
	}
	output := fmt.Sprintf("generic evidence passed: observations=%d tool_calls=%d", len(observations), len(calls))
	result, err := t.store.CreateTestResult(ctx, domain.TestResult{
		AttemptID: attempt.ID,
		Name:      "generic-evidence",
		Status:    domain.TestStatusPassed,
		Output:    output,
	})
	if err != nil {
		return Result{}, err
	}
	reviewingTask, err := t.store.TransitionTask(ctx, testingTask.ID, domain.TaskStatusReviewing, "generic evidence tester passed")
	if err != nil {
		return Result{}, err
	}
	return Result{Task: reviewingTask, TestResult: result}, nil
}

func genericEvidenceFailure(task domain.Task, observations []domain.Observation, calls []domain.ToolCall) string {
	hasFinish := false
	hasStateEvidence := false
	hasTestRun := false
	hasTestsPassed := false
	hasResearchToolEvidence := false
	successfulCalls := 0
	for _, observation := range observations {
		switch observation.Type {
		case "agent.finish":
			hasFinish = strings.TrimSpace(observation.Summary) != ""
		case "state.file_changed", "state.artifact_written", "state.memory_persisted", "state.tests_passed", "state.command_passed", "state.repository_observed", "state.browser_observed", "state.tool_succeeded":
			hasStateEvidence = strings.TrimSpace(observation.Summary) != ""
		}
		if observation.Type == "state.tests_passed" {
			hasTestRun = true
			hasTestsPassed = true
		}
		if observation.Type == "state.tests_failed" && taskRequiresDiagnosticTestRun(task) {
			hasTestRun = true
			hasStateEvidence = true
		}
	}
	for _, call := range calls {
		if call.Status == domain.ToolCallStatusSucceeded {
			successfulCalls++
			if isResearchEvidenceTool(call.Name) {
				hasResearchToolEvidence = true
			}
		}
		if call.Name == "test.run" {
			hasTestRun = true
			if call.Status == domain.ToolCallStatusSucceeded {
				hasTestsPassed = true
			}
		}
	}
	if len(calls) > 0 && successfulCalls == 0 && !(taskRequiresDiagnosticTestRun(task) && hasTestRun) {
		return "no successful tool calls recorded"
	}
	if !hasFinish {
		return "missing agent.finish observation"
	}
	if !hasStateEvidence {
		return "missing interpreted state evidence from tool execution"
	}
	if taskRequiresPassingTests(task) && !hasTestsPassed {
		return "task acceptance requires tests, but no passing test.run evidence exists"
	}
	if taskRequiresDiagnosticTestRun(task) && !hasTestRun {
		return "task acceptance requires test.run evidence"
	}
	if taskRequiresResearchEvidence(task) && !hasResearchToolEvidence {
		return "research task requires successful discovery tool evidence"
	}
	if reason := requiredFileEvidenceFailure(task, calls); reason != "" {
		return reason
	}
	return ""
}

func requiredFileEvidenceFailure(task domain.Task, calls []domain.ToolCall) string {
	required := requiredFileNames(task)
	if len(required) < 2 {
		return ""
	}
	written := map[string]struct{}{}
	for _, call := range calls {
		if call.Status != domain.ToolCallStatusSucceeded {
			continue
		}
		switch call.Name {
		case "file.write", "artifact.write", "document.write":
			for _, path := range callPaths(call) {
				if base := filepath.Base(path); base != "." && base != "/" && strings.TrimSpace(base) != "" {
					written[base] = struct{}{}
				}
			}
		}
	}
	var missing []string
	for _, name := range required {
		if _, ok := written[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return ""
	}
	return "task requires file artifact(s) not written in this attempt: " + strings.Join(missing, ", ")
}

func requiredFileNames(task domain.Task) []string {
	text := strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " ")
	seen := map[string]struct{}{}
	var out []string
	for _, field := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ' ' || r == '\n' || r == '\t' || r == ',' || r == '，' || r == '、' || r == ';' || r == '；' || r == ':' || r == '：' || r == '(' || r == ')' || r == '[' || r == ']' || r == '"' || r == '\''
	}) {
		field = strings.TrimSpace(field)
		lower := strings.ToLower(field)
		if !(strings.HasSuffix(lower, ".html") || strings.HasSuffix(lower, ".css") || strings.HasSuffix(lower, ".js") || strings.HasSuffix(lower, ".md")) {
			continue
		}
		name := filepath.Base(field)
		if name == "." || strings.TrimSpace(name) == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func callPaths(call domain.ToolCall) []string {
	var paths []string
	for _, raw := range []string{call.OutputJSON, call.InputJSON} {
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			continue
		}
		for _, key := range []string{"path", "artifact_ref"} {
			if value, _ := payload[key].(string); strings.TrimSpace(value) != "" {
				paths = append(paths, value)
			}
		}
	}
	return paths
}

func taskRequiresResearchEvidence(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	for _, marker := range []string{
		"研究上下文",
		"可用资料",
		"收集完成任务所需的事实",
		"context-gathering",
		"context gathering",
		"research",
		"grounding",
		"读取 readme",
		"获取当前",
		"读取项目",
	} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func isResearchEvidenceTool(name string) bool {
	switch strings.TrimSpace(name) {
	case "code.index", "code.search", "code.symbols", "file.read", "browser.open", "browser.extract", "browser.dom_summary", "git.status", "git.diff":
		return true
	default:
		return false
	}
}

func taskRequiresPassingTests(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	if strings.Contains(text, "go test") && (strings.Contains(text, "pass") || strings.Contains(text, "ok") || strings.Contains(text, "通过") || strings.Contains(text, "退出码0") || strings.Contains(text, "status 0")) {
		return true
	}
	for _, marker := range []string{
		"tests pass",
		"tests passed",
		"test passed",
		"passing tests",
		"all tests pass",
		"all tests passed",
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

func taskRequiresDiagnosticTestRun(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	if taskRequiresPassingTests(task) {
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
