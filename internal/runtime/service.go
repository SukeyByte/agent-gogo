package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/chain"
	comm "github.com/SukeyByte/agent-gogo/internal/communication"
	"github.com/SukeyByte/agent-gogo/internal/contextbuilder"
	"github.com/SukeyByte/agent-gogo/internal/discovery"
	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/executor"
	"github.com/SukeyByte/agent-gogo/internal/function"
	intentpkg "github.com/SukeyByte/agent-gogo/internal/intent"
	"github.com/SukeyByte/agent-gogo/internal/memory"
	"github.com/SukeyByte/agent-gogo/internal/observability"
	"github.com/SukeyByte/agent-gogo/internal/persona"
	"github.com/SukeyByte/agent-gogo/internal/planner"
	"github.com/SukeyByte/agent-gogo/internal/provider"
	"github.com/SukeyByte/agent-gogo/internal/reviewer"
	"github.com/SukeyByte/agent-gogo/internal/scheduler"
	"github.com/SukeyByte/agent-gogo/internal/skill"
	"github.com/SukeyByte/agent-gogo/internal/taskaware"
	"github.com/SukeyByte/agent-gogo/internal/tester"
	"github.com/SukeyByte/agent-gogo/internal/validator"
)

type Store interface {
	CreateProject(ctx context.Context, project domain.Project) (domain.Project, error)
	GetProject(ctx context.Context, id string) (domain.Project, error)
	UpdateProjectStatus(ctx context.Context, projectID string, status domain.ProjectStatus) (domain.Project, error)
	CreateTask(ctx context.Context, task domain.Task) (domain.Task, error)
	GetTask(ctx context.Context, id string) (domain.Task, error)
	CreateTaskDependency(ctx context.Context, dependency domain.TaskDependency) (domain.TaskDependency, error)
	TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error)
	ListTasksByProject(ctx context.Context, projectID string) ([]domain.Task, error)
	ListTaskDependenciesByProject(ctx context.Context, projectID string) ([]domain.TaskDependency, error)
	CreateTaskAttempt(ctx context.Context, taskID string) (domain.TaskAttempt, error)
	ListTaskAttemptsByTask(ctx context.Context, taskID string) ([]domain.TaskAttempt, error)
	CompleteTaskAttempt(ctx context.Context, attemptID string, status domain.AttemptStatus, message string) (domain.TaskAttempt, error)
	CreateObservation(ctx context.Context, observation domain.Observation) (domain.Observation, error)
	ListObservationsByAttempt(ctx context.Context, attemptID string) ([]domain.Observation, error)
	CreateTestResult(ctx context.Context, result domain.TestResult) (domain.TestResult, error)
	ListTestResultsByAttempt(ctx context.Context, attemptID string) ([]domain.TestResult, error)
	CreateReviewResult(ctx context.Context, result domain.ReviewResult) (domain.ReviewResult, error)
	ListReviewResultsByAttempt(ctx context.Context, attemptID string) ([]domain.ReviewResult, error)
	ListToolCallsByAttempt(ctx context.Context, attemptID string) ([]domain.ToolCall, error)
	ListArtifactsByProject(ctx context.Context, projectID string) ([]domain.Artifact, error)
	AddTaskEvent(ctx context.Context, event domain.TaskEvent) (domain.TaskEvent, error)
	ListTaskEvents(ctx context.Context, taskID string) ([]domain.TaskEvent, error)
}

type CommunicationDispatcher interface {
	Dispatch(ctx context.Context, intent comm.CommunicationIntent) (comm.DeliveryReceipt, error)
}

type SessionContextSaver interface {
	SaveSessionRuntimeContext(ctx context.Context, sctx domain.SessionRuntimeContext) error
}

type Service struct {
	store                Store
	planner              planner.Planner
	validator            validator.TaskValidator
	scheduler            scheduler.Scheduler
	executor             executor.Executor
	tester               tester.Tester
	reviewer             reviewer.Reviewer
	chainRouter          chain.Router
	intentAnalyzer       intentpkg.Analyzer
	communication        CommunicationDispatcher
	communicationChannel string
	communicationSession string
	sessionSaver         SessionContextSaver
	sessionID            string
	functions            function.Registry
	skills               *skill.Registry
	personas             *persona.Registry
	memories             *memory.Index
	memoryPersistPath    string
	contextSerializer    contextbuilder.ContextSerializer
	logger               observability.Logger
	activePersonas       []contextbuilder.Persona
	contextByProjectID   map[string]string
	decisionByProjectID  map[string]chain.Decision
	profileByProjectID   map[string]intentpkg.Profile
	contextMaxChars      int
	discovery            discovery.Loop
}

type CreateProjectRequest struct {
	Name string
	Goal string
}

type ChannelEvent struct {
	Type      string
	ChannelID string
	SessionID string
	ProjectID string
	TaskID    string
	Text      string
	Payload   map[string]string
}

type UserConfirmation struct {
	ConfirmationID string
	ProjectID      string
	TaskID         string
	AttemptID      string
	ActionID       string
	Approved       bool
	Message        string
}

type TaskRunResult struct {
	ProjectID    string
	Task         domain.Task
	Attempt      domain.TaskAttempt
	TestResult   domain.TestResult
	ReviewResult domain.ReviewResult
	Events       []domain.TaskEvent
}

