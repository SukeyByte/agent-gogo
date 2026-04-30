package validator

import (
	"context"
	"fmt"
	"strings"

	"github.com/sukeke/agent-gogo/internal/capability"
	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/textutil"
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
