package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sukeke/agent-gogo/internal/codeindex"
)

func searchCode(ctx context.Context, root string, args map[string]any) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	query, _ := args["query"].(string)
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query is required")
	}
	limit := intArg(args["limit"], 20)
	paths := stringSliceArg(args["paths"])
	if len(paths) == 0 {
		paths = []string{"."}
	}
	matches := []map[string]any{}
	lowerQuery := strings.ToLower(query)
	for _, requested := range paths {
		base, err := safeJoin(root, requested)
		if err != nil {
			return nil, err
		}
		err = filepath.WalkDir(base, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return nil
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.IsDir() {
				if shouldSkipDir(entry.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			if len(matches) >= limit || shouldSkipFile(path) {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil || !strings.Contains(strings.ToLower(string(data)), lowerQuery) {
				return nil
			}
			line, snippet := firstMatchingLine(string(data), lowerQuery)
			rel := artifactRef(root, path)
			matches = append(matches, map[string]any{
				"path":    rel,
				"line":    line,
				"snippet": snippet,
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"query": query, "matches": matches}, nil
}

func readFile(ctx context.Context, root string, args map[string]any) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relPath, _ := args["path"].(string)
	if strings.TrimSpace(relPath) == "" {
		return nil, errors.New("path is required")
	}
	target, err := safeWorkspacePath(root, relPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	if info.Size() > 1_000_000 {
		return nil, errors.New("file is too large to read")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"path":    artifactRef(root, target),
		"bytes":   len(data),
		"content": string(data),
	}, nil
}

func writeWorkspaceFile(ctx context.Context, root string, args map[string]any) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relPath, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if strings.TrimSpace(relPath) == "" {
		return nil, errors.New("path is required")
	}
	target, err := safeWorkspacePath(root, relPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{
		"path":  artifactRef(root, target),
		"bytes": len([]byte(content)),
	}, nil
}

func patchWorkspaceFile(ctx context.Context, root string, args map[string]any) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relPath, _ := args["path"].(string)
	oldText, _ := args["old"].(string)
	newText, _ := args["new"].(string)
	if strings.TrimSpace(relPath) == "" {
		return nil, errors.New("path is required")
	}
	if oldText == "" {
		return nil, errors.New("old text is required")
	}
	target, err := safeWorkspacePath(root, relPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, err
	}
	content := string(data)
	if !strings.Contains(content, oldText) {
		return nil, errors.New("old text not found")
	}
	updated := strings.Replace(content, oldText, newText, 1)
	if err := os.WriteFile(target, []byte(updated), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{
		"path":          artifactRef(root, target),
		"bytes":         len([]byte(updated)),
		"replacements":  1,
		"delta_bytes":   len([]byte(updated)) - len(data),
		"changed_bytes": len([]byte(newText)),
	}, nil
}

func fileDiff(ctx context.Context, root string, args map[string]any) (map[string]any, error) {
	path, _ := args["path"].(string)
	if strings.TrimSpace(path) != "" {
		if _, err := safeWorkspacePath(root, path); err != nil {
			return nil, err
		}
		output, err := gitDiffPath(ctx, root, path)
		return map[string]any{"path": path, "diff": output}, err
	}
	output, err := gitCommand(ctx, root, "diff")
	return map[string]any{"diff": output}, err
}

func runCommand(ctx context.Context, root string, command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", errors.New("command is required")
	}
	if token := unsupportedShellToken(command); token != "" {
		return "", fmt.Errorf("shell.run supports one exec-style command; unsupported shell token %q", token)
	}
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", errors.New("command is required")
	}
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = root
	data, err := cmd.CombinedOutput()
	return truncate(string(data), 4000), err
}

func unsupportedShellToken(command string) string {
	for _, token := range []string{"&&", "||", "$(", "|", ";", ">", "<", "`", "*", "?"} {
		if strings.Contains(command, token) {
			return token
		}
	}
	return ""
}

func gitBranch(ctx context.Context, root string, args map[string]any) (map[string]any, error) {
	name, _ := args["name"].(string)
	checkout, _ := args["checkout"].(bool)
	create, _ := args["create"].(bool)
	if strings.TrimSpace(name) == "" {
		output, err := gitCommand(ctx, root, "branch", "--show-current")
		return map[string]any{"branch": strings.TrimSpace(output)}, err
	}
	if create {
		output, err := gitCommand(ctx, root, "switch", "-c", name)
		return map[string]any{"branch": name, "created": true, "summary": output}, err
	}
	if checkout {
		output, err := gitCommand(ctx, root, "switch", name)
		return map[string]any{"branch": name, "checked_out": true, "summary": output}, err
	}
	return nil, errors.New("set create or checkout for named branch")
}

func gitCommand(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	data, err := cmd.CombinedOutput()
	return truncate(string(data), 8000), err
}

func gitDiffPath(ctx context.Context, root string, path string) (string, error) {
	output, err := gitCommand(ctx, root, "diff", "--", path)
	if err != nil || strings.TrimSpace(output) != "" {
		return output, err
	}
	if _, err := gitCommand(ctx, root, "ls-files", "--error-unmatch", "--", path); err == nil {
		return output, nil
	}
	target, err := safeWorkspacePath(root, path)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(target); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, "git", "diff", "--no-index", "--", "/dev/null", path)
	cmd.Dir = root
	data, diffErr := cmd.CombinedOutput()
	if len(data) > 0 {
		return truncate(string(data), 8000), nil
	}
	return "", diffErr
}

func writeArtifact(ctx context.Context, root string, args map[string]any) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relPath, _ := args["path"].(string)
	content, _ := args["content"].(string)
	summary, _ := args["summary"].(string)
	if strings.TrimSpace(relPath) == "" {
		return nil, errors.New("path is required")
	}
	target, err := safeJoin(root, relPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{
		"artifact_ref": artifactRef(root, target),
		"summary":      summary,
		"bytes":        len([]byte(content)),
	}, nil
}

func saveMemory(ctx context.Context, root string, args map[string]any) (map[string]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key, _ := args["key"].(string)
	scope, _ := args["scope"].(string)
	summary, _ := args["summary"].(string)
	body, _ := args["body"].(string)
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("memory key is required")
	}
	if strings.TrimSpace(summary) == "" {
		return nil, errors.New("memory summary is required")
	}
	if strings.TrimSpace(body) == "" {
		return nil, errors.New("memory body is required")
	}
	if strings.TrimSpace(scope) == "" {
		scope = "project"
	}
	tags := stringSliceArg(args["tags"])
	fileName := safeFileName(key) + ".md"
	content := "# " + summary + "\n\n" +
		"scope: " + scope + "\n" +
		"tags: " + strings.Join(tags, ",") + "\n\n" +
		body + "\n"
	target, err := safeJoin(root, filepath.Join("memory", fileName))
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return nil, err
	}
	return map[string]any{
		"memory_ref": artifactRef(root, target),
		"scope":      scope,
		"summary":    summary,
		"bytes":      len([]byte(content)),
	}, nil
}