func NewService(store Store) *Service {
	return &Service{
		store:               store,
		planner:             planner.NewFixedPlanner(),
		validator:           validator.NewMinimalTaskValidator(),
		scheduler:           scheduler.NewReadyScheduler(store),
		executor:            executor.NewMinimalExecutor(store),
		tester:              tester.NewGenericEvidenceTester(store),
		reviewer:            reviewer.NewEvidenceReviewer(store),
		memories:            memory.NewIndex(),
		contextByProjectID:  map[string]string{},
		decisionByProjectID: map[string]chain.Decision{},
		profileByProjectID:  map[string]intentpkg.Profile{},
	}
}

func NewServiceWithComponents(
	store Store,
	planner planner.Planner,
	validator validator.TaskValidator,
	scheduler scheduler.Scheduler,
	executor executor.Executor,
	tester tester.Tester,
	reviewer reviewer.Reviewer,
) *Service {
	return &Service{
		store:               store,
		planner:             planner,
		validator:           validator,
		scheduler:           scheduler,
		executor:            executor,
		tester:              tester,
		reviewer:            reviewer,
		memories:            memory.NewIndex(),
		contextByProjectID:  map[string]string{},
		decisionByProjectID: map[string]chain.Decision{},
		profileByProjectID:  map[string]intentpkg.Profile{},
	}
}

func (s *Service) UseCommunication(channelID string, sessionID string, dispatcher CommunicationDispatcher) {
	s.communicationChannel = channelID
	s.communicationSession = sessionID
	s.communication = dispatcher
}

func (s *Service) UseLLM(llm provider.LLMProvider, model string) {
	s.chainRouter = chain.NewLLMRouter(llm, model)
	s.intentAnalyzer = intentpkg.NewLLMAnalyzer(llm, model)
	s.planner = planner.NewLLMPlanner(llm, model)
}

func (s *Service) UseContextAssets(functions function.Registry, skills *skill.Registry, personas *persona.Registry, memories *memory.Index, serializer contextbuilder.ContextSerializer, logger observability.Logger) {
	s.functions = functions
	s.skills = skills
	s.personas = personas
	s.memories = memories
	s.contextSerializer = serializer
	s.logger = logger
}

func (s *Service) UseMemoryPersistence(path string) {
	s.memoryPersistPath = strings.TrimSpace(path)
}

func (s *Service) UseContextBudget(maxChars int) {
	s.contextMaxChars = maxChars
}

func (s *Service) UseDiscoveryLoop(loop discovery.Loop) {
	s.discovery = loop
}

func (s *Service) UseSession(saver SessionContextSaver, sessionID string) {
	s.sessionSaver = saver
	s.sessionID = strings.TrimSpace(sessionID)
}

func (s *Service) AddActivePersona(persona contextbuilder.Persona) {
	s.activePersonas = append(s.activePersonas, persona)
}

func (s *Service) CreateProject(ctx context.Context, req CreateProjectRequest) (domain.Project, error) {
	if err := ctx.Err(); err != nil {
		return domain.Project{}, err
	}
	project, err := s.store.CreateProject(ctx, domain.Project{
		Name: req.Name,
		Goal: req.Goal,
	})
	if err != nil {
		return domain.Project{}, err
	}
	if err := s.emit(ctx, comm.NewMessageIntent(s.communicationChannel, fmt.Sprintf("Project created: %s", project.Name)), project.ID); err != nil {
		return domain.Project{}, err
	}
	return project, nil
}

func (s *Service) PlanProject(ctx context.Context, projectID string) ([]domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	chainDecision, err := s.routeProject(ctx, project)
	if err != nil {
		return nil, err
	}
	if err := s.log(ctx, "chain.router", chainDecision); err != nil {
		return nil, err
	}
	intentProfile, err := s.analyzeProject(ctx, project, chainDecision)
	if err != nil {
		return nil, err
	}
	if err := s.log(ctx, "intent.analyze", intentProfile); err != nil {
		return nil, err
	}
	planningContext, err := s.buildRuntimeContext(ctx, project, "", chainDecision, intentProfile)
	if err != nil {
		return nil, err
	}
	discoveryResult, err := s.runDiscovery(ctx, project, chainDecision, intentProfile)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(discoveryResult.Summary) != "" {
		planningContext = appendPlanningDiscovery(planningContext, discoveryResult)
		planningContext = limitContextText(planningContext, s.contextMaxChars)
	}
	if s.contextByProjectID == nil {
		s.contextByProjectID = map[string]string{}
	}
	s.contextByProjectID[project.ID] = planningContext
	if s.decisionByProjectID == nil {
		s.decisionByProjectID = map[string]chain.Decision{}
	}
	if s.profileByProjectID == nil {
		s.profileByProjectID = map[string]intentpkg.Profile{}
	}
	s.decisionByProjectID[project.ID] = chainDecision
	s.profileByProjectID[project.ID] = intentProfile
	drafts, err := s.planner.PlanProject(ctx, planner.PlanRequest{
		Project:       project,
		UserInput:     project.Goal,
		ChainDecision: chainDecision,
		IntentProfile: intentProfile,
		ContextText:   planningContext,
	})
	if err != nil {
		return nil, err
	}
	drafts = normalizeTaskDependencies(drafts)
	if err := s.log(ctx, "planner.tasks", drafts); err != nil {
		return nil, err
	}

	type plannedTask struct {
		draft   domain.Task
		created domain.Task
	}
	planned := make([]plannedTask, 0, len(drafts))
	titleToID := map[string]string{}
	for _, draft := range drafts {
		draft.ProjectID = project.ID
		created, err := s.store.CreateTask(ctx, draft)
		if err != nil {
			return nil, err
		}
		planned = append(planned, plannedTask{draft: draft, created: created})
		titleToID[created.Title] = created.ID
	}
	for _, item := range planned {
		for _, dependsOn := range item.draft.DependsOn {
			dependencyID, ok := titleToID[dependsOn]
			if !ok {
				return nil, fmt.Errorf("planned task %q depends on unknown task %q", item.created.Title, dependsOn)
			}
			if dependencyID == item.created.ID {
				return nil, fmt.Errorf("planned task %q cannot depend on itself", item.created.Title)
			}
			if _, err := s.store.CreateTaskDependency(ctx, domain.TaskDependency{
				TaskID:          item.created.ID,
				DependsOnTaskID: dependencyID,
			}); err != nil {
				return nil, err
			}
		}
	}

	readyTasks := make([]domain.Task, 0, len(planned))
	validationErrors := make([]string, 0)
	for _, item := range planned {
		created := item.created
		if err := s.validator.ValidateTask(ctx, created); err != nil {
			blocked, transitionErr := s.store.TransitionTask(ctx, created.ID, domain.TaskStatusBlocked, "capability validation blocked planned task: "+err.Error())
			if transitionErr != nil {
				return nil, transitionErr
			}
			s.emitTaskBlocked(ctx, project.ID, blocked, "Task blocked by capability validation: "+created.Title+" - "+err.Error())
			validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", created.Title, err.Error()))
			continue
		}
		ready, err := s.store.TransitionTask(ctx, created.ID, domain.TaskStatusReady, "validator accepted planned task")
		if err != nil {
			return nil, err
		}
		readyTasks = append(readyTasks, ready)
	}
	if err := s.emit(ctx, comm.NewMessageIntent(s.communicationChannel, fmt.Sprintf("Planned %d task(s)", len(readyTasks))), project.ID); err != nil {
		return nil, err
	}
	if len(validationErrors) > 0 {
		if err := s.emit(ctx, comm.NewBlockedIntent(s.communicationChannel, fmt.Sprintf("Blocked %d planned task(s) by capability policy", len(validationErrors)), map[string]any{
			"project_id": project.ID,
			"status":     string(domain.TaskStatusBlocked),
			"errors":     validationErrors,
		}), project.ID); err != nil {
			return nil, err
		}
		if len(readyTasks) == 0 {
			return nil, fmt.Errorf("no runnable tasks after capability validation: %s", strings.Join(validationErrors, "; "))
		}
	}
	s.saveSessionContext(ctx, project.ID)
	return readyTasks, nil
}

