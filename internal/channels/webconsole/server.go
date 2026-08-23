package webconsole

import (
	"context"

	"github.com/SukeyByte/agent-gogo/internal/domain"
)

// Store is the persistence contract backing the Web Console API.
type Store interface {
	ListProjects(ctx context.Context) ([]domain.Project, error)
	GetProject(ctx context.Context, id string) (domain.Project, error)
	GetTask(ctx context.Context, id string) (domain.Task, error)
	ListTasksByProject(ctx context.Context, projectID string) ([]domain.Task, error)
	ListTaskAttemptsByTask(ctx context.Context, taskID string) ([]domain.TaskAttempt, error)
	ListTaskEvents(ctx context.Context, taskID string) ([]domain.TaskEvent, error)
	ListObservationsByAttempt(ctx context.Context, attemptID string) ([]domain.Observation, error)
	ListToolCallsByAttempt(ctx context.Context, attemptID string) ([]domain.ToolCall, error)
	ListTestResultsByAttempt(ctx context.Context, attemptID string) ([]domain.TestResult, error)
	ListReviewResultsByAttempt(ctx context.Context, attemptID string) ([]domain.ReviewResult, error)
	ListArtifactsByProject(ctx context.Context, projectID string) ([]domain.Artifact, error)
}

// ConfigView is the public runtime configuration exposed to the Web Console.
type ConfigView struct {
	WorkspacePath          string   `json:"workspace_path"`
	SQLitePath             string   `json:"sqlite_path"`
	ArtifactPath           string   `json:"artifact_path"`
	LogPath                string   `json:"log_path"`
	SkillRoots             []string `json:"skill_roots"`
	PersonaPath            string   `json:"persona_path"`
	ChannelID              string   `json:"channel_id"`
	SessionID              string   `json:"session_id"`
	ContextMaxChars        int      `json:"context_max_chars"`
	MaxTasksPerProject     int      `json:"max_tasks_per_project"`
	RequireConfirmHighRisk bool     `json:"require_confirm_high_risk"`
	AllowShell             bool     `json:"allow_shell"`
	ShellAllowlist         []string `json:"shell_allowlist"`
	LLMTimeoutSeconds      int      `json:"llm_timeout_seconds"`
	BrowserHeadless        bool     `json:"browser_headless"`
	BrowserTimeoutSeconds  int      `json:"browser_timeout_seconds"`
	LLMProvider            string   `json:"llm_provider"`
	LLMModel               string   `json:"llm_model"`
	LLMBaseURL             string   `json:"llm_base_url"`
	LLMAPIKey              string   `json:"llm_api_key"`
}
