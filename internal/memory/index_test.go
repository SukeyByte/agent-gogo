package memory

import (
	"context"
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
