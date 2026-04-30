package tools

import (
	"sort"

	"github.com/sukeke/agent-gogo/internal/capability"
	"github.com/sukeke/agent-gogo/internal/observability"
)

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
