package taskaware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/sukeke/agent-gogo/internal/contextbuilder"
	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/memory"
	"github.com/sukeke/agent-gogo/internal/textutil"
)

const (
	maxDigestTasks       = 8
	maxDigestEvents      = 12
	maxDigestEvidence    = 12
	maxDigestDecisions   = 8
	maxTaskObservations  = 8
	maxExtractedMemories = 4
)

type Store interface {
	ListTasksByProject(ctx context.Context, projectID string) ([]domain.Task, error)
	ListTaskDependenciesByProject(ctx context.Context, projectID string) ([]domain.TaskDependency, error)
	ListTaskEvents(ctx context.Context, taskID string) ([]domain.TaskEvent, error)
	ListTaskAttemptsByTask(ctx context.Context, taskID string) ([]domain.TaskAttempt, error)
	ListObservationsByAttempt(ctx context.Context, attemptID string) ([]domain.Observation, error)
	ListToolCallsByAttempt(ctx context.Context, attemptID string) ([]domain.ToolCall, error)
	ListTestResultsByAttempt(ctx context.Context, attemptID string) ([]domain.TestResult, error)
	ListReviewResultsByAttempt(ctx context.Context, attemptID string) ([]domain.ReviewResult, error)
	ListArtifactsByProject(ctx context.Context, projectID string) ([]domain.Artifact, error)
}

type ContextSnapshot struct {
	ProjectState       contextbuilder.ProjectState
	TaskState          contextbuilder.TaskState
	AcceptanceCriteria []contextbuilder.AcceptanceCriterion
	EvidenceRefs       []contextbuilder.EvidenceRef
	QueryText          string
}

type taskFacts struct {
	Events            []domain.TaskEvent
	Attempts          []domain.TaskAttempt
	Observations      []domain.Observation
	ToolCalls         []domain.ToolCall
	TestResults       []domain.TestResult
	ReviewResults     []domain.ReviewResult
	EvidenceSummaries []contextbuilder.EvidenceSummary
	EvidenceRefs      []contextbuilder.EvidenceRef
	Decisions         []contextbuilder.DecisionRecord
}

