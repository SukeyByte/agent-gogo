package chain

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SukeyByte/agent-gogo/internal/provider"
)

func TestDeepSeekRouterProjectScaleSmoke(t *testing.T) {
	if os.Getenv("RUN_DEEPSEEK_SMOKE") != "1" {
		t.Skip("set RUN_DEEPSEEK_SMOKE=1 to run the real DeepSeek router smoke test")
	}
	apiKey := firstNonEmpty(os.Getenv("DEEPSEEK_API_KEY"), os.Getenv("AGENT_GOGO_LLM_API_KEY"))
	if apiKey == "" {
		t.Skip("set DEEPSEEK_API_KEY or AGENT_GOGO_LLM_API_KEY to run the real DeepSeek router smoke test")
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
	input := "请把当前 agent-gogo 的 runtime、Web Console、Capability Resolver、Observer、Memory 和验收文档收敛成一个真正通用 agent 主链路，要求能规划、执行、恢复和汇报状态。"
	if strings.Contains(input, "传奇") || strings.Contains(strings.ToLower(input), "legendary") {
		t.Fatal("smoke input must not contain project-scale label words")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	decision, err := NewLLMRouter(llm, provider.DefaultDeepSeekModel).Route(ctx, Request{UserInput: input})
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if !IsProjectScale(decision) {
		t.Fatalf("expected project-scale route from AI scale signals, got %#v", decision)
	}
	if !decision.RequiresDAG && decision.EstimatedSteps < 4 {
		t.Fatalf("expected requires_dag or estimated_steps>=4, got %#v", decision)
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
