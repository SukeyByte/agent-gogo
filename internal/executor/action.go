package executor

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/tools"
)

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

var taskPathPattern = regexp.MustCompile("(?:^|[\\s\"'(<（])((?:\\.?/)?(?:artifacts|docs|web|internal|cmd|pkg|data|logs|testdata|tmp|public|src)/[A-Za-z0-9._/@+=-]+)")

func normalizeActionArgsForTask(task domain.Task, tool string, args map[string]any) map[string]any {
	if !toolUsesWorkspacePath(tool) || len(args) == 0 {
		return args
	}
	actual, ok := args["path"].(string)
	if !ok {
		return args
	}
	actual = strings.TrimSpace(actual)
	if actual == "" {
		return args
	}
	normalizedActual := normalizeWorkspacePath(actual)
	expected := matchingTaskPath(task, normalizedActual)
	if expected == "" && normalizedActual != actual {
		expected = normalizedActual
	}
	if expected == "" || expected == actual {
		return args
	}
	next := make(map[string]any, len(args))
	for key, value := range args {
		next[key] = value
	}
	next["path"] = expected
	return next
}

func toolUsesWorkspacePath(tool string) bool {
	switch tool {
	case "file.read", "file.write", "file.patch", "document.write", "artifact.write":
		return true
	default:
		return false
	}
}

func matchingTaskPath(task domain.Task, actual string) string {
	paths := taskReferencedPaths(task)
	if len(paths) == 0 {
		return ""
	}
	actual = normalizeWorkspacePath(actual)
	actualBase := path.Base(actual)
	for _, candidate := range paths {
		if candidate == actual {
			return ""
		}
		if path.Base(candidate) == actualBase {
			return candidate
		}
	}
	if len(paths) == 1 && !strings.Contains(actual, "/") {
		return paths[0]
	}
	return ""
}

func taskReferencedPaths(task domain.Task) []string {
	texts := []string{task.Title, task.Description}
	texts = append(texts, task.AcceptanceCriteria...)
	seen := map[string]bool{}
	paths := []string{}
	for _, text := range texts {
		for _, match := range taskPathPattern.FindAllStringSubmatch(text, -1) {
			if len(match) < 2 {
				continue
			}
			candidate := normalizeWorkspacePath(match[1])
			if candidate == "" || seen[candidate] {
				continue
			}
			seen[candidate] = true
			paths = append(paths, candidate)
		}
	}
	return paths
}

func normalizeWorkspacePath(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimRight(value, ".,;:，。；：)")
	value = strings.TrimPrefix(value, "./")
	for _, prefix := range []string{"artifacts/", "docs/", "web/", "internal/", "cmd/", "pkg/", "data/", "logs/", "testdata/", "tmp/", "public/", "src/"} {
		if idx := strings.Index(value, prefix); idx >= 0 {
			return value[idx:]
		}
	}
	return value
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
		"open_browser":    "browser.open",
		"browser_click":   "browser.click",
		"click_browser":   "browser.click",
		"browser_type":    "browser.type",
		"type_text":       "browser.type",
		"browser_input":   "browser.input",
		"input_text":      "browser.input",
		"browser_wait":    "browser.wait",
		"wait_browser":    "browser.wait",
		"browser_extract": "browser.extract",
		"extract_text":    "browser.extract",
		"dom_summary":     "browser.dom_summary",
		"screenshot":      "browser.screenshot",
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
