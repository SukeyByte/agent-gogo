package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/sukeke/agent-gogo/internal/chain"
	comm "github.com/sukeke/agent-gogo/internal/communication"
	"github.com/sukeke/agent-gogo/internal/contextbuilder"
	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/executor"
	"github.com/sukeke/agent-gogo/internal/function"
	intentpkg "github.com/sukeke/agent-gogo/internal/intent"
	"github.com/sukeke/agent-gogo/internal/memory"
	"github.com/sukeke/agent-gogo/internal/observability"
	"github.com/sukeke/agent-gogo/internal/persona"
	"github.com/sukeke/agent-gogo/internal/planner"
	"github.com/sukeke/agent-gogo/internal/provider"
	"github.com/sukeke/agent-gogo/internal/reviewer"
	"github.com/sukeke/agent-gogo/internal/scheduler"
	"github.com/sukeke/agent-gogo/internal/skill"
	"github.com/sukeke/agent-gogo/internal/taskaware"
	"github.com/sukeke/agent-gogo/internal/tester"
	"github.com/sukeke/agent-gogo/internal/textutil"
	"github.com/sukeke/agent-gogo/internal/validator"
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
	ProjectID string
	TaskID    string
	AttemptID string
	ActionID  string
	Approved  bool
	Message   string
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
		tester:              tester.NewMinimalTester(store),
		reviewer:            reviewer.NewMinimalReviewer(store),
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
	return readyTasks, nil
}

func (s *Service) buildRuntimeContext(ctx context.Context, project domain.Project, currentTaskID string, decision chain.Decision, profile intentpkg.Profile) (string, error) {
	if s.contextSerializer == nil {
		return "", nil
	}
	awareness, err := taskaware.BuildContextSnapshot(ctx, s.store, project, currentTaskID)
	if err != nil {
		return "", err
	}
	if err := s.log(ctx, "taskaware.digest", map[string]any{
		"project_id":      project.ID,
		"current_task_id": currentTaskID,
		"project_summary": awareness.ProjectState.Summary,
		"task_state":      awareness.TaskState,
	}); err != nil {
		return "", err
	}

	var activeFunctionSchemas []contextbuilder.FunctionSchema
	var deferredFunctionCards []contextbuilder.FunctionCard
	if s.functions != nil {
		cards, err := s.functions.Search(ctx, function.SearchRequest{
			Query:                project.Goal,
			TaskType:             profile.TaskType,
			Domains:              profile.Domains,
			RequiredCapabilities: requiredCapabilities(decision, profile),
			Limit:                8,
		})
		if err != nil {
			return "", err
		}
		if err := s.log(ctx, "function.search", cards); err != nil {
			return "", err
		}
		deferredFunctionCards = functionCardsForContext(cards)
		active, err := s.functions.Activate(ctx, firstFunctionCards(cards, 4))
		if err != nil {
			return "", err
		}
		activeFunctionSchemas = active.ContextSchemas()
	}

	activeSkills, deferredSkills, err := s.searchSkills(ctx, project, profile, decision)
	if err != nil {
		return "", err
	}
	activePersonas, err := s.searchPersonas(ctx, project, profile)
	if err != nil {
		return "", err
	}
	relevantMemories, err := s.searchMemories(ctx, project, profile, awareness.QueryText)
	if err != nil {
		return "", err
	}

	pack := contextbuilder.ContextPack{
		RuntimeRules: []contextbuilder.Message{
			{ID: "runtime-001", Role: "system", Text: "Use the runtime state machine, tool evidence, tester, and reviewer before marking work done.", VersionHash: "runtime-v1"},
		},
		SecurityRules: []contextbuilder.Message{
			{ID: "security-001", Role: "system", Text: "All side effects must go through Tool Runtime or Communication Runtime. Do not expose API keys.", VersionHash: "security-v1"},
		},
		ChannelCapabilities: []contextbuilder.ChannelCapability{
			{
				ChannelType:           s.communicationChannel,
				Capabilities:          []string{"send_message", "notify_done", "ask_confirmation"},
				SupportedMessageTypes: []string{"text", "artifact"},
				SupportsConfirmation:  true,
			},
		},
		IntentProfile:              profile.ContextProfile(),
		ActiveCapabilities:         capabilitySpecs(activeFunctionSchemas),
		ActiveFunctionSchemas:      activeFunctionSchemas,
		DeferredFunctionCandidates: deferredFunctionCards,
		ActiveSkillInstructions:    activeSkills,
		DeferredSkillCandidates:    deferredSkills,
		ActivePersonas:             activePersonas,
		RelevantMemories:           relevantMemories,
		ProjectState:               awareness.ProjectState,
		TaskState:                  awareness.TaskState,
		AcceptanceCriteria:         awareness.AcceptanceCriteria,
		EvidenceRefs:               awareness.EvidenceRefs,
		CurrentUserInput:           project.Goal,
	}
	serialized, err := s.contextSerializer.Serialize(ctx, pack)
	if err != nil {
		return "", err
	}
	if err := s.log(ctx, "contextbuilder.serialize", map[string]any{
		"layer_keys": serialized.LayerKeys,
		"block_keys": serialized.BlockKeys,
		"text":       serialized.Text,
	}); err != nil {
		return "", err
	}
	return serialized.Text, nil
}

