package prompts

import (
	"embed"
	"os"
	"path/filepath"
	"strings"
)

//go:embed defaults/*.md
var defaults embed.FS

func Text(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if dir := strings.TrimSpace(os.Getenv("AGENT_GOGO_PROMPT_DIR")); dir != "" {
		if data, err := os.ReadFile(filepath.Join(dir, name+".md")); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	if data, err := defaults.ReadFile(filepath.Join("defaults", name+".md")); err == nil {
		return strings.TrimSpace(string(data))
	}
	return ""
}
