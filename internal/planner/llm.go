package planner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/chain"
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
	output.Tasks = pruneResearchAndReflectionForSimpleBrowser(req, output.Tasks)
	output.Tasks = pruneResearchAndReflectionForContentGeneration(req, output.Tasks)
	if maxTasks := maxTasksForRequest(req); len(output.Tasks) > maxTasks {
		return nil, fmt.Errorf("planner returned %d tasks, above max %d for this request", len(output.Tasks), maxTasks)
	}
	output.Tasks = ensureResearchAndReflectionTasks(req, output.Tasks)
	output.Tasks = ensureL3MinimumDecomposition(req, output.Tasks)
	output.Tasks = normalizePlannedDependencies(output.Tasks)

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

func normalizePlannedDependencies(planned []plannedTask) []plannedTask {
	if len(planned) == 0 {
		return planned
	}
	out := append([]plannedTask(nil), planned...)
	titleSet := map[string]struct{}{}
	for _, task := range out {
		if title := strings.TrimSpace(task.Title); title != "" {
			titleSet[title] = struct{}{}
		}
	}
	researchTitle := firstTaskTitleMatching(out, []string{"研究", "調研", "调研", "research", "context", "gather", "收集", "获取", "取得", "读取", "搜索"})
	reflectionTitle := firstTaskTitleMatching(out, []string{"反思", "reflection", "review plan", "验收口径", "驗收口徑", "decomposition", "acceptance criteria"})
	finalMarkers := []string{"最终", "最終", "验收", "驗收", "验证", "驗證", "检查", "檢查", "自检", "自檢", "自查", "复核", "覆核", "完整性", "汇报", "匯報", "总结", "總結", "汇总", "彙總", "final", "verify", "validate", "validation", "self-check", "self check", "selfcheck", "correctness", "completeness", "report", "summary"}
	taskByTitle := map[string]plannedTask{}
	finalByTitle := map[string]bool{}
	for _, task := range out {
		title := strings.TrimSpace(task.Title)
		if title == "" {
			continue
		}
		taskByTitle[title] = task
		finalByTitle[title] = title != researchTitle && title != reflectionTitle && taskMatches(task, finalMarkers)
	}
	for i := range out {
		title := strings.TrimSpace(out[i].Title)
		isFinalTask := finalByTitle[title]
		deps := make([]string, 0, len(out[i].DependsOn)+1)
		seen := map[string]struct{}{}
		addDep := func(dep string) {
			dep = strings.TrimSpace(dep)
			if dep == "" || dep == title {
				return
			}
			if _, ok := titleSet[dep]; !ok {
				return
			}
			if !isFinalTask {
				if _, ok := taskByTitle[dep]; ok && finalByTitle[dep] {
					return
				}
			}
			if _, ok := seen[dep]; ok {
				return
			}
			seen[dep] = struct{}{}
			deps = append(deps, dep)
		}
		switch title {
		case researchTitle:
			// Research is the root discovery task. Let it run first even if the model
			// accidentally attached downstream dependencies to it.
		case reflectionTitle:
			if researchTitle != "" && researchTitle != reflectionTitle {
				addDep(researchTitle)
			}
		default:
			for _, dep := range out[i].DependsOn {
				addDep(dep)
			}
			if reflectionTitle != "" {
				addDep(reflectionTitle)
			} else if researchTitle != "" {
				addDep(researchTitle)
			}
			if isFinalTask {
				for _, candidate := range out {
					if strings.TrimSpace(candidate.Title) == title || strings.TrimSpace(candidate.Title) == "" {
						continue
					}
					if taskMatches(candidate, finalMarkers) {
						continue
					}
					addDep(candidate.Title)
				}
			}
		}
		out[i].DependsOn = deps
	}
	return pruneCyclicDependencies(out)
}

func pruneCyclicDependencies(planned []plannedTask) []plannedTask {
	out := append([]plannedTask(nil), planned...)
	depsByTitle := map[string][]string{}
	for _, task := range out {
		depsByTitle[task.Title] = append([]string(nil), task.DependsOn...)
	}
	for i := range out {
		title := strings.TrimSpace(out[i].Title)
		kept := out[i].DependsOn[:0]
		for _, dep := range out[i].DependsOn {
			depsByTitle[title] = kept
			if reachesDependency(depsByTitle, dep, title, map[string]bool{}) {
				continue
			}
			kept = append(kept, dep)
		}
		out[i].DependsOn = kept
		depsByTitle[title] = append([]string(nil), kept...)
	}
	return out
}

func reachesDependency(depsByTitle map[string][]string, from string, target string, seen map[string]bool) bool {
	from = strings.TrimSpace(from)
	target = strings.TrimSpace(target)
	if from == "" || target == "" {
		return false
	}
	if from == target {
		return true
	}
	if seen[from] {
		return false
	}
	seen[from] = true
	for _, next := range depsByTitle[from] {
		if reachesDependency(depsByTitle, next, target, seen) {
			return true
		}
	}
	return false
}