func safeJoin(root string, requested string) (string, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	target := filepath.Clean(filepath.Join(absRoot, requested))
	if target != absRoot && !strings.HasPrefix(target, absRoot+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes workspace root: %s", requested)
	}
	return target, nil
}

func safeWorkspacePath(root string, requested string) (string, error) {
	if hasPathSegment(requested, ".git") {
		return "", fmt.Errorf("path touches blocked workspace segment: %s", requested)
	}
	return safeJoin(root, requested)
}

func hasPathSegment(path string, segment string) bool {
	for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(path)), "/") {
		if part == segment {
			return true
		}
	}
	return false
}

func artifactRef(root string, target string) string {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(target))
	}
	rel, err := filepath.Rel(absRoot, target)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(target))
	}
	return filepath.ToSlash(rel)
}

func limitFileSummaries(files []codeindex.FileSummary, limit int) []codeindex.FileSummary {
	if limit <= 0 || len(files) <= limit {
		return append([]codeindex.FileSummary(nil), files...)
	}
	return append([]codeindex.FileSummary(nil), files[:limit]...)
}

func limitSymbols(symbols []codeindex.Symbol, limit int) []codeindex.Symbol {
	if limit <= 0 || len(symbols) <= limit {
		return append([]codeindex.Symbol(nil), symbols...)
	}
	return append([]codeindex.Symbol(nil), symbols[:limit]...)
}

func safeFileName(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r == '-' || r == '_':
			builder.WriteRune(r)
		default:
			builder.WriteRune('-')
		}
	}
	name := strings.Trim(builder.String(), "-")
	if name == "" {
		return "memory"
	}
	return name
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules", ".cache", "data":
		return true
	default:
		return false
	}
}

func shouldSkipFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.Size() > 1_000_000 {
		return true
	}
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".sqlite", ".db", ".pdf":
		return true
	default:
		return false
	}
}

func firstMatchingLine(content string, lowerQuery string) (int, string) {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), lowerQuery) {
			return i + 1, truncate(strings.TrimSpace(line), 240)
		}
	}
	return 0, ""
}

func stringSliceArg(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func intArg(value any, fallback int) int {
	switch typed := value.(type) {
	case int:
		if typed > 0 {
			return typed
		}
	case float64:
		if typed > 0 {
			return int(typed)
		}
	case string:
		var parsed int
		if _, err := fmt.Sscanf(typed, "%d", &parsed); err == nil && parsed > 0 {
			return parsed
		}
	}
	return fallback
}

func truncate(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max] + "...[truncated]"
}
