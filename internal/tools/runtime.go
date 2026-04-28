package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/sukeke/agent-gogo/internal/domain"
)

var ErrToolNotFound = errors.New("tool not found")

type Store interface {
	CreateToolCall(ctx context.Context, call domain.ToolCall) (domain.ToolCall, error)
}

type Spec struct {
	Name         string
	Description  string
	RiskLevel    string
	InputSchema  map[string]any
	OutputSchema map[string]any
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

type Runtime struct {
	store    Store
	specs    map[string]Spec
	handlers map[string]Handler
}

func NewRuntime(store Store) *Runtime {
	return &Runtime{
		store:    store,
		specs:    map[string]Spec{},
		handlers: map[string]Handler{},
	}
}

func NewMockRuntime(store Store) *Runtime {
	runtime := NewRuntime(store)
	runtime.Register(Spec{
		Name:        "code.search",
		Description: "Mock code search tool.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		query, _ := args["query"].(string)
		return Result{
			Success: true,
			Output: map[string]any{
				"query": query,
				"matches": []map[string]string{
					{"path": "internal/runtime/service.go", "summary": "mock match"},
				},
			},
			EvidenceRef: "mock://tool/code.search",
		}, nil
	})
	runtime.Register(Spec{
		Name:        "test.run",
		Description: "Mock test runner.",
		RiskLevel:   "medium",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		command, _ := args["command"].(string)
		if command == "" {
			command = "go test ./..."
		}
		return Result{
			Success: true,
			Output: map[string]any{
				"command": command,
				"passed":  true,
				"summary": "mock tests passed",
			},
			EvidenceRef: "mock://tool/test.run",
		}, nil
	})
	return runtime
}

func (r *Runtime) Register(spec Spec, handler Handler) {
	r.specs[spec.Name] = spec
	r.handlers[spec.Name] = handler
}

func (r *Runtime) ListSpecs() []Spec {
	specs := make([]Spec, 0, len(r.specs))
	for _, spec := range r.specs {
		specs = append(specs, spec)
	}
	sort.SliceStable(specs, func(i, j int) bool {
		return specs[i].Name < specs[j].Name
	})
	return specs
}

func (r *Runtime) Call(ctx context.Context, req CallRequest) (CallResponse, error) {
	if err := ctx.Err(); err != nil {
		return CallResponse{}, err
	}
	inputJSON, err := stableJSON(req.Args)
	if err != nil {
		return CallResponse{}, err
	}

	handler, ok := r.handlers[req.Name]
	if !ok {
		result := Result{Success: false, Error: ErrToolNotFound.Error()}
		call, auditErr := r.audit(ctx, req, inputJSON, result)
		if auditErr != nil {
			return CallResponse{}, auditErr
		}
		return CallResponse{Result: result, ToolCall: call}, ErrToolNotFound
	}

	result, handlerErr := handler(ctx, copyArgs(req.Args))
	if handlerErr != nil {
		result.Success = false
		result.Error = handlerErr.Error()
	}
	call, err := r.audit(ctx, req, inputJSON, result)
	if err != nil {
		return CallResponse{}, err
	}
	if handlerErr != nil {
		return CallResponse{Result: result, ToolCall: call}, handlerErr
	}
	if !result.Success && result.Error != "" {
		return CallResponse{Result: result, ToolCall: call}, fmt.Errorf("tool %s failed: %s", req.Name, result.Error)
	}
	return CallResponse{Result: result, ToolCall: call}, nil
}

func (r *Runtime) audit(ctx context.Context, req CallRequest, inputJSON string, result Result) (domain.ToolCall, error) {
	outputJSON, err := stableJSON(result.Output)
	if err != nil {
		return domain.ToolCall{}, err
	}
	status := domain.ToolCallStatusSucceeded
	if !result.Success {
		status = domain.ToolCallStatusFailed
	}
	call := domain.ToolCall{
		AttemptID:   req.AttemptID,
		Name:        req.Name,
		InputJSON:   inputJSON,
		OutputJSON:  outputJSON,
		Status:      status,
		Error:       result.Error,
		EvidenceRef: result.EvidenceRef,
	}
	if r.store == nil {
		return call, nil
	}
	return r.store.CreateToolCall(ctx, call)
}

func stableJSON(value any) (string, error) {
	if value == nil {
		value = map[string]any{}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func copyArgs(args map[string]any) map[string]any {
	if args == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(args))
	for key, value := range args {
		result[key] = value
	}
	return result
}
