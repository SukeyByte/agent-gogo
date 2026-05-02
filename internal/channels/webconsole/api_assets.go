package webconsole

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SukeyByte/agent-gogo/internal/memory"
	"github.com/SukeyByte/agent-gogo/internal/persona"
	"github.com/SukeyByte/agent-gogo/internal/skill"
)

type jsonSkill struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	AllowedTools []string          `json:"allowed_tools"`
	Path         string            `json:"path"`
	VersionHash  string            `json:"version_hash"`
	Frontmatter  map[string]string `json:"frontmatter"`
	Body         string            `json:"body"`
}

type jsonPersona struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Type         string   `json:"type"`
	Description  string   `json:"description"`
	Path         string   `json:"path"`
	VersionHash  string   `json:"version_hash"`
	StyleRules   []string `json:"style_rules"`
	Boundaries   []string `json:"boundaries"`
	Instructions string   `json:"instructions"`
}

type jsonMemory struct {
	ID           string   `json:"id"`
	Scope        string   `json:"scope"`
	Type         string   `json:"type"`
	Tags         []string `json:"tags"`
	Summary      string   `json:"summary"`
	Body         string   `json:"body"`
	Confidence   float64  `json:"confidence"`
	ArtifactRef  string   `json:"artifact_ref"`
	SourceTaskID string   `json:"source_task_id"`
	VersionHash  string   `json:"version_hash"`
}

func (s *APIServer) handleListSkills(w http.ResponseWriter, r *http.Request) {
	if s.skills == nil {
		writeJSON(w, []jsonSkill{})
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	cards, err := s.skills.Search(r.Context(), query, 0)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]jsonSkill, 0, len(cards))
	for _, card := range cards {
		pkg, err := s.skills.Load(r.Context(), card.ID)
		if err != nil {
			continue
		}
		out = append(out, skillToJSON(pkg))
	}
	writeJSON(w, out)
}

func (s *APIServer) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	if s.skills == nil {
		writeJSONError(w, http.StatusNotFound, "skill not found")
		return
	}
	id := extractID(r.URL.Path, "/api/skills/")
	pkg, err := s.skills.Load(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "skill not found")
		return
	}
	writeJSON(w, skillToJSON(pkg))
}

func (s *APIServer) handleListPersonas(w http.ResponseWriter, r *http.Request) {
	if s.personas == nil {
		writeJSON(w, []jsonPersona{})
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	cards, err := s.personas.Search(r.Context(), query, 0)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]jsonPersona, 0, len(cards))
	for _, card := range cards {
		item, err := s.personas.Load(r.Context(), card.ID)
		if err != nil {
			continue
		}
		out = append(out, personaToJSON(item))
	}
	writeJSON(w, out)
}

func (s *APIServer) handleGetPersona(w http.ResponseWriter, r *http.Request) {
	if s.personas == nil {
		writeJSONError(w, http.StatusNotFound, "persona not found")
		return
	}
	id := extractID(r.URL.Path, "/api/personas/")
	item, err := s.personas.Load(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "persona not found")
		return
	}
	writeJSON(w, personaToJSON(item))
}

func (s *APIServer) handleListMemory(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeJSON(w, []jsonMemory{})
		return
	}
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	cards, err := s.memories.Search(r.Context(), query, scope, 0)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]jsonMemory, 0, len(cards))
	for _, card := range cards {
		item, err := s.memories.Load(r.Context(), card.ID)
		if err != nil {
			continue
		}
		out = append(out, memoryToJSON(item))
	}
	writeJSON(w, out)
}

func (s *APIServer) handleGetMemory(w http.ResponseWriter, r *http.Request) {
	if s.memories == nil {
		writeJSONError(w, http.StatusNotFound, "memory not found")
		return
	}
	id := extractID(r.URL.Path, "/api/memory/")
	item, err := s.memories.Load(r.Context(), id)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "memory not found")
		return
	}
	writeJSON(w, memoryToJSON(item))
}

func skillToJSON(pkg skill.Package) jsonSkill {
	return jsonSkill{
		ID:           pkg.ID,
		Name:         pkg.Name,
		Description:  pkg.Description,
		AllowedTools: cloneStringSlice(pkg.AllowedTools),
		Path:         pkg.Path,
		VersionHash:  pkg.VersionHash,
		Frontmatter:  cloneStringMap(pkg.Frontmatter),
		Body:         pkg.Instructions,
	}
}

func personaToJSON(item persona.Persona) jsonPersona {
	return jsonPersona{
		ID:           item.ID,
		Name:         item.Name,
		Type:         item.Type,
		Description:  item.Description,
		Path:         item.Path,
		VersionHash:  item.VersionHash,
		StyleRules:   []string{},
		Boundaries:   []string{},
		Instructions: item.Instructions,
	}
}

