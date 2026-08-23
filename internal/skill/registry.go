package skill

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

var ErrSkillNotFound = errors.New("skill not found")

type Card struct {
	ID           string
	Name         string
	Description  string
	AllowedTools []string
	Path         string
	VersionHash  string
	Reason       string
}

type Package struct {
	Card
	Instructions string
	Frontmatter  map[string]string
}

type Registry struct {
	cards map[string]Card
}

func Discover(ctx context.Context, roots ...string) (*Registry, error) {
	registry := &Registry{cards: map[string]Card{}}
	for _, root := range roots {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if root == "" {
			continue
		}
		if _, err := os.Stat(root); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || entry.Name() != "SKILL.md" {
				return nil
			}
			card, err := parseCard(ctx, path)
			if err != nil {
				return err
			}
			registry.cards[card.ID] = card
			return nil
		}); err != nil {
			return nil, err
		}
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
		haystack := strings.ToLower(card.Name + " " + card.Description + " " + strings.Join(card.AllowedTools, " "))
		if query == "" || strings.Contains(haystack, query) || tokenMatch(haystack, query) {
			card.Reason = "skill metadata matched query"
			cards = append(cards, card)
		}
	}
	sort.SliceStable(cards, func(i, j int) bool {
		if cards[i].Name != cards[j].Name {
			return cards[i].Name < cards[j].Name
		}
		return cards[i].VersionHash < cards[j].VersionHash
	})
	if limit > 0 && len(cards) > limit {
		cards = cards[:limit]
	}
	return cards, nil
}

func (r *Registry) Load(ctx context.Context, id string) (Package, error) {
	if err := ctx.Err(); err != nil {
		return Package{}, err
	}
	card, ok := r.cards[id]
	if !ok {
		return Package{}, ErrSkillNotFound
	}
	body, frontmatter, err := readSkill(ctx, card.Path)
	if err != nil {
		return Package{}, err
	}
	return Package{Card: card, Instructions: body, Frontmatter: frontmatter}, nil
}

func (pkg Package) ContextInstruction() contextbuilder.SkillInstruction {
	return contextbuilder.SkillInstruction{
		ID:           pkg.ID,
		Name:         pkg.Name,
		VersionHash:  pkg.VersionHash,
		Instructions: pkg.Instructions,
		AllowedTools: append([]string(nil), pkg.AllowedTools...),
	}
}

func parseCard(ctx context.Context, path string) (Card, error) {
	body, frontmatter, err := readSkill(ctx, path)
	if err != nil {
		return Card{}, err
	}
	name := frontmatter["name"]
	if name == "" {
		name = skillID(path)
	}
	description := frontmatter["description"]
	if description == "" {
		description = firstLine(body)
	}
	allowedTools := parseList(frontmatter["allowed-tools"])
	hash, err := fileHash(ctx, path)
	if err != nil {
		return Card{}, err
	}
	return Card{
		ID:           skillID(path),
		Name:         name,
		Description:  description,
		AllowedTools: allowedTools,
		Path:         path,
		VersionHash:  hash,
	}, nil
}

func readSkill(ctx context.Context, path string) (string, map[string]string, error) {
	data, err := readSkillBytes(ctx, path)
	if err != nil {
		return "", nil, err
	}
	frontmatter, body := parseFrontmatter(string(data))
	return strings.TrimSpace(body), frontmatter, nil
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
		if !ok {
			continue
		}
		frontmatter[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
	}
	body := strings.TrimPrefix(rest[end:], "\n---")
	return frontmatter, body
}

func parseList(value string) []string {
	if value == "" {
		return []string{}
	}
	value = strings.Trim(value, "[]")
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ' '
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), `"'`)
		if part != "" {
			out = append(out, part)
		}
	}
	sort.Strings(out)
	return out
}

func fileHash(ctx context.Context, path string) (string, error) {
	data, err := readSkillBytes(ctx, path)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:]), nil
}

func readSkillBytes(ctx context.Context, path string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func skillID(path string) string {
	path = strings.TrimRight(path, "/")
	return filepath.Base(filepath.Dir(path))
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

func tokenMatch(haystack string, query string) bool {
	for _, token := range strings.Fields(query) {
		if strings.Contains(haystack, token) {
			return true
		}
	}
	return false
}

// AddCard adds a pre-parsed Card to the registry (used after install).
func (r *Registry) AddCard(card Card) {
	r.cards[card.ID] = card
}

// ParseSkillCard parses a SKILL.md file and returns its Card.
func ParseSkillCard(ctx context.Context, path string) (Card, error) {
	return parseCard(ctx, path)
}

// --- Mutation methods ---

func formatYAMLList(items []string) string {
	if len(items) == 0 {
		return ""
	}
	sorted := append([]string(nil), items...)
	sort.Strings(sorted)
	return "[" + strings.Join(sorted, ",") + "]"
}

func sanitizeDirName(name string) string {
	var out strings.Builder
	for _, ch := range name {
		if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '_' {
			out.WriteRune(ch)
		}
	}
	result := out.String()
	if result == "" {
		result = "skill"
	}
	return result
}

// WriteSkillFile writes a SKILL.md file with YAML frontmatter + body.
func WriteSkillFile(ctx context.Context, dir string, name string, description string, allowedTools []string, body string) (Card, error) {
	if err := ctx.Err(); err != nil {
		return Card{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Card{}, err
	}
	path := filepath.Join(dir, "SKILL.md")
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString("name: " + name + "\n")
	sb.WriteString("description: " + description + "\n")
	sb.WriteString("allowed-tools: " + formatYAMLList(allowedTools) + "\n")
	sb.WriteString("---\n")
	if body != "" {
		sb.WriteString(body)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		return Card{}, err
	}
	return parseCard(ctx, path)
}

func (r *Registry) Create(ctx context.Context, root string, name string, description string, allowedTools []string, body string) (Card, error) {
	dirName := sanitizeDirName(strings.ToLower(strings.ReplaceAll(name, " ", "-")))
	dir := filepath.Join(root, dirName)
	card, err := WriteSkillFile(ctx, dir, name, description, allowedTools, body)
	if err != nil {
		return Card{}, err
	}
	r.cards[card.ID] = card
	return card, nil
}

func (r *Registry) Delete(ctx context.Context, id string) error {
	card, ok := r.cards[id]
	if !ok {
		return ErrSkillNotFound
	}
	dir := filepath.Dir(card.Path)
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	delete(r.cards, id)
	return nil
}
