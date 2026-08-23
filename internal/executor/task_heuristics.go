package executor

import (
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/domain"
)

func finishEvidenceReady(task domain.Task, events []actionEvent) (bool, string) {
	hasSuccessfulToolEvidence := false
	hasSourceRead := false
	hasBrowserRead := false
	hasTestRun := false
	hasPassingTest := false
	hasResearchEvidence := false
	for _, event := range events {
		switch event.State {
		case "succeeded", "changed", "verified", "observed":
			if event.Tool != "" {
				hasSuccessfulToolEvidence = true
			}
			if isResearchEvidenceTool(event.Tool) {
				hasResearchEvidence = true
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
	if taskNeedsBrowserRead(task) && !hasBrowserRead {
		return false, "finish requires browser evidence for this task"
	}
	if reason := browserInteractionMissingReason(task, events); reason != "" {
		return false, reason
	}
	if taskNeedsResearchEvidence(task) && hasResearchEvidence {
		return true, ""
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
		if taskNeedsBrowserRead(task) && !taskNeedsGeneratedText(task) && hasBrowserRead && browserInteractionMissingReason(task, events) == "" && strings.HasPrefix(event.Tool, "browser.") && event.State == "observed" {
			return "browser evidence captured for this web-reading task: " + firstNonEmpty(event.Summary, event.Output), true
		}
		if taskNeedsReadOnly(task) && event.Tool == "file.read" && event.State == "succeeded" {
			return "file content captured for this read-only task: " + firstNonEmpty(event.Summary, event.Output), true
		}
		if taskNeedsEvidenceReport(task) && hasSourceRead && hasBrowserRead && event.Tool == "file.read" && event.State == "succeeded" {
			return "browser evidence report captured for this task: " + firstNonEmpty(event.Summary, event.Output), true
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

func taskNeedsEvidenceReport(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	if browserInteractionMissingReason(task, nil) != "" {
		return false
	}
	return containsAny(text, []string{
		"browser evidence",
		"browser status",
		"final status",
		"compile and report",
		"report browser",
		"evidence and final",
		"浏览器证据",
		"瀏覽器證據",
		"最终状态",
		"最終狀態",
		"汇报浏览器",
		"匯報瀏覽器",
	})
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

func browserInteractionMissingReason(task domain.Task, events []actionEvent) string {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	needsInput := containsAny(text, []string{"type ", "input", "enter ", "fill ", "输入", "填入", "填写"})
	needsClick := containsAny(text, []string{"click", "button", "点击", "按钮"})
	needsWait := containsAny(text, []string{"wait", "appear", "appears", "出现", "等待"})
	if !needsInput && !needsClick && !needsWait {
		return ""
	}
	hasInput := false
	hasClick := false
	hasWait := false
	expected := expectedBrowserText(text)
	for _, event := range events {
		if !browserEventSucceeded(event) {
			continue
		}
		switch event.Tool {
		case "browser.input", "browser.type":
			hasInput = true
		case "browser.click":
			hasClick = true
		case "browser.wait":
			hasWait = true
		}
		if expected != "" && strings.Contains(strings.ToLower(event.Output), expected) {
			hasWait = true
		}
	}
	if needsInput && !hasInput {
		return "finish requires browser.input or browser.type evidence for this task"
	}
	if needsClick && !hasClick {
		return "finish requires browser.click evidence for this task"
	}
	if needsWait && !hasWait {
		return "finish requires browser.wait evidence or observed target text for this task"
	}
	return ""
}

func browserEventSucceeded(event actionEvent) bool {
	return strings.HasPrefix(event.Tool, "browser.") && (event.State == "observed" || event.State == "succeeded" || event.State == "changed" || event.State == "verified")
}

func containsAny(text string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func expectedBrowserText(text string) string {
	if idx := strings.Index(text, "done:"); idx >= 0 {
		end := idx
		for end < len(text) {
			ch := text[end]
			if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == ':' || ch == '-' || ch == '_' {
				end++
				continue
			}
			break
		}
		return strings.TrimSpace(text[idx:end])
	}
	return ""
}

func taskNeedsGeneratedText(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	for _, marker := range []string{
		"summarize",
		"summary",
		"write a summary",
		"write",
		"draft",
		"compose",
		"outline",
		"plan",
		"chapter",
		"总结",
		"總結",
		"概括",
		"规划",
		"規劃",
		"大纲",
		"大綱",
		"创作",
		"創作",
		"写作",
		"寫作",
		"章节",
		"章節",
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

func taskNeedsResearchEvidence(task domain.Task) bool {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	for _, marker := range []string{
		"研究上下文",
		"可用资料",
		"context-gathering",
		"context gathering",
		"research",
		"grounding",
		"收集完成任务所需的事实",
		"必要资料",
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
