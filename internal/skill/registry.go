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

	"github.com/sukeke/agent-gogo/internal/contextbuilder"
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
		if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || entry.Name() != "SKILL.md" {
				return nil
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
	body, frontmatter, err := readSkill(card.Path)
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

func parseCard(path string) (Card, error) {
	body, frontmatter, err := readSkill(path)
	if err != nil {
		return Card{}, err
	}
	name := frontmatter["name"]
	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}
	description := frontmatter["description"]
	if description == "" {
		description = firstLine(body)
	}
	allowedTools := parseList(frontmatter["allowed-tools"])
	hash, err := fileHash(path)
	if err != nil {
		return Card{}, err
	}
	return Card{
		ID:           filepath.Base(filepath.Dir(path)),
		Name:         name,
		Description:  description,
		AllowedTools: allowedTools,
		Path:         path,
		VersionHash:  hash,
	}, nil
}

func readSkill(path string) (string, map[string]string, error) {
	data, err := os.ReadFile(path)
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

func tokenMatch(haystack string, query string) bool {
	for _, token := range strings.Fields(query) {
		if strings.Contains(haystack, token) {
			return true
		}
	}
	return false
}
