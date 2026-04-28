package domain

import "time"

type ProjectStatus string

const (
	ProjectStatusActive    ProjectStatus = "ACTIVE"
	ProjectStatusCompleted ProjectStatus = "COMPLETED"
	ProjectStatusArchived  ProjectStatus = "ARCHIVED"
)

type AttemptStatus string

const (
	AttemptStatusRunning   AttemptStatus = "RUNNING"
	AttemptStatusSucceeded AttemptStatus = "SUCCEEDED"
	AttemptStatusFailed    AttemptStatus = "FAILED"
	AttemptStatusCancelled AttemptStatus = "CANCELLED"
)

type ToolCallStatus string

const (
	ToolCallStatusPending   ToolCallStatus = "PENDING"
	ToolCallStatusSucceeded ToolCallStatus = "SUCCEEDED"
	ToolCallStatusFailed    ToolCallStatus = "FAILED"
)

type TestStatus string

const (
	TestStatusPassed TestStatus = "PASSED"
	TestStatusFailed TestStatus = "FAILED"
)

type ReviewStatus string

const (
	ReviewStatusApproved ReviewStatus = "APPROVED"
	ReviewStatusRejected ReviewStatus = "REJECTED"
)

type Project struct {
	ID        string
	Name      string
	Goal      string
	Status    ProjectStatus
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Task struct {
	ID                 string
	ProjectID          string
	Title              string
	Description        string
	Status             TaskStatus
	AcceptanceCriteria []string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type TaskDependency struct {
	ID              string
	TaskID          string
	DependsOnTaskID string
	CreatedAt       time.Time
}

type TaskAttempt struct {
	ID        string
	TaskID    string
	Number    int
	Status    AttemptStatus
	StartedAt time.Time
	EndedAt   *time.Time
	Error     string
}

type TaskEvent struct {
	ID        string
	TaskID    string
	AttemptID string
	Type      string
	FromState TaskStatus
	ToState   TaskStatus
	Message   string
	Payload   string
	CreatedAt time.Time
}

type ToolCall struct {
	ID          string
	AttemptID   string
	Name        string
	InputJSON   string
	OutputJSON  string
	Status      ToolCallStatus
	Error       string
	EvidenceRef string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Observation struct {
	ID          string
	AttemptID   string
	ToolCallID  string
	Type        string
	Summary     string
	EvidenceRef string
	Payload     string
	CreatedAt   time.Time
}

type TestResult struct {
	ID          string
	AttemptID   string
	Name        string
	Status      TestStatus
	Output      string
	EvidenceRef string
	CreatedAt   time.Time
}

type ReviewResult struct {
	ID          string
	AttemptID   string
	Status      ReviewStatus
	Summary     string
	EvidenceRef string
	CreatedAt   time.Time
}

type Artifact struct {
	ID          string
	AttemptID   string
	ProjectID   string
	Type        string
	Path        string
	Description string
	CreatedAt   time.Time
}
