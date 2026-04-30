package tools

import (
	"context"
	"errors"

	"github.com/sukeke/agent-gogo/internal/capability"
	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/observability"
)

var ErrToolNotFound = errors.New("tool not found")
var ErrCapabilityBlocked = errors.New("tool capability blocked")
var ErrConfirmationRequired = errors.New("tool confirmation required")

type Store interface {
	CreateToolCall(ctx context.Context, call domain.ToolCall) (domain.ToolCall, error)
}

type Spec struct {
	Name          string
	Description   string
	RiskLevel     string
	InputSchema   map[string]any
	OutputSchema  map[string]any
	RequiresShell bool
}

type CallRequest struct {
	AttemptID string
	Name      string
	Args      map[string]any
}

type Result struct {
	Success     bool
	Output      map[string]any
	Error       string
	EvidenceRef string
	Metadata    map[string]string
}

type CallResponse struct {
	Result   Result
	ToolCall domain.ToolCall
}

type Handler func(ctx context.Context, args map[string]any) (Result, error)

type SecurityPolicy struct {
	AllowedTools              map[string]bool
	AllowShell                bool
	ShellAllowlist            []string
	RequireConfirmationAtRisk string
}

type ConfirmationRequest struct {
	ToolName  string
	RiskLevel string
	Args      map[string]any
}

type ConfirmationGate interface {
	Confirm(ctx context.Context, req ConfirmationRequest) (bool, error)
}

type AutoConfirmationGate struct {
	Approved bool
}

func (g AutoConfirmationGate) Confirm(ctx context.Context, req ConfirmationRequest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return g.Approved, nil
}

type Runtime struct {
	store            Store
	specs            map[string]Spec
	handlers         map[string]Handler
	security         SecurityPolicy
	confirmationGate ConfirmationGate
	capabilities     *capability.Registry
	logger           observability.Logger
	codeIndexCache   *codeIndexCache
}
