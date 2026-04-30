package handlers

import (
	"context"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/domain"
)

type Store interface {
	ListProjects(ctx context.Context) ([]domain.Project, error)
	GetProject(ctx context.Context, id string) (domain.Project, error)
	GetTask(ctx context.Context, id string) (domain.Task, error)
	ListTasksByProject(ctx context.Context, projectID string) ([]domain.Task, error)
	ListTaskAttemptsByTask(ctx context.Context, taskID string) ([]domain.TaskAttempt, error)
	ListTaskEvents(ctx context.Context, taskID string) ([]domain.TaskEvent, error)
	ListObservationsByAttempt(ctx context.Context, attemptID string) ([]domain.Observation, error)
	ListToolCallsByAttempt(ctx context.Context, attemptID string) ([]domain.ToolCall, error)
	ListArtifactsByProject(ctx context.Context, projectID string) ([]domain.Artifact, error)
}

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
	BrowserTimeoutSeconds  int      `json:"browser_timeout_seconds"`
}

type Server struct {
	store  Store
	config ConfigView
}

func NewServer(store Store, config ConfigView) http.Handler {
	server := &Server{store: store, config: config}
	mux := http.NewServeMux()
	mux.HandleFunc("/", server.dashboard)
	mux.HandleFunc("/chat", server.chat)
	mux.HandleFunc("/projects", server.projects)
	mux.HandleFunc("/projects/", server.projectDetail)
	mux.HandleFunc("/tasks/", server.taskDetail)
	mux.HandleFunc("/browser", server.browser)
	mux.HandleFunc("/skills", server.skills)
	mux.HandleFunc("/personas", server.personas)
	mux.HandleFunc("/memory", server.memory)
	mux.HandleFunc("/files", server.files)
	mux.HandleFunc("/config", server.configPage)
	return mux
}

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		renderError(w, err)
		return
	}
	stats := dashboardStats{Projects: len(projects)}
	for _, project := range projects {
		tasks, err := s.store.ListTasksByProject(r.Context(), project.ID)
		if err != nil {
			renderError(w, err)
			return
		}
		stats.Tasks += len(tasks)
		for _, task := range tasks {
			if task.Status == domain.TaskStatusDone {
				stats.DoneTasks++
			}
			if task.Status == domain.TaskStatusBlocked || task.Status == domain.TaskStatusFailed || task.Status == domain.TaskStatusReviewFailed {
				stats.ProblemTasks++
			}
		}
	}
	render(w, "Dashboard", "dashboard", map[string]any{
		"Stats":    stats,
		"Projects": limitProjects(projects, 8),
	})
}

func (s *Server) chat(w http.ResponseWriter, r *http.Request) {
	render(w, "Chat", "chat", map[string]any{
		"ChannelID": s.config.ChannelID,
		"SessionID": s.config.SessionID,
	})
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/projects" {
		http.NotFound(w, r)
		return
	}
	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		renderError(w, err)
		return
	}
	render(w, "Projects", "projects", map[string]any{"Projects": projects})
}

func (s *Server) projectDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/projects/"), "/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	project, err := s.store.GetProject(r.Context(), id)
	if err != nil {
		renderError(w, err)
		return
	}
	tasks, err := s.store.ListTasksByProject(r.Context(), id)
	if err != nil {
		renderError(w, err)
		return
	}
	artifacts, err := s.store.ListArtifactsByProject(r.Context(), id)
	if err != nil {
		renderError(w, err)
		return
	}
	render(w, "Project Detail", "project", map[string]any{
		"Project":   project,
		"Tasks":     tasks,
		"Artifacts": artifacts,
	})
}

func (s *Server) taskDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/tasks/"), "/")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	task, err := s.store.GetTask(r.Context(), id)
	if err != nil {
		renderError(w, err)
		return
	}
	attempts, err := s.store.ListTaskAttemptsByTask(r.Context(), id)
	if err != nil {
		renderError(w, err)
		return
	}
	events, err := s.store.ListTaskEvents(r.Context(), id)
	if err != nil {
		renderError(w, err)
		return
	}
	var attemptDetails []attemptDetail
	for _, attempt := range attempts {
		observations, err := s.store.ListObservationsByAttempt(r.Context(), attempt.ID)
		if err != nil {
			renderError(w, err)
			return
		}
		calls, err := s.store.ListToolCallsByAttempt(r.Context(), attempt.ID)
		if err != nil {
			renderError(w, err)
			return
		}
		attemptDetails = append(attemptDetails, attemptDetail{Attempt: attempt, Observations: observations, ToolCalls: calls})
	}
	render(w, "Task Detail", "task", map[string]any{
		"Task":     task,
		"Attempts": attemptDetails,
		"Events":   events,
	})
}

