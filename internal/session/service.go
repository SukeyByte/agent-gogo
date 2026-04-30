package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/SukeyByte/agent-gogo/internal/domain"
)

type SessionStore interface {
	CreateSession(ctx context.Context, session domain.Session) (domain.Session, error)
	GetSession(ctx context.Context, id string) (domain.Session, error)
	UpdateSession(ctx context.Context, session domain.Session) (domain.Session, error)
	TouchSession(ctx context.Context, id string) error
	FindActiveSessionByChannel(ctx context.Context, channelType, channelID string) (domain.Session, error)
	FindActiveSessionByUser(ctx context.Context, userID string) (domain.Session, error)
	ListSessionsByUser(ctx context.Context, userID string, status domain.SessionStatus) ([]domain.Session, error)
	ExpireIdleSessions(ctx context.Context, maxIdle time.Duration) (int, error)

	SaveSessionRuntimeContext(ctx context.Context, sctx domain.SessionRuntimeContext) error
	GetSessionRuntimeContext(ctx context.Context, sessionID string) (domain.SessionRuntimeContext, error)
	DeleteSessionRuntimeContext(ctx context.Context, sessionID string) error
}

type ProjectCreator interface {
	CreateProject(ctx context.Context, name string, goal string) (domain.Project, error)
}

type ActiveSession struct {
	Session    domain.Session
	RuntimeCtx *domain.SessionRuntimeContext
	CancelFunc context.CancelFunc
}

type Config struct {
	MaxIdle time.Duration
}

func DefaultConfig() Config {
	return Config{
		MaxIdle: 24 * time.Hour,
	}
}

type Service struct {
	store  SessionStore
	config Config
	active map[string]*ActiveSession
	mu     sync.RWMutex
}

func NewService(store SessionStore, config Config) *Service {
	if config.MaxIdle <= 0 {
		config.MaxIdle = 24 * time.Hour
	}
	return &Service{
		store:  store,
		config: config,
		active: map[string]*ActiveSession{},
	}
}

type CreateRequest struct {
	UserID      string
	ChannelType string
	ChannelID   string
	Goal        string
	Title       string
}

func (s *Service) Create(ctx context.Context, req CreateRequest) (domain.Session, error) {
	channelType := strings.TrimSpace(req.ChannelType)
	if channelType == "" {
		channelType = "cli"
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = extractTitle(req.Goal)
	}
	session := domain.Session{
		UserID:      strings.TrimSpace(req.UserID),
		ChannelType: channelType,
		ChannelID:   strings.TrimSpace(req.ChannelID),
		Status:      domain.SessionStatusActive,
		Title:       title,
		Metadata:    "{}",
	}
	created, err := s.store.CreateSession(ctx, session)
	if err != nil {
		return domain.Session{}, err
	}
	s.mu.Lock()
	s.active[created.ID] = &ActiveSession{Session: created}
	s.mu.Unlock()
	return created, nil
}

func (s *Service) Get(ctx context.Context, id string) (domain.Session, error) {
	s.mu.RLock()
	if active, ok := s.active[id]; ok {
		s.mu.RUnlock()
		return active.Session, nil
	}
	s.mu.RUnlock()
	return s.store.GetSession(ctx, id)
}

type FindOrCreateRequest struct {
	ChannelType string
	ChannelID   string
	UserID      string
	Goal        string
}

func (s *Service) FindOrCreate(ctx context.Context, req FindOrCreateRequest) (domain.Session, error) {
	userID := strings.TrimSpace(req.UserID)

	// 1. 按 user 查活跃 session（跨 channel 共享）
	if userID != "" {
		session, err := s.store.FindActiveSessionByUser(ctx, userID)
		if err == nil && session.ID != "" {
			// 更新最后活跃通道
			session.ChannelType = req.ChannelType
			session.ChannelID = req.ChannelID
			updated, _ := s.store.UpdateSession(ctx, session)
			_ = s.store.TouchSession(ctx, session.ID)
			s.mu.Lock()
			if _, ok := s.active[session.ID]; !ok {
				s.active[session.ID] = &ActiveSession{Session: updated}
			}
			s.mu.Unlock()
			return updated, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return domain.Session{}, err
		}
	}

	// 2. 按 channel 查活跃 session（兼容无 userID 场景）
	session, err := s.store.FindActiveSessionByChannel(ctx, req.ChannelType, req.ChannelID)
	if err == nil && session.ID != "" {
		_ = s.store.TouchSession(ctx, session.ID)
		s.mu.Lock()
		if _, ok := s.active[session.ID]; !ok {
			s.active[session.ID] = &ActiveSession{Session: session}
		}
		s.mu.Unlock()
		return session, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return domain.Session{}, err
	}

	// 3. 都找不到，创建新 session
	return s.Create(ctx, CreateRequest{
		UserID:      userID,
		ChannelType: req.ChannelType,
		ChannelID:   req.ChannelID,
		Goal:        req.Goal,
	})
}

