package function

import (
	"context"

	"github.com/sukeke/agent-gogo/internal/contextbuilder"
)

type Card struct {
	Name                string
	Description         string
	Tags                []string
	TaskTypes           []string
	RiskLevel           string
	InputSummary        string
	OutputSummary       string
	Provider            string
	RequiredPermissions []string
	SchemaRef           string
	VersionHash         string
	Reason              string
}

type Schema struct {
	Name          string
	Description   string
	Tags          []string
	TaskTypes     []string
	RiskLevel     string
	InputSummary  string
	OutputSummary string
	Provider      string
	SchemaRef     string
	VersionHash   string
	InputSchema   map[string]any
	OutputSchema  map[string]any
}

type SearchRequest struct {
	Query                string
	TaskType             string
	Domains              []string
	RequiredCapabilities []string
	Limit                int
}

type Registry interface {
	Search(ctx context.Context, req SearchRequest) ([]Card, error)
	LoadSchema(ctx context.Context, schemaRef string) (Schema, error)
	Activate(ctx context.Context, cards []Card) (ActiveSet, error)
}

type ActiveSet struct {
	Schemas []Schema
}

func (set ActiveSet) ContextSchemas() []contextbuilder.FunctionSchema {
	result := make([]contextbuilder.FunctionSchema, 0, len(set.Schemas))
	for _, schema := range set.Schemas {
		result = append(result, contextbuilder.FunctionSchema{
			Name:          schema.Name,
			Description:   schema.Description,
			Tags:          append([]string(nil), schema.Tags...),
			TaskTypes:     append([]string(nil), schema.TaskTypes...),
			RiskLevel:     schema.RiskLevel,
			InputSummary:  schema.InputSummary,
			OutputSummary: schema.OutputSummary,
			Provider:      schema.Provider,
			SchemaRef:     schema.SchemaRef,
			VersionHash:   schema.VersionHash,
			InputSchema:   copyMap(schema.InputSchema),
			OutputSchema:  copyMap(schema.OutputSchema),
		})
	}
	return result
}

func (schema Schema) Card(reason string) Card {
	return Card{
		Name:                schema.Name,
		Description:         schema.Description,
		Tags:                append([]string(nil), schema.Tags...),
		TaskTypes:           append([]string(nil), schema.TaskTypes...),
		RiskLevel:           schema.RiskLevel,
		InputSummary:        schema.InputSummary,
		OutputSummary:       schema.OutputSummary,
		Provider:            schema.Provider,
		SchemaRef:           schema.SchemaRef,
		VersionHash:         schema.VersionHash,
		Reason:              reason,
		RequiredPermissions: nil,
	}
}

func copyMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
