package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/sukeke/agent-gogo/internal/domain"
)

// --- JSON DTOs ---

type jsonProject struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Goal      string `json:"goal"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type jsonTask struct {
	ID                   string   `json:"id"`
	ProjectID            string   `json:"project_id"`
	Title                string   `json:"title"`
	Description          string   `json:"description"`
	Phase                string   `json:"phase"`
	Status               string   `json:"status"`
	AcceptanceCriteria   []string `json:"acceptance_criteria"`
	RequiredCapabilities []string `json:"required_capabilities"`
	DependsOn            []string `json:"depends_on"`
	CreatedAt            string   `json:"created_at"`
	UpdatedAt            string   `json:"updated_at"`
}

type jsonTaskAttempt struct {
	ID        string `json:"id"`
	TaskID    string `json:"task_id"`
	Number    int    `json:"number"`
	Status    string `json:"status"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at,omitempty"`
	Error     string `json:"error"`
}

type jsonTaskEvent struct {
	ID        string `json:"id"`
	TaskID    string `json:"task_id"`
	AttemptID string `json:"attempt_id"`
	Type      string `json:"type"`
	FromState string `json:"from_state"`
	ToState   string `json:"to_state"`
	Message   string `json:"message"`
	Payload   string `json:"payload"`
	CreatedAt string `json:"created_at"`
}

type jsonToolCall struct {
	ID          string `json:"id"`
	AttemptID   string `json:"attempt_id"`
	Name        string `json:"name"`
	InputJSON   string `json:"input_json"`
	OutputJSON  string `json:"output_json"`
	Status      string `json:"status"`
	Error       string `json:"error"`
	EvidenceRef string `json:"evidence_ref"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type jsonObservation struct {
	ID          string `json:"id"`
	AttemptID   string `json:"attempt_id"`
	ToolCallID  string `json:"tool_call_id"`
	Type        string `json:"type"`
	Summary     string `json:"summary"`
	EvidenceRef string `json:"evidence_ref"`
	Payload     string `json:"payload"`
	CreatedAt   string `json:"created_at"`
}

type jsonArtifact struct {
	ID          string `json:"id"`
	AttemptID   string `json:"attempt_id"`
	ProjectID   string `json:"project_id"`
	Type        string `json:"type"`
	Path        string `json:"path"`
	Description string `json:"description"`
	CreatedAt   string `json:"created_at"`
}

type jsonStats struct {
	ProjectCount int `json:"project_count"`
	TaskCount    int `json:"task_count"`
	DoneCount    int `json:"done_count"`
	RunningCount int `json:"running_count"`
	FailedCount  int `json:"failed_count"`
}

type jsonSession struct {
	ID           string `json:"id"`
	UserID       string `json:"user_id"`
	ChannelType  string `json:"channel_type"`
	ChannelID    string `json:"channel_id"`
	ProjectID    string `json:"project_id"`
	Status       string `json:"status"`
	Title        string `json:"title"`
	LastActiveAt string `json:"last_active_at"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

type jsonSessionContext struct {
	SessionID      string `json:"session_id"`
	ProjectID      string `json:"project_id"`
	ChainDecision  string `json:"chain_decision"`
	IntentProfile  string `json:"intent_profile"`
	ContextText    string `json:"context_text"`
	MemorySnapshot string `json:"memory_snapshot"`
	UpdatedAt      string `json:"updated_at"`
}

// --- Mappers ---

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func formatTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func projectToJSON(p domain.Project) jsonProject {
	return jsonProject{
		ID: p.ID, Name: p.Name, Goal: p.Goal, Status: string(p.Status),
		CreatedAt: formatTime(p.CreatedAt), UpdatedAt: formatTime(p.UpdatedAt),
	}
}

func taskToJSON(t domain.Task) jsonTask {
	return jsonTask{
		ID: t.ID, ProjectID: t.ProjectID, Title: t.Title, Description: t.Description,
		Phase: t.Phase, Status: string(t.Status), AcceptanceCriteria: t.AcceptanceCriteria,
		RequiredCapabilities: t.RequiredCapabilities, DependsOn: t.DependsOn,
		CreatedAt: formatTime(t.CreatedAt), UpdatedAt: formatTime(t.UpdatedAt),
	}
}

func attemptToJSON(a domain.TaskAttempt) jsonTaskAttempt {
	return jsonTaskAttempt{
		ID: a.ID, TaskID: a.TaskID, Number: a.Number, Status: string(a.Status),
		StartedAt: formatTime(a.StartedAt), EndedAt: formatTimePtr(a.EndedAt), Error: a.Error,
	}
}

func eventToJSON(e domain.TaskEvent) jsonTaskEvent {
	return jsonTaskEvent{
		ID: e.ID, TaskID: e.TaskID, AttemptID: e.AttemptID, Type: e.Type,
		FromState: string(e.FromState), ToState: string(e.ToState), Message: e.Message,
		Payload: e.Payload, CreatedAt: formatTime(e.CreatedAt),
	}
}

func toolCallToJSON(tc domain.ToolCall) jsonToolCall {
	return jsonToolCall{
		ID: tc.ID, AttemptID: tc.AttemptID, Name: tc.Name, InputJSON: tc.InputJSON,
		OutputJSON: tc.OutputJSON, Status: string(tc.Status), Error: tc.Error,
		EvidenceRef: tc.EvidenceRef, CreatedAt: formatTime(tc.CreatedAt), UpdatedAt: formatTime(tc.UpdatedAt),
	}
}

func observationToJSON(o domain.Observation) jsonObservation {
	return jsonObservation{
		ID: o.ID, AttemptID: o.AttemptID, ToolCallID: o.ToolCallID, Type: o.Type,
		Summary: o.Summary, EvidenceRef: o.EvidenceRef, Payload: o.Payload, CreatedAt: formatTime(o.CreatedAt),
	}
}

func artifactToJSON(a domain.Artifact) jsonArtifact {
	return jsonArtifact{
		ID: a.ID, AttemptID: a.AttemptID, ProjectID: a.ProjectID, Type: a.Type,
		Path: a.Path, Description: a.Description, CreatedAt: formatTime(a.CreatedAt),
	}
}

// --- API Handlers ---

func (s *APIServer) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]jsonProject, len(projects))
	for i, p := range projects {
		out[i] = projectToJSON(p)
	}
	writeJSON(w, out)
}

