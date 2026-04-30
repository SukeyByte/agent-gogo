package webanswer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/SukeyByte/agent-gogo/internal/browser"
	"github.com/SukeyByte/agent-gogo/internal/domain"
	coreexecutor "github.com/SukeyByte/agent-gogo/internal/executor"
	"github.com/SukeyByte/agent-gogo/internal/provider"
)

type Store interface {
	coreexecutor.BrowserActionStore
	TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error)
	CreateTaskAttempt(ctx context.Context, taskID string) (domain.TaskAttempt, error)
}

type Executor struct {
	store      Store
	browser    *coreexecutor.BrowserExecutor
	llm        provider.LLMProvider
	model      string
	defaultURL string
	userGoal   string
	now        func() time.Time
}

func NewExecutor(store Store, browserRuntime *browser.Runtime, llm provider.LLMProvider, model string, defaultURL string, userGoal string) *Executor {
	return &Executor{
		store:      store,
		browser:    coreexecutor.NewBrowserExecutor(store, browserRuntime),
		llm:        llm,
		model:      model,
		defaultURL: strings.TrimSpace(defaultURL),
		userGoal:   strings.TrimSpace(userGoal),
		now:        time.Now,
	}
}

func (e *Executor) Execute(ctx context.Context, task domain.Task) (coreexecutor.Result, error) {
	if err := ctx.Err(); err != nil {
		return coreexecutor.Result{}, err
	}
	if e.llm == nil {
		return coreexecutor.Result{}, errors.New("llm provider is required")
	}
	inProgress, err := e.store.TransitionTask(ctx, task.ID, domain.TaskStatusInProgress, "web answer demo executor started")
	if err != nil {
		return coreexecutor.Result{}, err
	}
	attempt, err := e.store.CreateTaskAttempt(ctx, task.ID)
	if err != nil {
		return coreexecutor.Result{}, err
	}
	url := firstNonEmpty(extractURL(task.Description), extractURL(task.Title), extractURL(strings.Join(task.AcceptanceCriteria, " ")), e.defaultURL)
	if url == "" {
		return coreexecutor.Result{}, errors.New("no URL found for web answer task")
	}
	goal := firstNonEmpty(e.userGoal, task.Description, task.Title)
	if err := e.browser.AddEvent(ctx, task.ID, attempt.ID, "executor.input", fmt.Sprintf("url=%s goal=%s", url, goal)); err != nil {
		return coreexecutor.Result{}, err
	}

	snapshot, _, err := e.browser.Open(ctx, task.ID, attempt.ID, url)
	if err != nil {
		return coreexecutor.Result{}, err
	}
	if shouldClickPlumBlossom(goal, snapshot) {
		next, _, err := e.browser.Click(ctx, task.ID, attempt.ID, "梅花易数")
		if err != nil {
			return coreexecutor.Result{}, err
		}
		snapshot = next
	}
	if strings.Contains(snapshot.DOMSummary, "获取答案") {
		next, _, err := e.browser.Click(ctx, task.ID, attempt.ID, "获取答案")
		if err != nil {
			return coreexecutor.Result{}, err
		}
		snapshot = next
	}

	answer, answerCall, err := e.answer(ctx, attempt.ID, goal, snapshot)
	if err != nil {
		return coreexecutor.Result{}, err
	}
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   attempt.ID,
		ToolCallID:  answerCall.ID,
		Type:        "llm.answer",
		Summary:     answer,
		EvidenceRef: snapshot.URL,
		Payload: jsonString(map[string]any{
			"answer": answer,
			"url":    snapshot.URL,
			"goal":   goal,
		}),
	}); err != nil {
		return coreexecutor.Result{}, err
	}
	implemented, err := e.store.TransitionTask(ctx, inProgress.ID, domain.TaskStatusImplemented, "web answer demo produced grounded answer")
	if err != nil {
		return coreexecutor.Result{}, err
	}
	return coreexecutor.Result{Task: implemented, Attempt: attempt}, nil
}

func (e *Executor) answer(ctx context.Context, attemptID string, goal string, snapshot browser.Snapshot) (string, domain.ToolCall, error) {
	req := provider.ChatRequest{
		Model: e.model,
		Messages: []provider.ChatMessage{
			{Role: "system", Content: webAnswerSystemPrompt},
			{Role: "user", Content: jsonString(map[string]string{
				"date":           e.now().Format("2006-01-02"),
				"url":            snapshot.URL,
				"user_goal":      goal,
				"page_text":      snapshot.DOMSummary,
				"visible_answer": extractVisibleAnswer(snapshot.DOMSummary),
			})},
		},
		Metadata: map[string]string{"source_url": snapshot.URL},
	}
	resp, err := e.llm.Chat(ctx, req)
	input := jsonString(req)
	if err != nil {
		call, createErr := e.store.CreateToolCall(ctx, domain.ToolCall{
			AttemptID: attemptID,
			Name:      "llm.answer",
			InputJSON: input,
			Status:    domain.ToolCallStatusFailed,
			Error:     err.Error(),
		})
		if createErr != nil {
			return "", domain.ToolCall{}, createErr
		}
		return "", call, err
	}
	call, err := e.store.CreateToolCall(ctx, domain.ToolCall{
		AttemptID:   attemptID,
		Name:        "llm.answer",
		InputJSON:   input,
		OutputJSON:  jsonString(resp),
		Status:      domain.ToolCallStatusSucceeded,
		EvidenceRef: snapshot.URL,
	})
	if err != nil {
		return "", domain.ToolCall{}, err
	}
	return strings.TrimSpace(resp.Text), call, nil
}

const webAnswerSystemPrompt = `You are agent-gogo's web answer demo executor.
Return a concise Chinese answer grounded in the supplied page_text.
If visible_answer is non-empty, treat it as the page's visible answer and summarize it faithfully in Chinese.
Do not say the answer is missing when visible_answer contains answer text.`

var urlPattern = regexp.MustCompile(`https?://[^\s"'<>，。)）]+`)

func extractURL(text string) string {
	return urlPattern.FindString(text)
}

func extractVisibleAnswer(text string) string {
	start := strings.Index(text, "你的答案")
	if start < 0 {
		return ""
	}
	answer := strings.TrimSpace(text[start+len("你的答案"):])
	for _, marker := range []string{"auto_awesome", "想要更深入了解自己", "style 每日运势"} {
		if idx := strings.Index(answer, marker); idx >= 0 {
			answer = strings.TrimSpace(answer[:idx])
		}
	}
	return answer
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func shouldClickPlumBlossom(goal string, snapshot browser.Snapshot) bool {
	goalContainsPlum := strings.Contains(goal, "梅花易数") || strings.Contains(strings.ToLower(goal), "plum")
	currentURL := strings.ToLower(snapshot.URL)
	return goalContainsPlum && strings.Contains(snapshot.DOMSummary, "梅花易数") && !strings.Contains(currentURL, "plum")
}

func jsonString(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
