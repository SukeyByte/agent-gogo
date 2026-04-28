package function

import (
	"context"
	"testing"

	"github.com/sukeke/agent-gogo/internal/contextbuilder"
)

func TestSearchReturnsLightweightCardsOnly(t *testing.T) {
	registry := NewMockRegistry()
	cards, err := registry.Search(context.Background(), SearchRequest{
		Query:                "run tests",
		TaskType:             "code",
		RequiredCapabilities: []string{"test.run"},
		Limit:                2,
	})
	if err != nil {
		t.Fatalf("search functions: %v", err)
	}
	if len(cards) == 0 {
		t.Fatal("expected function cards")
	}
	if cards[0].Name != "test.run" {
		t.Fatalf("expected test.run first, got %s", cards[0].Name)
	}
	if cards[0].SchemaRef == "" {
		t.Fatal("expected schema ref on card")
	}
}

func TestLoadSchemaAndActivateAreOnDemand(t *testing.T) {
	registry := NewMockRegistry()
	cards, err := registry.Search(context.Background(), SearchRequest{
		RequiredCapabilities: []string{"code.search", "test.run"},
	})
	if err != nil {
		t.Fatalf("search functions: %v", err)
	}

	active, err := registry.Activate(context.Background(), cards)
	if err != nil {
		t.Fatalf("activate functions: %v", err)
	}
	if len(active.Schemas) != 2 {
		t.Fatalf("expected two active schemas, got %d", len(active.Schemas))
	}
	if active.Schemas[0].Name != "code.search" || active.Schemas[1].Name != "test.run" {
		t.Fatalf("expected active schemas sorted by name, got %#v", active.Schemas)
	}
	if len(active.Schemas[0].InputSchema) == 0 {
		t.Fatal("expected loaded schema to include input schema")
	}

	contextSchemas := active.ContextSchemas()
	pack := contextbuilder.ContextPack{
		ActiveFunctionSchemas: contextSchemas,
	}
	serialized, err := contextbuilder.NewSerializer(contextbuilder.SerializerOptions{}).Serialize(context.Background(), pack)
	if err != nil {
		t.Fatalf("serialize context: %v", err)
	}
	if len(serialized.BlockKeys.L1) == 0 {
		t.Fatal("expected active function schemas to participate in L1 block keys")
	}
	if serialized.BlockKeys.L1["active_function_schemas"] == "" {
		t.Fatalf("expected active_function_schemas block key, got %#v", serialized.BlockKeys.L1)
	}
}

func TestLoadSchemaRejectsUnknownRef(t *testing.T) {
	registry := NewMockRegistry()
	if _, err := registry.LoadSchema(context.Background(), "fn:nope@v1"); err == nil {
		t.Fatal("expected missing schema error")
	}
}