func memoryToJSON(item memory.Item) jsonMemory {
	return jsonMemory{
		ID:           item.ID,
		Scope:        item.Scope,
		Type:         item.Type,
		Tags:         cloneStringSlice(item.Tags),
		Summary:      item.Summary,
		Body:         item.Body,
		Confidence:   item.Confidence,
		ArtifactRef:  item.ArtifactRef,
		SourceTaskID: item.SourceTaskID,
		VersionHash:  item.VersionHash,
	}
}

func urlQueryEscape(s string) string {
	return url.QueryEscape(s)
}

func cloneStringSlice(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	return append([]string(nil), values...)
}

func cloneStringMap(values map[string]string) map[string]string {
	out := make(map[string]string, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

// --- Memory mutation handlers ---

func (s *APIServer) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if s.memories == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "memory index not available")
		return
	}
	var req struct {
		Scope   string   `json:"scope"`
		Type    string   `json:"type"`
		Summary string   `json:"summary"`
		Body    string   `json:"body"`
		Tags    []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Summary) == "" {
		writeJSONError(w, http.StatusBadRequest, "summary is required")
		return
	}
	scope := req.Scope
	if scope == "" {
		scope = "working"
	}
	typ := req.Type
	if typ == "" {
		typ = "fuzzy"
	}
	item := memory.Item{
		Card: memory.Card{
			ID:          fmt.Sprintf("mem-%d", time.Now().UnixNano()),
			Scope:       scope,
			Type:        typ,
			Tags:        req.Tags,
			Summary:     req.Summary,
			Confidence:  0.8,
			VersionHash: fmt.Sprintf("m%x", time.Now().UnixNano()),
		},
		Body: req.Body,
	}
	s.memories.Add(item)
	if err := s.memories.Persist(r.Context()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "persist failed: "+err.Error())
		return
	}
	writeJSON(w, memoryToJSON(item))
}

func (s *APIServer) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if s.memories == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "memory index not available")
		return
	}
	id := extractID(strings.TrimSuffix(r.URL.Path, "/delete"), "/api/memory/")
	if strings.TrimSpace(id) == "" {
		writeJSONError(w, http.StatusBadRequest, "memory id is required")
		return
	}
	if _, err := s.memories.Load(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusNotFound, "memory not found")
		return
	}
	s.memories.Remove(id)
	if err := s.memories.Persist(r.Context()); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "persist failed: "+err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

// --- Skill GitHub search + install handlers ---

type githubRepoItem struct {
	Owner       string `json:"owner"`
	Repo        string `json:"repo"`
	FullName    string `json:"full_name"`
	Description string `json:"description"`
	Stars       int    `json:"stars"`
	HTMLURL     string `json:"html_url"`
	DefaultBranch string `json:"default_branch"`
}

type githubSkillFile struct {
	Owner  string `json:"owner"`
	Repo   string `json:"repo"`
	Path   string `json:"path"`
	Branch string `json:"branch"`
}

func githubDo(ctx context.Context, url string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "agent-gogo")
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return body, resp.StatusCode, nil
}

