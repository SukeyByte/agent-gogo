package runtime

import (
	"context"
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
	for _, item := range planned {
		created := item.created
		if err := s.validator.ValidateTask(ctx, created); err != nil {
			return nil, err
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
	s.saveSessionContext(ctx, project.ID)
	return readyTasks, nil
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
			_ = s.createRepairTask(ctx, projectID, executionErr.Task, executionErr.Attempt, "executor.failed", err)
		}
		return TaskRunResult{}, err
	}
	if err := s.log(ctx, "executor.result", executed); err != nil {
		return TaskRunResult{}, err
	}
	tested, err := s.tester.Test(ctx, executed.Task, executed.Attempt)
	if err != nil {
		_ = s.createRepairTask(ctx, projectID, executed.Task, executed.Attempt, "tester.failed", err)
		return TaskRunResult{}, err
	}
	if err := s.log(ctx, "tester.result", tested); err != nil {
		return TaskRunResult{}, err
	}
	reviewed, err := s.reviewer.Review(ctx, tested.Task, executed.Attempt)
	if err != nil {
		_ = s.createRepairTask(ctx, projectID, tested.Task, executed.Attempt, "reviewer.rejected", err)
		return TaskRunResult{}, err
	}
	if err := s.log(ctx, "reviewer.result", reviewed); err != nil {
		return TaskRunResult{}, err
	}
	if err := s.extractTaskMemories(ctx, projectID, reviewed.Task, reviewed.Attempt, reviewed.ReviewResult); err != nil {
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
		_, err = s.PlanProject(ctx, project.ID)
		return err
	case "task.retry":
		return s.RetryTask(ctx, event.TaskID)
	case "project.replan":
		return s.ReplanProject(ctx, event.ProjectID, event.Text)
	default:
		return s.log(ctx, "runtime.channel_event", event)
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

func (s *Service) createRepairTask(ctx context.Context, projectID string, failedTask domain.Task, attempt domain.TaskAttempt, eventType string, cause error) error {
	message := strings.TrimSpace(cause.Error())
	if message == "" {
		message = "runtime verification failed"
	}
	if latest, err := s.store.GetTask(ctx, failedTask.ID); err == nil {
		failedTask = latest
	}
	if failedTask.Status != domain.TaskStatusFailed && failedTask.Status != domain.TaskStatusDone && failedTask.Status != domain.TaskStatusCancelled {
		if domain.CanTransitionTask(failedTask.Status, domain.TaskStatusFailed) {
			transitioned, err := s.store.TransitionTask(ctx, failedTask.ID, domain.TaskStatusFailed, message)
			if err != nil {
				return err
			}
			failedTask = transitioned
		}
	}
	if _, err := s.store.AddTaskEvent(ctx, domain.TaskEvent{
		TaskID:    failedTask.ID,
		AttemptID: attempt.ID,
		Type:      eventType,
		Message:   message,
		Payload:   fmt.Sprintf(`{"failed_task_id":%q}`, failedTask.ID),
	}); err != nil {
		return err
	}
	if s.memories != nil {
		item := taskaware.FailureMemory(projectID, failedTask, attempt, eventType, message)
		s.memories.Add(item)
		if err := s.persistMemories(ctx); err != nil {
			return err
		}
		_ = s.log(ctx, "memory.extract", map[string]any{
			"project_id": projectID,
			"task_id":    failedTask.ID,
			"memory_id":  item.ID,
			"type":       item.Type,
		})
	}
	repair, err := s.store.CreateTask(ctx, domain.Task{
		ProjectID:          projectID,
		Title:              "Fix: " + failedTask.Title,
		Description:        "Repair failed task after " + eventType + ": " + message,
		Status:             domain.TaskStatusDraft,
		AcceptanceCriteria: []string{"Failure evidence is understood", "A targeted fix is applied", "Tests pass after repair"},
	})
	if err != nil {
		return err
	}
	_, err = s.store.TransitionTask(ctx, repair.ID, domain.TaskStatusReady, "repair task generated after failure")
	return err
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
