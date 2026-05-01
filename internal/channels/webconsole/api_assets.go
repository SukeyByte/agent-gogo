package webconsole

import (
	"net/http"
	"strings"

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
