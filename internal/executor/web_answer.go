package executor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/sukeke/agent-gogo/internal/browser"
	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/provider"
)

type WebAnswerStore interface {
	TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error)
	CreateTaskAttempt(ctx context.Context, taskID string) (domain.TaskAttempt, error)
	CreateToolCall(ctx context.Context, call domain.ToolCall) (domain.ToolCall, error)
	CreateObservation(ctx context.Context, observation domain.Observation) (domain.Observation, error)
	AddTaskEvent(ctx context.Context, event domain.TaskEvent) (domain.TaskEvent, error)
}

type WebAnswerExecutor struct {
	store      WebAnswerStore
	browser    *browser.Runtime
	llm        provider.LLMProvider
	model      string
	defaultURL string
	userGoal   string
	now        func() time.Time
}

func NewWebAnswerExecutor(store WebAnswerStore, browserRuntime *browser.Runtime, llm provider.LLMProvider, model string, defaultURL string, userGoal string) *WebAnswerExecutor {
	return &WebAnswerExecutor{
		store:      store,
		browser:    browserRuntime,
		llm:        llm,
		model:      model,
		defaultURL: strings.TrimSpace(defaultURL),
		userGoal:   strings.TrimSpace(userGoal),
		now:        time.Now,
	}
}

func (e *WebAnswerExecutor) Execute(ctx context.Context, task domain.Task) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if e.browser == nil {
		return Result{}, errors.New("browser runtime is required")
	}
	if e.llm == nil {
		return Result{}, errors.New("llm provider is required")
	}

	inProgress, err := e.store.TransitionTask(ctx, task.ID, domain.TaskStatusInProgress, "web answer executor started")
	if err != nil {
		return Result{}, err
	}
	attempt, err := e.store.CreateTaskAttempt(ctx, task.ID)
	if err != nil {
		return Result{}, err
	}

	url := firstNonEmpty(extractURL(task.Description), extractURL(task.Title), extractURL(strings.Join(task.AcceptanceCriteria, " ")), e.defaultURL)
	if url == "" {
		return Result{}, errors.New("no URL found for web answer task")
	}
	goal := firstNonEmpty(e.userGoal, task.Description, task.Title)
	if err := e.addEvent(ctx, task.ID, attempt.ID, "executor.input", fmt.Sprintf("url=%s goal=%s", url, goal)); err != nil {
		return Result{}, err
	}

	openSnapshot, openCall, err := e.openURL(ctx, attempt.ID, url)
	if err != nil {
		return Result{}, err
	}
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   attempt.ID,
		ToolCallID:  openCall.ID,
		Type:        "browser.open",
		Summary:     openSnapshot.DOMSummary,
		EvidenceRef: openSnapshot.URL,
		Payload:     mustJSON(openSnapshot),
	}); err != nil {
		return Result{}, err
	}
	if err := e.addEvent(ctx, task.ID, attempt.ID, "executor.indexed", fmt.Sprintf("indexed %d chars from %s", len([]rune(openSnapshot.DOMSummary)), openSnapshot.URL)); err != nil {
		return Result{}, err
	}
	snapshot := openSnapshot
	if shouldClickPlumBlossom(goal, snapshot) {
		clickSnapshot, err := e.clickAndObserve(ctx, task.ID, attempt.ID, "梅花易数")
		if err != nil {
			return Result{}, err
		}
		snapshot = clickSnapshot
	}
	if strings.Contains(snapshot.DOMSummary, "获取答案") {
		clickSnapshot, err := e.clickAndObserve(ctx, task.ID, attempt.ID, "获取答案")
		if err != nil {
			return Result{}, err
		}
		snapshot = clickSnapshot
	}

	answer, answerCall, err := e.answer(ctx, attempt.ID, goal, snapshot)
	if err != nil {
		return Result{}, err
	}
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   attempt.ID,
		ToolCallID:  answerCall.ID,
		Type:        "llm.answer",
		Summary:     answer,
		EvidenceRef: openSnapshot.URL,
		Payload: mustJSON(map[string]any{
			"answer": answer,
			"url":    openSnapshot.URL,
			"goal":   goal,
		}),
	}); err != nil {
		return Result{}, err
	}

	implemented, err := e.store.TransitionTask(ctx, inProgress.ID, domain.TaskStatusImplemented, "web answer executor produced grounded answer")
	if err != nil {
		return Result{}, err
	}
	return Result{Task: implemented, Attempt: attempt}, nil
}

