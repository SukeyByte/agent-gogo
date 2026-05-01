package capability

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrToolUnavailable = errors.New("tool unavailable")
var ErrCapabilityUnavailable = errors.New("capability unavailable")

type ToolSpec struct {
	Name          string
	Description   string
	RiskLevel     string
	RequiresShell bool
}

type Policy struct {
	AllowedTools              map[string]bool
	AllowShell                bool
	ShellAllowlist            []string
	RequireConfirmationAtRisk string
}

type ToolRequest struct {
	ToolName string
	Args     map[string]any
	Policy   Policy
}

type ToolResolution struct {
	Tool                 ToolSpec
	Capabilities         []string
	RequiresConfirmation bool
	ConfirmationReason   string
}

type AvailabilityRequest struct {
	RequiredCapabilities []string
	Policy               Policy
}

type Availability struct {
	Available            bool
	MissingCapabilities  []string
	BlockedCapabilities  []string
	AvailableTools       []ToolSpec
	RequiresConfirmation bool
}

type Registry struct {
	tools              map[string]ToolSpec
	capabilityToTools  map[string]map[string]struct{}
	toolToCapabilities map[string]map[string]struct{}
}

func NewRegistry(specs ...ToolSpec) *Registry {
	registry := &Registry{
		tools:              map[string]ToolSpec{},
		capabilityToTools:  map[string]map[string]struct{}{},
		toolToCapabilities: map[string]map[string]struct{}{},
	}
	for _, spec := range specs {
		registry.RegisterTool(spec)
	}
	return registry
}

func (r *Registry) RegisterTool(spec ToolSpec, capabilities ...string) {
	r.ensure()
	spec.Name = strings.TrimSpace(spec.Name)
	if spec.Name == "" {
		return
	}
	r.tools[spec.Name] = spec
	if len(capabilities) == 0 {
		capabilities = inferredCapabilities(spec.Name)
	}
	for _, capabilityName := range capabilities {
		r.mapCapability(capabilityName, spec.Name)
	}
}

func (r *Registry) AddMapping(capabilityName string, toolName string) {
	r.ensure()
	if _, ok := r.tools[toolName]; !ok {
		return
	}
	r.mapCapability(capabilityName, toolName)
}

func (r *Registry) ResolveTool(ctx context.Context, req ToolRequest) (ToolResolution, error) {
	if err := ctx.Err(); err != nil {
		return ToolResolution{}, err
	}
	r.ensure()
	toolName := strings.TrimSpace(req.ToolName)
	spec, ok := r.tools[toolName]
	if !ok {
		return ToolResolution{}, fmt.Errorf("%w: %s", ErrToolUnavailable, toolName)
	}
	if req.Policy.AllowedTools != nil && !req.Policy.AllowedTools[toolName] {
		return ToolResolution{}, fmt.Errorf("%w: %s is not allowed", ErrToolUnavailable, toolName)
	}
	if spec.RequiresShell && !req.Policy.AllowShell {
		return ToolResolution{}, fmt.Errorf("%w: shell is disabled for %s", ErrToolUnavailable, toolName)
	}
	if spec.RequiresShell && len(req.Policy.ShellAllowlist) > 0 {
		command, _ := req.Args["command"].(string)
		if !ShellCommandAllowed(command, req.Policy.ShellAllowlist) {
			return ToolResolution{}, fmt.Errorf("%w: shell command is not allowlisted for %s", ErrToolUnavailable, toolName)
		}
	}
	resolution := ToolResolution{
		Tool:         spec,
		Capabilities: sortedSetValues(r.toolToCapabilities[toolName]),
	}
	if RiskRequiresConfirmation(spec.RiskLevel, req.Policy.RequireConfirmationAtRisk) {
		resolution.RequiresConfirmation = true
		resolution.ConfirmationReason = fmt.Sprintf("%s risk is at or above %s", spec.RiskLevel, req.Policy.RequireConfirmationAtRisk)
	}
	return resolution, nil
}

func (r *Registry) CheckAvailability(ctx context.Context, req AvailabilityRequest) (Availability, error) {
	if err := ctx.Err(); err != nil {
		return Availability{}, err
	}
	r.ensure()
	required := sortedUnique(req.RequiredCapabilities)
	if len(required) == 0 {
		required = sortedSetValues(r.capabilityToTools)
	}
	availability := Availability{Available: true}
	toolSeen := map[string]struct{}{}
	for _, capabilityName := range required {
		toolNames := sortedSetValues(r.capabilityToTools[normalizeCapability(capabilityName)])
		if len(toolNames) == 0 {
			availability.Available = false
			availability.MissingCapabilities = append(availability.MissingCapabilities, capabilityName)
			continue
		}
		var blockers []string
		var matched bool
		for _, toolName := range toolNames {
			resolution, err := r.ResolveTool(ctx, ToolRequest{ToolName: toolName, Policy: req.Policy})
			if err != nil {
				blockers = append(blockers, err.Error())
				continue
			}
			matched = true
			if resolution.RequiresConfirmation {
				availability.RequiresConfirmation = true
			}
			if _, ok := toolSeen[resolution.Tool.Name]; !ok {
				toolSeen[resolution.Tool.Name] = struct{}{}
				availability.AvailableTools = append(availability.AvailableTools, resolution.Tool)
			}
		}
		if !matched {
			availability.Available = false
			availability.BlockedCapabilities = append(availability.BlockedCapabilities, blockers...)
		}
	}
	sort.SliceStable(availability.AvailableTools, func(i, j int) bool {
		return availability.AvailableTools[i].Name < availability.AvailableTools[j].Name
	})
	return availability, nil
}

