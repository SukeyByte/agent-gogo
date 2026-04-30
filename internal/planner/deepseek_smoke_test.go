package planner

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sukeke/agent-gogo/internal/chain"
	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/intent"
	"github.com/sukeke/agent-gogo/internal/provider"
)

func TestDeepSeekPlannerStructuredSmoke(t *testing.T) {
	if os.Getenv("RUN_DEEPSEEK_SMOKE") != "1" {
		t.Skip("set RUN_DEEPSEEK_SMOKE=1 to run the real DeepSeek planner smoke test")
	}
	apiKey := firstNonEmpty(os.Getenv("DEEPSEEK_API_KEY"), os.Getenv("AGENT_GOGO_LLM_API_KEY"))
	if apiKey == "" {
		t.Skip("set DEEPSEEK_API_KEY or AGENT_GOGO_LLM_API_KEY to run the real DeepSeek planner smoke test")
	}
	thinking := false
	llm, err := provider.NewDeepSeekProvider(provider.DeepSeekConfig{
		APIKey:          apiKey,
		ThinkingEnabled: &thinking,
		HTTPClient:      &http.Client{Timeout: 90 * time.Second},
	})
	if err != nil {
		t.Fatalf("new deepseek provider: %v", err)
	}
	planner := NewLLMPlanner(llm, provider.DefaultDeepSeekModel)

	cases := []struct {
		name       string
		goal       string
		taskType   string
		domainName string
		capability string
	}{
		{
			name:       "code",
			goal:       "修复当前 Go 仓库里一个失败测试，并说明需要运行的机械验收。",
			taskType:   "code",
			domainName: "go",
			capability: "file.patch",
		},
		{
			name:       "web",
			goal:       "读取 https://example.com 网页并总结主要内容。",
			taskType:   "browser",
			domainName: "web",
			capability: "browser.open",
		},
		{
			name:       "document",
			goal:       "根据 README 内容写一段项目简介文档。",
			taskType:   "document",
			domainName: "documentation",
			capability: "file.read",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			tasks, err := planner.PlanProject(ctx, PlanRequest{
				Project: domain.Project{
					ID:   "project-" + tc.name,
					Goal: tc.goal,
				},
				UserInput: tc.goal,
				ChainDecision: chain.Decision{
					Level:     chain.LevelProject,
					NeedPlan:  true,
					NeedTools: true,
				},
				IntentProfile: intent.Profile{
					TaskType:             tc.taskType,
					Complexity:           "high",
					Domains:              []string{tc.domainName},
					RequiredCapabilities: []string{tc.capability},
				},
			})
			if err != nil {
				t.Fatalf("plan project: %v", err)
			}
			if len(tasks) < 3 {
				t.Fatalf("expected research/reflection plus implementation tasks, got %d: %#v", len(tasks), tasks)
			}
			if !containsTaskText(tasks, []string{"研究", "research", "context", "gather", "获取", "读取", "搜索"}) {
				t.Fatalf("expected research task in %#v", tasks)
			}
			if !containsTaskText(tasks, []string{"反思", "reflection", "decomposition", "验收口径", "acceptance criteria"}) {
				t.Fatalf("expected reflection task in %#v", tasks)
			}
		})
	}
}

func containsTaskText(tasks []domain.Task, markers []string) bool {
	for _, task := range tasks {
		text := strings.ToLower(task.Title + " " + task.Description + " " + strings.Join(task.AcceptanceCriteria, " "))
		for _, marker := range markers {
			if strings.Contains(text, strings.ToLower(marker)) {
				return true
			}
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
