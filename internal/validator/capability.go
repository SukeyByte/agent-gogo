package validator

import (
	"context"
	"fmt"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/capability"
	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/textutil"
)

type CapabilityTaskValidator struct {
	inner    TaskValidator
	registry *capability.Registry
	policy   capability.Policy
}

func NewCapabilityTaskValidator(inner TaskValidator, registry *capability.Registry, policy capability.Policy) *CapabilityTaskValidator {
	if inner == nil {
		inner = NewMinimalTaskValidator()
	}
	return &CapabilityTaskValidator{inner: inner, registry: registry, policy: policy}
}

func (v *CapabilityTaskValidator) ValidateTask(ctx context.Context, task domain.Task) error {
	if err := v.inner.ValidateTask(ctx, task); err != nil {
		return err
	}
	required := textutil.SortedUniqueStrings(task.RequiredCapabilities)
	if len(required) == 0 {
		required = InferRequiredCapabilities(task)
	}
	required = prunePassiveTaskCapabilities(task, required)
	if len(required) == 0 || v.registry == nil {
		return nil
	}
	availability, err := v.registry.CheckAvailability(ctx, capability.AvailabilityRequest{
		RequiredCapabilities: required,
		Policy:               v.policy,
	})
	if err != nil {
		return err
	}
	if availability.Available {
		return nil
	}
	return fmt.Errorf("task capability unavailable: required=%v missing=%v blocked=%v", required, availability.MissingCapabilities, availability.BlockedCapabilities)
}

func InferRequiredCapabilities(task domain.Task) []string {
	text := strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
	var required []string
	add := func(capabilityName string, markers ...string) {
		for _, marker := range markers {
			if strings.Contains(text, strings.ToLower(marker)) {
				required = append(required, capabilityName)
				return
			}
		}
	}
	add("browser", "http://", "https://", "web page", "webpage", "browser", "dom", "网页", "页面", "浏览器")
	add("read", "read file", "file content", "inspect file", "source file", "读取文件", "查看文件", "文件内容", "源文件")
	add("write", "write file", "create file", "document", "artifact", "patch", "modify", "fix", "修改", "修复", "写入", "撰写")
	add("execute", "go test", "run tests", "test.run", "shell.run", "execute command", "运行测试", "执行")
	add("verify", "go test", "tests pass", "passing tests", "validate", "verify", "验证", "通过")
	add("inspect_changes", "git diff", "git status", "diff", "查看变更")
	add("memory", "memory", "remember", "记忆", "保存经验")
	return textutil.SortedUniqueStrings(required)
}

func prunePassiveTaskCapabilities(task domain.Task, required []string) []string {
	if len(required) == 0 {
		return nil
	}
	text := taskCapabilityText(task)
	readOnly := hasAny(text, "不修改", "不改", "不写入", "不写文件", "只读", "只读取", "read-only", "do not modify", "do not write", "without modifying")
	noShell := hasAny(text, "不运行命令", "不执行命令", "不要运行命令", "不要执行命令", "不运行 shell", "不运行shell", "不要运行 shell", "不要运行shell", "no shell", "do not run shell", "do not run commands", "without running commands")
	needsExecution := hasAny(text, "go test", "npm test", "npm run", "run test", "run tests", "shell.run", "execute command", "运行测试", "跑测试", "执行命令", "运行命令")
	needsVerification := hasAny(text, "go test", "npm test", "npm run", "test pass", "tests pass", "测试通过", "跑测试", "git diff", "git status", "diff")
	hasPassiveEvidence := containsCapability(required, "read") || containsCapability(required, "write") || containsCapability(required, "browser") || containsCapability(required, "create_artifact") || containsCapability(required, "inspect") || containsCapability(required, "inspect_changes")
	var pruned []string
	for _, capabilityName := range required {
		switch strings.ToLower(strings.TrimSpace(capabilityName)) {
		case "execute":
			if needsExecution && !noShell {
				pruned = append(pruned, capabilityName)
			}
		case "verify":
			if needsVerification || (!readOnly && !noShell && !hasPassiveEvidence) {
				pruned = append(pruned, capabilityName)
			}
		case "write", "create_artifact", "inspect_changes":
			if !readOnly {
				pruned = append(pruned, capabilityName)
			}
		default:
			pruned = append(pruned, capabilityName)
		}
	}
	return textutil.SortedUniqueStrings(pruned)
}

func containsCapability(values []string, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == want {
			return true
		}
	}
	return false
}

func taskCapabilityText(task domain.Task) string {
	return strings.ToLower(strings.Join(append([]string{task.Title, task.Description}, task.AcceptanceCriteria...), " "))
}

func hasAny(text string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}