func (r *Registry) ToolsForCapability(capabilityName string) []ToolSpec {
	r.ensure()
	names := sortedSetValues(r.capabilityToTools[normalizeCapability(capabilityName)])
	tools := make([]ToolSpec, 0, len(names))
	for _, name := range names {
		if spec, ok := r.tools[name]; ok {
			tools = append(tools, spec)
		}
	}
	return tools
}

func (r *Registry) ToolSpecs() []ToolSpec {
	r.ensure()
	specs := make([]ToolSpec, 0, len(r.tools))
	for _, spec := range r.tools {
		specs = append(specs, spec)
	}
	sort.SliceStable(specs, func(i, j int) bool {
		return specs[i].Name < specs[j].Name
	})
	return specs
}

func RiskRequiresConfirmation(toolRisk string, threshold string) bool {
	if strings.TrimSpace(threshold) == "" {
		return false
	}
	return riskRank(toolRisk) >= riskRank(threshold)
}

func ShellCommandAllowed(command string, allowlist []string) bool {
	commandParts := strings.Fields(command)
	if len(commandParts) == 0 {
		return false
	}
	for _, allowed := range allowlist {
		allowedParts := strings.Fields(allowed)
		if len(allowedParts) == 0 || len(commandParts) < len(allowedParts) {
			continue
		}
		matches := true
		for i, allowedPart := range allowedParts {
			if commandParts[i] != allowedPart {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func (r *Registry) ensure() {
	if r.tools == nil {
		r.tools = map[string]ToolSpec{}
	}
	if r.capabilityToTools == nil {
		r.capabilityToTools = map[string]map[string]struct{}{}
	}
	if r.toolToCapabilities == nil {
		r.toolToCapabilities = map[string]map[string]struct{}{}
	}
}

func (r *Registry) mapCapability(capabilityName string, toolName string) {
	capabilityName = normalizeCapability(capabilityName)
	if capabilityName == "" || strings.TrimSpace(toolName) == "" {
		return
	}
	if r.capabilityToTools[capabilityName] == nil {
		r.capabilityToTools[capabilityName] = map[string]struct{}{}
	}
	if r.toolToCapabilities[toolName] == nil {
		r.toolToCapabilities[toolName] = map[string]struct{}{}
	}
	r.capabilityToTools[capabilityName][toolName] = struct{}{}
	r.toolToCapabilities[toolName][capabilityName] = struct{}{}
}

func inferredCapabilities(toolName string) []string {
	toolName = strings.TrimSpace(toolName)
	if toolName == "" {
		return nil
	}
	capabilities := []string{toolName}
	parts := strings.Split(toolName, ".")
	if len(parts) > 1 && parts[0] != "" {
		capabilities = append(capabilities, parts[0])
	}
	switch toolName {
	case "code.search", "code.index", "code.symbols", "file.read":
		capabilities = append(capabilities, "inspect", "read")
	case "file.write", "file.patch", "artifact.write", "document.write":
		capabilities = append(capabilities, "write", "create_artifact")
	case "file.diff", "git.diff", "git.status":
		capabilities = append(capabilities, "verify", "inspect_changes")
	case "test.run", "shell.run":
		capabilities = append(capabilities, "execute", "verify")
	case "memory.save":
		capabilities = append(capabilities, "memory", "persist_memory")
	}
	return sortedUnique(capabilities)
}

func sortedSetValues[T any](set map[string]T) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func sortedUnique(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizeCapability(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func normalizeCapability(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	switch value {
	case "read-file", "file-read", "readfile":
		return "read"
	case "edit-file", "file-edit", "modify-file", "file-modify", "code-generation", "codegen":
		return "write"
	case "git-diff", "diff":
		return "inspect_changes"
	case "web", "webpage", "web-page", "web-read", "browser-read":
		return "browser"
	case "document-understanding", "document-read", "doc-read", "summarization", "summarisation", "summary", "summarize":
		return "read"
	case "test", "testing", "run-tests":
		return "verify"
	}
	return value
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