func needsResearchAndReflection(req PlanRequest) bool {
	if isSimpleBrowserReadRequest(req) {
		return false
	}
	if isContentGenerationRequest(req) {
		return false
	}
	fields := []string{
		req.UserInput,
		req.Project.Goal,
		req.IntentProfile.TaskType,
		req.IntentProfile.Complexity,
	}
	fields = append(fields, req.IntentProfile.Domains...)
	fields = append(fields, req.IntentProfile.RequiredCapabilities...)
	text := strings.ToLower(strings.Join(fields, " "))
	if chain.IsProjectScale(req.ChainDecision) {
		return true
	}
	for _, marker := range []string{"medium", "high", "complex", "code", "web", "browser", "debug", "fix", "修复", "调试", "研究", "网页"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

// isContentGenerationRequest reports whether the request is pure content
// generation (writing, drafting, storytelling) that gains little from
// research/reflection scaffolding. The structured intent profile is checked
// first so new domains classified by the intent analyzer need no planner
// changes; the raw-text keywords are a fallback for thin profiles.
func isContentGenerationRequest(req PlanRequest) bool {
	text := strings.ToLower(strings.Join([]string{
		req.UserInput,
		req.Project.Goal,
		req.IntentProfile.TaskType,
		req.IntentProfile.Complexity,
		strings.Join(req.IntentProfile.Domains, " "),
	}, " "))
	if hasAnyText(text, "code", "debug", "browser", "http://", "https://", "网页", "浏览器", "调试", "研究", "调研") {
		return false
	}
	signal := strings.ToLower(strings.TrimSpace(req.IntentProfile.TaskType + " " + strings.Join(req.IntentProfile.Domains, " ")))
	structured := hasAnyText(signal, "writ", "creative", "content", "draft", "story", "fiction", "novel", "写作", "创作", "撰写", "故事", "小说")
	return structured || hasAnyText(text, "write", "draft", "creative writing", "story", "fiction", "novel", "chapter", "写作", "撰写", "创作", "起草", "故事", "小说", "章节", "短篇", "长篇")
}

func hasAnyText(text string, markers ...string) bool {
	for _, marker := range markers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func isSimpleBrowserReadRequest(req PlanRequest) bool {
	if req.ChainDecision.Level == chain.LevelProject || req.ChainDecision.RequiresDAG {
		return false
	}
	if req.ChainDecision.Level != "" && req.ChainDecision.Level != chain.LevelDirect && req.ChainDecision.Level != chain.LevelAssist {
		return false
	}
	if req.ChainDecision.EstimatedSteps > 6 {
		return false
	}
	text := strings.ToLower(strings.Join([]string{
		req.UserInput,
		req.Project.Goal,
		req.IntentProfile.TaskType,
		req.IntentProfile.Complexity,
		strings.Join(req.IntentProfile.Domains, " "),
	}, " "))
	if !(req.ChainDecision.NeedBrowser || strings.Contains(text, "http://") || strings.Contains(text, "https://") || strings.Contains(text, "browser") || strings.Contains(text, "网页") || strings.Contains(text, "页面")) {
		return false
	}
	for _, marker := range []string{"code", "debug", "fix", "patch", "write file", "create file", "login", "submit", "代码", "修复", "登录", "提交"} {
		if strings.Contains(text, marker) {
			return false
		}
	}
	return true
}

func pruneResearchAndReflectionForContentGeneration(req PlanRequest, planned []plannedTask) []plannedTask {
	if !isContentGenerationRequest(req) {
		return planned
	}
	pruned := make([]plannedTask, 0, len(planned))
	removed := map[string]struct{}{}
	for _, task := range planned {
		if taskMatches(task, []string{"研究", "research", "context", "反思", "reflection", "验收口径", "decomposition"}) {
			if title := strings.TrimSpace(task.Title); title != "" {
				removed[title] = struct{}{}
			}
			continue
		}
		pruned = append(pruned, task)
	}
	if len(pruned) == 0 {
		return planned
	}
	for i := range pruned {
		deps := pruned[i].DependsOn[:0]
		for _, dependency := range pruned[i].DependsOn {
			if _, ok := removed[dependency]; ok {
				continue
			}
			deps = append(deps, dependency)
		}
		pruned[i].DependsOn = deps
	}
	return pruned
}

func pruneResearchAndReflectionForSimpleBrowser(req PlanRequest, planned []plannedTask) []plannedTask {
	if !isSimpleBrowserReadRequest(req) {
		return planned
	}
	pruned := make([]plannedTask, 0, len(planned))
	removed := map[string]struct{}{}
	for _, task := range planned {
		if taskMatches(task, []string{"研究", "research", "context", "反思", "reflection", "验收口径", "decomposition"}) {
			if title := strings.TrimSpace(task.Title); title != "" {
				removed[title] = struct{}{}
			}
			continue
		}
		pruned = append(pruned, task)
	}
	if len(pruned) == 0 {
		return planned
	}
	for i := range pruned {
		deps := pruned[i].DependsOn[:0]
		for _, dependency := range pruned[i].DependsOn {
			if _, ok := removed[dependency]; ok {
				continue
			}
			deps = append(deps, dependency)
		}
		pruned[i].DependsOn = deps
	}
	return pruned
}

func ensureL3MinimumDecomposition(req PlanRequest, planned []plannedTask) []plannedTask {
	if !chain.IsProjectScale(req.ChainDecision) || len(planned) >= 4 {
		return planned
	}
	out := append([]plannedTask(nil), planned...)
	reflectionTitle := firstTaskTitleMatching(out, []string{"反思", "reflection", "验收口径", "decomposition"})
	lastTitle := lastTaskTitle(out)
	concreteCount := 0
	for _, task := range out {
		if !taskMatches(task, []string{"研究", "research", "context", "反思", "reflection", "验收口径", "decomposition"}) {
			concreteCount++
		}
	}
	if concreteCount == 0 {
		depends := []string{}
		if reflectionTitle != "" {
			depends = append(depends, reflectionTitle)
		} else if lastTitle != "" {
			depends = append(depends, lastTitle)
		}
		out = append(out, plannedTask{
			Phase:                l3FallbackPhase(out),
			Title:                "执行首个可验收切片",
			Goal:                 "把项目级目标压缩成一个可运行、可验收的最小切片并执行。",
			Description:          "不要把整个目标当成一个大任务；先完成一个有证据、可继续扩展的切片。",
			Type:                 fallbackTaskType(req),
			DependsOn:            depends,
			RequiredCapabilities: fallbackCapabilities(req),
			Acceptance: []string{
				"已完成一个最小可验收切片",
				"切片结果有工具证据或可检查产物",
				"剩余工作可继续按 DAG 推进",
			},
		})
		lastTitle = "执行首个可验收切片"
	}
	if len(out) < 4 {
		depends := []string{}
		if lastTitle != "" {
			depends = append(depends, lastTitle)
		}
		out = append(out, plannedTask{
			Phase:                l3FallbackPhase(out),
			Title:                "汇总执行状态与结果",
			Goal:                 "汇总已完成切片、当前状态、证据和下一步，确保 channel 可以收到明确结果。",
			Description:          "把运行状态和结果整理成用户可读的完成信号。",
			Type:                 "general",
			DependsOn:            depends,
			RequiredCapabilities: []string{"verify"},
			Acceptance: []string{
				"已说明哪些任务完成、哪些仍待处理",
				"已引用关键证据或产物",
				"已给出明确的 channel 汇报结果",
			},
		})
	}
	return out
}

func maxTasksForRequest(req PlanRequest) int {
	text := strings.ToLower(strings.Join([]string{req.IntentProfile.Complexity, req.IntentProfile.TaskType, req.Project.Goal, req.UserInput}, " "))
	if chain.IsProjectScale(req.ChainDecision) || strings.Contains(text, "high") || strings.Contains(text, "complex") || strings.Contains(text, "project") || strings.Contains(text, "项目") {
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

func lastTaskTitle(tasks []plannedTask) string {
	for i := len(tasks) - 1; i >= 0; i-- {
		if title := strings.TrimSpace(tasks[i].Title); title != "" {
			return title
		}
	}
	return ""
}

func l3FallbackPhase(tasks []plannedTask) string {
	for _, task := range tasks {
		if phase := strings.TrimSpace(task.Phase); phase != "" {
			return phase
		}
	}
	return "执行与汇报"
}

func fallbackTaskType(req PlanRequest) string {
	switch {
	case req.ChainDecision.NeedBrowser:
		return "browser"
	case req.ChainDecision.NeedCode:
		return "code"
	case req.ChainDecision.NeedDocs:
		return "document"
	}
	taskType := strings.ToLower(strings.TrimSpace(req.IntentProfile.TaskType))
	switch {
	case strings.Contains(taskType, "browser"), strings.Contains(taskType, "web"):
		return "browser"
	case strings.Contains(taskType, "code"):
		return "code"
	case strings.Contains(taskType, "document"), strings.Contains(taskType, "doc"):
		return "document"
	default:
		return "general"
	}
}

func fallbackCapabilities(req PlanRequest) []string {
	caps := []string{"read"}
	if req.ChainDecision.NeedBrowser || strings.Contains(strings.ToLower(req.IntentProfile.TaskType), "web") {
		caps = append(caps, "browser")
	}
	if req.ChainDecision.NeedCode || strings.Contains(strings.ToLower(req.IntentProfile.TaskType), "code") {
		caps = append(caps, "write", "verify")
	}
	if req.ChainDecision.NeedDocs || strings.Contains(strings.ToLower(req.IntentProfile.TaskType), "document") {
		caps = append(caps, "create_artifact", "verify")
	}
	return textutil.SortedUniqueStrings(caps)
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