func normalizeTaskDependencies(tasks []domain.Task) []domain.Task {
	if len(tasks) == 0 {
		return tasks
	}
	out := append([]domain.Task(nil), tasks...)
	titleSet := map[string]struct{}{}
	for _, task := range out {
		if title := strings.TrimSpace(task.Title); title != "" {
			titleSet[title] = struct{}{}
		}
	}
	researchTitle := firstDomainTaskTitleMatching(out, []string{"研究", "調研", "调研", "research", "context", "gather", "收集", "获取", "取得", "读取", "搜索"})
	reflectionTitle := firstDomainTaskTitleMatching(out, []string{"反思", "reflection", "review plan", "验收口径", "驗收口徑", "decomposition", "acceptance criteria"})
	finalMarkers := []string{"最终", "最終", "验收", "驗收", "验证", "驗證", "检查", "檢查", "自检", "自檢", "自查", "复核", "覆核", "完整性", "汇报", "匯報", "总结", "總結", "汇总", "彙總", "final", "verify", "validate", "validation", "self-check", "self check", "selfcheck", "correctness", "completeness", "report", "summary"}
	taskByTitle := map[string]domain.Task{}
	finalByTitle := map[string]bool{}
	for _, task := range out {
		title := strings.TrimSpace(task.Title)
		if title == "" {
			continue
		}
		taskByTitle[title] = task
		finalByTitle[title] = title != researchTitle && title != reflectionTitle && domainTaskMatches(task, finalMarkers)
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
					if domainTaskMatches(candidate, finalMarkers) {
						continue
					}
					addDep(candidate.Title)
				}
			}
		}
		out[i].DependsOn = deps
	}
	return pruneDomainDependencyCycles(out)
}

func firstDomainTaskTitleMatching(tasks []domain.Task, markers []string) string {
	for _, task := range tasks {
		if domainTaskMatches(task, markers) && strings.TrimSpace(task.Title) != "" {
			return task.Title
		}
	}
	return ""
}

func domainTaskMatches(task domain.Task, markers []string) bool {
	text := strings.ToLower(task.Title + " " + task.Description + " " + strings.Join(task.AcceptanceCriteria, " "))
	for _, marker := range markers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func pruneDomainDependencyCycles(tasks []domain.Task) []domain.Task {
	out := append([]domain.Task(nil), tasks...)
	depsByTitle := map[string][]string{}
	for _, task := range out {
		depsByTitle[task.Title] = append([]string(nil), task.DependsOn...)
	}
	for i := range out {
		title := strings.TrimSpace(out[i].Title)
		kept := out[i].DependsOn[:0]
		for _, dep := range out[i].DependsOn {
			depsByTitle[title] = kept
			if domainDependencyReaches(depsByTitle, dep, title, map[string]bool{}) {
				continue
			}
			kept = append(kept, dep)
		}
		out[i].DependsOn = kept
		depsByTitle[title] = append([]string(nil), kept...)
	}
	return out
}

func domainDependencyReaches(depsByTitle map[string][]string, from string, target string, seen map[string]bool) bool {
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
		if domainDependencyReaches(depsByTitle, next, target, seen) {
			return true
		}
	}
	return false
}

