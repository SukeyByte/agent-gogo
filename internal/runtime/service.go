package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"

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
	stateMu              sync.RWMutex
	contextByProjectID   map[string]string
	decisionByProjectID  map[string]chain.Decision
	profileByProjectID   map[string]intentpkg.Profile
	parallelism          int
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

// TaskFailedError marks the failure of a single task. A repair task may have
// been queued, so run loops can continue with remaining or recovery tasks
// instead of aborting the whole project run.

// TaskFailedError marks the failure of a single task. A repair task may have
// been queued, so run loops can continue with remaining or recovery tasks
// instead of aborting the whole project run.
type TaskFailedError struct {
	Stage     string
	TaskTitle string
	Err       error
}

func (e *TaskFailedError) Error() string {
	if e.TaskTitle == "" {
		return "task failed at " + e.Stage + ": " + e.Err.Error()
	}
	return "task " + e.TaskTitle + " failed at " + e.Stage + ": " + e.Err.Error()
}

func (e *TaskFailedError) Unwrap() error { return e.Err }

type TaskRunResult struct {
	ProjectID    string
	Task         domain.Task
	Attempt      domain.TaskAttempt
	TestResult   domain.TestResult
	ReviewResult domain.ReviewResult
	Events       []domain.TaskEvent
}

func (s *Service) setState(projectID string, contextText string, decision chain.Decision, profile intentpkg.Profile) {
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	if s.contextByProjectID == nil {
		s.contextByProjectID = map[string]string{}
	}
	if s.decisionByProjectID == nil {
		s.decisionByProjectID = map[string]chain.Decision{}
	}
	if s.profileByProjectID == nil {
		s.profileByProjectID = map[string]intentpkg.Profile{}
	}
	s.contextByProjectID[projectID] = contextText
	s.decisionByProjectID[projectID] = decision
	s.profileByProjectID[projectID] = profile
}

func (s *Service) getState(projectID string) (string, chain.Decision, intentpkg.Profile) {
	s.stateMu.RLock()
	defer s.stateMu.RUnlock()
	return s.contextByProjectID[projectID], s.decisionByProjectID[projectID], s.profileByProjectID[projectID]
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

// UseParallelism sets how many tasks of one project may run at once.
// Values below 1 keep sequential execution.
func (s *Service) UseParallelism(workers int) {
	if workers > 0 {
		s.parallelism = workers
	}
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

const maxRepairDepth = 3

func (s *Service) log(ctx context.Context, stage string, payload any) error {
	if s.logger == nil {
		return nil
	}
	return s.logger.Log(ctx, stage, payload)
}
