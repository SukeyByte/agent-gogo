package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

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

func NewRuntime(store Store) *Runtime {
	return &Runtime{
		store:          store,
		specs:          map[string]Spec{},
		handlers:       map[string]Handler{},
		capabilities:   capability.NewRegistry(),
		codeIndexCache: newCodeIndexCache(),
		security: SecurityPolicy{
			AllowShell: true,
		},
	}
}

func NewBuiltinRuntime(store Store, root string) *Runtime {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	runtime := NewRuntime(store)
	runtime.Register(Spec{
		Name:        "code.search",
		Description: "Search local source files for matching text.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := searchCode(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return Result{
			Success:     true,
			Output:      output,
			EvidenceRef: "file://code.search",
		}, nil
	})
	runtime.Register(Spec{
		Name:        "code.index",
		Description: "Build a lightweight repository map and symbol index.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := runtime.indexCode(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return Result{Success: true, Output: output, EvidenceRef: "code://index"}, nil
	})
	runtime.Register(Spec{
		Name:        "code.symbols",
		Description: "Search the lightweight symbol index.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := runtime.codeSymbols(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return Result{Success: true, Output: output, EvidenceRef: "code://symbols"}, nil
	})
	runtime.Register(Spec{
		Name:        "file.read",
		Description: "Read a workspace file by relative path. Use this instead of shell.run for file contents.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := readFile(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		ref, _ := output["path"].(string)
		return Result{Success: true, Output: output, EvidenceRef: ref}, nil
	})
	runtime.Register(Spec{
		Name:        "file.write",
		Description: "Write a workspace file.",
		RiskLevel:   "medium",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := writeWorkspaceFile(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		runtime.invalidateCodeIndex(root)
		ref, _ := output["path"].(string)
		return Result{Success: true, Output: output, EvidenceRef: ref}, nil
	})
	runtime.Register(Spec{
		Name:        "file.patch",
		Description: "Apply a small string replacement patch to a workspace file.",
		RiskLevel:   "medium",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := patchWorkspaceFile(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		runtime.invalidateCodeIndex(root)
		ref, _ := output["path"].(string)
		return Result{Success: true, Output: output, EvidenceRef: ref}, nil
	})
	runtime.Register(Spec{
		Name:        "file.diff",
		Description: "Return git diff for a workspace path.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := fileDiff(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return Result{Success: true, Output: output, EvidenceRef: "git://diff"}, nil
	})
	runtime.Register(Spec{
		Name:          "test.run",
		Description:   "Run the configured test command. Use this instead of shell.run for test validation.",
		RiskLevel:     "medium",
		RequiresShell: true,
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		command, _ := args["command"].(string)
		if command == "" {
			command = "go test ./..."
		}
		output, err := runCommand(ctx, root, command)
		success := err == nil
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		return Result{
			Success: success,
			Output: map[string]any{
				"command": command,
				"passed":  success,
				"summary": output,
			},
			Error:       errText,
			EvidenceRef: "exec://test.run",
		}, err
	})
	runtime.Register(Spec{
		Name:          "shell.run",
		Description:   "Run one allowlisted exec-style command in the workspace. No pipes, redirects, semicolons, glob wildcards, command substitution, environment assignments, or chained commands.",
		RiskLevel:     "medium",
		RequiresShell: true,
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		command, _ := args["command"].(string)
		output, err := runCommand(ctx, root, command)
		success := err == nil
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		return Result{
			Success: success,
			Output: map[string]any{
				"command": command,
				"passed":  success,
				"summary": output,
			},
			Error:       errText,
			EvidenceRef: "exec://shell.run",
		}, err
	})
	runtime.Register(Spec{
		Name:        "git.status",
		Description: "Return git status --short.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := gitCommand(ctx, root, "status", "--short")
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return Result{Success: true, Output: map[string]any{"status": output}, EvidenceRef: "git://status"}, nil
	})
	runtime.Register(Spec{
		Name:        "git.diff",
		Description: "Return git diff.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		path, _ := args["path"].(string)
		var output string
		var err error
		if strings.TrimSpace(path) == "" {
			output, err = gitCommand(ctx, root, "diff")
		} else {
			output, err = gitDiffPath(ctx, root, path)
		}
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return Result{Success: true, Output: map[string]any{"diff": output}, EvidenceRef: "git://diff"}, nil
	})
	runtime.Register(Spec{
		Name:        "git.branch",
		Description: "Create or checkout a git branch.",
		RiskLevel:   "medium",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := gitBranch(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		return Result{Success: true, Output: output, EvidenceRef: "git://branch"}, nil
	})
	runtime.Register(Spec{
		Name:        "git.commit",
		Description: "Create a git commit from staged changes.",
		RiskLevel:   "high",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		message, _ := args["message"].(string)
		if strings.TrimSpace(message) == "" {
			return Result{Success: false, Error: "commit message is required"}, errors.New("commit message is required")
		}
		output, err := gitCommand(ctx, root, "commit", "-m", message)
		if err != nil {
			return Result{Success: false, Error: err.Error(), Output: map[string]any{"summary": output}}, err
		}
		return Result{Success: true, Output: map[string]any{"summary": output}, EvidenceRef: "git://commit"}, nil
	})
	runtime.Register(Spec{
		Name:        "git.rollback",
		Description: "Safely restore one explicit workspace path.",
		RiskLevel:   "high",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		path, _ := args["path"].(string)
		if strings.TrimSpace(path) == "" {
			return Result{Success: false, Error: "path is required"}, errors.New("path is required")
		}
		if _, err := safeWorkspacePath(root, path); err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		output, err := gitCommand(ctx, root, "restore", "--", path)
		if err != nil {
			return Result{Success: false, Error: err.Error(), Output: map[string]any{"summary": output}}, err
		}
		return Result{Success: true, Output: map[string]any{"path": path, "summary": output}, EvidenceRef: "git://rollback"}, nil
	})
	runtime.Register(Spec{
		Name:        "artifact.write",
		Description: "Write an artifact under the configured workspace root.",
		RiskLevel:   "medium",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := writeArtifact(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		ref, _ := output["artifact_ref"].(string)
		return Result{
			Success:     true,
			Output:      output,
			EvidenceRef: ref,
		}, nil
	})
	runtime.Register(Spec{
		Name:        "document.write",
		Description: "Write a markdown document under the configured workspace root.",
		RiskLevel:   "medium",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := writeArtifact(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		ref, _ := output["artifact_ref"].(string)
		return Result{Success: true, Output: output, EvidenceRef: ref}, nil
	})
	runtime.Register(Spec{
		Name:        "memory.save",
		Description: "Persist a compact project memory markdown artifact.",
		RiskLevel:   "low",
	}, func(ctx context.Context, args map[string]any) (Result, error) {
		output, err := saveMemory(ctx, root, args)
		if err != nil {
			return Result{Success: false, Error: err.Error()}, err
		}
		ref, _ := output["memory_ref"].(string)
		return Result{Success: true, Output: output, EvidenceRef: ref}, nil
	})
	return runtime
}

func (r *Runtime) Register(spec Spec, handler Handler) {
	r.specs[spec.Name] = spec
	r.handlers[spec.Name] = handler
	if r.capabilities == nil {
		r.capabilities = capability.NewRegistry()
	}
	r.capabilities.RegisterTool(capability.ToolSpec{
		Name:          spec.Name,
		Description:   spec.Description,
		RiskLevel:     spec.RiskLevel,
		RequiresShell: spec.RequiresShell,
	})
}

func (r *Runtime) UseSecurityPolicy(policy SecurityPolicy, gate ConfirmationGate) {
	r.security = policy
	r.confirmationGate = gate
}

func (r *Runtime) UseCapabilityRegistry(registry *capability.Registry) {
	if registry == nil {
		registry = capability.NewRegistry()
	}
	r.capabilities = registry
	for _, spec := range r.specs {
		r.capabilities.RegisterTool(capability.ToolSpec{
			Name:          spec.Name,
			Description:   spec.Description,
			RiskLevel:     spec.RiskLevel,
			RequiresShell: spec.RequiresShell,
		})
	}
}

func (r *Runtime) CapabilityRegistry() *capability.Registry {
	if r.capabilities == nil {
		r.capabilities = capability.NewRegistry()
	}
	return r.capabilities
}

func (r *Runtime) CapabilityPolicy() capability.Policy {
	return capability.Policy{
		AllowedTools:              r.security.AllowedTools,
		AllowShell:                r.security.AllowShell,
		ShellAllowlist:            append([]string(nil), r.security.ShellAllowlist...),
		RequireConfirmationAtRisk: r.security.RequireConfirmationAtRisk,
	}
}

func (r *Runtime) UseLogger(logger observability.Logger) {
	r.logger = logger
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
	req = normalizeCallRequest(req)
	inputJSON, err := stableJSON(req.Args)
	if err != nil {
		return CallResponse{}, err
	}
	r.log(ctx, "tool.call.request", map[string]any{
		"attempt_id": req.AttemptID,
		"name":       req.Name,
		"args":       copyArgs(req.Args),
	})

	handler, ok := r.handlers[req.Name]
	if !ok {
		result := Result{Success: false, Error: ErrToolNotFound.Error()}
		call, auditErr := r.audit(ctx, req, inputJSON, result)
		if auditErr != nil {
			return CallResponse{}, auditErr
		}
		return CallResponse{Result: result, ToolCall: call}, ErrToolNotFound
	}
	spec := r.specs[req.Name]
	if err := r.resolveCapability(ctx, spec, req); err != nil {
		result := Result{Success: false, Error: err.Error()}
		call, auditErr := r.audit(ctx, req, inputJSON, result)
		if auditErr != nil {
			return CallResponse{}, auditErr
		}
		return CallResponse{Result: result, ToolCall: call}, err
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

func normalizeCallRequest(req CallRequest) CallRequest {
	req.Args = copyArgs(req.Args)
	if req.Name != "test.run" {
		return req
	}
	command, _ := req.Args["command"].(string)
	command = strings.TrimSpace(command)
	switch {
	case command == "":
		req.Args["command"] = "go test ./..."
	case command == "./..." || command == "..." || strings.HasPrefix(command, "./"):
		req.Args["command"] = "go test " + command
	}
	return req
}

func (r *Runtime) resolveCapability(ctx context.Context, spec Spec, req CallRequest) error {
	if r.capabilities == nil {
		r.capabilities = capability.NewRegistry()
	}
	r.capabilities.RegisterTool(capability.ToolSpec{
		Name:          spec.Name,
		Description:   spec.Description,
		RiskLevel:     spec.RiskLevel,
		RequiresShell: spec.RequiresShell,
	})
	resolution, err := r.capabilities.ResolveTool(ctx, capability.ToolRequest{
		ToolName: req.Name,
		Args:     copyArgs(req.Args),
		Policy: capability.Policy{
			AllowedTools:              r.security.AllowedTools,
			AllowShell:                r.security.AllowShell,
			ShellAllowlist:            append([]string(nil), r.security.ShellAllowlist...),
			RequireConfirmationAtRisk: r.security.RequireConfirmationAtRisk,
		},
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCapabilityBlocked, err)
	}
	if resolution.RequiresConfirmation {
		if r.confirmationGate == nil {
			return fmt.Errorf("%w: %s", ErrConfirmationRequired, req.Name)
		}
		approved, err := r.confirmationGate.Confirm(ctx, ConfirmationRequest{
			ToolName:  req.Name,
			RiskLevel: spec.RiskLevel,
			Args:      copyArgs(req.Args),
		})
		if err != nil {
			return err
		}
		if !approved {
			return fmt.Errorf("%w: %s rejected", ErrConfirmationRequired, req.Name)
		}
	}
	return nil
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
		r.log(ctx, "tool.call.response", map[string]any{
			"attempt_id":   call.AttemptID,
			"name":         call.Name,
			"status":       call.Status,
			"error":        call.Error,
			"evidence_ref": call.EvidenceRef,
			"output":       result.Output,
		})
		return call, nil
	}
	created, err := r.store.CreateToolCall(ctx, call)
	if err != nil {
		return domain.ToolCall{}, err
	}
	r.log(ctx, "tool.call.response", map[string]any{
		"id":           created.ID,
		"attempt_id":   created.AttemptID,
		"name":         created.Name,
		"status":       created.Status,
		"error":        created.Error,
		"evidence_ref": created.EvidenceRef,
		"output":       result.Output,
	})
	return created, nil
}

func (r *Runtime) log(ctx context.Context, stage string, payload any) {
	if r.logger == nil {
		return
	}
	_ = r.logger.Log(ctx, stage, payload)
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
