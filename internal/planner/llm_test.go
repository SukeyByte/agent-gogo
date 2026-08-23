package planner

import (
	"context"
	"strings"
	"testing"

	"github.com/SukeyByte/agent-gogo/internal/chain"
	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/intent"
	"github.com/SukeyByte/agent-gogo/internal/provider"
)

func TestLLMPlannerUsesProviderJSON(t *testing.T) {
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		if req.ResponseFormat == nil || req.ResponseFormat.Type != "json_schema" {
			t.Fatalf("expected planner to request structured json, got %#v", req.ResponseFormat)
		}
		return provider.ChatResponse{
			Model: req.Model,
			Text: `{
				"tasks":[
					{
						"title":"Scan project",
						"goal":"Identify project structure and tests",
						"type":"code",
						"depends_on":[],
						"acceptance":["project structure summarized","test command identified"]
					},
					{
						"title":"Run tests",
						"goal":"Run current test suite",
						"type":"code",
						"depends_on":["Scan project"],
						"acceptance":["tests executed","result recorded"]
					}
				]
			}`,
		}, nil
	})
	planner := NewLLMPlanner(llm, "gpt-test")
	tasks, err := planner.PlanProject(context.Background(), PlanRequest{
		Project: domain.Project{ID: "project-1", Name: "agent-gogo", Goal: "make it real"},
		ChainDecision: chain.Decision{
			Level: chain.LevelProject,
		},
		IntentProfile: intent.Profile{
			TaskType: "code",
		},
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks with research/reflection prefix, got %d", len(tasks))
	}
	if tasks[0].Title != "研究上下文与可用资料" {
		t.Fatalf("expected research task first, got %q", tasks[0].Title)
	}
	if tasks[1].Title != "反思任务拆解与验收口径" {
		t.Fatalf("expected reflection task second, got %q", tasks[1].Title)
	}
	if tasks[0].Status != domain.TaskStatusDraft {
		t.Fatalf("expected draft task, got %s", tasks[0].Status)
	}
	if len(tasks[0].AcceptanceCriteria) == 0 {
		t.Fatal("expected acceptance criteria")
	}
}

func TestEnsureResearchAndReflectionPreservesResearchBeforeReflection(t *testing.T) {
	out := ensureResearchAndReflectionTasks(PlanRequest{
		Project: domain.Project{Goal: "根据 README 写项目简介文档"},
		ChainDecision: chain.Decision{
			Level: chain.LevelProject,
		},
		IntentProfile: intent.Profile{
			TaskType:   "document",
			Complexity: "high",
		},
	}, []plannedTask{
		{
			Title:      "读取 README 文件",
			Goal:       "读取项目 README 内容",
			Type:       "runtime",
			Acceptance: []string{"成功读取 README 文件内容"},
		},
		{
			Title:      "撰写项目简介文档",
			Goal:       "根据 README 内容写简介",
			Type:       "document",
			Acceptance: []string{"文档包含项目名称"},
		},
	})
	if len(out) != 3 {
		t.Fatalf("expected research, reflection, implementation tasks, got %#v", out)
	}
	if out[0].Title != "读取 README 文件" {
		t.Fatalf("expected existing research task first, got %q", out[0].Title)
	}
	if out[1].Title != "反思任务拆解与验收口径" {
		t.Fatalf("expected injected reflection second, got %q", out[1].Title)
	}
	if len(out[1].DependsOn) != 1 || out[1].DependsOn[0] != "读取 README 文件" {
		t.Fatalf("expected reflection to depend on research, got %#v", out[1].DependsOn)
	}
	if len(out[2].DependsOn) != 1 || out[2].DependsOn[0] != "反思任务拆解与验收口径" {
		t.Fatalf("expected implementation to depend on reflection, got %#v", out[2].DependsOn)
	}
}

