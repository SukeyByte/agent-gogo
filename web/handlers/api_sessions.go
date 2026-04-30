package handlers

import (
	"net/http"
	"strings"

	"github.com/sukeke/agent-gogo/internal/domain"
)

func (s *APIServer) handleSessionAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if s.sessions == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "session store not available")
		return
	}
	path := strings.TrimSuffix(r.URL.Path, "/")
	action := path[strings.LastIndex(path, "/")+1:]
	sessionPath := strings.TrimSuffix(path, "/"+action)
	sessionID := extractID(sessionPath, "/api/sessions/")
	if strings.TrimSpace(sessionID) == "" {
		writeJSONError(w, http.StatusBadRequest, "session id is required")
		return
	}
	switch action {
	case "pause":
		sess, err := s.setSessionStatus(r, sessionID, domain.SessionStatusPaused)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, sessionToJSON(sess))
	case "resume":
		sess, err := s.setSessionStatus(r, sessionID, domain.SessionStatusActive)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if s.sender != nil {
			_ = s.sender.HandleChannelEvent(r.Context(), InboundEvent{
				Type:      "session.resume",
				ChannelID: s.channelID,
				SessionID: sessionID,
				ProjectID: sess.ProjectID,
			})
		}
		writeJSON(w, sessionToJSON(sess))
	case "expire":
		sess, err := s.setSessionStatus(r, sessionID, domain.SessionStatusExpired)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, sessionToJSON(sess))
	case "delete":
		_ = s.sessions.DeleteSessionRuntimeContext(r.Context(), sessionID)
		if err := s.sessions.DeleteSession(r.Context(), sessionID); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"status": "deleted"})
	default:
		writeJSONError(w, http.StatusNotFound, "unknown session action")
	}
}

func (s *APIServer) setSessionStatus(r *http.Request, sessionID string, status domain.SessionStatus) (domain.Session, error) {
	sess, err := s.sessions.GetSession(r.Context(), sessionID)
	if err != nil {
		return domain.Session{}, err
	}
	sess.Status = status
	return s.sessions.UpdateSession(r.Context(), sess)
}
