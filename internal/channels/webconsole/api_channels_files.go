package webconsole

import (
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type jsonChannel struct {
	ID           string                `json:"id"`
	Type         string                `json:"type"`
	Name         string                `json:"name"`
	Enabled      bool                  `json:"enabled"`
	Capabilities jsonChannelCapability `json:"capabilities"`
}

type jsonChannelCapability struct {
	SupportedMessageTypes []string `json:"supported_message_types"`
	SupportedInteractions []string `json:"supported_interactions"`
	SupportsConfirmation  bool     `json:"supports_confirmation"`
	SupportsStreaming     bool     `json:"supports_streaming"`
	SupportsFileRequest   bool     `json:"supports_file_request"`
}

type jsonFileEntry struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Type     string `json:"type"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
}

func (s *APIServer) handleListChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	channelID := strings.TrimSpace(s.channelID)
	if channelID == "" {
		channelID = "web"
	}
	writeJSON(w, []jsonChannel{
		{
			ID:      channelID,
			Type:    "web",
			Name:    "Web Console",
			Enabled: true,
			Capabilities: jsonChannelCapability{
				SupportedMessageTypes: []string{"text", "task_card", "artifact"},
				SupportedInteractions: []string{"modal", "button"},
				SupportsConfirmation:  true,
				SupportsStreaming:     true,
				SupportsFileRequest:   true,
			},
		},
		{
			ID:      "cli",
			Type:    "cli",
			Name:    "CLI",
			Enabled: s.config.ChannelID == "cli",
			Capabilities: jsonChannelCapability{
				SupportedMessageTypes: []string{"text"},
				SupportedInteractions: []string{"prompt", "confirm_yes_no"},
				SupportsConfirmation:  true,
				SupportsStreaming:     false,
				SupportsFileRequest:   false,
			},
		},
	})
}

func (s *APIServer) handleListFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	root := strings.TrimSpace(s.config.ArtifactPath)
	if root == "" {
		writeJSON(w, []jsonFileEntry{})
		return
	}
	dir, rel, ok := safeArtifactPath(root, r.URL.Query().Get("path"))
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "invalid artifact path")
		return
	}
	info, err := os.Stat(dir)
	if os.IsNotExist(err) {
		writeJSON(w, []jsonFileEntry{})
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !info.IsDir() {
		writeJSON(w, []jsonFileEntry{fileEntryJSON(rel, info)})
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]jsonFileEntry, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		entryRel := filepath.ToSlash(filepath.Join(rel, entry.Name()))
		out = append(out, fileEntryJSON(entryRel, info))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type == "dir"
		}
		return out[i].Path < out[j].Path
	})
	writeJSON(w, out)
}

func (s *APIServer) handleReadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "GET only")
		return
	}
	target, rel, ok := safeArtifactPath(s.config.ArtifactPath, r.URL.Query().Get("path"))
	if !ok || strings.TrimSpace(rel) == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid artifact path")
		return
	}
	info, err := os.Stat(target)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "file not found")
		return
	}
	if info.IsDir() {
		writeJSONError(w, http.StatusBadRequest, "path is a directory")
		return
	}
	if info.Size() > 512*1024 {
		writeJSONError(w, http.StatusBadRequest, "file too large to preview")
		return
	}
	data, err := os.ReadFile(target)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, map[string]any{
		"path":    rel,
		"content": string(data),
		"size":    info.Size(),
	})
}

func fileEntryJSON(rel string, info os.FileInfo) jsonFileEntry {
	typ := "file"
	size := info.Size()
	if info.IsDir() {
		typ = "dir"
		size = 0
	}
	return jsonFileEntry{
		Name:     info.Name(),
		Path:     filepath.ToSlash(rel),
		Type:     typ,
		Size:     size,
		Modified: formatTime(info.ModTime()),
	}
}

func safeArtifactPath(root string, requested string) (string, string, bool) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", "", false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", false
	}
	rel := filepath.Clean(strings.TrimSpace(requested))
	if rel == "." || rel == string(filepath.Separator) {
		rel = ""
	}
	rel = strings.TrimPrefix(filepath.ToSlash(rel), "/")
	if rel == ".." || strings.HasPrefix(rel, "../") || strings.Contains(rel, "/../") {
		return "", "", false
	}
	target := filepath.Join(absRoot, filepath.FromSlash(rel))
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", "", false
	}
	if absTarget != absRoot && !strings.HasPrefix(absTarget, absRoot+string(filepath.Separator)) {
		return "", "", false
	}
	return absTarget, rel, true
}