func (s *Service) runDiscovery(ctx context.Context, project domain.Project, decision chain.Decision, profile intentpkg.Profile) (discovery.Result, error) {
	if s.discovery == nil {
		return discovery.Result{}, nil
	}
	result, err := s.discovery.Discover(ctx, discovery.Request{
		Project:       project,
		ChainDecision: decision,
		IntentProfile: profile,
	})
	if err != nil {
		return discovery.Result{}, err
	}
	if err := s.log(ctx, "discovery.preplan", result); err != nil {
		return discovery.Result{}, err
	}
	return result, nil
}

func appendPlanningDiscovery(contextText string, result discovery.Result) string {
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		return contextText
	}
	if strings.TrimSpace(contextText) == "" {
		return "[DISCOVERY]\n" + summary
	}
	return contextText + "\n\n[DISCOVERY]\n" + summary
}

func (s *Service) routeProject(ctx context.Context, project domain.Project) (chain.Decision, error) {
	if s.chainRouter == nil {
		return chain.Decision{}, nil
	}
	return s.chainRouter.Route(ctx, chain.Request{
		UserInput: project.Goal,
		ProjectID: project.ID,
		Channel:   s.communicationChannel,
	})
}

func (s *Service) analyzeProject(ctx context.Context, project domain.Project, decision chain.Decision) (intentpkg.Profile, error) {
	if s.intentAnalyzer == nil {
		return intentpkg.Profile{}, nil
	}
	return s.intentAnalyzer.Analyze(ctx, intentpkg.Request{
		UserInput:     project.Goal,
		ChainDecision: decision,
	})
}

func (s *Service) RunNextTask(ctx context.Context, projectID string) (TaskRunResult, error) {
	if err := ctx.Err(); err != nil {
		return TaskRunResult{}, err
	}
	task, err := s.scheduler.NextReadyTask(ctx, projectID)
	if err != nil {
		return TaskRunResult{}, err
	}
	if err := s.log(ctx, "scheduler.ready", task); err != nil {
		return TaskRunResult{}, err
	}
	if err := s.emitTaskProgress(ctx, projectID, task, domain.TaskStatusInProgress, "Task started: "+task.Title); err != nil {
		return TaskRunResult{}, err
	}
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return TaskRunResult{}, err
	}
	decision := s.decisionByProjectID[projectID]
	profile := s.profileByProjectID[projectID]
	taskContext, err := s.buildRuntimeContext(ctx, project, task.ID, decision, profile)
	if err != nil {
		return TaskRunResult{}, err
	}
	if s.contextByProjectID == nil {
		s.contextByProjectID = map[string]string{}
	}
	s.contextByProjectID[projectID] = taskContext
	if consumer, ok := s.executor.(runtimeContextConsumer); ok {
		consumer.UseRuntimeContext(projectID, s.contextByProjectID[projectID])
	}
	executed, err := s.executor.Execute(ctx, task)
	if err != nil {
		var executionErr *executor.ExecutionError
		if errors.As(err, &executionErr) && executionErr.Attempt.ID != "" {
			if _, repairErr := s.createRepairTask(ctx, projectID, executionErr.Task, executionErr.Attempt, "executor.failed", err); repairErr != nil {
				err = errors.Join(err, repairErr)
			}
		}
		s.emitTaskBlocked(ctx, projectID, task, "Task failed during execution: "+err.Error())
		return TaskRunResult{}, err
	}
	if err := s.log(ctx, "executor.result", executed); err != nil {
		return TaskRunResult{}, err
	}
	if err := s.emitTaskProgress(ctx, projectID, executed.Task, domain.TaskStatusImplemented, "Task implemented: "+executed.Task.Title); err != nil {
		return TaskRunResult{}, err
	}
	if err := s.emitTaskProgress(ctx, projectID, executed.Task, domain.TaskStatusTesting, "Task testing: "+executed.Task.Title); err != nil {
		return TaskRunResult{}, err
	}
	tested, err := s.tester.Test(ctx, executed.Task, executed.Attempt)
	if err != nil {
		if _, repairErr := s.createRepairTask(ctx, projectID, executed.Task, executed.Attempt, "tester.failed", err); repairErr != nil {
			err = errors.Join(err, repairErr)
		}
		s.emitTaskBlocked(ctx, projectID, executed.Task, "Task failed during testing: "+err.Error())
		return TaskRunResult{}, err
	}
	if err := s.log(ctx, "tester.result", tested); err != nil {
		return TaskRunResult{}, err
	}
	if err := s.emitTaskProgress(ctx, projectID, tested.Task, domain.TaskStatusReviewing, "Task reviewing: "+tested.Task.Title); err != nil {
		return TaskRunResult{}, err
	}
	reviewed, err := s.reviewer.Review(ctx, tested.Task, executed.Attempt)
	if err != nil {
		if _, repairErr := s.createRepairTask(ctx, projectID, tested.Task, executed.Attempt, "reviewer.rejected", err); repairErr != nil {
			err = errors.Join(err, repairErr)
		}
		s.emitTaskBlocked(ctx, projectID, tested.Task, "Task failed during review: "+err.Error())
		return TaskRunResult{}, err
	}
	if err := s.log(ctx, "reviewer.result", reviewed); err != nil {
		return TaskRunResult{}, err
	}
	if err := s.extractTaskMemories(ctx, projectID, reviewed.Task, reviewed.Attempt, reviewed.ReviewResult); err != nil {
		return TaskRunResult{}, err
	}
	if err := s.completeRepairedTask(ctx, projectID, reviewed.Task); err != nil {
		return TaskRunResult{}, err
	}
	events, err := s.store.ListTaskEvents(ctx, reviewed.Task.ID)
	if err != nil {
		return TaskRunResult{}, err
	}
	result := TaskRunResult{
		ProjectID:    projectID,
		Task:         reviewed.Task,
		Attempt:      reviewed.Attempt,
		TestResult:   tested.TestResult,
		ReviewResult: reviewed.ReviewResult,
		Events:       events,
	}
	if err := s.emit(ctx, comm.NewDoneIntent(s.communicationChannel, fmt.Sprintf("Task done: %s", reviewed.Task.Title)), projectID); err != nil {
		return TaskRunResult{}, err
	}
	s.saveSessionContext(ctx, projectID)
	return result, nil
}

