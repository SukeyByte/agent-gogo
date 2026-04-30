package handlers

import (
	"encoding/json"
	"net/http"
)

func (s *APIServer) handlePostMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	var req InboundEvent
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if req.Type == "" {
		req.Type = "goal.submitted"
	}
	if req.ChannelID == "" {
		req.ChannelID = s.channelID
	}
	if req.SessionID == "" {
		req.SessionID = s.sessionID
	}

	if s.sender == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "runtime not available")
		return
	}

	if err := s.sender.HandleChannelEvent(r.Context(), req); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}

func (s *APIServer) handlePostConfirmation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}

	var req InboundConfirmation
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if s.sender == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "runtime not available")
		return
	}

	if err := s.sender.HandleUserConfirmation(r.Context(), req); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{"status": "accepted"})
}
