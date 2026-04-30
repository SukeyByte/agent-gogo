package function

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/textutil"
)

var ErrNotFound = errors.New("function schema not found")

type CatalogRegistry struct {
	schemas map[string]Schema
}

func NewCatalogRegistry(schemas ...Schema) *CatalogRegistry {
	if len(schemas) == 0 {
		schemas = defaultSchemas()
	}
	registry := &CatalogRegistry{schemas: make(map[string]Schema, len(schemas))}
	for _, schema := range schemas {
		registry.schemas[schema.SchemaRef] = normalizeSchema(schema)
	}
	return registry
}

func (r *CatalogRegistry) Search(ctx context.Context, req SearchRequest) ([]Card, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	candidates := make([]scoredCard, 0, len(r.schemas))
	for _, schema := range r.schemas {
		score := scoreSchema(schema, req)
		if score == 0 && hasSearchFilters(req) {
			continue
		}
		candidates = append(candidates, scoredCard{
			card:  schema.Card(searchReason(schema, req, score)),
			score: score,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		if candidates[i].card.Name != candidates[j].card.Name {
			return candidates[i].card.Name < candidates[j].card.Name
		}
		return candidates[i].card.SchemaRef < candidates[j].card.SchemaRef
	})

	limit := req.Limit
	if limit <= 0 || limit > len(candidates) {
		limit = len(candidates)
	}
	result := make([]Card, 0, limit)
	for i := 0; i < limit; i++ {
		result = append(result, candidates[i].card)
	}
	return result, nil
}

func (r *CatalogRegistry) LoadSchema(ctx context.Context, schemaRef string) (Schema, error) {
	if err := ctx.Err(); err != nil {
		return Schema{}, err
	}
	schema, ok := r.schemas[schemaRef]
	if !ok {
		return Schema{}, ErrNotFound
	}
	return cloneSchema(schema), nil
}

func (r *CatalogRegistry) Activate(ctx context.Context, cards []Card) (ActiveSet, error) {
	if err := ctx.Err(); err != nil {
		return ActiveSet{}, err
	}
	seen := map[string]struct{}{}
	schemas := make([]Schema, 0, len(cards))
	for _, card := range cards {
		if _, ok := seen[card.SchemaRef]; ok {
			continue
		}
		seen[card.SchemaRef] = struct{}{}
		schema, err := r.LoadSchema(ctx, card.SchemaRef)
		if err != nil {
			return ActiveSet{}, err
		}
		schemas = append(schemas, schema)
	}
	sort.SliceStable(schemas, func(i, j int) bool {
		if schemas[i].Name != schemas[j].Name {
			return schemas[i].Name < schemas[j].Name
		}
		return schemas[i].VersionHash < schemas[j].VersionHash
	})
	return ActiveSet{Schemas: schemas}, nil
}

type scoredCard struct {
	card  Card
	score int
}

func scoreSchema(schema Schema, req SearchRequest) int {
	score := 0
	haystack := strings.ToLower(strings.Join([]string{
		schema.Name,
		schema.Description,
		strings.Join(schema.Tags, " "),
		strings.Join(schema.TaskTypes, " "),
		schema.InputSummary,
		schema.OutputSummary,
	}, " "))
	for _, token := range tokenize(req.Query) {
		if strings.Contains(haystack, token) {
			score += 2
		}
	}
	if req.TaskType != "" && containsString(schema.TaskTypes, req.TaskType) {
		score += 3
	}
	for _, domain := range req.Domains {
		if containsString(schema.Tags, domain) {
			score++
		}
	}
	for _, capability := range req.RequiredCapabilities {
		if schema.Name == capability || containsString(schema.Tags, capability) {
			score += 4
		}
	}
	if !hasSearchFilters(req) {
		score = 1
	}
	return score
}

func hasSearchFilters(req SearchRequest) bool {
	return strings.TrimSpace(req.Query) != "" || req.TaskType != "" || len(req.Domains) > 0 || len(req.RequiredCapabilities) > 0
}

func searchReason(schema Schema, req SearchRequest, score int) string {
	if score == 0 {
		return "default candidate"
	}
	if containsString(req.RequiredCapabilities, schema.Name) {
		return "required capability matched"
	}
	if req.TaskType != "" && containsString(schema.TaskTypes, req.TaskType) {
		return "task type matched"
	}
	return "query matched function metadata"
}

func tokenize(value string) []string {
	fields := strings.Fields(strings.ToLower(value))
	result := fields[:0]
	for _, field := range fields {
		field = strings.Trim(field, ".,:;!?()[]{}")
		if field == "" {
			continue
		}
		result = append(result, field)
	}
	return result
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func normalizeSchema(schema Schema) Schema {
	schema.Tags = textutil.SortedUniqueStrings(schema.Tags)
	schema.TaskTypes = textutil.SortedUniqueStrings(schema.TaskTypes)
	if schema.Provider == "" {
		schema.Provider = "builtin"
	}
	return schema
}

func cloneSchema(schema Schema) Schema {
	schema.Tags = append([]string(nil), schema.Tags...)
	schema.TaskTypes = append([]string(nil), schema.TaskTypes...)
	schema.InputSchema = copyMap(schema.InputSchema)
	schema.OutputSchema = copyMap(schema.OutputSchema)
	return schema
}

func defaultSchemas() []Schema {
	schemas := []Schema{
		{
			Name:          "code.search",
			Description:   "Search indexed source files for relevant symbols or text.",
			Tags:          []string{"code", "search"},
			TaskTypes:     []string{"code", "runtime"},
			RiskLevel:     "low",
			InputSummary:  "query string and optional path filters",
			OutputSummary: "matched file references and snippets",
			Provider:      "builtin",
			SchemaRef:     "fn:code.search@v1",
			VersionHash:   "builtin-code-search-v1",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"query"},
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
					"paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
			},
			OutputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"matches": map[string]any{"type": "array"},
				},
			},
		},
		{
			Name:          "test.run",
			Description:   "Run a configured test command and return a compact result.",
			Tags:          []string{"code", "test"},
			TaskTypes:     []string{"code", "runtime"},
			RiskLevel:     "medium",
			InputSummary:  "test command",
			OutputSummary: "pass/fail status and output summary",
			Provider:      "builtin",
			SchemaRef:     "fn:test.run@v1",
			VersionHash:   "builtin-test-run-v1",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"command"},
				"properties": map[string]any{
					"command": map[string]any{"type": "string"},
				},
			},
			OutputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"passed":  map[string]any{"type": "boolean"},
					"summary": map[string]any{"type": "string"},
				},
			},
		},
		{
			Name:          "artifact.write",
			Description:   "Write an artifact reference for a generated result.",
			Tags:          []string{"artifact", "write"},
			TaskTypes:     []string{"runtime", "document"},
			RiskLevel:     "medium",
			InputSummary:  "artifact path and content summary",
			OutputSummary: "artifact reference",
			Provider:      "builtin",
			SchemaRef:     "fn:artifact.write@v1",
			VersionHash:   "builtin-artifact-write-v1",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":    map[string]any{"type": "string"},
					"summary": map[string]any{"type": "string"},
				},
			},
			OutputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"artifact_ref": map[string]any{"type": "string"},
				},
			},
		},
		{
			Name:          "document.write",
			Description:   "Write a generated document artifact under the workspace artifact root.",
			Tags:          []string{"artifact", "document", "write"},
			TaskTypes:     []string{"document", "writing", "story", "runtime"},
			RiskLevel:     "medium",
			InputSummary:  "document path, markdown content, and summary",
			OutputSummary: "document artifact reference",
			Provider:      "builtin",
			SchemaRef:     "fn:document.write@v1",
			VersionHash:   "builtin-document-write-v1",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"path", "content"},
				"properties": map[string]any{
					"path":    map[string]any{"type": "string"},
					"content": map[string]any{"type": "string"},
					"summary": map[string]any{"type": "string"},
				},
			},
			OutputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"artifact_ref": map[string]any{"type": "string"},
					"bytes":        map[string]any{"type": "number"},
				},
			},
		},
		{
			Name:          "memory.save",
			Description:   "Persist key project memory as a compact markdown artifact.",
			Tags:          []string{"memory", "write", "project"},
			TaskTypes:     []string{"writing", "story", "runtime"},
			RiskLevel:     "low",
			InputSummary:  "memory key, scope, summary, body, and tags",
			OutputSummary: "memory artifact reference",
			Provider:      "builtin",
			SchemaRef:     "fn:memory.save@v1",
			VersionHash:   "builtin-memory-save-v1",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []string{"key", "summary", "body"},
				"properties": map[string]any{
					"key":     map[string]any{"type": "string"},
					"scope":   map[string]any{"type": "string"},
					"summary": map[string]any{"type": "string"},
					"body":    map[string]any{"type": "string"},
					"tags":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
			},
			OutputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"memory_ref": map[string]any{"type": "string"},
					"bytes":      map[string]any{"type": "number"},
				},
			},
		},
	}
	return append(schemas, engineeringSchemas()...)
}

