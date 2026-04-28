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
		Name:        "test.run",
		Description: "Run a configured test command.",
		RiskLevel:   "medium",
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
			rel, _ := filepath.Rel(root, path)
			matches = append(matches, map[string]any{
				"path":    filepath.ToSlash(rel),
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
	rel, _ := filepath.Rel(root, target)
	return map[string]any{
		"artifact_ref": filepath.ToSlash(rel),
		"summary":      summary,
		"bytes":        len([]byte(content)),
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
