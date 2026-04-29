package memory

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sukeke/agent-gogo/internal/contextbuilder"
	"github.com/sukeke/agent-gogo/internal/textutil"
)

var ErrMemoryNotFound = errors.New("memory not found")

type Card struct {
	ID              string
	Scope           string
	Type            string
	Tags            []string
	Summary         string
	ArtifactRef     string
	EvidenceRef     string
	SourceTaskID    string
	SourceAttemptID string
	Confidence      float64
	VersionHash     string
}

type Item struct {
	Card
	Body string
}

type Index struct {
	items       map[string]Item
	persistPath string
}

func NewIndex(items ...Item) *Index {
	index := &Index{items: map[string]Item{}}
	for _, item := range items {
		index.Add(item)
	}
	return index
}

func NewPersistentIndex(ctx context.Context, path string) (*Index, error) {
	index, err := LoadJSONL(ctx, path)
	if err != nil {
		return nil, err
	}
	index.persistPath = path
	return index, nil
}

func (i *Index) Add(item Item) {
	if i.items == nil {
		i.items = map[string]Item{}
	}
	item.Tags = textutil.SortedUniqueStrings(item.Tags)
	i.items[item.ID] = item
}

func (i *Index) Items() []Item {
	items := make([]Item, 0, len(i.items))
	for _, item := range i.items {
		items = append(items, item)
	}
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].ID != items[b].ID {
			return items[a].ID < items[b].ID
		}
		return items[a].VersionHash < items[b].VersionHash
	})
	return items
}

func (i *Index) Persist(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(i.persistPath) == "" {
		return nil
	}
	return i.SaveJSONL(ctx, i.persistPath)
}

func (i *Index) SaveJSONL(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	for _, item := range i.Items() {
		if err := encoder.Encode(item); err != nil {
			_ = file.Close()
			return err
		}
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func LoadJSONL(ctx context.Context, path string) (*Index, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	index := NewIndex()
	path = strings.TrimSpace(path)
	if path == "" {
		return index, nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return index, nil
	}
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	for {
		var item Item
		if err := decoder.Decode(&item); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, err
		}
		index.Add(item)
	}
	return index, nil
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
		haystack := strings.ToLower(item.Summary + " " + item.Body + " " + item.Type + " " + strings.Join(item.Tags, " "))
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
		ID:              item.ID,
		Scope:           item.Scope,
		Type:            item.Type,
		Tags:            append([]string(nil), item.Tags...),
		VersionHash:     item.VersionHash,
		Summary:         item.Summary,
		ArtifactRef:     item.ArtifactRef,
		EvidenceRef:     item.EvidenceRef,
		SourceTaskID:    item.SourceTaskID,
		SourceAttemptID: item.SourceAttemptID,
		Confidence:      item.Confidence,
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
