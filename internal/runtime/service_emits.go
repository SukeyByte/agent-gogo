package runtime

import (
	"context"
	"encoding/json"
	"fmt"

	comm "github.com/SukeyByte/agent-gogo/internal/communication"

	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/taskaware"
)

func (s *Service) extractTaskMemories(ctx context.Context, projectID string, task domain.Task, attempt domain.TaskAttempt, review domain.ReviewResult) error {
	if s.memories == nil {
		return nil
	}
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return err
	}
	items, err := taskaware.ExtractTaskMemories(ctx, s.store, project, task, attempt, review)
	if err != nil {
		return err
	}
	for _, item := range items {
		s.memories.Add(item)
		if _, err := s.store.AddTaskEvent(ctx, domain.TaskEvent{
			TaskID:    task.ID,
			AttemptID: attempt.ID,
			Type:      "memory.extracted",
			Message:   item.Summary,
			Payload:   fmt.Sprintf(`{"memory_id":%q,"type":%q,"evidence_ref":%q}`, item.ID, item.Type, item.EvidenceRef),
		}); err != nil {
			return err
		}
	}
	if err := s.persistMemories(ctx); err != nil {
		return err
	}
	return s.log(ctx, "memory.extract", map[string]any{
		"project_id": projectID,
		"task_id":    task.ID,
		"count":      len(items),
	})
}

func (s *Service) persistMemories(ctx context.Context) error {
	if s.memories == nil {
		return nil
	}
	if s.memoryPersistPath != "" {
		return s.memories.SaveJSONL(ctx, s.memoryPersistPath)
	}
	return s.memories.Persist(ctx)
}

func (s *Service) saveSessionContext(ctx context.Context, projectID string) {
	if s.sessionSaver == nil || s.sessionID == "" {
		return
	}
	decision := s.decisionByProjectID[projectID]
	profile := s.profileByProjectID[projectID]
	planningContext := s.contextByProjectID[projectID]

	decisionJSON, _ := json.Marshal(decision)
	profileJSON, _ := json.Marshal(profile)

	var memoryJSON []byte
	if s.memories != nil {
		items := s.memories.Items()
		memoryJSON, _ = json.Marshal(items)
	}

	var personasJSON []byte
	personasJSON, _ = json.Marshal(s.activePersonas)

	sctx := domain.SessionRuntimeContext{
		SessionID:      s.sessionID,
		ProjectID:      projectID,
		ChainDecision:  string(decisionJSON),
		IntentProfile:  string(profileJSON),
		ContextText:    planningContext,
		MemorySnapshot: string(memoryJSON),
		ActivePersonas: string(personasJSON),
	}
	if err := s.sessionSaver.SaveSessionRuntimeContext(ctx, sctx); err != nil {
		s.log(ctx, "session.save_context_failed", map[string]any{"session_id": s.sessionID, "error": err.Error()})
		return
	}
	s.log(ctx, "session.context_saved", map[string]any{"session_id": s.sessionID, "project_id": projectID})
}

func (s *Service) emitTaskProgress(ctx context.Context, projectID string, task domain.Task, status domain.TaskStatus, text string) error {
	return s.emit(ctx, comm.NewProgressIntent(s.communicationChannel, text, map[string]any{
		"project_id": projectID,
		"task_id":    task.ID,
		"task_title": task.Title,
		"status":     string(status),
	}), projectID)
}

func (s *Service) emitTaskBlocked(ctx context.Context, projectID string, task domain.Task, text string) {
	_ = s.emit(ctx, comm.NewBlockedIntent(s.communicationChannel, text, map[string]any{
		"project_id": projectID,
		"task_id":    task.ID,
		"task_title": task.Title,
		"status":     string(domain.TaskStatusBlocked),
	}), projectID)
}

func (s *Service) emitProjectBlocked(ctx context.Context, projectID string, text string) {
	_ = s.emit(ctx, comm.NewBlockedIntent(s.communicationChannel, text, map[string]any{
		"project_id": projectID,
		"status":     string(domain.TaskStatusBlocked),
	}), projectID)
}

func (s *Service) emit(ctx context.Context, intent comm.CommunicationIntent, projectID string) error {
	if s.communication == nil || s.communicationChannel == "" {
		return nil
	}
	intent.ChannelID = s.communicationChannel
	intent.SessionID = s.communicationSession
	intent.ProjectID = projectID
	_, err := s.communication.Dispatch(ctx, intent)
	return err
}
