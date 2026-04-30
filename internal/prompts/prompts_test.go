package prompts

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTextLoadsExternalPromptOverride(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "planner.md"), []byte("external planner prompt"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	t.Setenv("AGENT_GOGO_PROMPT_DIR", dir)
	if got := Text("planner"); got != "external planner prompt" {
		t.Fatalf("expected external prompt, got %q", got)
	}
}

func TestTextLoadsEmbeddedDefaultPrompt(t *testing.T) {
	t.Setenv("AGENT_GOGO_PROMPT_DIR", filepath.Join(t.TempDir(), "missing"))
	if got := Text("generic_executor"); got == "" {
		t.Fatal("expected embedded generic executor prompt")
	}
}