func (s *Service) RunProjectTasks(ctx context.Context, projectID string, maxTasks int) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if maxTasks <= 0 {
		maxTasks = 50
	}
	if err := s.emit(ctx, comm.NewProgressIntent(s.communicationChannel, "Project run started", map[string]any{
		"project_id": projectID,
		"status":     "RUNNING",
	}), projectID); err != nil {
		return 0, err
	}
	ranTasks := 0
	iterations := 0
	for iterations < maxTasks {
		iterations++
		result, err := s.RunNextTask(ctx, projectID)
		if errors.Is(err, sql.ErrNoRows) {
			if blocked, blockErr := s.blockTasksWaitingOnBlockedDependencies(ctx, projectID); blockErr != nil {
				return ranTasks, blockErr
			} else if blocked > 0 {
				continue
			}
			if summary, incomplete, summaryErr := s.projectIncompleteSummary(ctx, projectID); summaryErr != nil {
				return ranTasks, summaryErr
			} else if incomplete {
				s.emitProjectBlocked(ctx, projectID, fmt.Sprintf("Project run paused after %d task(s): %s", ranTasks, summary))
				return ranTasks, nil
			}
			if _, err := s.store.UpdateProjectStatus(ctx, projectID, domain.ProjectStatusCompleted); err != nil {
				return ranTasks, err
			}
			if err := s.emit(ctx, comm.NewDoneIntent(s.communicationChannel, fmt.Sprintf("Project run finished: %d task(s) completed", ranTasks)), projectID); err != nil {
				return ranTasks, err
			}
			return ranTasks, nil
		}
		if err != nil {
			if hasReady, readyErr := s.hasReadyTask(ctx, projectID); readyErr != nil {
				return ranTasks, readyErr
			} else if hasReady {
				_ = s.emit(ctx, comm.NewProgressIntent(s.communicationChannel, "Project run continuing with recovery task after failure", map[string]any{
					"project_id": projectID,
					"status":     "RECOVERING",
					"error":      err.Error(),
				}), projectID)
				continue
			}
			s.emitProjectBlocked(ctx, projectID, fmt.Sprintf("Project run stopped after %d task(s): %s", ranTasks, err.Error()))
			return ranTasks, err
		}
		ranTasks++
		if result.Task.Status == domain.TaskStatusDone {
			continue
		}
	}
	err := fmt.Errorf("max task limit reached: %d", maxTasks)
	s.emitProjectBlocked(ctx, projectID, err.Error())
	return ranTasks, err
}

func (s *Service) hasReadyTask(ctx context.Context, projectID string) (bool, error) {
	_, err := s.scheduler.NextReadyTask(ctx, projectID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Service) blockTasksWaitingOnBlockedDependencies(ctx context.Context, projectID string) (int, error) {
	tasks, err := s.store.ListTasksByProject(ctx, projectID)
	if err != nil {
		return 0, err
	}
	dependencies, err := s.store.ListTaskDependenciesByProject(ctx, projectID)
	if err != nil {
		return 0, err
	}
	statusByID := make(map[string]domain.TaskStatus, len(tasks))
	taskByID := make(map[string]domain.Task, len(tasks))
	for _, task := range tasks {
		statusByID[task.ID] = task.Status
		taskByID[task.ID] = task
	}
	dependencyTitles := map[string][]string{}
	for _, dependency := range dependencies {
		if !dependencyBlocksDependents(statusByID[dependency.DependsOnTaskID]) {
			continue
		}
		if blockedDependency, ok := taskByID[dependency.DependsOnTaskID]; ok {
			dependencyTitles[dependency.TaskID] = append(dependencyTitles[dependency.TaskID], blockedDependency.Title)
		} else {
			dependencyTitles[dependency.TaskID] = append(dependencyTitles[dependency.TaskID], dependency.DependsOnTaskID)
		}
	}
	blocked := 0
	for _, task := range tasks {
		if task.Status != domain.TaskStatusReady && task.Status != domain.TaskStatusDraft {
			continue
		}
		titles := dependencyTitles[task.ID]
		if len(titles) == 0 {
			continue
		}
		reason := "blocked dependency cannot complete: " + strings.Join(titles, ", ")
		updated, err := s.store.TransitionTask(ctx, task.ID, domain.TaskStatusBlocked, reason)
		if err != nil {
			return blocked, err
		}
		s.emitTaskBlocked(ctx, projectID, updated, "Task blocked by dependency: "+task.Title+" - "+reason)
		blocked++
	}
	return blocked, nil
}

func dependencyBlocksDependents(status domain.TaskStatus) bool {
	switch status {
	case domain.TaskStatusBlocked, domain.TaskStatusNeedUserInput, domain.TaskStatusFailed, domain.TaskStatusReviewFailed, domain.TaskStatusCancelled:
		return true
	default:
		return false
	}
}

func (s *Service) projectIncompleteSummary(ctx context.Context, projectID string) (string, bool, error) {
	tasks, err := s.store.ListTasksByProject(ctx, projectID)
	if err != nil {
		return "", false, err
	}
	counts := map[domain.TaskStatus]int{}
	incomplete := 0
	for _, task := range tasks {
		if task.Status == domain.TaskStatusDone || task.Status == domain.TaskStatusCancelled {
			continue
		}
		incomplete++
		counts[task.Status]++
	}
	if incomplete == 0 {
		return "", false, nil
	}
	order := []domain.TaskStatus{
		domain.TaskStatusDraft,
		domain.TaskStatusReady,
		domain.TaskStatusInProgress,
		domain.TaskStatusImplemented,
		domain.TaskStatusTesting,
		domain.TaskStatusReviewing,
		domain.TaskStatusBlocked,
		domain.TaskStatusNeedUserInput,
		domain.TaskStatusReviewFailed,
		domain.TaskStatusFailed,
	}
	parts := make([]string, 0, len(counts))
	for _, status := range order {
		if count := counts[status]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", status, count))
		}
	}
	return fmt.Sprintf("%d incomplete task(s) remain (%s)", incomplete, strings.Join(parts, ", ")), true, nil
}