func TestEnsureResearchAndReflectionSkipsSimpleBrowserRead(t *testing.T) {
	out := ensureResearchAndReflectionTasks(PlanRequest{
		Project: domain.Project{Goal: "只打开 https://example.com 并总结，不修改文件"},
		ChainDecision: chain.Decision{
			Level:          chain.LevelAssist,
			NeedBrowser:    true,
			EstimatedSteps: 2,
		},
		IntentProfile: intent.Profile{
			TaskType:   "browser",
			Complexity: "simple",
		},
	}, []plannedTask{
		{
			Title:                "打开页面并总结",
			Goal:                 "打开 https://example.com 并总结页面内容",
			Type:                 "browser",
			Acceptance:           []string{"页面已总结"},
			RequiredCapabilities: []string{"browser"},
		},
	})
	if len(out) != 1 {
		t.Fatalf("expected simple browser read to avoid injected reflection task, got %#v", out)
	}
	if out[0].Title != "打开页面并总结" {
		t.Fatalf("unexpected task %q", out[0].Title)
	}
}

func TestEnsureResearchAndReflectionSkipsDirectBrowserInteraction(t *testing.T) {
	out := ensureResearchAndReflectionTasks(PlanRequest{
		Project: domain.Project{Goal: "打开 http://127.0.0.1:18766/index.html，在 Message 输入框输入 hello-browser，点击 Go，等待 done:hello-browser"},
		ChainDecision: chain.Decision{
			Level:          chain.LevelAssist,
			NeedBrowser:    true,
			EstimatedSteps: 4,
		},
		IntentProfile: intent.Profile{
			TaskType:   "browser",
			Complexity: "simple",
		},
	}, []plannedTask{
		{
			Title:                "完成浏览器交互并验证结果",
			Goal:                 "输入 hello-browser，点击 Go，并等待 done:hello-browser 出现",
			Type:                 "browser",
			Acceptance:           []string{"页面出现 done:hello-browser"},
			RequiredCapabilities: []string{"browser"},
		},
	})
	if len(out) != 1 {
		t.Fatalf("expected direct browser interaction to avoid injected research/reflection tasks, got %#v", out)
	}
}

func TestEnsureResearchAndReflectionSkipsCreativeWriting(t *testing.T) {
	req := PlanRequest{
		Project: domain.Project{Goal: "创作原创科幻故事《潮汐城回声》的三章短篇，遇到阻断请 agent 自己修复继续"},
		ChainDecision: chain.Decision{
			Level:       chain.LevelProject,
			RequiresDAG: true,
		},
		IntentProfile: intent.Profile{
			TaskType:   "document",
			Complexity: "high",
		},
	}
	out := ensureResearchAndReflectionTasks(req, []plannedTask{
		{
			Title:      "规划三章标题和大纲",
			Goal:       "规划三章故事",
			Type:       "document",
			Acceptance: []string{"大纲完成"},
		},
		{
			Title:      "写作三章正文",
			Goal:       "写入三章正文",
			Type:       "document",
			DependsOn:  []string{"规划三章标题和大纲"},
			Acceptance: []string{"正文完成"},
		},
	})
	if len(out) != 2 {
		t.Fatalf("expected no forced research/reflection for creative writing, got %#v", out)
	}
}

func TestPruneResearchAndReflectionForCreativeWriting(t *testing.T) {
	req := PlanRequest{
		Project: domain.Project{Goal: "长篇写作：三章科幻故事"},
		ChainDecision: chain.Decision{
			Level:       chain.LevelProject,
			RequiresDAG: true,
		},
		IntentProfile: intent.Profile{TaskType: "document"},
	}
	out := pruneResearchAndReflectionForContentGeneration(req, []plannedTask{
		{Title: "反思任务拆解与验收口径", Goal: "反思", Type: "general", Acceptance: []string{"反思完成"}},
		{Title: "写作三章正文", Goal: "写故事", Type: "document", DependsOn: []string{"反思任务拆解与验收口径"}, Acceptance: []string{"正文完成"}},
	})
	if len(out) != 1 || out[0].Title != "写作三章正文" {
		t.Fatalf("expected only concrete writing task, got %#v", out)
	}
	if len(out[0].DependsOn) != 0 {
		t.Fatalf("expected removed reflection dependency, got %#v", out[0].DependsOn)
	}
}

