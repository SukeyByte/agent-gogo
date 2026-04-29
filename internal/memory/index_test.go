package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestSearchAndLoadMemory(t *testing.T) {
	index := NewIndex(Item{
		Card: Card{
			ID:          "mem-001",
			Scope:       "project",
			Type:        "decision",
			Tags:        []string{"runtime", "sqlite"},
			Summary:     "SQLite is the default store",
			ArtifactRef: "artifact://decision",
			VersionHash: "mem-v1",
		},
		Body: "Use SQLite for local-first persistence.",
	})
	cards, err := index.Search(context.Background(), "sqlite", "project", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("expected one card, got %d", len(cards))
	}
	if strings.Contains(cards[0].Summary, "local-first persistence") {
		t.Fatal("card should not include full memory body")
	}
	item, err := index.Load(context.Background(), cards[0].ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !strings.Contains(item.Body, "local-first") {
		t.Fatalf("expected full body, got %q", item.Body)
	}
	if item.ContextMemory().ArtifactRef == "" {
		t.Fatal("expected context memory artifact ref")
	}
}

func TestPersistentIndexRoundTrip(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "memories.jsonl")
	index, err := NewPersistentIndex(ctx, path)
	if err != nil {
		t.Fatalf("new persistent index: %v", err)
	}
	index.Add(Item{
		Card: Card{
			ID:          "mem-002",
			Scope:       "project",
			Type:        "failure",
			Tags:        []string{"test"},
			Summary:     "Remember failed command",
			VersionHash: "mem-v1",
		},
		Body: "The first command failed because shell was disabled.",
	})
	if err := index.Persist(ctx); err != nil {
		t.Fatalf("persist: %v", err)
	}
	loaded, err := NewPersistentIndex(ctx, path)
	if err != nil {
		t.Fatalf("load persistent index: %v", err)
	}
	cards, err := loaded.Search(ctx, "shell disabled", "project", 10)
	if err != nil {
		t.Fatalf("search loaded: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("expected one loaded memory, got %d", len(cards))
	}
}
