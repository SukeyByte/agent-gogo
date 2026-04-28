package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

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
	logger           observability.Logger
}

func NewRuntime(store Store) *Runtime {
	return &Runtime{
		store:    store,
		specs:    map[string]Spec{},
		handlers: map[string]Handler{},
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
		Name:          "test.run",
		Description:   "Run a configured test command.",
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
}

func (r *Runtime) UseSecurityPolicy(policy SecurityPolicy, gate ConfirmationGate) {
	r.security = policy
	r.confirmationGate = gate
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

func (r *Runtime) resolveCapability(ctx context.Context, spec Spec, req CallRequest) error {
	if r.security.AllowedTools != nil && !r.security.AllowedTools[req.Name] {
		return fmt.Errorf("%w: %s is not allowed", ErrCapabilityBlocked, req.Name)
	}
	if spec.RequiresShell && !r.security.AllowShell {
		return fmt.Errorf("%w: shell is disabled for %s", ErrCapabilityBlocked, req.Name)
	}
	if riskRequiresConfirmation(spec.RiskLevel, r.security.RequireConfirmationAtRisk) {
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

func searchCode(ctx context.Context, root string, args map[string]any) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	query, _ := args["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query is required")
	}
	limit := intArg(args["limit"], 20)
	paths := stringSliceArg(args["paths"])
	if len(paths) == 0 {
		paths = []string{"."}
	}
	matches := []map[string]any{}
	lowerQuery := strings.ToLower(query)
	for _, requested := range paths {
		base, err := safeJoin(root, requested)
		if err != nil {
			return nil, err
		}
		err = filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() {
				if shouldSkipDir(entry.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if len(matches) >= limit || shouldSkipFile(path) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil || !strings.Contains(strings.ToLower(string(data)), lowerQuery) {
				return nil
			}
			line, snippet := firstMatchingLine(string(data), lowerQuery)
			rel := artifactRef(root, path)
			matches = append(matches, map[string]any{
				"path":    rel,
				"line":    line,
				"snippet": snippet,
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"query": query, "matches": matches}, nil
}

func runCommand(ctx context.Context, root string, command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("command is required")
	}
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = root
	data, err := cmd.CombinedOutput()
	return truncate(string(data), 4000), err
}

func writeArtifact(ctx context.Context, root string, args map[string]any) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relPath, _ := args["path"].(string)
	content, _ := args["content"].(string)
	summary, _ := args["summary"].(string)
	if strings.TrimSpace(relPath) == "" {
		return nil, errors.New("path is required")
	}
	target, err := safeJoin(root, relPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{
		"artifact_ref": artifactRef(root, target),
		"summary":      summary,
		"bytes":        len([]byte(content)),
	}, nil
}

func saveMemory(ctx context.Context, root string, args map[string]any) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, _ := args["key"].(string)
	scope, _ := args["scope"].(string)
	summary, _ := args["summary"].(string)
	body, _ := args["body"].(string)
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("memory key is required")
	}
	if strings.TrimSpace(summary) == "" {
		return nil, errors.New("memory summary is required")
	}
	if strings.TrimSpace(body) == "" {
		return nil, errors.New("memory body is required")
	}
	if strings.TrimSpace(scope) == "" {
		scope = "project"
	}
	tags := stringSliceArg(args["tags"])
	fileName := safeFileName(key) + ".md"
	content := "# " + summary + "\n\n" +
		"scope: " + scope + "\n" +
		"tags: " + strings.Join(tags, ",") + "\n\n" +
		body + "\n"
	target, err := safeJoin(root, filepath.Join("memory", fileName))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{
		"memory_ref": artifactRef(root, target),
		"scope":      scope,
		"summary":    summary,
		"bytes":      len([]byte(content)),
	}, nil
}

func safeJoin(root string, requested string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Clean(filepath.Join(absRoot, requested))
	if target != absRoot && !strings.HasPrefix(target, absRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes workspace root: %s", requested)
	}
	return target, nil
}

func artifactRef(root string, target string) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(target))
	}
	rel, err := filepath.Rel(absRoot, target)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(target))
	}
	return filepath.ToSlash(rel)
}

func riskRequiresConfirmation(toolRisk string, threshold string) bool {
	if strings.TrimSpace(threshold) == "" {
		return false
	}
	return riskRank(toolRisk) >= riskRank(threshold)
}

func riskRank(risk string) int {
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func safeFileName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	name := strings.Trim(builder.String(), "-")
	if name == "" {
		return "memory"
	}
	return name
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules", ".cache", "data":
		return true
	default:
		return false
	}
}

func shouldSkipFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.Size() > 1_000_000 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".sqlite", ".db", ".pdf":
		return true
	default:
		return false
	}
}

func firstMatchingLine(content string, lowerQuery string) (int, string) {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), lowerQuery) {
			return i + 1, truncate(strings.TrimSpace(line), 240)
		}
	}
	return 0, ""
}

func stringSliceArg(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func intArg(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	}
	return fallback
}

func truncate(value string, max int) string {
	if len(value) <= max {
		return value
	}
	return value[:max] + "...[truncated]"
}