func TestNormalizePlannedDependenciesBreaksReflectionCycle(t *testing.T) {
	out := normalizePlannedDependencies([]plannedTask{
		{
			Title:      "规划故事大纲与章节标题",
			Goal:       "规划三章大纲",
			Type:       "document",
			DependsOn:  []string{"反思任务拆解与验收口径"},
			Acceptance: []string{"大纲完成"},
		},
		{
			Title:      "撰写第一章",
			Goal:       "写第一章",
			Type:       "document",
			DependsOn:  []string{"规划故事大纲与章节标题"},
			Acceptance: []string{"第一章完成"},
		},
		{
			Title:      "检查文件存在并汇报",
			Goal:       "检查文件",
			Type:       "general",
			DependsOn:  []string{"撰写第一章"},
			Acceptance: []string{"检查完成"},
		},
		{
			Title:      "反思任务拆解与验收口径",
			Goal:       "反思任务拆解",
			Type:       "general",
			DependsOn:  []string{"检查文件存在并汇报"},
			Acceptance: []string{"反思完成"},
		},
	})
	if hasDependency(out, "反思任务拆解与验收口径", "检查文件存在并汇报") {
		t.Fatalf("reflection task must not depend on downstream final check: %#v", out)
	}
	if !hasDependency(out, "规划故事大纲与章节标题", "反思任务拆解与验收口径") {
		t.Fatalf("first concrete task should depend on reflection: %#v", out)
	}
	if hasDependencyCycle(out) {
		t.Fatalf("expected normalized plan to be acyclic: %#v", out)
	}
}

func TestNormalizePlannedDependenciesKeepsFinalVerificationAfterWriting(t *testing.T) {
	out := normalizePlannedDependencies([]plannedTask{
		{
			Title:      "制定故事大纲与创作计划",
			Goal:       "为《潮汐城回声》创作一个包含三章的故事大纲，每章约500字，确定章节标题和核心情节。",
			Type:       "document",
			Acceptance: []string{"故事大纲包含三章，每章有明确的标题和核心情节描述", "每章字数规划在500字左右", "目标文件路径已确认"},
		},
		{
			Title:      "撰写第一章",
			Goal:       "完成第一章的正文写作，约500字，并写入目标文件。",
			Type:       "document",
			DependsOn:  []string{"制定故事大纲与创作计划"},
			Acceptance: []string{"内容原创，符合科幻风格", "文件已包含第一章内容（标题 + 正文）", "第一章正文存在且字数在450-550字之间"},
		},
		{
			Title:      "撰写第二章",
			Goal:       "完成第二章的正文写作，约500字，并追加到目标文件。",
			Type:       "document",
			DependsOn:  []string{"撰写第一章"},
			Acceptance: []string{"内容与第一章衔接，情节连贯", "文件已包含第二章内容（标题 + 正文）", "第二章正文存在且字数在450-550字之间"},
		},
		{
			Title:      "撰写第三章",
			Goal:       "完成第三章的正文写作，约500字，并追加到目标文件。",
			Type:       "document",
			DependsOn:  []string{"撰写第二章"},
			Acceptance: []string{"内容与前两章连贯，有适当收尾或悬念", "文件已包含第三章内容（标题 + 正文）", "第三章正文存在且字数在450-550字之间"},
		},
		{
			Title:      "检查文件并汇报三章标题和文件路径",
			Goal:       "验证目标文件存在且包含三章完整内容，提取三章标题，并汇报最终结果。",
			Type:       "general",
			DependsOn:  []string{"撰写第三章"},
			Acceptance: []string{"提取出每一章的标题", "文件包含三章完整内容（标题 + 正文）", "确认文件 artifacts/m10_6_full_validation_v4/writing/harbor-notes.md 存在", "输出汇报信息，包含三章标题和文件路径"},
		},
		{
			Title:      "反思任务拆解与验收口径",
			Goal:       "基于研究结果反思任务拆解是否站得住脚，明确最小可执行任务、风险和机械验收标准。",
			Type:       "general",
			Acceptance: []string{"已明确后续实现任务的机械验收标准", "已识别关键风险、缺失信息和需要重规划的条件", "已说明当前任务拆解为什么足以达成用户目标"},
		},
	})
	if hasDependency(out, "反思任务拆解与验收口径", "检查文件并汇报三章标题和文件路径") {
		t.Fatalf("reflection must not depend on final verification: %#v", out)
	}
	if !hasDependency(out, "检查文件并汇报三章标题和文件路径", "撰写第三章") {
		t.Fatalf("final verification should stay after chapter writing: %#v", out)
	}
	if !hasDependency(out, "检查文件并汇报三章标题和文件路径", "反思任务拆解与验收口径") {
		t.Fatalf("final verification should also wait for reflection: %#v", out)
	}
	if hasDependencyCycle(out) {
		t.Fatalf("expected normalized plan to be acyclic: %#v", out)
	}
}

