package codeindex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildIndexesGoSymbols(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "internal", "sample.go"), []byte("package internal\n\ntype Widget struct{}\n\nfunc BuildWidget() {}\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	index, err := Build(context.Background(), root, Options{})
	if err != nil {
		t.Fatalf("build index: %v", err)
	}
	symbols := index.SearchSymbols("Widget", "", 10)
	if len(symbols) != 2 {
		t.Fatalf("expected two widget symbols, got %#v", symbols)
	}
}