func (s *Server) browser(w http.ResponseWriter, r *http.Request) {
	render(w, "Browser View", "browser", map[string]any{
		"Message": "Browser snapshots appear here when browser tools record observations.",
	})
}

func (s *Server) skills(w http.ResponseWriter, r *http.Request) {
	render(w, "Skills", "skills", map[string]any{"SkillRoots": s.config.SkillRoots})
}

func (s *Server) personas(w http.ResponseWriter, r *http.Request) {
	render(w, "Personas", "personas", map[string]any{"PersonaPath": s.config.PersonaPath})
}

func (s *Server) memory(w http.ResponseWriter, r *http.Request) {
	render(w, "Memory", "memory", map[string]any{"ArtifactPath": s.config.ArtifactPath})
}

func (s *Server) files(w http.ResponseWriter, r *http.Request) {
	render(w, "Files", "files", map[string]any{"WorkspacePath": s.config.WorkspacePath})
}

func (s *Server) configPage(w http.ResponseWriter, r *http.Request) {
	render(w, "Config", "config", map[string]any{"Config": s.config})
}

type dashboardStats struct {
	Projects     int
	Tasks        int
	DoneTasks    int
	ProblemTasks int
}

type attemptDetail struct {
	Attempt      domain.TaskAttempt
	Observations []domain.Observation
	ToolCalls    []domain.ToolCall
}

