package planner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/llmjson"
	"github.com/SukeyByte/agent-gogo/internal/prompts"
	"github.com/SukeyByte/agent-gogo/internal/provider"
	"github.com/SukeyByte/agent-gogo/internal/textutil"
)

type LLMPlanner struct {
	llm   provider.LLMProvider
	model string
}

func NewLLMPlanner(llm provider.LLMProvider, model string) *LLMPlanner {
	return &LLMPlanner{llm: llm, model: model}
}

func (p *LLMPlanner) PlanProject(ctx context.Context, req PlanRequest) ([]domain.Task, error) {
	if p.llm == nil {
		return nil, errors.New("llm provider is required")
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	var output plannerOutput
	if err := llmjson.ChatObject(ctx, llmjson.Request{
		LLM:        p.llm,
		Model:      p.model,
		System:     plannerSystemPrompt,
		User:       string(payload),
		SchemaName: "project_plan",
		Schema:     plannerOutputSchema(),
		Metadata:   map[string]string{"stage": "planner.plan"},
		MaxRepairs: 1,
	}, &output); err != nil {
		return nil, err
	}
	if len(output.Tasks) == 0 {
		return nil, errors.New("planner returned no tasks")
	}
	if maxTasks := maxTasksForRequest(req); len(output.Tasks) > maxTasks {
		return nil, fmt.Errorf("planner returned %d tasks, above max %d for this request", len(output.Tasks), maxTasks)
	}
	output.Tasks = ensureResearchAndReflectionTasks(req, output.Tasks)

	tasks := make([]domain.Task, 0, len(output.Tasks))
	for _, planned := range output.Tasks {
		title := strings.TrimSpace(planned.Title)
		if title == "" {
			return nil, errors.New("planner task title is required")
		}
		criteria := textutil.SortedUniqueStrings(planned.Acceptance)
		if len(criteria) == 0 {
			return nil, errors.New("planner task acceptance criteria are required")
		}
		description := strings.TrimSpace(planned.Goal)
		if description == "" {
			description = strings.TrimSpace(planned.Description)
		}
		tasks = append(tasks, domain.Task{
			ProjectID:            req.Project.ID,
			Title:                title,
			Description:          description,
			Phase:                strings.TrimSpace(planned.Phase),
			Status:               domain.TaskStatusDraft,
			AcceptanceCriteria:   criteria,
			RequiredCapabilities: plannedRequiredCapabilities(planned),
			DependsOn:            textutil.SortedUniqueStrings(planned.DependsOn),
		})
	}
	return tasks, nil
}

type plannerOutput struct {
	Phases []plannedPhase `json:"phases"`
	Tasks  []plannedTask  `json:"tasks"`
}

type plannedPhase struct {
	Title       string `json:"title"`
	Goal        string `json:"goal"`
	Description string `json:"description"`
}

type plannedTask struct {
	Phase                string   `json:"phase"`
	Title                string   `json:"title"`
	Goal                 string   `json:"goal"`
	Description          string   `json:"description"`
	Type                 string   `json:"type"`
	DependsOn            []string `json:"depends_on"`
	Acceptance           []string `json:"acceptance"`
	RequiredCapabilities []string `json:"required_capabilities"`
}

var plannerSystemPrompt = prompts.Text("planner")

func ensureResearchAndReflectionTasks(req PlanRequest, planned []plannedTask) []plannedTask {
	if !needsResearchAndReflection(req) {
		return planned
	}
	researchMarkers := []string{"研究", "調研", "调研", "research", "context", "gather", "收集", "获取", "取得", "读取", "搜索"}
	reflectionMarkers := []string{"反思", "reflection", "review plan", "验收口径", "驗收口徑", "decomposition", "acceptance criteria"}
	researchTitle := "研究上下文与可用资料"
	reflectionTitle := "反思任务拆解与验收口径"
	hasResearch := hasTaskMatching(planned, researchMarkers)
	hasReflection := hasTaskMatching(planned, reflectionMarkers)
	if hasResearch && hasReflection {
		return planned
	}
	prefix := []plannedTask{}
	researchDependency := firstTaskTitleMatching(planned, researchMarkers)
	if !hasResearch {
		prefix = append(prefix, plannedTask{
			Phase:                "发现与反思",
			Title:                "研究上下文与可用资料",
			Goal:                 "先读取、搜索或浏览必要资料，确认任务事实、约束、现有实现和可用工具，不直接进入实现。",
			Type:                 "runtime",
			RequiredCapabilities: []string{"inspect", "read"},
			Acceptance: []string{
				"已用可用工具收集完成任务所需的事实和上下文",
				"已记录关键约束、未知项和可用工具证据",
				"没有在缺少资料的情况下直接做实现假设",
			},
		})
		researchDependency = researchTitle
	}
	finalDependency := firstTaskTitleMatching(planned, reflectionMarkers)
	if !hasReflection {
		depends := []string{}
		if researchDependency != "" {
			depends = append(depends, researchDependency)
		}
		prefix = append(prefix, plannedTask{
			Phase:                "发现与反思",
			Title:                reflectionTitle,
			Goal:                 "基于研究结果反思任务拆解是否站得住脚，明确最小可执行任务、风险和机械验收标准。",
			Type:                 "general",
			DependsOn:            depends,
			RequiredCapabilities: []string{"verify"},
			Acceptance: []string{
				"已说明当前任务拆解为什么足以达成用户目标",
				"已识别关键风险、缺失信息和需要重规划的条件",
				"已明确后续实现任务的机械验收标准",
			},
		})
		finalDependency = reflectionTitle
	}
	for i := range planned {
		if len(planned[i].DependsOn) != 0 {
			continue
		}
		if taskMatches(planned[i], researchMarkers) {
			continue
		}
		if taskMatches(planned[i], reflectionMarkers) {
			if researchDependency != "" {
				planned[i].DependsOn = appendIfMissing(planned[i].DependsOn, researchDependency)
			}
			continue
		}
		if finalDependency != "" {
			planned[i].DependsOn = []string{finalDependency}
		}
	}
	if hasResearch && !hasReflection && len(prefix) > 0 {
		insertAfter := firstTaskIndexMatching(planned, researchMarkers)
		if insertAfter >= 0 {
			out := make([]plannedTask, 0, len(planned)+len(prefix))
			out = append(out, planned[:insertAfter+1]...)
			out = append(out, prefix...)
			out = append(out, planned[insertAfter+1:]...)
			return out
		}
	}
	return append(prefix, planned...)
}

func needsResearchAndReflection(req PlanRequest) bool {
	fields := []string{
		req.UserInput,
		req.Project.Goal,
		req.IntentProfile.TaskType,
		req.IntentProfile.Complexity,
	}
	fields = append(fields, req.IntentProfile.Domains...)
	fields = append(fields, req.IntentProfile.RequiredCapabilities...)
	text := strings.ToLower(strings.Join(fields, " "))
	if req.ChainDecision.Level == "L3" {
		return true
	}
	for _, marker := range []string{"medium", "high", "complex", "code", "web", "browser", "debug", "fix", "修复", "调试", "研究", "网页"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func maxTasksForRequest(req PlanRequest) int {
	text := strings.ToLower(strings.Join([]string{req.IntentProfile.Complexity, req.IntentProfile.TaskType, req.Project.Goal, req.UserInput}, " "))
	if req.ChainDecision.Level == "L3" || strings.Contains(text, "high") || strings.Contains(text, "complex") || strings.Contains(text, "project") || strings.Contains(text, "项目") {
		return 15
	}
	if strings.Contains(text, "medium") || strings.Contains(text, "planned") || strings.Contains(text, "中等") {
		return 7
	}
	return 3
}

func hasTaskMatching(tasks []plannedTask, markers []string) bool {
	for _, task := range tasks {
		if taskMatches(task, markers) {
			return true
		}
	}
	return false
}

func firstTaskTitleMatching(tasks []plannedTask, markers []string) string {
	for _, task := range tasks {
		if taskMatches(task, markers) && strings.TrimSpace(task.Title) != "" {
			return task.Title
		}
	}
	return ""
}

func firstTaskIndexMatching(tasks []plannedTask, markers []string) int {
	for i, task := range tasks {
		if taskMatches(task, markers) {
			return i
		}
	}
	return -1
}

func taskMatches(task plannedTask, markers []string) bool {
	text := strings.ToLower(task.Title + " " + task.Goal + " " + task.Description + " " + strings.Join(task.Acceptance, " "))
	for _, marker := range markers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func appendIfMissing(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func plannedRequiredCapabilities(task plannedTask) []string {
	return textutil.SortedUniqueStrings(task.RequiredCapabilities)
}

func plannerOutputSchema() map[string]any {
	phaseSchema := map[string]any{
		"type": "object",
		"required": []string{
			"title",
			"goal",
			"description",
		},
		"additionalProperties": false,
		"properties": map[string]any{
			"title":       map[string]any{"type": "string"},
			"goal":        map[string]any{"type": "string"},
			"description": map[string]any{"type": "string"},
		},
	}
	taskSchema := map[string]any{
		"type": "object",
		"required": []string{
			"phase",
			"title",
			"goal",
			"description",
			"type",
			"depends_on",
			"acceptance",
			"required_capabilities",
		},
		"additionalProperties": false,
		"properties": map[string]any{
			"phase":                 map[string]any{"type": "string"},
			"title":                 map[string]any{"type": "string"},
			"goal":                  map[string]any{"type": "string"},
			"description":           map[string]any{"type": "string"},
			"type":                  map[string]any{"type": "string", "enum": []string{"code", "browser", "document", "runtime", "general"}},
			"depends_on":            arraySchema("string"),
			"acceptance":            arraySchema("string"),
			"required_capabilities": arraySchema("string"),
		},
	}
	return map[string]any{
		"type":                 "object",
		"required":             []string{"phases", "tasks"},
		"additionalProperties": false,
		"properties": map[string]any{
			"phases": map[string]any{"type": "array", "items": phaseSchema},
			"tasks":  map[string]any{"type": "array", "items": taskSchema},
		},
	}
}

func arraySchema(itemType string) map[string]any {
	return map[string]any{"type": "array", "items": map[string]any{"type": itemType}}
}
