package executor

import (
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/domain"
)

func TestImpliedUncalledToolsBlocksFinish(t *testing.T) {
	task := domain.Task{
		Title:              "编写 docs/alpha.md 并记录笔记",
		Description:        "创建文件并调用 notes.add 记录摘要",
		AcceptanceCriteria: []string{"docs/alpha.md 已写入", "notes.add 工具被成功调用"},
	}
	tools := []string{"file.write", "file.read", "mcp.notes.add", "mcp.notes.list"}

	// Only file.write happened; notes.add is named by the task but uncalled.
	events := []actionEvent{{Step: 1, Action: "tool_call", Tool: "file.write", State: "succeeded"}}
	uncalled := impliedUncalledTools(task, events, tools)
	if len(uncalled) != 1 || uncalled[0] != "mcp.notes.add" {
		t.Fatalf("expected mcp.notes.add uncalled, got %#v", uncalled)
	}
	ok, reason := finishEvidenceReady(task, events, tools)
	if ok || !contains(reason, "notes") {
		t.Fatalf("expected blocked finish mentioning notes, got ok=%v reason=%q", ok, reason)
	}

	// After the tool was called, the gate passes.
	events = append(events, actionEvent{Step: 2, Action: "tool_call", Tool: "mcp.notes.add", State: "succeeded"})
	if uncalled := impliedUncalledTools(task, events, tools); len(uncalled) != 0 {
		t.Fatalf("expected no uncalled tools, got %#v", uncalled)
	}
}

func TestImpliedUncalledToolsIgnoresProseAndPaths(t *testing.T) {
	task := domain.Task{
		Title:              "写 docs/alpha.md",
		Description:        "内容为一句话介绍",
		AcceptanceCriteria: []string{"文件 docs/alpha.md 存在且内容为一句话"},
	}
	tools := []string{"file.write", "file.read", "notes.add"}
	events := []actionEvent{{Step: 1, Action: "tool_call", Tool: "file.write", State: "succeeded"}}
	// "alpha.md" inside the path must not be treated as a tool reference.
	if uncalled := impliedUncalledTools(task, events, tools); len(uncalled) != 0 {
		t.Fatalf("paths in prose should not block finish, got %#v", uncalled)
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