func BuildContextSnapshot(ctx context.Context, store Store, project domain.Project, currentTaskID string) (ContextSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return ContextSnapshot{}, err
	}
	if store == nil {
		return emptySnapshot(project), nil
	}
	tasks, err := store.ListTasksByProject(ctx, project.ID)
	if err != nil {
		return ContextSnapshot{}, err
	}
	dependencies, err := store.ListTaskDependenciesByProject(ctx, project.ID)
	if err != nil {
		return ContextSnapshot{}, err
	}
	artifacts, err := store.ListArtifactsByProject(ctx, project.ID)
	if err != nil {
		return ContextSnapshot{}, err
	}

	taskByID := make(map[string]domain.Task, len(tasks))
	for _, task := range tasks {
		taskByID[task.ID] = task
	}
	dependsOn := map[string][]contextbuilder.TaskLink{}
	blocks := map[string][]contextbuilder.TaskLink{}
	for _, dependency := range dependencies {
		task, taskOK := taskByID[dependency.TaskID]
		parent, parentOK := taskByID[dependency.DependsOnTaskID]
		if !taskOK || !parentOK {
			continue
		}
		dependsOn[task.ID] = append(dependsOn[task.ID], taskLink(parent))
		blocks[parent.ID] = append(blocks[parent.ID], taskLink(task))
	}

	statusCounts := countStatuses(tasks)
	factsByTask := map[string]taskFacts{}
	completed := []contextbuilder.TaskSummary{}
	active := []contextbuilder.TaskSummary{}
	problems := []contextbuilder.TaskSummary{}
	allEvents := []eventWithTime{}
	allEvidence := []evidenceWithTime{}
	decisions := []contextbuilder.DecisionRecord{}

	for _, task := range tasks {
		facts, err := loadTaskFacts(ctx, store, task)
		if err != nil {
			return ContextSnapshot{}, err
		}
		factsByTask[task.ID] = facts
		summary := summarizeTask(task, facts, dependsOn[task.ID], blocks[task.ID])
		switch {
		case task.Status == domain.TaskStatusDone:
			completed = append(completed, summary)
		case task.Status == domain.TaskStatusFailed || task.Status == domain.TaskStatusReviewFailed || task.Status == domain.TaskStatusBlocked:
			problems = append(problems, summary)
		default:
			active = append(active, summary)
		}
		for _, event := range facts.Events {
			allEvents = append(allEvents, eventWithTime{
				CreatedAtKey: event.CreatedAt.UnixNano(),
				Event: contextbuilder.EventSummary{
					TaskID:    task.ID,
					TaskTitle: task.Title,
					AttemptID: event.AttemptID,
					Type:      event.Type,
					FromState: string(event.FromState),
					ToState:   string(event.ToState),
					Message:   truncate(event.Message, 220),
				},
			})
		}
		for index, evidence := range facts.EvidenceSummaries {
			item := evidenceWithTime{Evidence: evidence}
			if index < len(facts.EvidenceRefs) {
				item.CreatedAtKey = facts.EvidenceRefs[index].CreatedAt.UnixNano()
				item.Ref = facts.EvidenceRefs[index]
			}
			allEvidence = append(allEvidence, item)
		}
		decisions = append(decisions, facts.Decisions...)
	}
	for _, artifact := range artifacts {
		allEvidence = append(allEvidence, evidenceWithTime{
			CreatedAtKey: artifact.CreatedAt.UnixNano(),
			Evidence: contextbuilder.EvidenceSummary{
				ID:          artifact.ID,
				TaskID:      "",
				TaskTitle:   "project artifact",
				Type:        "artifact." + artifact.Type,
				Summary:     firstNonEmpty(artifact.Description, artifact.Path),
				EvidenceRef: artifact.Path,
			},
			Ref: contextbuilder.EvidenceRef{
				ID:          artifact.ID,
				Type:        "artifact." + artifact.Type,
				Summary:     firstNonEmpty(artifact.Description, artifact.Path),
				ArtifactRef: artifact.Path,
				CreatedAt:   artifact.CreatedAt,
			},
		})
	}

	recentEvents := newestEvents(allEvents, maxDigestEvents)
	recentEvidence, evidenceRefs := newestEvidence(allEvidence, maxDigestEvidence)
	digest := contextbuilder.ProjectDigest{
		TaskCount:      len(tasks),
		StatusCounts:   statusCounts,
		CompletedTasks: limitTaskSummaries(completed, maxDigestTasks),
		ActiveTasks:    limitTaskSummaries(active, maxDigestTasks),
		ProblemTasks:   limitTaskSummaries(problems, maxDigestTasks),
		RecentEvents:   recentEvents,
		RecentEvidence: recentEvidence,
		Decisions:      limitDecisions(decisions, maxDigestDecisions),
	}
	projectState := contextbuilder.ProjectState{
		ID:      project.ID,
		Name:    project.Name,
		Goal:    project.Goal,
		Status:  string(project.Status),
		Summary: summarizeProject(digest),
		Digest:  digest,
	}
	taskState, criteria := currentTaskState(currentTaskID, tasks, factsByTask, dependsOn, blocks)
	return ContextSnapshot{
		ProjectState:       projectState,
		TaskState:          taskState,
		AcceptanceCriteria: criteria,
		EvidenceRefs:       evidenceRefs,
		QueryText:          queryText(project, taskState, digest),
	}, nil
}