func (s *APIServer) handleGetProject(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path, "/api/projects/")
	p, err := s.store.GetProject(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "project not found")
		return
	}
	writeJSON(w, projectToJSON(p))
}

func (s *APIServer) handleListTasks(w http.ResponseWriter, r *http.Request) {
	projectID := extractID(r.URL.Path, "/api/projects/")
	projectID = strings.TrimSuffix(projectID, "/tasks")
	tasks, err := s.store.ListTasksByProject(r.Context(), projectID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]jsonTask, len(tasks))
	for i, t := range tasks {
		out[i] = taskToJSON(t)
	}
	writeJSON(w, out)
}

func (s *APIServer) handleGetTask(w http.ResponseWriter, r *http.Request) {
	id := extractID(r.URL.Path, "/api/tasks/")
	t, err := s.store.GetTask(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "task not found")
		return
	}
	writeJSON(w, taskToJSON(t))
}

func (s *APIServer) handleListAttempts(w http.ResponseWriter, r *http.Request) {
	taskID := extractID(r.URL.Path, "/api/tasks/")
	taskID = strings.TrimSuffix(taskID, "/attempts")
	attempts, err := s.store.ListTaskAttemptsByTask(r.Context(), taskID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]jsonTaskAttempt, len(attempts))
	for i, a := range attempts {
		out[i] = attemptToJSON(a)
	}
	writeJSON(w, out)
}

func (s *APIServer) handleListEvents(w http.ResponseWriter, r *http.Request) {
	taskID := extractID(r.URL.Path, "/api/tasks/")
	taskID = strings.TrimSuffix(taskID, "/events")
	events, err := s.store.ListTaskEvents(r.Context(), taskID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]jsonTaskEvent, len(events))
	for i, e := range events {
		out[i] = eventToJSON(e)
	}
	writeJSON(w, out)
}

func (s *APIServer) handleListToolCalls(w http.ResponseWriter, r *http.Request) {
	attemptID := extractID(r.URL.Path, "/api/attempts/")
	attemptID = strings.TrimSuffix(attemptID, "/tool-calls")
	calls, err := s.store.ListToolCallsByAttempt(r.Context(), attemptID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]jsonToolCall, len(calls))
	for i, c := range calls {
		out[i] = toolCallToJSON(c)
	}
	writeJSON(w, out)
}

