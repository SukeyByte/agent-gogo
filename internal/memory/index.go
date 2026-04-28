package memory

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/sukeke/agent-gogo/internal/contextbuilder"
	"github.com/sukeke/agent-gogo/internal/textutil"
)

var ErrMemoryNotFound = errors.New("memory not found")

type Card struct {
	ID          string
	Scope       string
	Type        string
	Tags        []string
	Summary     string
	ArtifactRef string
	VersionHash string
}

type Item struct {
	Card
	Body string
}

type Index struct {
	items map[string]Item
}

func NewIndex(items ...Item) *Index {
	index := &Index{items: map[string]Item{}}
	for _, item := range items {
		index.Add(item)
	}
	return index
}

func (i *Index) Add(item Item) {
	item.Tags = textutil.SortedUniqueStrings(item.Tags)
	i.items[item.ID] = item
}

func (i *Index) Search(ctx context.Context, query string, scope string, limit int) ([]Card, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	query = strings.ToLower(query)
	cards := make([]Card, 0, len(i.items))
	for _, item := range i.items {
		if scope != "" && item.Scope != scope {
			continue
		}
		haystack := strings.ToLower(item.Summary + " " + item.Body + " " + strings.Join(item.Tags, " "))
		if query == "" || strings.Contains(haystack, query) || tokenMatch(haystack, query) {
			cards = append(cards, item.Card)
		}
	}
	sort.SliceStable(cards, func(a, b int) bool {
		if cards[a].ID != cards[b].ID {
			return cards[a].ID < cards[b].ID
		}
		return cards[a].VersionHash < cards[b].VersionHash
	})
	if limit > 0 && len(cards) > limit {
		cards = cards[:limit]
	}
	return cards, nil
}

func (i *Index) Load(ctx context.Context, id string) (Item, error) {
	if err := ctx.Err(); err != nil {
		return Item{}, err
	}
	item, ok := i.items[id]
	if !ok {
		return Item{}, ErrMemoryNotFound
	}
	return item, nil
}

func (item Item) ContextMemory() contextbuilder.MemoryItem {
	return contextbuilder.MemoryItem{
		ID:          item.ID,
		Scope:       item.Scope,
		VersionHash: item.VersionHash,
		Summary:     item.Summary,
		ArtifactRef: item.ArtifactRef,
	}
}

func tokenMatch(haystack string, query string) bool {
	for _, token := range strings.Fields(query) {
		if strings.Contains(haystack, token) {
			return true
		}
	}
	return false
}