func ExtractTaskMemories(ctx context.Context, store Store, project domain.Project, task domain.Task, attempt domain.TaskAttempt, review domain.ReviewResult) ([]memory.Item, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	facts, err := loadTaskFacts(ctx, store, task)
	if err != nil {
		return nil, err
	}
	items := []memory.Item{}
	body := renderTaskMemoryBody(task, facts, review)
	summary := "Task completed: " + task.Title
	if strings.TrimSpace(review.Summary) != "" {
		summary = "Task completed: " + task.Title + " - " + truncate(review.Summary, 160)
	}
	items = append(items, newMemoryItem(project.ID, task.ID, attempt.ID, "decision", summary, body, firstEvidenceRef(facts, review.EvidenceRef), []string{"task", "decision", "review"}, 0.82))
	for _, observation := range facts.Observations {
		if len(items) >= maxExtractedMemories {
			break
		}
		if strings.TrimSpace(observation.Summary) == "" {
			continue
		}
		tags := []string{"task", "observation", observation.Type}
		body := strings.Join([]string{
			"Task: " + task.Title,
			"Observation type: " + observation.Type,
			"Summary: " + observation.Summary,
			"Evidence: " + observation.EvidenceRef,
			"Payload: " + truncate(observation.Payload, 600),
		}, "\n")
		items = append(items, newMemoryItem(project.ID, task.ID, attempt.ID, "observation", "Observation from "+task.Title+": "+truncate(observation.Summary, 180), body, observation.EvidenceRef, tags, 0.74))
	}
	return items, nil
}

func FailureMemory(projectID string, task domain.Task, attempt domain.TaskAttempt, eventType string, message string) memory.Item {
	body := strings.Join([]string{
		"Task: " + task.Title,
		"Task ID: " + task.ID,
		"Attempt ID: " + attempt.ID,
		"Failure type: " + eventType,
		"Failure message: " + message,
	}, "\n")
	return newMemoryItem(projectID, task.ID, attempt.ID, "failure", "Task failed: "+task.Title+" - "+truncate(message, 180), body, "", []string{"task", "failure", eventType}, 0.86)
}

func emptySnapshot(project domain.Project) ContextSnapshot {
	return ContextSnapshot{
		ProjectState: contextbuilder.ProjectState{
			ID:      project.ID,
			Name:    project.Name,
			Goal:    project.Goal,
			Status:  string(project.Status),
			Summary: "No task facts available yet.",
		},
		QueryText: project.Goal,
	}
}

