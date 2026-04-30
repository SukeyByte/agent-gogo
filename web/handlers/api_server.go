package handlers

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/memory"
	"github.com/SukeyByte/agent-gogo/internal/persona"
	"github.com/SukeyByte/agent-gogo/internal/skill"
)

type SessionStore interface {
	ListSessions(ctx context.Context) ([]domain.Session, error)
	GetSession(ctx context.Context, id string) (domain.Session, error)
	UpdateSession(ctx context.Context, session domain.Session) (domain.Session, error)
	DeleteSession(ctx context.Context, id string) error
	GetSessionRuntimeContext(ctx context.Context, sessionID string) (domain.SessionRuntimeContext, error)
	DeleteSessionRuntimeContext(ctx context.Context, sessionID string) error
}

type APIServer struct {
	store     Store
	sessions  SessionStore
	sender    ChannelEventSender
	hub       *SSEHub
	config    ConfigView
	channelID string
	sessionID string
	distDir   string
	skills    *skill.Registry
	personas  *persona.Registry
	memories  *memory.Index
}

func NewAPIServer(store Store, sender ChannelEventSender, hub *SSEHub, config ConfigView, channelID, sessionID, distDir string) *APIServer {
	return &APIServer{
		store:     store,
		sessions:  nil, // set via UseSessionStore after creation
		sender:    sender,
		hub:       hub,
		config:    config,
		channelID: channelID,
		sessionID: sessionID,
		distDir:   distDir,
	}
}

func (s *APIServer) UseSessionStore(sessions SessionStore) {
	s.sessions = sessions
}

func (s *APIServer) UseAssets(skills *skill.Registry, personas *persona.Registry, memories *memory.Index) {
	s.skills = skills
	s.personas = personas
	s.memories = memories
}

func (s *APIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/projects/", s.apiProjectRoutes)
	mux.HandleFunc("/api/projects", s.handleListProjects)
	mux.HandleFunc("/api/tasks/", s.apiTaskRoutes)
	mux.HandleFunc("/api/attempts/", s.apiAttemptRoutes)
	mux.HandleFunc("/api/sessions/", s.apiSessionRoutes)
	mux.HandleFunc("/api/sessions", s.handleListSessions)
	mux.HandleFunc("/api/message", s.handlePostMessage)
	mux.HandleFunc("/api/confirmation", s.handlePostConfirmation)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/skills/", s.handleGetSkill)
	mux.HandleFunc("/api/skills", s.handleListSkills)
	mux.HandleFunc("/api/personas/", s.handleGetPersona)
	mux.HandleFunc("/api/personas", s.handleListPersonas)
	mux.HandleFunc("/api/memory/", s.handleGetMemory)
	mux.HandleFunc("/api/memory", s.handleListMemory)
	mux.HandleFunc("/api/events", s.handleSSE)

	// Try API routes first
	handler, pattern := mux.Handler(r)
	if pattern != "" {
		handler.ServeHTTP(w, r)
		return
	}

	// Static files / SPA fallback
	if s.distDir != "" {
		s.serveSPA(w, r)
		return
	}

	http.NotFound(w, r)
}

func (s *APIServer) apiProjectRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/tasks") {
		s.handleListTasks(w, r)
	} else if strings.HasSuffix(path, "/artifacts") {
		s.handleListArtifacts(w, r)
	} else {
		s.handleGetProject(w, r)
	}
}

func (s *APIServer) apiTaskRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/attempts") {
		s.handleListAttempts(w, r)
	} else if strings.HasSuffix(path, "/events") {
		s.handleListEvents(w, r)
	} else {
		s.handleGetTask(w, r)
	}
}

func (s *APIServer) apiAttemptRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/tool-calls") {
		s.handleListToolCalls(w, r)
	} else if strings.HasSuffix(path, "/observations") {
		s.handleListObservations(w, r)
	} else {
		http.NotFound(w, r)
	}
}

func (s *APIServer) apiSessionRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/context"):
		s.handleGetSessionContext(w, r)
	case strings.HasSuffix(path, "/pause"), strings.HasSuffix(path, "/resume"), strings.HasSuffix(path, "/expire"), strings.HasSuffix(path, "/delete"):
		s.handleSessionAction(w, r)
	default:
		s.handleGetSession(w, r)
	}
}

func (s *APIServer) serveSPA(w http.ResponseWriter, r *http.Request) {
	// Check if distDir exists
	if _, err := os.Stat(s.distDir); os.IsNotExist(err) {
		http.NotFound(w, r)
		return
	}

	// Try to serve static file
	cleanPath := filepath.Clean(r.URL.Path)
	if cleanPath == "/" {
		cleanPath = "/index.html"
	}
	filePath := filepath.Join(s.distDir, cleanPath)

	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		// Set content type for common assets
		ext := filepath.Ext(filePath)
		switch ext {
		case ".js":
			w.Header().Set("Content-Type", "application/javascript")
		case ".css":
			w.Header().Set("Content-Type", "text/css")
		case ".html":
			w.Header().Set("Content-Type", "text/html")
		case ".svg":
			w.Header().Set("Content-Type", "image/svg+xml")
		case ".png":
			w.Header().Set("Content-Type", "image/png")
		case ".ico":
			w.Header().Set("Content-Type", "image/x-icon")
		case ".json":
			w.Header().Set("Content-Type", "application/json")
		case ".woff", ".woff2":
			w.Header().Set("Content-Type", "font/woff2")
		}
		http.ServeFile(w, r, filePath)
		return
	}

	// SPA fallback: serve index.html for client-side routing
	indexPath := filepath.Join(s.distDir, "index.html")
	if _, err := os.Stat(indexPath); err == nil {
		http.ServeFile(w, r, indexPath)
		return
	}

	http.NotFound(w, r)
}

// WalkDistDir returns true if the dist directory contains built assets
func (s *APIServer) HasDistAssets() bool {
	if s.distDir == "" {
		return false
	}
	entries, err := os.ReadDir(s.distDir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.Name() == "index.html" {
			return true
		}
	}
	return false
}