func render(w http.ResponseWriter, title string, active string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	data["Title"] = title
	data["Active"] = active
	data["Nav"] = navItems()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := consoleTemplate.Execute(w, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func renderError(w http.ResponseWriter, err error) {
	w.WriteHeader(http.StatusInternalServerError)
	render(w, "Error", "error", map[string]any{"Error": err.Error()})
}

func navItems() []navItem {
	return []navItem{
		{Path: "/", Key: "dashboard", Label: "Dashboard"},
		{Path: "/chat", Key: "chat", Label: "Chat"},
		{Path: "/projects", Key: "projects", Label: "Projects"},
		{Path: "/browser", Key: "browser", Label: "Browser View"},
		{Path: "/skills", Key: "skills", Label: "Skills"},
		{Path: "/personas", Key: "personas", Label: "Personas"},
		{Path: "/memory", Key: "memory", Label: "Memory"},
		{Path: "/files", Key: "files", Label: "Files"},
		{Path: "/config", Key: "config", Label: "Config"},
	}
}

type navItem struct {
	Path  string
	Key   string
	Label string
}

func limitProjects(projects []domain.Project, limit int) []domain.Project {
	projects = append([]domain.Project(nil), projects...)
	sort.SliceStable(projects, func(i, j int) bool {
		return projects[i].UpdatedAt.After(projects[j].UpdatedAt)
	})
	if limit > 0 && len(projects) > limit {
		return projects[:limit]
	}
	return projects
}

var consoleTemplate = template.Must(template.New("console").Funcs(template.FuncMap{
	"eq": func(a, b any) bool { return fmt.Sprint(a) == fmt.Sprint(b) },
}).Parse(consoleHTML))

const consoleHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{.Title}} · agent-gogo</title>
  <style>
    :root { color-scheme: light; --ink:#172026; --muted:#5d6972; --line:#d9e0e6; --panel:#ffffff; --bg:#f5f7f9; --accent:#1d6f6f; --bad:#a33b30; --ok:#2f6b3f; }
    * { box-sizing: border-box; }
    body { margin:0; font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background:var(--bg); color:var(--ink); }
    a { color: inherit; }
    .shell { min-height:100vh; display:grid; grid-template-columns: 220px 1fr; }
    nav { border-right:1px solid var(--line); background:#fff; padding:20px 14px; position:sticky; top:0; height:100vh; }
    .brand { font-weight:700; margin:0 0 20px; }
    nav a { display:block; text-decoration:none; padding:9px 10px; border-radius:6px; color:var(--muted); font-size:14px; }
    nav a.active { background:#e6f0ef; color:var(--accent); font-weight:650; }
    main { padding:28px; max-width:1180px; width:100%; }
    h1 { margin:0 0 18px; font-size:28px; letter-spacing:0; }
    h2 { margin:24px 0 10px; font-size:18px; }
    .grid { display:grid; grid-template-columns: repeat(4, minmax(0,1fr)); gap:12px; }
    .metric, .panel, .row { background:var(--panel); border:1px solid var(--line); border-radius:8px; }
    .metric { padding:16px; }
    .metric span { display:block; color:var(--muted); font-size:12px; }
    .metric strong { display:block; margin-top:8px; font-size:26px; }
    .panel { padding:16px; margin-top:12px; overflow:auto; }
    table { width:100%; border-collapse:collapse; font-size:14px; }
    th, td { text-align:left; padding:10px 8px; border-bottom:1px solid var(--line); vertical-align:top; }
    th { color:var(--muted); font-weight:600; }
    .status { display:inline-block; padding:3px 7px; border-radius:999px; background:#eef2f5; color:var(--muted); font-size:12px; }
    .status.DONE, .status.SUCCEEDED, .status.PASSED, .status.APPROVED { color:var(--ok); background:#eaf3ec; }
    .status.FAILED, .status.BLOCKED, .status.REVIEW_FAILED, .status.REJECTED { color:var(--bad); background:#f8ebe9; }
    pre { white-space:pre-wrap; word-break:break-word; margin:0; font-size:13px; }
    .muted { color:var(--muted); }
    .stack { display:grid; gap:12px; }
    @media (max-width: 820px) { .shell { grid-template-columns:1fr; } nav { position:relative; height:auto; border-right:0; border-bottom:1px solid var(--line); } .grid { grid-template-columns:repeat(2,minmax(0,1fr)); } main { padding:20px; } }
  </style>
</head>
<body>
  <div class="shell">
    <nav>
      <p class="brand">agent-gogo</p>
      {{range .Nav}}<a href="{{.Path}}" class="{{if eq $.Active .Key}}active{{end}}">{{.Label}}</a>{{end}}
    </nav>
    <main>
      <h1>{{.Title}}</h1>
      {{if .Error}}<div class="panel"><strong>Error</strong><pre>{{.Error}}</pre></div>{{end}}
      {{if eq .Active "dashboard"}}
        <div class="grid">
          <div class="metric"><span>Projects</span><strong>{{.Stats.Projects}}</strong></div>
          <div class="metric"><span>Tasks</span><strong>{{.Stats.Tasks}}</strong></div>
          <div class="metric"><span>Done</span><strong>{{.Stats.DoneTasks}}</strong></div>
          <div class="metric"><span>Needs attention</span><strong>{{.Stats.ProblemTasks}}</strong></div>
        </div>
        <h2>Recent Projects</h2>
        <div class="panel">{{template "projectTable" .Projects}}</div>
      {{end}}
      {{if eq .Active "chat"}}<div class="panel"><p>Channel: <strong>{{.ChannelID}}</strong></p><p>Session: <strong>{{.SessionID}}</strong></p><p class="muted">Chat submission is wired through Runtime Service channel events next; this page is the console surface for that flow.</p></div>{{end}}
      {{if eq .Active "projects"}}<div class="panel">{{template "projectTable" .Projects}}</div>{{end}}
      {{if eq .Active "project"}}
        <div class="panel"><p><strong>{{.Project.Name}}</strong> <span class="status {{.Project.Status}}">{{.Project.Status}}</span></p><p>{{.Project.Goal}}</p></div>
        <h2>Tasks</h2><div class="panel">{{template "taskTable" .Tasks}}</div>
        <h2>Artifacts</h2><div class="panel">{{template "artifactTable" .Artifacts}}</div>
      {{end}}
      {{if eq .Active "task"}}
        <div class="panel"><p><strong>{{.Task.Title}}</strong> <span class="status {{.Task.Status}}">{{.Task.Status}}</span></p><p>{{.Task.Description}}</p></div>
        <h2>Attempts</h2>
        <div class="stack">{{range .Attempts}}<div class="panel"><p>Attempt {{.Attempt.Number}} <span class="status {{.Attempt.Status}}">{{.Attempt.Status}}</span></p><h2>Tool Calls</h2>{{template "toolTable" .ToolCalls}}<h2>Observations</h2>{{template "observationTable" .Observations}}</div>{{else}}<div class="panel muted">No attempts yet.</div>{{end}}</div>
        <h2>Events</h2><div class="panel">{{template "eventTable" .Events}}</div>
      {{end}}
      {{if eq .Active "browser"}}<div class="panel"><p>{{.Message}}</p></div>{{end}}
      {{if eq .Active "skills"}}<div class="panel"><pre>{{range .SkillRoots}}{{.}}
{{end}}</pre></div>{{end}}
      {{if eq .Active "personas"}}<div class="panel"><p>Persona path: <strong>{{.PersonaPath}}</strong></p></div>{{end}}
      {{if eq .Active "memory"}}<div class="panel"><p>Memory and artifacts path: <strong>{{.ArtifactPath}}</strong></p></div>{{end}}
      {{if eq .Active "files"}}<div class="panel"><p>Workspace path: <strong>{{.WorkspacePath}}</strong></p></div>{{end}}
      {{if eq .Active "config"}}<div class="panel"><pre>{{printf "%+v" .Config}}</pre></div>{{end}}
    </main>
  </div>
</body>
</html>
{{define "projectTable"}}<table><thead><tr><th>Name</th><th>Status</th><th>Goal</th><th>Updated</th></tr></thead><tbody>{{range .}}<tr><td><a href="/projects/{{.ID}}">{{.Name}}</a></td><td><span class="status {{.Status}}">{{.Status}}</span></td><td>{{.Goal}}</td><td>{{.UpdatedAt}}</td></tr>{{else}}<tr><td colspan="4" class="muted">No projects yet.</td></tr>{{end}}</tbody></table>{{end}}
{{define "taskTable"}}<table><thead><tr><th>Task</th><th>Status</th><th>Description</th></tr></thead><tbody>{{range .}}<tr><td><a href="/tasks/{{.ID}}">{{.Title}}</a></td><td><span class="status {{.Status}}">{{.Status}}</span></td><td>{{.Description}}</td></tr>{{else}}<tr><td colspan="3" class="muted">No tasks yet.</td></tr>{{end}}</tbody></table>{{end}}
{{define "artifactTable"}}<table><thead><tr><th>Type</th><th>Path</th><th>Description</th></tr></thead><tbody>{{range .}}<tr><td>{{.Type}}</td><td>{{.Path}}</td><td>{{.Description}}</td></tr>{{else}}<tr><td colspan="3" class="muted">No artifacts yet.</td></tr>{{end}}</tbody></table>{{end}}
{{define "toolTable"}}<table><thead><tr><th>Name</th><th>Status</th><th>Evidence</th><th>Error</th></tr></thead><tbody>{{range .}}<tr><td>{{.Name}}</td><td><span class="status {{.Status}}">{{.Status}}</span></td><td>{{.EvidenceRef}}</td><td>{{.Error}}</td></tr>{{else}}<tr><td colspan="4" class="muted">No tool calls.</td></tr>{{end}}</tbody></table>{{end}}
{{define "observationTable"}}<table><thead><tr><th>Type</th><th>Summary</th><th>Evidence</th></tr></thead><tbody>{{range .}}<tr><td>{{.Type}}</td><td>{{.Summary}}</td><td>{{.EvidenceRef}}</td></tr>{{else}}<tr><td colspan="3" class="muted">No observations.</td></tr>{{end}}</tbody></table>{{end}}
{{define "eventTable"}}<table><thead><tr><th>Type</th><th>Transition</th><th>Message</th></tr></thead><tbody>{{range .}}<tr><td>{{.Type}}</td><td>{{.FromState}} -> {{.ToState}}</td><td>{{.Message}}</td></tr>{{else}}<tr><td colspan="3" class="muted">No events.</td></tr>{{end}}</tbody></table>{{end}}`