func loadTaskFacts(ctx context.Context, store Store, task domain.Task) (taskFacts, error) {
	events, err := store.ListTaskEvents(ctx, task.ID)
	if err != nil {
		return taskFacts{}, err
	}
	attempts, err := store.ListTaskAttemptsByTask(ctx, task.ID)
	if err != nil {
		return taskFacts{}, err
	}
	facts := taskFacts{Events: events, Attempts: attempts}
	for _, attempt := range attempts {
		observations, err := store.ListObservationsByAttempt(ctx, attempt.ID)
		if err != nil {
			return taskFacts{}, err
		}
		toolCalls, err := store.ListToolCallsByAttempt(ctx, attempt.ID)
		if err != nil {
			return taskFacts{}, err
		}
		testResults, err := store.ListTestResultsByAttempt(ctx, attempt.ID)
		if err != nil {
			return taskFacts{}, err
		}
		reviewResults, err := store.ListReviewResultsByAttempt(ctx, attempt.ID)
		if err != nil {
			return taskFacts{}, err
		}
		facts.Observations = append(facts.Observations, observations...)
		facts.ToolCalls = append(facts.ToolCalls, toolCalls...)
		facts.TestResults = append(facts.TestResults, testResults...)
		facts.ReviewResults = append(facts.ReviewResults, reviewResults...)
		for _, observation := range observations {
			facts.EvidenceSummaries = append(facts.EvidenceSummaries, contextbuilder.EvidenceSummary{
				ID:          observation.ID,
				TaskID:      task.ID,
				TaskTitle:   task.Title,
				Type:        "observation." + observation.Type,
				Summary:     truncate(observation.Summary, 220),
				EvidenceRef: observation.EvidenceRef,
			})
			facts.EvidenceRefs = append(facts.EvidenceRefs, contextbuilder.EvidenceRef{
				ID:          observation.ID,
				Type:        "observation." + observation.Type,
				Summary:     truncate(observation.Summary, 220),
				ArtifactRef: observation.EvidenceRef,
				CreatedAt:   observation.CreatedAt,
			})
		}
		for _, call := range toolCalls {
			facts.EvidenceSummaries = append(facts.EvidenceSummaries, contextbuilder.EvidenceSummary{
				ID:          call.ID,
				TaskID:      task.ID,
				TaskTitle:   task.Title,
				Type:        "tool." + call.Name,
				Summary:     string(call.Status),
				EvidenceRef: call.EvidenceRef,
			})
			facts.EvidenceRefs = append(facts.EvidenceRefs, contextbuilder.EvidenceRef{
				ID:          call.ID,
				Type:        "tool." + call.Name,
				Summary:     string(call.Status),
				ArtifactRef: call.EvidenceRef,
				CreatedAt:   call.CreatedAt,
			})
		}
		for _, result := range testResults {
			facts.EvidenceSummaries = append(facts.EvidenceSummaries, contextbuilder.EvidenceSummary{
				ID:          result.ID,
				TaskID:      task.ID,
				TaskTitle:   task.Title,
				Type:        "test." + result.Name,
				Summary:     string(result.Status) + ": " + truncate(result.Output, 180),
				EvidenceRef: result.EvidenceRef,
			})
			facts.EvidenceRefs = append(facts.EvidenceRefs, contextbuilder.EvidenceRef{
				ID:          result.ID,
				Type:        "test." + result.Name,
				Summary:     string(result.Status),
				ArtifactRef: result.EvidenceRef,
				CreatedAt:   result.CreatedAt,
			})
		}
		for _, review := range reviewResults {
			facts.Decisions = append(facts.Decisions, contextbuilder.DecisionRecord{
				TaskID:      task.ID,
				TaskTitle:   task.Title,
				AttemptID:   attempt.ID,
				Status:      string(review.Status),
				Summary:     truncate(review.Summary, 240),
				EvidenceRef: review.EvidenceRef,
			})
		}
	}
	sortFacts(&facts)
	return facts, nil
}

func sortFacts(facts *taskFacts) {
	sort.SliceStable(facts.Events, func(i, j int) bool {
		return facts.Events[i].CreatedAt.Before(facts.Events[j].CreatedAt) || (facts.Events[i].CreatedAt.Equal(facts.Events[j].CreatedAt) && facts.Events[i].ID < facts.Events[j].ID)
	})
	sort.SliceStable(facts.Attempts, func(i, j int) bool {
		if facts.Attempts[i].Number != facts.Attempts[j].Number {
			return facts.Attempts[i].Number < facts.Attempts[j].Number
		}
		return facts.Attempts[i].ID < facts.Attempts[j].ID
	})
	sort.SliceStable(facts.Observations, func(i, j int) bool {
		return facts.Observations[i].CreatedAt.Before(facts.Observations[j].CreatedAt) || (facts.Observations[i].CreatedAt.Equal(facts.Observations[j].CreatedAt) && facts.Observations[i].ID < facts.Observations[j].ID)
	})
	sort.SliceStable(facts.ToolCalls, func(i, j int) bool {
		return facts.ToolCalls[i].CreatedAt.Before(facts.ToolCalls[j].CreatedAt) || (facts.ToolCalls[i].CreatedAt.Equal(facts.ToolCalls[j].CreatedAt) && facts.ToolCalls[i].ID < facts.ToolCalls[j].ID)
	})
}