func (s *Service) RetryTask(ctx context.Context, taskID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	task, err := s.store.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	switch task.Status {
	case domain.TaskStatusReady:
		_, err = s.store.AddTaskEvent(ctx, domain.TaskEvent{
			TaskID:  task.ID,
			Type:    "runtime.retry_requested",
			Message: "task is already ready",
		})
		return err
	case domain.TaskStatusDraft:
		_, err = s.store.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "retry requested from draft")
		return err
	case domain.TaskStatusBlocked, domain.TaskStatusNeedUserInput, domain.TaskStatusReviewFailed, domain.TaskStatusFailed:
		_, err = s.store.TransitionTask(ctx, task.ID, domain.TaskStatusReady, "retry requested")
		return err
	default:
		return fmt.Errorf("task %s cannot be retried from %s", task.ID, task.Status)
	}
}

func (s *Service) ReplanProject(ctx context.Context, projectID string, reason string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "replan requested"
	}
	if err := s.log(ctx, "runtime.replan", map[string]string{"project_id": project.ID, "reason": reason}); err != nil {
		return err
	}
	if err := s.emit(ctx, comm.NewMessageIntent(s.communicationChannel, "Replanning project: "+reason), project.ID); err != nil {
		return err
	}
	_, err = s.PlanProject(ctx, project.ID)
	return err
}

func (s *Service) HandleChannelEvent(ctx context.Context, event ChannelEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	switch strings.TrimSpace(event.Type) {
	case "goal.submitted":
		name := strings.TrimSpace(event.Payload["name"])
		if name == "" {
			name = "Channel project"
		}
		project, err := s.CreateProject(ctx, CreateProjectRequest{Name: name, Goal: event.Text})
		if err != nil {
			return err
		}
		if _, err = s.PlanProject(ctx, project.ID); err != nil {
			s.emitProjectBlocked(ctx, project.ID, "Project blocked during planning: "+err.Error())
			return nil
		}
		go s.runProjectTasksInBackground(project.ID)
		return nil
	case "task.retry":
		return s.RetryTask(ctx, event.TaskID)
	case "project.replan":
		return s.ReplanProject(ctx, event.ProjectID, event.Text)
	default:
		return s.log(ctx, "runtime.channel_event", event)
	}
}

func (s *Service) runProjectTasksInBackground(projectID string) {
	if _, err := s.RunProjectTasks(context.Background(), projectID, 0); err != nil {
		_ = s.log(context.Background(), "runtime.project_run_failed", map[string]string{
			"project_id": projectID,
			"error":      err.Error(),
		})
	}
}

func (s *Service) HandleUserConfirmation(ctx context.Context, confirmation UserConfirmation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	task, err := s.store.GetTask(ctx, confirmation.TaskID)
	if err != nil {
		return err
	}
	message := strings.TrimSpace(confirmation.Message)
	if message == "" {
		message = "user confirmation received"
	}
	decision := "rejected"
	if confirmation.Approved {
		decision = "approved"
	}
	if _, err := s.store.AddTaskEvent(ctx, domain.TaskEvent{
		TaskID:    task.ID,
		AttemptID: confirmation.AttemptID,
		Type:      "user.confirmation." + decision,
		Message:   message,
		Payload:   fmt.Sprintf(`{"confirmation_id":%q,"action_id":%q}`, confirmation.ConfirmationID, confirmation.ActionID),
	}); err != nil {
		return err
	}
	if confirmation.Approved && task.Status == domain.TaskStatusNeedUserInput {
		_, err = s.store.TransitionTask(ctx, task.ID, domain.TaskStatusReady, message)
		return err
	}
	if !confirmation.Approved && task.Status == domain.TaskStatusNeedUserInput {
		_, err = s.store.TransitionTask(ctx, task.ID, domain.TaskStatusBlocked, message)
		return err
	}
	return nil
}

type runtimeContextConsumer interface {
	UseRuntimeContext(projectID string, contextText string)
}

const maxRepairDepth = 3

