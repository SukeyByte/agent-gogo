package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sukeke/agent-gogo/internal/chain"
	"github.com/sukeke/agent-gogo/internal/domain"
	"github.com/sukeke/agent-gogo/internal/intent"
	"github.com/sukeke/agent-gogo/internal/memory"
	"github.com/sukeke/agent-gogo/internal/tools"
)

type ToolRuntime interface {
	Call(ctx context.Context, req tools.CallRequest) (tools.CallResponse, error)
}

type Loop interface {
	Discover(ctx context.Context, req Request) (Result, error)
}

type Request struct {
	Project       domain.Project
	ChainDecision chain.Decision
	IntentProfile intent.Profile
}

type Evidence struct {
	Name        string         `json:"name"`
	Summary     string         `json:"summary"`
	EvidenceRef string         `json:"evidence_ref"`
	Output      map[string]any `json:"output,omitempty"`
}

type Result struct {
	Summary  string     `json:"summary"`
	Evidence []Evidence `json:"evidence"`
}

type ToolLoop struct {
	tools    ToolRuntime
	memories *memory.Index
}

func NewToolLoop(runtime ToolRuntime) *ToolLoop {
	return &ToolLoop{tools: runtime}
}

func (l *ToolLoop) UseMemory(index *memory.Index) *ToolLoop {
	l.memories = index
	return l
}

func (l *ToolLoop) Discover(ctx context.Context, req Request) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	var evidence []Evidence
	for _, probe := range l.probes(req) {
		item, ok := l.callProbe(ctx, probe)
		if ok {
			evidence = append(evidence, item)
		}
	}
	if l.memories != nil {
		cards, err := l.memories.Search(ctx, discoveryQuery(req), "project", 5)
		if err == nil && len(cards) > 0 {
			evidence = append(evidence, Evidence{
				Name:        "memory.search",
				Summary:     fmt.Sprintf("found %d relevant project memory item(s)", len(cards)),
				EvidenceRef: "memory://project",
				Output:      map[string]any{"cards": cards},
			})
		}
	}
	return Result{
		Summary:  summarizeEvidence(evidence),
		Evidence: evidence,
	}, nil
}

type probe struct {
	name string
	args map[string]any
}

func (l *ToolLoop) probes(req Request) []probe {
	query := discoveryQuery(req)
	probes := []probe{
		{name: "code.index", args: map[string]any{"limit": 30, "symbol_limit": 60, "max_files": 2000}},
		{name: "git.status", args: map[string]any{}},
	}
	if query != "" {
		probes = append(probes, probe{name: "code.search", args: map[string]any{"query": query, "limit": 20}})
	}
	for _, path := range []string{"README.md", "go.mod", "package.json"} {
		probes = append(probes, probe{name: "file.read", args: map[string]any{"path": path, "max_bytes": 6000}})
	}
	return probes
}

func (l *ToolLoop) callProbe(ctx context.Context, probe probe) (Evidence, bool) {
	if l.tools == nil {
		return Evidence{}, false
	}
	resp, err := l.tools.Call(ctx, tools.CallRequest{Name: probe.name, Args: probe.args})
	if err != nil || !resp.Result.Success {
		return Evidence{}, false
	}
	return Evidence{
		Name:        probe.name,
		Summary:     summarizeOutput(probe.name, resp.Result.Output),
		EvidenceRef: resp.Result.EvidenceRef,
		Output:      compactOutput(resp.Result.Output),
	}, true
}

func discoveryQuery(req Request) string {
	parts := []string{req.Project.Goal, req.IntentProfile.TaskType}
	parts = append(parts, req.IntentProfile.Domains...)
	parts = append(parts, req.IntentProfile.RequiredCapabilities...)
	parts = append(parts, req.ChainDecision.ToolNames...)
	query := strings.Join(parts, " ")
	query = strings.Join(strings.Fields(query), " ")
	if len(query) > 120 {
		query = query[:120]
	}
	return query
}

func summarizeEvidence(evidence []Evidence) string {
	if len(evidence) == 0 {
		return "DiscoveryLoop found no readable evidence; planner must keep assumptions explicit."
	}
	var lines []string
	for _, item := range evidence {
		lines = append(lines, "- "+item.Name+": "+item.Summary)
	}
	return strings.Join(lines, "\n")
}

func summarizeOutput(name string, output map[string]any) string {
	switch name {
	case "code.index":
		return fmt.Sprintf("files=%v symbols=%v languages=%s", output["file_count"], output["symbol_count"], jsonString(output["languages"]))
	case "code.search":
		return fmt.Sprintf("matches=%v query=%v", output["count"], output["query"])
	case "file.read":
		return fmt.Sprintf("read %v bytes from %v", output["bytes"], output["path"])
	case "git.status":
		status, _ := output["status"].(string)
		if strings.TrimSpace(status) == "" {
			return "git tree appears clean"
		}
		return "git status has changes: " + trim(status, 300)
	default:
		return trim(jsonString(output), 300)
	}
}

func compactOutput(output map[string]any) map[string]any {
	if output == nil {
		return nil
	}
	data, err := json.Marshal(output)
	if err != nil || len(data) <= 2000 {
		return output
	}
	return map[string]any{"summary": trim(string(data), 2000)}
}

func jsonString(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func trim(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