func summarizeTask(task domain.Task, facts taskFacts, dependsOn []contextbuilder.TaskLink, blocks []contextbuilder.TaskLink) contextbuilder.TaskSummary {
	return contextbuilder.TaskSummary{
		ID:                task.ID,
		Title:             task.Title,
		Status:            string(task.Status),
		Description:       truncate(task.Description, 180),
		AttemptCount:      len(facts.Attempts),
		DependsOn:         dependsOn,
		Blocks:            blocks,
		LatestObservation: latestObservationSummary(facts),
		LatestEvidenceRef: latestEvidenceRef(facts),
		LatestFailure:     latestFailure(facts),
	}
}

func currentTaskState(currentTaskID string, tasks []domain.Task, factsByTask map[string]taskFacts, dependsOn map[string][]contextbuilder.TaskLink, blocks map[string][]contextbuilder.TaskLink) (contextbuilder.TaskState, []contextbuilder.AcceptanceCriterion) {
	if currentTaskID == "" {
		return contextbuilder.TaskState{}, nil
	}
	var current domain.Task
	found := false
	for _, task := range tasks {
		if task.ID == currentTaskID {
			current = task
			found = true
			break
		}
	}
	if !found {
		return contextbuilder.TaskState{}, nil
	}
	facts := factsByTask[current.ID]
	criteria := make([]contextbuilder.AcceptanceCriterion, 0, len(current.AcceptanceCriteria))
	for index, text := range current.AcceptanceCriteria {
		criteria = append(criteria, contextbuilder.AcceptanceCriterion{
			ID:   fmt.Sprintf("%s-ac-%02d", current.ID, index+1),
			Text: text,
		})
	}
	return contextbuilder.TaskState{
		ID:                  current.ID,
		Title:               current.Title,
		Goal:                current.Title,
		Status:              string(current.Status),
		Description:         current.Description,
		AttemptCount:        len(facts.Attempts),
		DependsOn:           dependsOn[current.ID],
		Blocks:              blocks[current.ID],
		SiblingStatusCounts: siblingStatusCounts(current.ProjectID, current.ID, tasks),
		RecentAttempts:      recentAttempts(facts.Attempts, 4),
		RecentObservations:  limitEvidenceSummaries(facts.EvidenceSummaries, maxTaskObservations),
		RecentFailures:      recentFailures(facts, 5),
	}, criteria
}

func countStatuses(tasks []domain.Task) []contextbuilder.StatusCount {
	counts := map[string]int{}
	for _, task := range tasks {
		counts[string(task.Status)]++
	}
	statuses := make([]string, 0, len(counts))
	for status := range counts {
		statuses = append(statuses, status)
	}
	sort.Strings(statuses)
	result := make([]contextbuilder.StatusCount, 0, len(statuses))
	for _, status := range statuses {
		result = append(result, contextbuilder.StatusCount{Status: status, Count: counts[status]})
	}
	return result
}

func siblingStatusCounts(projectID string, currentTaskID string, tasks []domain.Task) []contextbuilder.StatusCount {
	siblings := make([]domain.Task, 0, len(tasks))
	for _, task := range tasks {
		if task.ProjectID == projectID && task.ID != currentTaskID {
			siblings = append(siblings, task)
		}
	}
	return countStatuses(siblings)
}

func taskLink(task domain.Task) contextbuilder.TaskLink {
	return contextbuilder.TaskLink{ID: task.ID, Title: task.Title, Status: string(task.Status)}
}

func latestObservationSummary(facts taskFacts) string {
	for i := len(facts.Observations) - 1; i >= 0; i-- {
		if strings.TrimSpace(facts.Observations[i].Summary) != "" {
			return truncate(facts.Observations[i].Summary, 220)
		}
	}
	return ""
}

func latestEvidenceRef(facts taskFacts) string {
	for i := len(facts.EvidenceRefs) - 1; i >= 0; i-- {
		if strings.TrimSpace(facts.EvidenceRefs[i].ArtifactRef) != "" {
			return facts.EvidenceRefs[i].ArtifactRef
		}
	}
	return ""
}