func (s *Service) BindProject(ctx context.Context, sessionID string, projectID string, goal string) (domain.Session, error) {
	session, err := s.Get(ctx, sessionID)
	if err != nil {
		return domain.Session{}, err
	}
	session.ProjectID = projectID
	if session.Title == "" {
		session.Title = extractTitle(goal)
	}
	updated, err := s.store.UpdateSession(ctx, session)
	if err != nil {
		return domain.Session{}, err
	}
	s.mu.Lock()
	if active, ok := s.active[sessionID]; ok {
		active.Session = updated
	}
	s.mu.Unlock()
	return updated, nil
}

func (s *Service) Pause(ctx context.Context, sessionID string) error {
	session, err := s.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	if session.Status != domain.SessionStatusActive {
		return fmt.Errorf("cannot pause session in status %s", session.Status)
	}
	session.Status = domain.SessionStatusPaused
	if _, err := s.store.UpdateSession(ctx, session); err != nil {
		return err
	}
	s.mu.Lock()
	if active, ok := s.active[sessionID]; ok {
		if active.CancelFunc != nil {
			active.CancelFunc()
		}
		active.Session.Status = domain.SessionStatusPaused
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) Resume(ctx context.Context, sessionID string) (domain.Session, error) {
	session, err := s.store.GetSession(ctx, sessionID)
	if err != nil {
		return domain.Session{}, err
	}
	if session.Status != domain.SessionStatusPaused && session.Status != domain.SessionStatusActive {
		return domain.Session{}, fmt.Errorf("cannot resume session in status %s", session.Status)
	}
	session.Status = domain.SessionStatusActive
	updated, err := s.store.UpdateSession(ctx, session)
	if err != nil {
		return domain.Session{}, err
	}
	s.mu.Lock()
	s.active[sessionID] = &ActiveSession{Session: updated}
	s.mu.Unlock()
	return updated, nil
}

func (s *Service) Complete(ctx context.Context, sessionID string) error {
	session, err := s.Get(ctx, sessionID)
	if err != nil {
		return err
	}
	session.Status = domain.SessionStatusCompleted
	if _, err := s.store.UpdateSession(ctx, session); err != nil {
		return err
	}
	s.mu.Lock()
	if active, ok := s.active[sessionID]; ok {
		if active.CancelFunc != nil {
			active.CancelFunc()
		}
		delete(s.active, sessionID)
	}
	s.mu.Unlock()
	return nil
}

func (s *Service) SaveRuntimeContext(ctx context.Context, sctx domain.SessionRuntimeContext) error {
	return s.store.SaveSessionRuntimeContext(ctx, sctx)
}

func (s *Service) GetRuntimeContext(ctx context.Context, sessionID string) (domain.SessionRuntimeContext, error) {
	return s.store.GetSessionRuntimeContext(ctx, sessionID)
}

// SaveSessionRuntimeContext satisfies runtime.SessionContextSaver.
func (s *Service) SaveSessionRuntimeContext(ctx context.Context, sctx domain.SessionRuntimeContext) error {
	return s.SaveRuntimeContext(ctx, sctx)
}

func (s *Service) Touch(ctx context.Context, sessionID string) error {
	return s.store.TouchSession(ctx, sessionID)
}

func (s *Service) SetCancelFunc(sessionID string, cancel context.CancelFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if active, ok := s.active[sessionID]; ok {
		active.CancelFunc = cancel
	}
}

func (s *Service) ExpireIdle(ctx context.Context) (int, error) {
	return s.store.ExpireIdleSessions(ctx, s.config.MaxIdle)
}

func (s *Service) ActiveCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.active)
}

func extractTitle(goal string) string {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return ""
	}
	runes := []rune(goal)
	if len(runes) > 40 {
		return string(runes[:40]) + "..."
	}
	return goal
}

func sessionToJSON(session domain.Session) string {
	data, _ := json.Marshal(session)
	return string(data)
}