func (s *APIServer) handleListObservations(w http.ResponseWriter, r *http.Request) {
	attemptID := extractID(r.URL.Path, "/api/attempts/")
	attemptID = strings.TrimSuffix(attemptID, "/observations")
	obs, err := s.store.ListObservationsByAttempt(r.Context(), attemptID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]jsonObservation, len(obs))
	for i, o := range obs {
		out[i] = observationToJSON(o)
	}
	writeJSON(w, out)
}

func (s *APIServer) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	projectID := extractID(r.URL.Path, "/api/projects/")
	projectID = strings.TrimSuffix(projectID, "/artifacts")
	arts, err := s.store.ListArtifactsByProject(r.Context(), projectID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]jsonArtifact, len(arts))
	for i, a := range arts {
		out[i] = artifactToJSON(a)
	}
	writeJSON(w, out)
}

func (s *APIServer) handleStats(w http.ResponseWriter, r *http.Request) {
	projects, _ := s.store.ListProjects(r.Context())
	stats := jsonStats{ProjectCount: len(projects)}
	for _, p := range projects {
		tasks, _ := s.store.ListTasksByProject(r.Context(), p.ID)
		stats.TaskCount += len(tasks)
		for _, t := range tasks {
			switch t.Status {
			case domain.TaskStatusDone:
				stats.DoneCount++
			case domain.TaskStatusInProgress, domain.TaskStatusImplemented, domain.TaskStatusTesting, domain.TaskStatusReviewing:
				stats.RunningCount++
			case domain.TaskStatusFailed, domain.TaskStatusReviewFailed:
				stats.FailedCount++
			}
		}
	}
	writeJSON(w, stats)
}

func (s *APIServer) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.config)
}

// --- Session Handlers ---

func sessionToJSON(sess domain.Session) jsonSession {
	return jsonSession{
		ID: sess.ID, UserID: sess.UserID, ChannelType: sess.ChannelType, ChannelID: sess.ChannelID,
		ProjectID: sess.ProjectID, Status: string(sess.Status), Title: sess.Title,
		LastActiveAt: formatTime(sess.LastActiveAt), CreatedAt: formatTime(sess.CreatedAt), UpdatedAt: formatTime(sess.UpdatedAt),
	}
}

func (s *APIServer) handleListSessions(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSON(w, []jsonSession{})
		return
	}
	sessions, err := s.sessions.ListSessions(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]jsonSession, len(sessions))
	for i, sess := range sessions {
		out[i] = sessionToJSON(sess)
	}
	writeJSON(w, out)
}

func (s *APIServer) handleGetSession(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}
	id := extractID(r.URL.Path, "/api/sessions/")
	sess, err := s.sessions.GetSession(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, sessionToJSON(sess))
}

func (s *APIServer) handleGetSessionContext(w http.ResponseWriter, r *http.Request) {
	if s.sessions == nil {
		writeJSONError(w, http.StatusNotFound, "session not found")
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/context")
	id := extractID(path, "/api/sessions/")
	sctx, err := s.sessions.GetSessionRuntimeContext(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "session context not found")
		return
	}
	writeJSON(w, jsonSessionContext{
		SessionID:      sctx.SessionID,
		ProjectID:      sctx.ProjectID,
		ChainDecision:  sctx.ChainDecision,
		IntentProfile:  sctx.IntentProfile,
		ContextText:    sctx.ContextText,
		MemorySnapshot: sctx.MemorySnapshot,
		UpdatedAt:      formatTime(sctx.UpdatedAt),
	})
}

func (s *APIServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSONError(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	events, unsubscribe := s.hub.Subscribe(s.channelID)
	defer unsubscribe()

	if lastID := r.Header.Get("Last-Event-ID"); lastID != "" {
		for _, ev := range s.hub.Replay(s.channelID, lastID) {
			writeSSE(w, flusher, ev)
		}
	}

	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			writeSSE(w, flusher, ev)
		case <-r.Context().Done():
			return
		}
	}
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event SSEEvent) {
	fmt.Fprintf(w, "id: %s\nevent: %s\ndata: %s\n\n", event.ID, event.Type, event.Data)
	flusher.Flush()
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	// Ensure typed nil slices serialize as [] not null
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.IsNil() {
		v = reflect.MakeSlice(rv.Type(), 0, 0).Interface()
	}
	json.NewEncoder(w).Encode(v)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func extractID(path, prefix string) string {
	id := strings.TrimPrefix(path, prefix)
	return strings.TrimSuffix(id, "/")
}

func extractFileExtension(name string) string {
	ext := filepath.Ext(name)
	if ext == "" {
		return ""
	}
	return ext[1:]
}
