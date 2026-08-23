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

func TestDefaultBrowserIsVisibleAndHeadlessEnvOverrides(t *testing.T) {
	cfg := Default()
	if cfg.Browser.Headless {
		t.Fatal("expected default browser to be visible for local validation")
	}

	t.Setenv("AGENT_GOGO_CONFIG", "")
	t.Setenv("AGENT_GOGO_BROWSER_HEADLESS", "true")
	loaded, err := Load("")
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !loaded.Browser.Headless {
		t.Fatal("expected AGENT_GOGO_BROWSER_HEADLESS=true to enable headless mode")
	}
}

func TestLoadMCPServersAndParallelism(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	body := `llm:
  provider: openai_compatible
runtime:
  max_parallel_tasks: 4
mcp_servers:
  - name: fetch
    command: /usr/local/bin/mcp-fetch
    args:
      - --port
      - "9090"
    enabled: true
  - name: search
    url: https://mcp.example.com/mcp
    enabled: false
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Runtime.MaxParallelTasks != 4 {
		t.Fatalf("max_parallel_tasks = %d, want 4", cfg.Runtime.MaxParallelTasks)
	}
	if len(cfg.MCPServers) != 2 {
		t.Fatalf("expected 2 mcp servers, got %d", len(cfg.MCPServers))
	}
	fetch := cfg.MCPServers[0]
	if fetch.Name != "fetch" || fetch.Command != "/usr/local/bin/mcp-fetch" || !fetch.Enabled {
		t.Fatalf("unexpected fetch server: %#v", fetch)
	}
	if len(fetch.Args) != 2 || fetch.Args[0] != "--port" || fetch.Args[1] != "9090" {
		t.Fatalf("unexpected fetch args: %v", fetch.Args)
	}
	search := cfg.MCPServers[1]
	if search.Name != "search" || search.URL != "https://mcp.example.com/mcp" || search.Enabled {
		t.Fatalf("unexpected search server: %#v", search)
	}
}