func TestLLMPlannerNormalizesCyclicWritingPlan(t *testing.T) {
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		return provider.ChatResponse{Text: `{
			"phases":[{"title":"创作","goal":"写三章故事","description":"完成规划、写作和检查"}],
			"tasks":[
				{"phase":"创作","title":"制定故事大纲与创作计划","goal":"为《潮汐城回声》创作一个包含三章的故事大纲，每章约500字，确定章节标题和核心情节。","description":"","type":"document","depends_on":["反思任务拆解与验收口径"],"acceptance":["故事大纲包含三章，每章有明确的标题和核心情节描述","每章字数规划在500字左右","目标文件路径已确认"],"required_capabilities":["read","write"]},
				{"phase":"创作","title":"撰写第一章","goal":"完成第一章的正文写作，约500字，并写入目标文件。","description":"","type":"document","depends_on":["制定故事大纲与创作计划"],"acceptance":["内容原创，符合科幻风格","文件已包含第一章内容（标题 + 正文）","第一章正文存在且字数在450-550字之间"],"required_capabilities":["create_artifact","read","write"]},
				{"phase":"创作","title":"撰写第二章","goal":"完成第二章的正文写作，约500字，并追加到目标文件。","description":"","type":"document","depends_on":["撰写第一章"],"acceptance":["内容与第一章衔接，情节连贯","文件已包含第二章内容（标题 + 正文）","第二章正文存在且字数在450-550字之间"],"required_capabilities":["create_artifact","read","write"]},
				{"phase":"创作","title":"撰写第三章","goal":"完成第三章的正文写作，约500字，并追加到目标文件。","description":"","type":"document","depends_on":["撰写第二章"],"acceptance":["内容与前两章连贯，有适当收尾或悬念","文件已包含第三章内容（标题 + 正文）","第三章正文存在且字数在450-550字之间"],"required_capabilities":["create_artifact","read","write"]},
				{"phase":"创作","title":"检查文件并汇报三章标题和文件路径","goal":"验证目标文件存在且包含三章完整内容，提取三章标题，并汇报最终结果。","description":"","type":"general","depends_on":["撰写第三章"],"acceptance":["提取出每一章的标题","文件包含三章完整内容（标题 + 正文）","确认文件 artifacts/m10_6_full_validation_v4/writing/harbor-notes.md 存在","输出汇报信息，包含三章标题和文件路径"],"required_capabilities":["inspect","read","verify"]},
				{"phase":"创作","title":"反思任务拆解与验收口径","goal":"基于研究结果反思任务拆解是否站得住脚，明确最小可执行任务、风险和机械验收标准。","description":"","type":"general","depends_on":["检查文件并汇报三章标题和文件路径"],"acceptance":["已明确后续实现任务的机械验收标准","已识别关键风险、缺失信息和需要重规划的条件","已说明当前任务拆解为什么足以达成用户目标"],"required_capabilities":["verify"]}
			]
		}`}, nil
	})
	planner := NewLLMPlanner(llm, "test")
	tasks, err := planner.PlanProject(context.Background(), PlanRequest{
		Project: domain.Project{ID: "project", Goal: "创作三章故事"},
		ChainDecision: chain.Decision{
			Level:       chain.LevelProject,
			RequiresDAG: true,
		},
		IntentProfile: intent.Profile{TaskType: "document", Complexity: "high"},
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	planned := make([]plannedTask, 0, len(tasks))
	for _, task := range tasks {
		planned = append(planned, plannedTask{Title: task.Title, DependsOn: task.DependsOn})
	}
	if hasDependencyCycle(planned) {
		t.Fatalf("expected planner output to be acyclic: %#v", planned)
	}
	if !hasDependency(planned, "检查文件并汇报三章标题和文件路径", "撰写第三章") {
		t.Fatalf("expected final verification to wait for writing: %#v", planned)
	}
}

func TestNormalizePlannedDependenciesKeepsGenericVerificationAfterWriting(t *testing.T) {
	out := normalizePlannedDependencies([]plannedTask{
		{Title: "规划故事大纲", Goal: "为三章故事建立大纲", Type: "document", DependsOn: []string{"反思任务拆解与验收口径"}, Acceptance: []string{"大纲完成"}},
		{Title: "撰写三章内容并写入文件", Goal: "写入三章正文", Type: "document", DependsOn: []string{"规划故事大纲"}, Acceptance: []string{"三章已写入"}},
		{Title: "验证文件并汇报", Goal: "验证文件存在并汇报路径", Type: "general", Acceptance: []string{"文件存在", "结果已汇报"}},
		{Title: "反思任务拆解与验收口径", Goal: "反思任务拆解与验收口径", Type: "general", DependsOn: []string{"验证文件并汇报"}, Acceptance: []string{"反思完成"}},
	})
	if hasDependency(out, "反思任务拆解与验收口径", "验证文件并汇报") {
		t.Fatalf("reflection must not depend on final verification: %#v", out)
	}
	if !hasDependency(out, "验证文件并汇报", "反思任务拆解与验收口径") {
		t.Fatalf("verification should wait for reflection: %#v", out)
	}
	if hasDependencyCycle(out) {
		t.Fatalf("expected normalized plan to be acyclic: %#v", out)
	}
}

func TestNormalizePlannedDependenciesMakesFinalVerificationWaitForWork(t *testing.T) {
	out := normalizePlannedDependencies([]plannedTask{
		{Title: "制定故事蓝图", Goal: "规划故事", Type: "document"},
		{Title: "执行增量写作", Goal: "写入三章内容", Type: "document"},
		{Title: "最终验收与汇报", Goal: "检查文件并汇报路径", Type: "general"},
		{Title: "汇总执行状态与结果", Goal: "总结项目结果", Type: "general"},
	})
	for _, finalTitle := range []string{"最终验收与汇报", "汇总执行状态与结果"} {
		for _, dep := range []string{"制定故事蓝图", "执行增量写作"} {
			if !hasDependency(out, finalTitle, dep) {
				t.Fatalf("%s should depend on %s: %#v", finalTitle, dep, out)
			}
		}
	}
	if hasDependencyCycle(out) {
		t.Fatalf("expected normalized plan to be acyclic: %#v", out)
	}
}

func TestNormalizePlannedDependenciesMakesSelfCheckWaitForWork(t *testing.T) {
	out := normalizePlannedDependencies([]plannedTask{
		{Title: "Read existing story content for website material", Goal: "Read story source", Type: "runtime"},
		{Title: "Create index.html with all required sections", Goal: "Write page", Type: "document"},
		{Title: "Create styles.css with complete visual design", Goal: "Write CSS", Type: "document"},
		{Title: "Create README.md with deployment instructions", Goal: "Write deployment docs", Type: "document"},
		{Title: "Self-check all three files for completeness and correctness", Goal: "Read files and verify", Type: "general"},
	})
	for _, dep := range []string{
		"Read existing story content for website material",
		"Create index.html with all required sections",
		"Create styles.css with complete visual design",
		"Create README.md with deployment instructions",
	} {
		if !hasDependency(out, "Self-check all three files for completeness and correctness", dep) {
			t.Fatalf("self-check should depend on %s: %#v", dep, out)
		}
	}
	if hasDependencyCycle(out) {
		t.Fatalf("expected normalized plan to be acyclic: %#v", out)
	}
}

func TestNormalizePlannedDependenciesRemovesBackEdgesToSelfCheck(t *testing.T) {
	out := normalizePlannedDependencies([]plannedTask{
		{Title: "研究上下文与可用资料", Goal: "Read sources", Type: "runtime"},
		{Title: "Plan site content and structure", Goal: "Plan website", Type: "general", DependsOn: []string{"Read back and self-check all three files"}},
		{Title: "Create index.html", Goal: "Write HTML", Type: "document", DependsOn: []string{"Read back and self-check all three files"}},
		{Title: "Create styles.css", Goal: "Write CSS", Type: "document", DependsOn: []string{"Create index.html"}},
		{Title: "Create README.md", Goal: "Write docs", Type: "document"},
		{Title: "Read back and self-check all three files", Goal: "Verify all files", Type: "general"},
	})
	if hasDependency(out, "Plan site content and structure", "Read back and self-check all three files") {
		t.Fatalf("work task must not depend on final self-check: %#v", out)
	}
	if hasDependency(out, "Create index.html", "Read back and self-check all three files") {
		t.Fatalf("implementation task must not depend on final self-check: %#v", out)
	}
	for _, dep := range []string{"Plan site content and structure", "Create index.html", "Create styles.css", "Create README.md"} {
		if !hasDependency(out, "Read back and self-check all three files", dep) {
			t.Fatalf("self-check should depend on %s: %#v", dep, out)
		}
	}
	if hasDependencyCycle(out) {
		t.Fatalf("expected normalized plan to be acyclic: %#v", out)
	}
}

func TestNormalizePlannedDependenciesRemovesBackEdgesToValidateTask(t *testing.T) {
	out := normalizePlannedDependencies([]plannedTask{
		{Title: "研究上下文与可用资料", Goal: "Read sources", Type: "runtime"},
		{Title: "Write index.html with full content", Goal: "Write HTML", Type: "document", DependsOn: []string{"Read and validate all three site files"}},
		{Title: "Write styles.css for the showcase page", Goal: "Write CSS", Type: "document", DependsOn: []string{"Read and validate all three site files", "Write index.html with full content"}},
		{Title: "Write README.md with preview and deployment instructions", Goal: "Write docs", Type: "document", DependsOn: []string{"Read and validate all three site files"}},
		{Title: "Read and validate all three site files", Goal: "Read files and validate", Type: "general"},
	})
	for _, task := range []string{
		"Write index.html with full content",
		"Write styles.css for the showcase page",
		"Write README.md with preview and deployment instructions",
	} {
		if hasDependency(out, task, "Read and validate all three site files") {
			t.Fatalf("%s must not depend on final validation: %#v", task, out)
		}
		if !hasDependency(out, "Read and validate all three site files", task) {
			t.Fatalf("final validation should depend on %s: %#v", task, out)
		}
	}
	if hasDependencyCycle(out) {
		t.Fatalf("expected normalized plan to be acyclic: %#v", out)
	}
}

func TestNormalizePlannedDependenciesPrunesGenericCycle(t *testing.T) {
	out := normalizePlannedDependencies([]plannedTask{
		{Title: "Task A", Goal: "A", Type: "general", DependsOn: []string{"Task C"}, Acceptance: []string{"a"}},
		{Title: "Task B", Goal: "B", Type: "general", DependsOn: []string{"Task A"}, Acceptance: []string{"b"}},
		{Title: "Task C", Goal: "C", Type: "general", DependsOn: []string{"Task B"}, Acceptance: []string{"c"}},
	})
	if hasDependencyCycle(out) {
		t.Fatalf("expected cycle to be pruned: %#v", out)
	}
}

func TestPruneResearchAndReflectionForSimpleBrowserRead(t *testing.T) {
	out := pruneResearchAndReflectionForSimpleBrowser(PlanRequest{
		Project: domain.Project{Goal: "只打开 https://example.com 并总结，不修改文件"},
		ChainDecision: chain.Decision{
			Level:          chain.LevelAssist,
			NeedBrowser:    true,
			EstimatedSteps: 1,
		},
		IntentProfile: intent.Profile{
			TaskType:   "browser",
			Complexity: "simple",
		},
	}, []plannedTask{
		{Title: "研究上下文与可用资料", Goal: "研究", Type: "runtime", Acceptance: []string{"研究完成"}},
		{Title: "反思任务拆解与验收口径", Goal: "反思", Type: "general", DependsOn: []string{"研究上下文与可用资料"}, Acceptance: []string{"反思完成"}},
		{Title: "打开页面并总结", Goal: "打开 https://example.com 并总结页面内容", Type: "browser", DependsOn: []string{"反思任务拆解与验收口径"}, Acceptance: []string{"页面已总结"}},
	})
	if len(out) != 1 {
		t.Fatalf("expected only concrete browser task, got %#v", out)
	}
	if out[0].Title != "打开页面并总结" {
		t.Fatalf("unexpected task %q", out[0].Title)
	}
	if len(out[0].DependsOn) != 0 {
		t.Fatalf("expected removed dependencies, got %#v", out[0].DependsOn)
	}
}

func TestLLMPlannerSplitsProjectScaleUmbrellaTask(t *testing.T) {
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		return provider.ChatResponse{
			Model: req.Model,
			Text: `{
				"phases":[{"title":"完整实现","goal":"完成复杂项目","description":"单阶段大目标"}],
				"tasks":[
					{
						"phase":"完整实现",
						"title":"完成整个通用 agent 改造",
						"goal":"一次性完成所有事情",
						"description":"这是一个过大的伞状任务",
						"type":"general",
						"depends_on":[],
						"acceptance":["目标整体完成"],
						"required_capabilities":["read"]
					}
				]
			}`,
		}, nil
	})
	planner := NewLLMPlanner(llm, "gpt-test")
	tasks, err := planner.PlanProject(context.Background(), PlanRequest{
		Project: domain.Project{ID: "project-scale", Name: "agent-gogo", Goal: "把 runtime、web console、capability、observer 和 memory 收敛成通用 agent"},
		ChainDecision: chain.Decision{
			Level:          chain.LevelPlanned,
			RequiresDAG:    true,
			EstimatedSteps: 6,
		},
		IntentProfile: intent.Profile{
			TaskType:   "general",
			Complexity: "high",
		},
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	if len(tasks) < 4 {
		t.Fatalf("expected project-scale umbrella task to be expanded to at least 4 tasks, got %d: %#v", len(tasks), tasks)
	}
	if tasks[len(tasks)-1].Title != "汇总执行状态与结果" {
		t.Fatalf("expected final channel result task, got %q", tasks[len(tasks)-1].Title)
	}
}

func TestLLMPlannerRejectsOverBroadFlatTaskList(t *testing.T) {
	llm := provider.ChatFunc(func(ctx context.Context, req provider.ChatRequest) (provider.ChatResponse, error) {
		return provider.ChatResponse{Text: `{"tasks":[
			{"title":"Task 1","goal":"g","description":"d","type":"general","depends_on":[],"acceptance":["a"]},
			{"title":"Task 2","goal":"g","description":"d","type":"general","depends_on":[],"acceptance":["a"]},
			{"title":"Task 3","goal":"g","description":"d","type":"general","depends_on":[],"acceptance":["a"]},
			{"title":"Task 4","goal":"g","description":"d","type":"general","depends_on":[],"acceptance":["a"]}
		]}`}, nil
	})
	planner := NewLLMPlanner(llm, "test")
	_, err := planner.PlanProject(context.Background(), PlanRequest{
		Project:       domain.Project{ID: "project", Goal: "simple task"},
		IntentProfile: intent.Profile{Complexity: "low"},
	})
	if err == nil || !strings.Contains(err.Error(), "above max") {
		t.Fatalf("expected task-count guard, got %v", err)
	}
}

func hasDependency(tasks []plannedTask, title string, dependency string) bool {
	for _, task := range tasks {
		if task.Title != title {
			continue
		}
		for _, dep := range task.DependsOn {
			if dep == dependency {
				return true
			}
		}
	}
	return false
}

func hasDependencyCycle(tasks []plannedTask) bool {
	depsByTitle := map[string][]string{}
	for _, task := range tasks {
		depsByTitle[task.Title] = task.DependsOn
	}
	for _, task := range tasks {
		if reachesDependency(depsByTitle, task.Title, task.Title, map[string]bool{}) {
			for _, dep := range task.DependsOn {
				if reachesDependency(depsByTitle, dep, task.Title, map[string]bool{}) {
					return true
				}
			}
		}
	}
	return false
}
