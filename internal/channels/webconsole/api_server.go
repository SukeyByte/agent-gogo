package webconsole

import (
	"context"
	"io/fs"
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
	store      Store
	sessions   SessionStore
	sender     ChannelEventSender
	hub        *SSEHub
	config     ConfigView
	channelID  string
	sessionID  string
	distDir    string
	embeddedFS fs.FS
	skills     *skill.Registry
	personas   *persona.Registry
	memories   *memory.Index
	configPath string
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

func (s *APIServer) UseConfigPath(path string) {
	s.configPath = path
}

// UseEmbeddedDist registers a build-time embedded copy of the frontend dist
// (rooted at the dist directory). It is used when the on-disk dist directory
// is unavailable so the binary works from any working directory.
func (s *APIServer) UseEmbeddedDist(dist fs.FS) {
	s.embeddedFS = dist
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
	mux.HandleFunc("/api/channels", s.handleListChannels)
	mux.HandleFunc("/api/files/content", s.handleReadFile)
	mux.HandleFunc("/api/files", s.handleListFiles)
	mux.HandleFunc("/api/skills/search-github", s.handleSearchGithubSkills)
	mux.HandleFunc("/api/skills/github-files", s.handleListGithubSkillFiles)
	mux.HandleFunc("/api/skills/install", s.handleInstallSkill)
	mux.HandleFunc("/api/skills/", s.apiSkillRoutes)
	mux.HandleFunc("/api/skills", s.handleListSkills)
	mux.HandleFunc("/api/personas/create", s.handleCreatePersona)
	mux.HandleFunc("/api/personas/", s.apiPersonaRoutes)
	mux.HandleFunc("/api/personas", s.handleListPersonas)
	mux.HandleFunc("/api/memory/create", s.handleCreateMemory)
	mux.HandleFunc("/api/memory/", s.apiMemoryRoutes)
	mux.HandleFunc("/api/memory", s.handleListMemory)
	mux.HandleFunc("/api/events", s.handleSSE)

	// Try API routes first
	handler, pattern := mux.Handler(r)
	if pattern != "" {
		handler.ServeHTTP(w, r)
		return
	}

	// Static files / SPA fallback
	if s.distDir != "" || s.embeddedFS != nil {
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
	} else if strings.HasSuffix(path, "/test-results") {
		s.handleListTestResults(w, r)
	} else if strings.HasSuffix(path, "/review-results") {
		s.handleListReviewResults(w, r)
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

func (s *APIServer) apiSkillRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/delete") {
		s.handleDeleteSkill(w, r)
	} else {
		s.handleGetSkill(w, r)
	}
}

func (s *APIServer) apiPersonaRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	switch {
	case strings.HasSuffix(path, "/update"):
		s.handleUpdatePersona(w, r)
	case strings.HasSuffix(path, "/delete"):
		s.handleDeletePersona(w, r)
	default:
		s.handleGetPersona(w, r)
	}
}

func (s *APIServer) apiMemoryRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if strings.HasSuffix(path, "/delete") {
		s.handleDeleteMemory(w, r)
	} else {
		s.handleGetMemory(w, r)
	}
}

func (s *APIServer) serveSPA(w http.ResponseWriter, r *http.Request) {
	cleanPath := filepath.Clean(r.URL.Path)
	if cleanPath == "/" || cleanPath == "." {
		cleanPath = "/index.html"
	}
	rel := strings.TrimPrefix(cleanPath, "/")
	if rel == "" || hasDotDotElement(rel) {
		http.NotFound(w, r)
		return
	}

	// On-disk dist wins when present so dev rebuilds are served without recompiling.
	if s.distDir != "" {
		filePath := filepath.Join(s.distDir, cleanPath)
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			s.serveDiskFile(w, r, filePath)
			return
		}
	}
	if s.embeddedFS != nil {
		if _, err := fs.Stat(s.embeddedFS, rel); err == nil {
			http.ServeFileFS(w, r, s.embeddedFS, rel)
			return
		}
	}

	// SPA fallback: serve index.html for client-side routing
	if s.distDir != "" {
		indexPath := filepath.Join(s.distDir, "index.html")
		if info, err := os.Stat(indexPath); err == nil && !info.IsDir() {
			s.serveDiskFile(w, r, indexPath)
			return
		}
	}
	if s.embeddedFS != nil {
		if _, err := fs.Stat(s.embeddedFS, "index.html"); err == nil {
			http.ServeFileFS(w, r, s.embeddedFS, "index.html")
			return
		}
	}
	http.NotFound(w, r)
}

func hasDotDotElement(rel string) bool {
	for _, part := range strings.Split(rel, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func (s *APIServer) serveDiskFile(w http.ResponseWriter, r *http.Request, filePath string) {
	// Set content type for common assets
	switch filepath.Ext(filePath) {
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
}

// WalkDistDir returns true if the dist directory contains built assets
func (s *APIServer) HasDistAssets() bool {
	if s.distDir != "" {
		if entries, err := os.ReadDir(s.distDir); err == nil {
			for _, e := range entries {
				if e.Name() == "index.html" {
					return true
				}
			}
		}
	}
	if s.embeddedFS != nil {
		if _, err := fs.Stat(s.embeddedFS, "index.html"); err == nil {
			return true
		}
	}
	return false
}