func (e *WebAnswerExecutor) openURL(ctx context.Context, attemptID string, url string) (browser.Snapshot, domain.ToolCall, error) {
	input := mustJSON(map[string]any{"url": url})
	snapshot, err := e.browser.Open(ctx, url)
	if err != nil {
		call, createErr := e.store.CreateToolCall(ctx, domain.ToolCall{
			AttemptID: attemptID,
			Name:      "browser.open",
			InputJSON: input,
			Status:    domain.ToolCallStatusFailed,
			Error:     err.Error(),
		})
		if createErr != nil {
			return browser.Snapshot{}, domain.ToolCall{}, createErr
		}
		return browser.Snapshot{}, call, err
	}
	call, err := e.store.CreateToolCall(ctx, domain.ToolCall{
		AttemptID:   attemptID,
		Name:        "browser.open",
		InputJSON:   input,
		OutputJSON:  mustJSON(snapshot),
		Status:      domain.ToolCallStatusSucceeded,
		EvidenceRef: snapshot.URL,
	})
	if err != nil {
		return browser.Snapshot{}, domain.ToolCall{}, err
	}
	return snapshot, call, nil
}

func (e *WebAnswerExecutor) click(ctx context.Context, attemptID string, text string) (browser.Snapshot, domain.ToolCall, error) {
	input := mustJSON(map[string]any{"text": text})
	snapshot, err := e.browser.Click(ctx, text)
	if err != nil {
		call, createErr := e.store.CreateToolCall(ctx, domain.ToolCall{
			AttemptID: attemptID,
			Name:      "browser.click",
			InputJSON: input,
			Status:    domain.ToolCallStatusFailed,
			Error:     err.Error(),
		})
		if createErr != nil {
			return browser.Snapshot{}, domain.ToolCall{}, createErr
		}
		return browser.Snapshot{}, call, err
	}
	call, err := e.store.CreateToolCall(ctx, domain.ToolCall{
		AttemptID:   attemptID,
		Name:        "browser.click",
		InputJSON:   input,
		OutputJSON:  mustJSON(snapshot),
		Status:      domain.ToolCallStatusSucceeded,
		EvidenceRef: snapshot.URL,
	})
	if err != nil {
		return browser.Snapshot{}, domain.ToolCall{}, err
	}
	return snapshot, call, nil
}

func (e *WebAnswerExecutor) clickAndObserve(ctx context.Context, taskID string, attemptID string, text string) (browser.Snapshot, error) {
	clickSnapshot, clickCall, err := e.click(ctx, attemptID, text)
	if err != nil {
		return browser.Snapshot{}, err
	}
	if _, err := e.store.CreateObservation(ctx, domain.Observation{
		AttemptID:   attemptID,
		ToolCallID:  clickCall.ID,
		Type:        "browser.click",
		Summary:     clickSnapshot.DOMSummary,
		EvidenceRef: clickSnapshot.URL,
		Payload:     mustJSON(clickSnapshot),
	}); err != nil {
		return browser.Snapshot{}, err
	}
	if err := e.addEvent(ctx, taskID, attemptID, "executor.clicked", fmt.Sprintf("clicked %s and indexed %d chars", text, len([]rune(clickSnapshot.DOMSummary)))); err != nil {
		return browser.Snapshot{}, err
	}
	return clickSnapshot, nil
}

func (e *WebAnswerExecutor) answer(ctx context.Context, attemptID string, goal string, snapshot browser.Snapshot) (string, domain.ToolCall, error) {
	today := e.now().Format("2006-01-02")
	userPayload := map[string]string{
		"date":           today,
		"url":            snapshot.URL,
		"user_goal":      goal,
		"page_text":      snapshot.DOMSummary,
		"visible_answer": extractVisibleAnswer(snapshot.DOMSummary),
		"instruction":    "请基于页面文本回答用户目标；如果 visible_answer 非空，把它作为页面已经给出的答案来整理。页面未显示卦名/卦象时请明确写页面未显示，不要把可见答案判为空。",
	}
	req := provider.ChatRequest{
		Model: e.model,
		Messages: []provider.ChatMessage{
			{Role: "system", Content: webAnswerSystemPrompt},
			{Role: "user", Content: mustJSON(userPayload)},
		},
		Metadata: map[string]string{"source_url": snapshot.URL},
	}
	resp, err := e.llm.Chat(ctx, req)
	input := mustJSON(req)
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
	answer := strings.TrimSpace(resp.Text)
	call, err := e.store.CreateToolCall(ctx, domain.ToolCall{
		AttemptID:   attemptID,
		Name:        "llm.answer",
		InputJSON:   input,
		OutputJSON:  mustJSON(resp),
		Status:      domain.ToolCallStatusSucceeded,
		EvidenceRef: snapshot.URL,
	})
	if err != nil {
		return "", domain.ToolCall{}, err
	}
	return answer, call, nil
}

func (e *WebAnswerExecutor) addEvent(ctx context.Context, taskID string, attemptID string, eventType string, message string) error {
	_, err := e.store.AddTaskEvent(ctx, domain.TaskEvent{
		TaskID:    taskID,
		AttemptID: attemptID,
		Type:      eventType,
		Message:   message,
	})
	return err
}

const webAnswerSystemPrompt = `You are agent-gogo's web answer executor.
Return a concise Chinese answer grounded in the supplied page_text.
If visible_answer is non-empty, treat it as the page's visible answer and summarize it faithfully in Chinese.
Include today's date, the source URL, visible digits when present, and note "页面未显示" for fields the page does not show.
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

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "{}"
	}
	return string(data)
}