func engineeringSchemas() []Schema {
	names := []struct {
		name        string
		description string
		tags        []string
		taskTypes   []string
		risk        string
		input       string
		output      string
	}{
		{"code.index", "Build a lightweight repository map and symbol index.", []string{"code", "index", "repository"}, []string{"code", "runtime"}, "low", "optional max file and output limits", "files, languages, and symbols"},
		{"code.symbols", "Search indexed source symbols by query or path.", []string{"code", "symbols", "search"}, []string{"code", "runtime"}, "low", "query, path, and limit", "matching symbols"},
		{"file.read", "Read a file inside the workspace root.", []string{"file", "read", "workspace"}, []string{"code", "document", "runtime"}, "low", "workspace relative file path", "file content"},
		{"file.write", "Write a file inside the workspace root.", []string{"file", "write", "workspace"}, []string{"code", "document", "runtime"}, "medium", "workspace relative path and content", "written path and byte count"},
		{"file.patch", "Apply a small old/new text replacement to a workspace file.", []string{"file", "patch", "workspace"}, []string{"code", "runtime"}, "medium", "path, old text, new text", "patched path and replacement count"},
		{"file.diff", "Return git diff for the workspace or a path.", []string{"file", "diff", "git"}, []string{"code", "runtime"}, "low", "optional path", "diff text"},
		{"shell.run", "Run an allowlisted shell command in the workspace.", []string{"shell", "command", "test"}, []string{"code", "runtime"}, "medium", "allowlisted command", "exit status and output"},
		{"git.status", "Return git status --short.", []string{"git", "status"}, []string{"code", "runtime"}, "low", "none", "status text"},
		{"git.diff", "Return git diff.", []string{"git", "diff"}, []string{"code", "runtime"}, "low", "optional path", "diff text"},
		{"git.branch", "Create or checkout a git branch.", []string{"git", "branch"}, []string{"code", "runtime"}, "medium", "branch name and create/checkout mode", "branch operation summary"},
		{"git.commit", "Create a git commit from staged changes.", []string{"git", "commit"}, []string{"code", "runtime"}, "high", "commit message", "commit summary"},
		{"git.rollback", "Restore one explicit workspace path.", []string{"git", "rollback"}, []string{"code", "runtime"}, "high", "workspace relative path", "restore summary"},
	}
	result := make([]Schema, 0, len(names))
	for _, item := range names {
		result = append(result, Schema{
			Name:          item.name,
			Description:   item.description,
			Tags:          item.tags,
			TaskTypes:     item.taskTypes,
			RiskLevel:     item.risk,
			InputSummary:  item.input,
			OutputSummary: item.output,
			Provider:      "builtin",
			SchemaRef:     "fn:" + item.name + "@v1",
			VersionHash:   "builtin-" + strings.ReplaceAll(item.name, ".", "-") + "-v1",
			InputSchema:   map[string]any{"type": "object"},
			OutputSchema:  map[string]any{"type": "object"},
		})
	}
	return result
}
