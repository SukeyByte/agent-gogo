package persona

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverSearchAndLoadPersona(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "reviewer.md")
	if err := os.WriteFile(path, []byte(`---
name: Reviewer
type: role
description: Reviews task output
---
# Reviewer

Check acceptance criteria and cite evidence.
`), 0o644); err != nil {
		t.Fatalf("write persona: %v", err)
	}
	registry, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	cards, err := registry.Search(context.Background(), "review", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("expected one card, got %d", len(cards))
	}
	persona, err := registry.Load(context.Background(), cards[0].ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !strings.Contains(persona.Instructions, "Check acceptance criteria") {
		t.Fatalf("expected full instructions, got %q", persona.Instructions)
	}
	if persona.ContextPersona().VersionHash == "" {
		t.Fatal("expected context persona version hash")
	}
}