func latestFailure(facts taskFacts) string {
	failures := recentFailures(facts, 1)
	if len(failures) == 0 {
		return ""
	}
	return failures[0]
}

func recentFailures(facts taskFacts, limit int) []string {
	var result []string
	for i := len(facts.Events) - 1; i >= 0; i-- {
		event := facts.Events[i]
		if event.ToState == domain.TaskStatusFailed || event.ToState == domain.TaskStatusReviewFailed || strings.Contains(event.Type, "failed") || strings.Contains(event.Type, "rejected") {
			if strings.TrimSpace(event.Message) != "" {
				result = append(result, truncate(event.Message, 240))
			}
		}
		if limit > 0 && len(result) >= limit {
			break
		}
	}
	return result
}

func recentAttempts(attempts []domain.TaskAttempt, limit int) []contextbuilder.AttemptSummary {
	start := 0
	if limit > 0 && len(attempts) > limit {
		start = len(attempts) - limit
	}
	result := make([]contextbuilder.AttemptSummary, 0, len(attempts)-start)
	for _, attempt := range attempts[start:] {
		result = append(result, contextbuilder.AttemptSummary{
			ID:     attempt.ID,
			Number: attempt.Number,
			Status: string(attempt.Status),
			Error:  truncate(attempt.Error, 180),
		})
	}
	return result
}

type eventWithTime struct {
	CreatedAtKey int64
	Event        contextbuilder.EventSummary
}

type evidenceWithTime struct {
	CreatedAtKey int64
	Evidence     contextbuilder.EvidenceSummary
	Ref          contextbuilder.EvidenceRef
}

func newestEvents(events []eventWithTime, limit int) []contextbuilder.EventSummary {
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].CreatedAtKey != events[j].CreatedAtKey {
			return events[i].CreatedAtKey > events[j].CreatedAtKey
		}
		return events[i].Event.Type < events[j].Event.Type
	})
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}
	result := make([]contextbuilder.EventSummary, 0, len(events))
	for _, event := range events {
		result = append(result, event.Event)
	}
	return result
}

func newestEvidence(evidence []evidenceWithTime, limit int) ([]contextbuilder.EvidenceSummary, []contextbuilder.EvidenceRef) {
	sort.SliceStable(evidence, func(i, j int) bool {
		if evidence[i].CreatedAtKey != evidence[j].CreatedAtKey {
			return evidence[i].CreatedAtKey > evidence[j].CreatedAtKey
		}
		return evidence[i].Evidence.ID < evidence[j].Evidence.ID
	})
	if limit > 0 && len(evidence) > limit {
		evidence = evidence[:limit]
	}
	summaries := make([]contextbuilder.EvidenceSummary, 0, len(evidence))
	refs := make([]contextbuilder.EvidenceRef, 0, len(evidence))
	for _, item := range evidence {
		summaries = append(summaries, item.Evidence)
		if item.Ref.ID != "" {
			refs = append(refs, item.Ref)
		}
	}
	return summaries, refs
}

func limitTaskSummaries(tasks []contextbuilder.TaskSummary, limit int) []contextbuilder.TaskSummary {
	sort.SliceStable(tasks, func(i, j int) bool {
		return compareTaskSummary(tasks[i], tasks[j])
	})
	if limit > 0 && len(tasks) > limit {
		return append([]contextbuilder.TaskSummary(nil), tasks[:limit]...)
	}
	return append([]contextbuilder.TaskSummary(nil), tasks...)
}

func compareTaskSummary(left contextbuilder.TaskSummary, right contextbuilder.TaskSummary) bool {
	if left.Status != right.Status {
		return left.Status < right.Status
	}
	if left.Title != right.Title {
		return left.Title < right.Title
	}
	return left.ID < right.ID
}

func limitEvidenceSummaries(items []contextbuilder.EvidenceSummary, limit int) []contextbuilder.EvidenceSummary {
	if limit > 0 && len(items) > limit {
		items = items[len(items)-limit:]
	}
	return append([]contextbuilder.EvidenceSummary(nil), items...)
}

