package function

import (
	"context"
	"errors"
	"sort"
	"strings"
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
	schema.Tags = sortedUnique(schema.Tags)
	schema.TaskTypes = sortedUnique(schema.TaskTypes)
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

func sortedUnique(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	out := result[:0]
	var previous string
	for i, value := range result {
		if i > 0 && value == previous {
			continue
		}
		out = append(out, value)
		previous = value
	}
	if out == nil {
		return []string{}
	}
	return out
}

func defaultSchemas() []Schema {
	return []Schema{
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
	}
}
