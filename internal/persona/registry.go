package persona

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/contextbuilder"
)

var ErrPersonaNotFound = errors.New("persona not found")

type Card struct {
	ID          string
	Name        string
	Type        string
	Description string
	Path        string
	VersionHash string
}

type Persona struct {
	Card
	Instructions string
}

type Registry struct {
	cards map[string]Card
}

func Discover(ctx context.Context, root string) (*Registry, error) {
	registry := &Registry{cards: map[string]Card{}}
	if root == "" {
		return registry, nil
	}
	if _, err := os.Stat(root); err != nil {
		if os.IsNotExist(err) {
			return registry, nil
		}
		return nil, err
	}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		card, err := parseCard(path)
		if err != nil {
			return err
		}
		registry.cards[card.ID] = card
		return nil
	}); err != nil {
		return nil, err
	}
	return registry, nil
}

func (r *Registry) Search(ctx context.Context, query string, limit int) ([]Card, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	query = strings.ToLower(query)
	cards := make([]Card, 0, len(r.cards))
	for _, card := range r.cards {
		haystack := strings.ToLower(card.Name + " " + card.Type + " " + card.Description)
		if query == "" || strings.Contains(haystack, query) || tokenMatch(haystack, query) {
			cards = append(cards, card)
		}
	}
	sort.SliceStable(cards, func(i, j int) bool {
		if cards[i].ID != cards[j].ID {
			return cards[i].ID < cards[j].ID
		}
		return cards[i].VersionHash < cards[j].VersionHash
	})
	if limit > 0 && len(cards) > limit {
		cards = cards[:limit]
	}
	return cards, nil
}

func (r *Registry) Load(ctx context.Context, id string) (Persona, error) {
	if err := ctx.Err(); err != nil {
		return Persona{}, err
	}
	card, ok := r.cards[id]
	if !ok {
		return Persona{}, ErrPersonaNotFound
	}
	body, _, err := readMarkdown(card.Path)
	if err != nil {
		return Persona{}, err
	}
	return Persona{Card: card, Instructions: strings.TrimSpace(body)}, nil
}

func (p Persona) ContextPersona() contextbuilder.Persona {
	return contextbuilder.Persona{
		ID:           p.ID,
		Name:         p.Name,
		VersionHash:  p.VersionHash,
		Instructions: p.Instructions,
	}
}

func parseCard(path string) (Card, error) {
	body, frontmatter, err := readMarkdown(path)
	if err != nil {
		return Card{}, err
	}
	id := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	name := valueOr(frontmatter["name"], id)
	hash, err := fileHash(path)
	if err != nil {
		return Card{}, err
	}
	return Card{
		ID:          id,
		Name:        name,
		Type:        valueOr(frontmatter["type"], "role"),
		Description: valueOr(frontmatter["description"], firstLine(body)),
		Path:        path,
		VersionHash: hash,
	}, nil
}

func readMarkdown(path string) (string, map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	frontmatter, body := parseFrontmatter(string(data))
	return body, frontmatter, nil
}

func parseFrontmatter(text string) (map[string]string, string) {
	frontmatter := map[string]string{}
	if !strings.HasPrefix(text, "---\n") {
		return frontmatter, text
	}
	rest := strings.TrimPrefix(text, "---\n")
	end := strings.Index(rest, "\n---")
	if end == -1 {
		return frontmatter, text
	}
	for _, line := range strings.Split(rest[:end], "\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok {
			frontmatter[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return frontmatter, strings.TrimPrefix(rest[end:], "\n---")
}

func fileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func firstLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if line != "" {
			return line
		}
	}
	return ""
}

func valueOr(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func tokenMatch(haystack string, query string) bool {
	for _, token := range strings.Fields(query) {
		if strings.Contains(haystack, token) {
			return true
		}
	}
	return false
}

// --- Mutation methods ---

func sanitizeFileName(name string) string {
	var out strings.Builder
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' || ch == '.' {
			out.WriteRune(ch)
		}
	}
	result := out.String()
	if result == "" || result == ".md" {
		result = "persona.md"
	}
	return result
}

// WritePersonaFile writes a persona .md file with YAML frontmatter + instructions.
func WritePersonaFile(path string, name string, personaType string, description string, instructions string) (Card, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Card{}, err
	}
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("name: " + name + "\n")
	sb.WriteString("type: " + personaType + "\n")
	sb.WriteString("description: " + description + "\n")
	sb.WriteString("---\n")
	if instructions != "" {
		sb.WriteString(instructions)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return Card{}, err
	}
	return parseCard(path)
}

func (r *Registry) Create(ctx context.Context, root string, name string, personaType string, description string, instructions string) (Card, error) {
	if err := ctx.Err(); err != nil {
		return Card{}, err
	}
	fileName := sanitizeFileName(strings.ToLower(strings.ReplaceAll(name, " ", "-")) + ".md")
	path := filepath.Join(root, fileName)
	if _, err := os.Stat(path); err == nil {
		return Card{}, errors.New("persona already exists: " + fileName)
	}
	card, err := WritePersonaFile(path, name, personaType, description, instructions)
	if err != nil {
		return Card{}, err
	}
	r.cards[card.ID] = card
	return card, nil
}

func (r *Registry) Update(ctx context.Context, id string, name string, personaType string, description string, instructions string) (Card, error) {
	if err := ctx.Err(); err != nil {
		return Card{}, err
	}
	existing, ok := r.cards[id]
	if !ok {
		return Card{}, ErrPersonaNotFound
	}
	card, err := WritePersonaFile(existing.Path, name, personaType, description, instructions)
	if err != nil {
		return Card{}, err
	}
	r.cards[id] = card
	return card, nil
}

func (r *Registry) Delete(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	card, ok := r.cards[id]
	if !ok {
		return ErrPersonaNotFound
	}
	if err := os.Remove(card.Path); err != nil {
		return err
	}
	delete(r.cards, id)
	return nil
}