func (s *Service) searchSkills(ctx context.Context, project domain.Project, profile intentpkg.Profile, decision chain.Decision) ([]contextbuilder.SkillInstruction, []contextbuilder.SkillPackageRef, error) {
	if s.skills == nil {
		return nil, nil, nil
	}
	query := strings.TrimSpace(project.Goal + " " + profile.TaskType + " " + strings.Join(decision.SkillTags, " "))
	cards, err := s.skills.Search(ctx, query, 4)
	if err != nil {
		return nil, nil, err
	}
	if len(cards) == 0 {
		cards, err = s.skills.Search(ctx, "story writing plot chapter fiction", 4)
		if err != nil {
			return nil, nil, err
		}
	}
	if err := s.log(ctx, "skill.search", cards); err != nil {
		return nil, nil, err
	}
	active := make([]contextbuilder.SkillInstruction, 0, minInt(2, len(cards)))
	for _, card := range firstSkillCards(cards, 2) {
		pkg, err := s.skills.Load(ctx, card.ID)
		if err != nil {
			return nil, nil, err
		}
		active = append(active, pkg.ContextInstruction())
		if err := s.log(ctx, "skill.load", map[string]any{"id": pkg.ID, "name": pkg.Name, "path": pkg.Path}); err != nil {
			return nil, nil, err
		}
	}
	deferred := make([]contextbuilder.SkillPackageRef, 0, len(cards))
	for _, card := range cards {
		deferred = append(deferred, contextbuilder.SkillPackageRef{
			ID:          card.ID,
			Name:        card.Name,
			VersionHash: card.VersionHash,
			Path:        card.Path,
			Reason:      card.Reason,
		})
	}
	return active, deferred, nil
}

func (s *Service) searchPersonas(ctx context.Context, project domain.Project, profile intentpkg.Profile) ([]contextbuilder.Persona, error) {
	active := append([]contextbuilder.Persona(nil), s.activePersonas...)
	if s.personas == nil {
		return active, nil
	}
	query := strings.TrimSpace(project.Goal + " " + profile.TaskType)
	cards, err := s.personas.Search(ctx, query, 2)
	if err != nil {
		return nil, err
	}
	if err := s.log(ctx, "persona.search", cards); err != nil {
		return nil, err
	}
	for _, card := range cards {
		persona, err := s.personas.Load(ctx, card.ID)
		if err != nil {
			return nil, err
		}
		active = append(active, persona.ContextPersona())
		if err := s.log(ctx, "persona.load", map[string]any{"id": persona.ID, "name": persona.Name, "path": persona.Path}); err != nil {
			return nil, err
		}
	}
	return active, nil
}

func (s *Service) searchMemories(ctx context.Context, project domain.Project, profile intentpkg.Profile, awarenessQuery string) ([]contextbuilder.MemoryItem, error) {
	if s.memories == nil {
		return nil, nil
	}
	cards, err := s.memories.Search(ctx, strings.TrimSpace(project.Goal+" "+profile.TaskType+" "+awarenessQuery), "project", 8)
	if err != nil {
		return nil, err
	}
	if err := s.log(ctx, "memory.search", cards); err != nil {
		return nil, err
	}
	items := make([]contextbuilder.MemoryItem, 0, len(cards))
	for _, card := range cards {
		item, err := s.memories.Load(ctx, card.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, item.ContextMemory())
	}
	return items, nil
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
		Payload:   fmt.Sprintf(`{"action_id":%q}`, confirmation.ActionID),
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

func requiredCapabilities(decision chain.Decision, profile intentpkg.Profile) []string {
	values := append([]string(nil), profile.RequiredCapabilities...)
	values = append(values, decision.ToolNames...)
	return sortedUniqueStrings(values)
}

func firstFunctionCards(cards []function.Card, limit int) []function.Card {
	if limit > 0 && len(cards) > limit {
		return append([]function.Card(nil), cards[:limit]...)
	}
	return append([]function.Card(nil), cards...)
}

func firstSkillCards(cards []skill.Card, limit int) []skill.Card {
	if limit > 0 && len(cards) > limit {
		return append([]skill.Card(nil), cards[:limit]...)
	}
	return append([]skill.Card(nil), cards...)
}

func functionCardsForContext(cards []function.Card) []contextbuilder.FunctionCard {
	result := make([]contextbuilder.FunctionCard, 0, len(cards))
	for _, card := range cards {
		result = append(result, contextbuilder.FunctionCard{
			Name:                card.Name,
			Description:         card.Description,
			Tags:                append([]string(nil), card.Tags...),
			TaskTypes:           append([]string(nil), card.TaskTypes...),
			RiskLevel:           card.RiskLevel,
			InputSummary:        card.InputSummary,
			OutputSummary:       card.OutputSummary,
			Provider:            card.Provider,
			RequiredPermissions: append([]string(nil), card.RequiredPermissions...),
			SchemaRef:           card.SchemaRef,
			VersionHash:         card.VersionHash,
			Reason:              card.Reason,
		})
	}
	return result
}

func capabilitySpecs(schemas []contextbuilder.FunctionSchema) []contextbuilder.CapabilitySpec {
	result := make([]contextbuilder.CapabilitySpec, 0, len(schemas))
	for _, schema := range schemas {
		result = append(result, contextbuilder.CapabilitySpec{
			Name:        schema.Name,
			Description: schema.Description,
			RiskLevel:   schema.RiskLevel,
			VersionHash: schema.VersionHash,
		})
	}
	return result
}

func sortedUniqueStrings(values []string) []string {
	return textutil.SortedUniqueStrings(values)
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}