func (s *Service) createRepairTask(ctx context.Context, projectID string, failedTask domain.Task, attempt domain.TaskAttempt, eventType string, cause error) (domain.Task, error) {
	message := strings.TrimSpace(cause.Error())
	if message == "" {
		message = "runtime verification failed"
	}
	if latest, err := s.store.GetTask(ctx, failedTask.ID); err == nil {
		failedTask = latest
	}
	repairTargetID := failedTask.ID
	if rootID, err := s.rootRepairTargetID(ctx, failedTask.ID); err != nil {
		return domain.Task{}, err
	} else if rootID != "" {
		repairTargetID = rootID
	}
	if failedTask.Status != domain.TaskStatusFailed && failedTask.Status != domain.TaskStatusDone && failedTask.Status != domain.TaskStatusCancelled {
		if domain.CanTransitionTask(failedTask.Status, domain.TaskStatusFailed) {
			transitioned, err := s.store.TransitionTask(ctx, failedTask.ID, domain.TaskStatusFailed, message)
			if err != nil {
				return domain.Task{}, err
			}
			failedTask = transitioned
		}
	}
	if _, err := s.store.AddTaskEvent(ctx, domain.TaskEvent{
		TaskID:    failedTask.ID,
		AttemptID: attempt.ID,
		Type:      eventType,
		Message:   message,
		Payload:   fmt.Sprintf(`{"failed_task_id":%q}`, repairTargetID),
	}); err != nil {
		return domain.Task{}, err
	}
	if s.memories != nil {
		item := taskaware.FailureMemory(projectID, failedTask, attempt, eventType, message)
		s.memories.Add(item)
		if err := s.persistMemories(ctx); err != nil {
			return domain.Task{}, err
		}
		_ = s.log(ctx, "memory.extract", map[string]any{
			"project_id": projectID,
			"task_id":    failedTask.ID,
			"memory_id":  item.ID,
			"type":       item.Type,
		})
	}
	if depth := repairDepth(failedTask.Title); depth >= maxRepairDepth {
		limitErr := fmt.Errorf("repair limit reached for task %q after %d nested repair attempt(s)", failedTask.Title, depth)
		if _, err := s.store.AddTaskEvent(ctx, domain.TaskEvent{
			TaskID:    failedTask.ID,
			AttemptID: attempt.ID,
			Type:      "repair.limit_reached",
			Message:   limitErr.Error(),
			Payload:   fmt.Sprintf(`{"failed_task_id":%q,"max_depth":%d}`, repairTargetID, maxRepairDepth),
		}); err != nil {
			return domain.Task{}, err
		}
		return domain.Task{}, limitErr
	}
	repair, err := s.store.CreateTask(ctx, domain.Task{
		ProjectID:   projectID,
		Title:       "Fix: " + failedTask.Title,
		Description: "Repair failed task after " + eventType + ": " + message + "\nOriginal task acceptance criteria: " + strings.Join(failedTask.AcceptanceCriteria, "; "),
		Status:      domain.TaskStatusDraft,
		AcceptanceCriteria: []string{
			"Failure evidence is understood or determined obsolete",
			"Original task acceptance criteria are now satisfied or a targeted fix is applied",
			"Original task can continue after repair",
		},
	})
	if err != nil {
		return domain.Task{}, err
	}
	if _, err := s.store.AddTaskEvent(ctx, domain.TaskEvent{
		TaskID:  repair.ID,
		Type:    "repair.linked",
		Message: "repair task linked to failed task",
		Payload: fmt.Sprintf(`{"failed_task_id":%q}`, repairTargetID),
	}); err != nil {
		return domain.Task{}, err
	}
	repair, err = s.store.TransitionTask(ctx, repair.ID, domain.TaskStatusReady, "repair task generated after failure")
	return repair, err
}

func repairDepth(title string) int {
	depth := 0
	title = strings.TrimSpace(title)
	for strings.HasPrefix(title, "Fix: ") {
		depth++
		title = strings.TrimSpace(strings.TrimPrefix(title, "Fix: "))
	}
	return depth
}

func (s *Service) completeRepairedTask(ctx context.Context, projectID string, repairTask domain.Task) error {
	current := repairTask
	seen := map[string]bool{}
	for i := 0; i < 16; i++ {
		if current.ID == "" || seen[current.ID] {
			return nil
		}
		seen[current.ID] = true
		failedTaskID, err := s.repairTargetID(ctx, current.ID)
		if err != nil {
			return err
		}
		if failedTaskID == "" {
			return nil
		}
		failedTask, err := s.store.GetTask(ctx, failedTaskID)
		if err != nil {
			return err
		}
		switch failedTask.Status {
		case domain.TaskStatusFailed, domain.TaskStatusReviewFailed:
			updated, err := s.store.TransitionTask(ctx, failedTask.ID, domain.TaskStatusDone, "completed by repair task: "+repairTask.Title)
			if err != nil {
				return err
			}
			if err := s.emitTaskProgress(ctx, projectID, updated, domain.TaskStatusDone, "Task repaired and marked done: "+updated.Title); err != nil {
				return err
			}
			if err := s.completeSupersededRepairTasks(ctx, projectID, updated.ID, repairTask.ID); err != nil {
				return err
			}
			current = updated
		default:
			current = failedTask
		}
	}
	return nil
}

