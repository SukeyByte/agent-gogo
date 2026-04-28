package runtime

import (
	"context"
	"fmt"

	"github.com/sukeke/agent-gogo/internal/chain"
	comm "github.com/sukeke/agent-gogo/internal/communication"
	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/executor"
	intentpkg "github.com/sukeke/agent-gogo/internal/intent"
	"github.com/sukeke/agent-gogo/internal/planner"
	"github.com/sukeke/agent-gogo/internal/provider"
	"github.com/sukeke/agent-gogo/internal/reviewer"
	"github.com/sukeke/agent-gogo/internal/scheduler"
	"github.com/sukeke/agent-gogo/internal/tester"
	"github.com/sukeke/agent-gogo/internal/validator"
)

type Store interface {
	CreateProject(ctx context.Context, project domain.Project) (domain.Project, error)
	GetProject(ctx context.Context, id string) (domain.Project, error)
	CreateTask(ctx context.Context, task domain.Task) (domain.Task, error)
	TransitionTask(ctx context.Context, taskID string, to domain.TaskStatus, message string) (domain.Task, error)
	ListTasksByProject(ctx context.Context, projectID string) ([]domain.Task, error)
	CreateTaskAttempt(ctx context.Context, taskID string) (domain.TaskAttempt, error)
	CompleteTaskAttempt(ctx context.Context, attemptID string, status domain.AttemptStatus, message string) (domain.TaskAttempt, error)
	CreateObservation(ctx context.Context, observation domain.Observation) (domain.Observation, error)
	CreateTestResult(ctx context.Context, result domain.TestResult) (domain.TestResult, error)
	CreateReviewResult(ctx context.Context, result domain.ReviewResult) (domain.ReviewResult, error)
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
}

type CreateProjectRequest struct {
	Name string
	Goal string
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
		store:     store,
		planner:   planner.NewFixedPlanner(),
		validator: validator.NewMinimalTaskValidator(),
		scheduler: scheduler.NewReadyScheduler(store),
		executor:  executor.NewMinimalExecutor(store),
		tester:    tester.NewMinimalTester(store),
		reviewer:  reviewer.NewMinimalReviewer(store),
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
		store:     store,
		planner:   planner,
		validator: validator,
		scheduler: scheduler,
		executor:  executor,
		tester:    tester,
		reviewer:  reviewer,
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
	intentProfile, err := s.analyzeProject(ctx, project, chainDecision)
	if err != nil {
		return nil, err
	}
	drafts, err := s.planner.PlanProject(ctx, planner.PlanRequest{
		Project:       project,
		UserInput:     project.Goal,
		ChainDecision: chainDecision,
		IntentProfile: intentProfile,
	})
	if err != nil {
		return nil, err
	}

	readyTasks := make([]domain.Task, 0, len(drafts))
	for _, draft := range drafts {
		draft.ProjectID = project.ID
		created, err := s.store.CreateTask(ctx, draft)
		if err != nil {
			return nil, err
		}
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
	executed, err := s.executor.Execute(ctx, task)
	if err != nil {
		return TaskRunResult{}, err
	}
	tested, err := s.tester.Test(ctx, executed.Task, executed.Attempt)
	if err != nil {
		return TaskRunResult{}, err
	}
	reviewed, err := s.reviewer.Review(ctx, tested.Task, executed.Attempt)
	if err != nil {
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
