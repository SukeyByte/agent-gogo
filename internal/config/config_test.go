package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSkillRootsListReplacesDefault(t *testing.T) {
	t.Setenv("AGENT_GOGO_CONFIG", "")
	root := t.TempDir()
	path := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(path, []byte(`storage:
  skill_roots:
    - "./one"
    - "./two"
`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.Storage.SkillRoots) != 2 {
		t.Fatalf("expected two explicit skill roots, got %#v", cfg.Storage.SkillRoots)
	}
	if cfg.Storage.SkillRoots[0] != "./one" || cfg.Storage.SkillRoots[1] != "./two" {
		t.Fatalf("unexpected skill roots: %#v", cfg.Storage.SkillRoots)
	}
}