func (s *Service) completeSupersededRepairTasks(ctx context.Context, projectID string, repairedTaskID string, successfulRepairTaskID string) error {
	tasks, err := s.store.ListTasksByProject(ctx, projectID)
	if err != nil {
		return err
	}
	for _, task := range tasks {
		if task.ID == successfulRepairTaskID {
			continue
		}
		switch task.Status {
		case domain.TaskStatusFailed, domain.TaskStatusReviewFailed:
		default:
			continue
		}
		targetID, err := s.repairTargetID(ctx, task.ID)
		if err != nil {
			return err
		}
		if targetID != repairedTaskID {
			continue
		}
		updated, err := s.store.TransitionTask(ctx, task.ID, domain.TaskStatusDone, "superseded by successful repair task")
		if err != nil {
			return err
		}
		if err := s.emitTaskProgress(ctx, projectID, updated, domain.TaskStatusDone, "Superseded repair task marked done: "+updated.Title); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) repairTargetID(ctx context.Context, repairTaskID string) (string, error) {
	events, err := s.store.ListTaskEvents(ctx, repairTaskID)
	if err != nil {
		return "", err
	}
	for _, event := range events {
		if event.Type != "repair.linked" {
			continue
		}
		var payload struct {
			FailedTaskID string `json:"failed_task_id"`
		}
		if err := json.Unmarshal([]byte(event.Payload), &payload); err != nil {
			return "", err
		}
		return strings.TrimSpace(payload.FailedTaskID), nil
	}
	return "", nil
}

func (s *Service) rootRepairTargetID(ctx context.Context, taskID string) (string, error) {
	currentID := strings.TrimSpace(taskID)
	seen := map[string]bool{}
	for i := 0; i < 16; i++ {
		if currentID == "" || seen[currentID] {
			return currentID, nil
		}
		seen[currentID] = true
		nextID, err := s.repairTargetID(ctx, currentID)
		if err != nil {
			return "", err
		}
		if nextID == "" {
			return currentID, nil
		}
		currentID = nextID
	}
	return currentID, nil
}

func (s *Service) extractTaskMemories(ctx context.Context, projectID string, task domain.Task, attempt domain.TaskAttempt, review domain.ReviewResult) error {
	if s.memories == nil {
		return nil
	}
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	items, err := taskaware.ExtractTaskMemories(ctx, s.store, project, task, attempt, review)
	if err != nil {
		return err
	}
	for _, item := range items {
		s.memories.Add(item)
		if _, err := s.store.AddTaskEvent(ctx, domain.TaskEvent{
			TaskID:    task.ID,
			AttemptID: attempt.ID,
			Type:      "memory.extracted",
			Message:   item.Summary,
			Payload:   fmt.Sprintf(`{"memory_id":%q,"type":%q,"evidence_ref":%q}`, item.ID, item.Type, item.EvidenceRef),
		}); err != nil {
			return err
		}
	}
	if err := s.persistMemories(ctx); err != nil {
		return err
	}
	return s.log(ctx, "memory.extract", map[string]any{
		"project_id": projectID,
		"task_id":    task.ID,
		"count":      len(items),
	})
}

func (s *Service) persistMemories(ctx context.Context) error {
	if s.memories == nil {
		return nil
	}
	if s.memoryPersistPath != "" {
		return s.memories.SaveJSONL(ctx, s.memoryPersistPath)
	}
	return s.memories.Persist(ctx)
}

func (s *Service) saveSessionContext(ctx context.Context, projectID string) {
	if s.sessionSaver == nil || s.sessionID == "" {
		return
	}
	decision := s.decisionByProjectID[projectID]
	profile := s.profileByProjectID[projectID]
	planningContext := s.contextByProjectID[projectID]

	decisionJSON, _ := json.Marshal(decision)
	profileJSON, _ := json.Marshal(profile)

	var memoryJSON []byte
	if s.memories != nil {
		items := s.memories.Items()
		memoryJSON, _ = json.Marshal(items)
	}

	var personasJSON []byte
	personasJSON, _ = json.Marshal(s.activePersonas)

	sctx := domain.SessionRuntimeContext{
		SessionID:      s.sessionID,
		ProjectID:      projectID,
		ChainDecision:  string(decisionJSON),
		IntentProfile:  string(profileJSON),
		ContextText:    planningContext,
		MemorySnapshot: string(memoryJSON),
		ActivePersonas: string(personasJSON),
	}
	if err := s.sessionSaver.SaveSessionRuntimeContext(ctx, sctx); err != nil {
		s.log(ctx, "session.save_context_failed", map[string]any{"session_id": s.sessionID, "error": err.Error()})
		return
	}
	s.log(ctx, "session.context_saved", map[string]any{"session_id": s.sessionID, "project_id": projectID})
}

func (s *Service) emitTaskProgress(ctx context.Context, projectID string, task domain.Task, status domain.TaskStatus, text string) error {
	return s.emit(ctx, comm.NewProgressIntent(s.communicationChannel, text, map[string]any{
		"project_id": projectID,
		"task_id":    task.ID,
		"task_title": task.Title,
		"status":     string(status),
	}), projectID)
}

func (s *Service) emitTaskBlocked(ctx context.Context, projectID string, task domain.Task, text string) {
	_ = s.emit(ctx, comm.NewBlockedIntent(s.communicationChannel, text, map[string]any{
		"project_id": projectID,
		"task_id":    task.ID,
		"task_title": task.Title,
		"status":     string(domain.TaskStatusBlocked),
	}), projectID)
}

func (s *Service) emitProjectBlocked(ctx context.Context, projectID string, text string) {
	_ = s.emit(ctx, comm.NewBlockedIntent(s.communicationChannel, text, map[string]any{
		"project_id": projectID,
		"status":     string(domain.TaskStatusBlocked),
	}), projectID)
}

func (s *Service) emit(ctx context.Context, intent comm.CommunicationIntent, projectID string) error {
	if s.communication == nil || s.communicationChannel == "" {
		return nil
	}
	intent.ChannelID = s.communicationChannel
	intent.SessionID = s.communicationSession
	intent.ProjectID = projectID
	_, err := s.communication.Dispatch(ctx, intent)
	return err
}

func (s *Service) log(ctx context.Context, stage string, payload any) error {
	if s.logger == nil {
		return nil
	}
	return s.logger.Log(ctx, stage, payload)
}
