package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func (s *APIServer) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, s.config)
	case http.MethodPost, http.MethodPatch:
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
			return
		}
		payload := flattenConfigPayload(body)
		if len(payload) == 0 {
			writeJSONError(w, http.StatusBadRequest, "no hot-update config fields found")
			return
		}
		s.applyConfigPayload(payload)
		if s.sender != nil {
			if err := s.sender.HandleChannelEvent(r.Context(), InboundEvent{
				Type:      "config.update",
				ChannelID: s.channelID,
				SessionID: s.sessionID,
				Payload:   payload,
			}); err != nil {
				writeJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
		}
		writeJSON(w, map[string]string{"status": "accepted"})
	default:
		writeJSONError(w, http.StatusMethodNotAllowed, "GET, POST, or PATCH only")
	}
}

func (s *APIServer) handleConfigCommand(w http.ResponseWriter, r *http.Request, text string) bool {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "/config") {
		return false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(text, "/config"))
	if raw == "" {
		writeJSON(w, s.config)
		return true
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid /config JSON: "+err.Error())
		return true
	}
	payload := flattenConfigPayload(body)
	if len(payload) == 0 {
		writeJSONError(w, http.StatusBadRequest, "no hot-update config fields found")
		return true
	}
	s.applyConfigPayload(payload)
	if s.sender != nil {
		if err := s.sender.HandleChannelEvent(r.Context(), InboundEvent{
			Type:      "config.update",
			ChannelID: s.channelID,
			SessionID: s.sessionID,
			Payload:   payload,
		}); err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return true
		}
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "accepted", "type": "config.update"})
	return true
}

func flattenConfigPayload(body map[string]any) map[string]string {
	payload := map[string]string{}
	put := func(key string, value any) {
		if value == nil {
			return
		}
		payload[key] = stringifyConfigValue(value)
	}
	if runtime, _ := body["runtime"].(map[string]any); runtime != nil {
		put("context_max_chars", firstPresent(runtime, "context_max_chars", "token_budget"))
		put("max_tasks_per_project", runtime["max_tasks_per_project"])
	}
	if security, _ := body["security"].(map[string]any); security != nil {
		put("allow_shell", security["allow_shell"])
		put("require_confirm_high_risk", security["require_confirm_high_risk"])
		put("shell_allowlist", security["shell_allowlist"])
	}
	if llm, _ := body["llm"].(map[string]any); llm != nil {
		put("llm_timeout_seconds", firstPresent(llm, "timeout", "timeout_seconds"))
	}
	if browser, _ := body["browser"].(map[string]any); browser != nil {
		put("browser_timeout_seconds", firstPresent(browser, "timeout", "timeout_seconds"))
	}
	for _, key := range []string{
		"context_max_chars",
		"max_tasks_per_project",
		"allow_shell",
		"require_confirm_high_risk",
		"shell_allowlist",
		"llm_timeout_seconds",
		"browser_timeout_seconds",
	} {
		if value, ok := body[key]; ok {
			put(key, value)
		}
	}
	return payload
}

func (s *APIServer) applyConfigPayload(payload map[string]string) {
	if value, ok := intPayload(payload, "context_max_chars"); ok {
		s.config.ContextMaxChars = value
	}
	if value, ok := intPayload(payload, "max_tasks_per_project"); ok {
		s.config.MaxTasksPerProject = value
	}
	if value, ok := boolPayload(payload, "allow_shell"); ok {
		s.config.AllowShell = value
	}
	if value, ok := boolPayload(payload, "require_confirm_high_risk"); ok {
		s.config.RequireConfirmHighRisk = value
	}
	if raw := strings.TrimSpace(payload["shell_allowlist"]); raw != "" {
		var values []string
		if err := json.Unmarshal([]byte(raw), &values); err == nil {
			s.config.ShellAllowlist = values
		} else {
			s.config.ShellAllowlist = splitCommaList(raw)
		}
	}
	if value, ok := intPayload(payload, "llm_timeout_seconds"); ok {
		s.config.LLMTimeoutSeconds = value
	}
	if value, ok := intPayload(payload, "browser_timeout_seconds"); ok {
		s.config.BrowserTimeoutSeconds = value
	}
}

func firstPresent(values map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := values[key]; ok {
			return value
		}
	}
	return nil
}

func stringifyConfigValue(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case bool:
		return strconv.FormatBool(typed)
	case float64:
		if typed == float64(int(typed)) {
			return strconv.Itoa(int(typed))
		}
		return fmt.Sprintf("%g", typed)
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(data)
	}
}

func intPayload(payload map[string]string, key string) (int, bool) {
	raw := strings.TrimSpace(payload[key])
	if raw == "" {
		return 0, false
	}
	value, err := strconv.Atoi(raw)
	return value, err == nil
}

func boolPayload(payload map[string]string, key string) (bool, bool) {
	raw := strings.TrimSpace(payload[key])
	if raw == "" {
		return false, false
	}
	value, err := strconv.ParseBool(raw)
	return value, err == nil
}

func splitCommaList(raw string) []string {
	var values []string
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			values = append(values, part)
		}
	}
	return values
}