func (s *APIServer) handleSearchGithubSkills(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeJSONError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}
	ghURL := "https://api.github.com/search/repositories?q=" + urlQueryEscape(query) + "+SKILL.md&per_page=10&sort=stars"
	body, status, err := githubDo(r.Context(), ghURL)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "github search failed: "+err.Error())
		return
	}
	if status != http.StatusOK {
		writeJSONError(w, status, "github returned "+fmt.Sprint(status)+": "+string(body))
		return
	}
	var result struct {
		Items []struct {
			Owner         struct{ Login string } `json:"owner"`
			Name          string                  `json:"name"`
			FullName      string                  `json:"full_name"`
			Description   string                  `json:"description"`
			StargazersCount int                   `json:"stargazers_count"`
			HTMLURL       string                  `json:"html_url"`
			DefaultBranch string                  `json:"default_branch"`
		} `json:"items"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to parse github response: "+err.Error())
		return
	}
	out := make([]githubRepoItem, 0, len(result.Items))
	for _, item := range result.Items {
		out = append(out, githubRepoItem{
			Owner:         item.Owner.Login,
			Repo:          item.Name,
			FullName:      item.FullName,
			Description:   item.Description,
			Stars:         item.StargazersCount,
			HTMLURL:       item.HTMLURL,
			DefaultBranch: item.DefaultBranch,
		})
	}
	writeJSON(w, out)
}

func (s *APIServer) handleListGithubSkillFiles(w http.ResponseWriter, r *http.Request) {
	owner := r.URL.Query().Get("owner")
	repo := r.URL.Query().Get("repo")
	if owner == "" || repo == "" {
		writeJSONError(w, http.StatusBadRequest, "owner and repo are required")
		return
	}
	branch := r.URL.Query().Get("branch")
	if branch == "" {
		branch = "main"
	}
	ghURL := "https://api.github.com/repos/" + owner + "/" + repo + "/git/trees/" + urlQueryEscape(branch) + "?recursive=1"
	body, status, err := githubDo(r.Context(), ghURL)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "github tree failed: "+err.Error())
		return
	}
	if status != http.StatusOK {
		writeJSONError(w, status, "github returned "+fmt.Sprint(status)+": "+string(body))
		return
	}
	var tree struct {
		Tree []struct {
			Path string `json:"path"`
			Type string `json:"type"`
		} `json:"tree"`
	}
	if err := json.Unmarshal(body, &tree); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to parse tree: "+err.Error())
		return
	}
	out := make([]githubSkillFile, 0)
	for _, entry := range tree.Tree {
		if entry.Type == "blob" && filepath.Base(entry.Path) == "SKILL.md" {
			out = append(out, githubSkillFile{
				Owner:  owner,
				Repo:   repo,
				Path:   entry.Path,
				Branch: branch,
			})
		}
	}
	writeJSON(w, out)
}

func (s *APIServer) handleInstallSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if s.skills == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "skill registry not available")
		return
	}
	var req struct {
		Owner  string `json:"owner"`
		Repo   string `json:"repo"`
		Path   string `json:"path"`
		Branch string `json:"branch"`
		Root   string `json:"root"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Owner == "" || req.Repo == "" || req.Path == "" {
		writeJSONError(w, http.StatusBadRequest, "owner, repo, and path are required")
		return
	}
	root := req.Root
	if root == "" && len(s.config.SkillRoots) > 0 {
		root = s.config.SkillRoots[0]
	}
	if root == "" {
		writeJSONError(w, http.StatusBadRequest, "no skill root configured")
		return
	}
	// Fetch file content from GitHub raw (no auth needed for public repos)
	branch := req.Branch
	if branch == "" {
		branch = "main"
	}
	rawURL := "https://raw.githubusercontent.com/" + req.Owner + "/" + req.Repo + "/" + branch + "/" + req.Path
	rawResp, err := http.Get(rawURL)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "github fetch failed: "+err.Error())
		return
	}
	defer rawResp.Body.Close()
	if rawResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(rawResp.Body)
		writeJSONError(w, rawResp.StatusCode, "github returned "+fmt.Sprint(rawResp.StatusCode)+": "+string(body))
		return
	}
	content, err := io.ReadAll(rawResp.Body)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to read file content: "+err.Error())
		return
	}
	// Determine skill ID from directory name
	dirName := filepath.Base(filepath.Dir(req.Path))
	if dirName == "." || dirName == "/" {
		dirName = strings.TrimSuffix(filepath.Base(req.Path), filepath.Ext(req.Path))
	}
	dir := filepath.Join(root, dirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to create skill directory: "+err.Error())
		return
	}
	skillPath := filepath.Join(dir, "SKILL.md")
	if err := os.WriteFile(skillPath, content, 0o644); err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to write skill file: "+err.Error())
		return
	}
	// Re-parse and add to registry
	card, err := skill.ParseSkillCard(r.Context(), skillPath)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to parse installed skill: "+err.Error())
		return
	}
	s.skills.AddCard(card)
	pkg, _ := s.skills.Load(r.Context(), card.ID)
	writeJSON(w, skillToJSON(pkg))
}

func (s *APIServer) handleDeleteSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if s.skills == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "skill registry not available")
		return
	}
	id := extractID(strings.TrimSuffix(r.URL.Path, "/delete"), "/api/skills/")
	if err := s.skills.Delete(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}

// --- Persona mutation handlers ---

func (s *APIServer) handleCreatePersona(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if s.personas == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "persona registry not available")
		return
	}
	var req struct {
		Name         string `json:"name"`
		Type         string `json:"type"`
		Description  string `json:"description"`
		Instructions string `json:"instructions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	root := s.config.PersonaPath
	if root == "" {
		writeJSONError(w, http.StatusBadRequest, "no persona path configured")
		return
	}
	card, err := s.personas.Create(r.Context(), root, req.Name, req.Type, req.Description, req.Instructions)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	persona, _ := s.personas.Load(r.Context(), card.ID)
	writeJSON(w, personaToJSON(persona))
}

func (s *APIServer) handleUpdatePersona(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if s.personas == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "persona registry not available")
		return
	}
	id := extractID(strings.TrimSuffix(r.URL.Path, "/update"), "/api/personas/")
	var req struct {
		Name         string `json:"name"`
		Type         string `json:"type"`
		Description  string `json:"description"`
		Instructions string `json:"instructions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	card, err := s.personas.Update(r.Context(), id, req.Name, req.Type, req.Description, req.Instructions)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	persona, _ := s.personas.Load(r.Context(), card.ID)
	writeJSON(w, personaToJSON(persona))
}

func (s *APIServer) handleDeletePersona(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "POST only")
		return
	}
	if s.personas == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "persona registry not available")
		return
	}
	id := extractID(strings.TrimSuffix(r.URL.Path, "/delete"), "/api/personas/")
	if err := s.personas.Delete(r.Context(), id); err != nil {
		writeJSONError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]string{"status": "deleted"})
}