func limitDecisions(items []contextbuilder.DecisionRecord, limit int) []contextbuilder.DecisionRecord {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].TaskTitle != items[j].TaskTitle {
			return items[i].TaskTitle < items[j].TaskTitle
		}
		return items[i].AttemptID < items[j].AttemptID
	})
	if limit > 0 && len(items) > limit {
		return append([]contextbuilder.DecisionRecord(nil), items[:limit]...)
	}
	return append([]contextbuilder.DecisionRecord(nil), items...)
}

func summarizeProject(digest contextbuilder.ProjectDigest) string {
	counts := make([]string, 0, len(digest.StatusCounts))
	for _, count := range digest.StatusCounts {
		counts = append(counts, fmt.Sprintf("%s=%d", count.Status, count.Count))
	}
	problem := ""
	if len(digest.ProblemTasks) > 0 {
		problem = "; problem: " + digest.ProblemTasks[0].Title
	}
	return fmt.Sprintf("%d task(s): %s%s", digest.TaskCount, strings.Join(counts, ", "), problem)
}

func queryText(project domain.Project, taskState contextbuilder.TaskState, digest contextbuilder.ProjectDigest) string {
	parts := []string{project.Goal, taskState.Title, taskState.Description}
	for _, task := range digest.CompletedTasks {
		parts = append(parts, task.Title, task.LatestObservation)
	}
	for _, task := range digest.ProblemTasks {
		parts = append(parts, task.Title, task.LatestFailure)
	}
	for _, decision := range digest.Decisions {
		parts = append(parts, decision.Summary)
	}
	return strings.Join(parts, " ")
}

func renderTaskMemoryBody(task domain.Task, facts taskFacts, review domain.ReviewResult) string {
	lines := []string{
		"Task: " + task.Title,
		"Status: " + string(task.Status),
		"Description: " + task.Description,
		"Review: " + string(review.Status) + " " + review.Summary,
	}
	for _, test := range facts.TestResults {
		lines = append(lines, "Test: "+test.Name+" "+string(test.Status)+" "+truncate(test.Output, 240))
	}
	for _, observation := range facts.Observations {
		lines = append(lines, "Observation: "+observation.Type+" "+truncate(observation.Summary, 240)+" "+observation.EvidenceRef)
	}
	for _, call := range facts.ToolCalls {
		lines = append(lines, "Tool: "+call.Name+" "+string(call.Status)+" "+call.EvidenceRef)
	}
	return strings.Join(lines, "\n")
}

func firstEvidenceRef(facts taskFacts, fallback string) string {
	if strings.TrimSpace(fallback) != "" {
		return fallback
	}
	for i := len(facts.EvidenceRefs) - 1; i >= 0; i-- {
		if strings.TrimSpace(facts.EvidenceRefs[i].ArtifactRef) != "" {
			return facts.EvidenceRefs[i].ArtifactRef
		}
	}
	return ""
}

func newMemoryItem(projectID string, taskID string, attemptID string, typ string, summary string, body string, evidenceRef string, tags []string, confidence float64) memory.Item {
	summary = truncate(strings.TrimSpace(summary), 260)
	body = strings.TrimSpace(body)
	version := stableHash(summary + "\n" + body)
	id := "auto-" + stableHash(projectID + "\n" + taskID + "\n" + attemptID + "\n" + typ + "\n" + summary)[:20]
	return memory.Item{
		Card: memory.Card{
			ID:              id,
			Scope:           "project",
			Type:            typ,
			Tags:            textutil.SortedUniqueStrings(tags),
			Summary:         summary,
			ArtifactRef:     evidenceRef,
			EvidenceRef:     evidenceRef,
			SourceTaskID:    taskID,
			SourceAttemptID: attemptID,
			Confidence:      confidence,
			VersionHash:     version,
		},
		Body: body,
	}
}

func stableHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}
