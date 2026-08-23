package skill

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiscoverSearchAndLoadSkill(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "go-runtime")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(`---
name: Go Runtime
description: Build Go runtime modules
allowed-tools: Read,Grep
---
# Go Runtime

Use go test and keep module boundaries clean.
`), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	registry, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	cards, err := registry.Search(context.Background(), "go", 10)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("expected one card, got %d", len(cards))
	}
	if strings.Contains(cards[0].Description, "Use go test") {
		t.Fatal("card should not contain full skill body")
	}
	pkg, err := registry.Load(context.Background(), cards[0].ID)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !strings.Contains(pkg.Instructions, "Use go test") {
		t.Fatalf("expected full instructions, got %q", pkg.Instructions)
	}
	if pkg.ContextInstruction().VersionHash == "" {
		t.Fatal("expected context instruction version hash")
	}
}

func TestDiscoverSkillsFromTempRoot(t *testing.T) {
	root := t.TempDir()
	for _, id := range []string{"code-review", "release-notes"} {
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir skill %s: %v", id, err)
		}
		body := "# " + id + "\n\nUse this skill for " + id + " work.\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
			t.Fatalf("write skill %s: %v", id, err)
		}
	}
	registry, err := Discover(context.Background(), root)
	if err != nil {
		t.Fatalf("discover skills: %v", err)
	}
	cards, err := registry.Search(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("search skills: %v", err)
	}
	seen := map[string]bool{}
	for _, card := range cards {
		seen[card.ID] = true
		if strings.Contains(card.Description, "# code-review") {
			t.Fatal("card should not contain full skill body")
		}
	}
	for _, id := range []string{"code-review", "release-notes"} {
		if !seen[id] {
			t.Fatalf("expected skill %q in local index, got %#v", id, seen)
		}
		pkg, err := registry.Load(context.Background(), id)
		if err != nil {
			t.Fatalf("load skill %s: %v", id, err)
		}
		if strings.TrimSpace(pkg.Instructions) == "" {
			t.Fatalf("expected skill %s instructions", id)
		}
	}
}
