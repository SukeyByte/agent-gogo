package contextbuilder

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSerializeIsDeterministicDespiteInputOrder(t *testing.T) {
	serializer := NewSerializer(SerializerOptions{})
	first, err := serializer.Serialize(context.Background(), samplePack())
	if err != nil {
		t.Fatalf("serialize first pack: %v", err)
	}
	second, err := serializer.Serialize(context.Background(), samplePackReordered())
	if err != nil {
		t.Fatalf("serialize reordered pack: %v", err)
	}

	if first.Text != second.Text {
		t.Fatalf("expected deterministic text\nfirst:\n%s\nsecond:\n%s", first.Text, second.Text)
	}
	if first.LayerKeys != second.LayerKeys {
		t.Fatalf("expected deterministic layer keys\nfirst: %#v\nsecond: %#v", first.LayerKeys, second.LayerKeys)
	}
}

func TestDynamicL3DoesNotInvalidateStableLayerKeys(t *testing.T) {
	serializer := NewSerializer(SerializerOptions{})
	base, err := serializer.Serialize(context.Background(), samplePack())
	if err != nil {
		t.Fatalf("serialize base pack: %v", err)
	}

	changed := samplePack()
	changed.CurrentUserInput = "new user input"
	changed.RecentMessages = append(changed.RecentMessages, Message{
		ID:        "msg-003",
		Role:      "assistant",
		Text:      "Working on it",
		CreatedAt: mustTime("2026-04-28T10:03:00Z"),
	})
	changed.EvidenceRefs = append(changed.EvidenceRefs, EvidenceRef{
		ID:          "ev-003",
		Type:        "log",
		Summary:     "latest dynamic evidence",
		ArtifactRef: "artifact://ev-003",
		CreatedAt:   mustTime("2026-04-28T10:04:00Z"),
	})

	next, err := serializer.Serialize(context.Background(), changed)
	if err != nil {
		t.Fatalf("serialize changed pack: %v", err)
	}

	if base.LayerKeys.L0 != next.LayerKeys.L0 {
		t.Fatal("expected L0 key to remain stable")
	}
	if base.LayerKeys.L1 != next.LayerKeys.L1 {
		t.Fatal("expected L1 key to remain stable")
	}
	if base.LayerKeys.L2 != next.LayerKeys.L2 {
		t.Fatal("expected L2 key to remain stable")
	}
	if base.LayerKeys.L3 == next.LayerKeys.L3 {
		t.Fatal("expected L3 key to change")
	}

	invalidations := DiffLayerKeys(base.LayerKeys, next.LayerKeys)
	if len(invalidations) != 1 || invalidations[0].Layer != LayerL3 {
		t.Fatalf("expected only L3 invalidation, got %#v", invalidations)
	}
	prefix := StablePrefix(base.LayerKeys, next.LayerKeys)
	if len(prefix) != 3 || prefix[0] != LayerL0 || prefix[1] != LayerL1 || prefix[2] != LayerL2 {
		t.Fatalf("expected L0-L2 stable prefix, got %#v", prefix)
	}
}

func TestTaskScopedChangesInvalidateOnlyL2AndLaterPrefix(t *testing.T) {
	serializer := NewSerializer(SerializerOptions{})
	base, err := serializer.Serialize(context.Background(), samplePack())
	if err != nil {
		t.Fatalf("serialize base pack: %v", err)
	}

	changed := samplePack()
	changed.AcceptanceCriteria = append(changed.AcceptanceCriteria, AcceptanceCriterion{
		ID:   "ac-003",
		Text: "cache invalidation is explained",
	})

	next, err := serializer.Serialize(context.Background(), changed)
	if err != nil {
		t.Fatalf("serialize changed pack: %v", err)
	}

	if base.LayerKeys.L0 != next.LayerKeys.L0 {
		t.Fatal("expected L0 key to remain stable")
	}
	if base.LayerKeys.L1 != next.LayerKeys.L1 {
		t.Fatal("expected L1 key to remain stable")
	}
	if base.LayerKeys.L2 == next.LayerKeys.L2 {
		t.Fatal("expected L2 key to change")
	}

	invalidations := DiffLayerKeys(base.LayerKeys, next.LayerKeys)
	if len(invalidations) != 1 || invalidations[0].Layer != LayerL2 {
		t.Fatalf("expected only L2 invalidation, got %#v", invalidations)
	}
	blockInvalidations := DiffBlockKeys(base.BlockKeys, next.BlockKeys)
	if len(blockInvalidations) != 1 || blockInvalidations[0].Layer != LayerL2 || blockInvalidations[0].Block != "acceptance_criteria" {
		t.Fatalf("expected acceptance_criteria block invalidation, got %#v", blockInvalidations)
	}
	prefix := StablePrefix(base.LayerKeys, next.LayerKeys)
	if len(prefix) != 2 || prefix[0] != LayerL0 || prefix[1] != LayerL1 {
		t.Fatalf("expected L0-L1 stable prefix, got %#v", prefix)
	}
}

func TestStableLayersDoNotSerializeDynamicTimestamps(t *testing.T) {
	serializer := NewSerializer(SerializerOptions{})
	serialized, err := serializer.Serialize(context.Background(), samplePack())
	if err != nil {
		t.Fatalf("serialize pack: %v", err)
	}

	stable := serialized.Layers.L0 + serialized.Layers.L1 + serialized.Layers.L2
	for _, forbidden := range []string{"created_at", "2026-04-28T10:01:00Z", "latest dynamic evidence", "current_user_input"} {
		if strings.Contains(stable, forbidden) {
			t.Fatalf("stable layers should not contain %q:\n%s", forbidden, stable)
		}
	}
	if !strings.Contains(serialized.Layers.L3, "created_at") {
		t.Fatal("expected L3 to contain dynamic timestamps")
	}
}

func TestRecentEvidenceWindowUsesNewestThenOutputsChronologically(t *testing.T) {
	serializer := NewSerializer(SerializerOptions{MaxEvidenceRefs: 2})
	pack := samplePack()
	pack.EvidenceRefs = []EvidenceRef{
		{ID: "ev-old", Type: "log", Summary: "old", CreatedAt: mustTime("2026-04-28T10:00:00Z")},
		{ID: "ev-newest", Type: "log", Summary: "newest", CreatedAt: mustTime("2026-04-28T10:03:00Z")},
		{ID: "ev-middle", Type: "log", Summary: "middle", CreatedAt: mustTime("2026-04-28T10:02:00Z")},
	}

	serialized, err := serializer.Serialize(context.Background(), pack)
	if err != nil {
		t.Fatalf("serialize pack: %v", err)
	}
	if strings.Contains(serialized.Layers.L3, "ev-old") {
		t.Fatalf("expected old evidence to be dropped:\n%s", serialized.Layers.L3)
	}
	middleIndex := strings.Index(serialized.Layers.L3, "ev-middle")
	newestIndex := strings.Index(serialized.Layers.L3, "ev-newest")
	if middleIndex == -1 || newestIndex == -1 || middleIndex > newestIndex {
		t.Fatalf("expected selected evidence to output chronologically:\n%s", serialized.Layers.L3)
	}
}

func samplePack() ContextPack {
	return ContextPack{
		RuntimeRules: []Message{
			{ID: "runtime-002", Role: "system", Text: "Use tool evidence.", VersionHash: "r2"},
			{ID: "runtime-001", Role: "system", Text: "Keep layers stable.", VersionHash: "r1"},
		},
		SecurityRules: []Message{
			{ID: "security-002", Role: "system", Text: "Confirm high risk actions.", VersionHash: "s2"},
			{ID: "security-001", Role: "system", Text: "Do not leak secrets.", VersionHash: "s1"},
		},
		ActivePersonas: []Persona{
			{ID: "persona-runtime", Name: "Runtime", VersionHash: "p2", Instructions: "Be deterministic."},
			{ID: "persona-engineer", Name: "Engineer", VersionHash: "p1", Instructions: "Prefer tests."},
		},
		ChannelCapabilities: []ChannelCapability{
			{
				ChannelType:           "web",
				Capabilities:          []string{"ask_confirmation", "send_message"},
				SupportedMessageTypes: []string{"text", "artifact"},
				SupportedInteractions: []string{"modal", "button"},
				MaxMessageLength:      12000,
				SupportsConfirmation:  true,
				SupportsStreaming:     true,
				PolicyLimits:          map[string]string{"risk": "confirm", "files": "request"},
			},
			{
				ChannelType:           "cli",
				Capabilities:          []string{"send_message", "ask_confirmation"},
				SupportedMessageTypes: []string{"text"},
				SupportedInteractions: []string{"prompt"},
				MaxMessageLength:      8000,
				SupportsConfirmation:  true,
			},
		},
		MetaFunctionSchemas: []FunctionSchema{
			{Name: "skill.search", Description: "Search skills", SchemaRef: "fn:skill.search@v1", VersionHash: "m2"},
			{Name: "function.search", Description: "Search functions", SchemaRef: "fn:function.search@v1", VersionHash: "m1"},
		},
		ActiveCapabilities: []CapabilitySpec{
			{Name: "test.run", Description: "Run tests", RiskLevel: "medium", VersionHash: "c2"},
			{Name: "code.search", Description: "Search code", RiskLevel: "low", VersionHash: "c1"},
		},
		ActiveFunctionSchemas: []FunctionSchema{
			{Name: "test.run", Description: "Run tests", Tags: []string{"test", "code"}, RiskLevel: "medium", SchemaRef: "fn:test.run@v1", VersionHash: "f2"},
			{Name: "code.search", Description: "Search code", Tags: []string{"code"}, RiskLevel: "low", SchemaRef: "fn:code.search@v1", VersionHash: "f1"},
		},
		DeferredFunctionCandidates: []FunctionCard{
			{Name: "browser.open", SchemaRef: "fn:browser.open@v1", VersionHash: "fc2", Reason: "Not needed yet"},
			{Name: "artifact.write", SchemaRef: "fn:artifact.write@v1", VersionHash: "fc1", Reason: "Can write later"},
		},
		ActiveSkillInstructions: []SkillInstruction{
			{ID: "skill-store", Name: "Store", VersionHash: "sk2", Instructions: "Persist facts.", AllowedTools: []string{"test.run", "code.search"}},
			{ID: "skill-go", Name: "Go", VersionHash: "sk1", Instructions: "Use go test.", AllowedTools: []string{"code.search"}},
		},
		DeferredSkillCandidates: []SkillPackageRef{
			{ID: "skill-browser", Name: "Browser", VersionHash: "sr2", Path: ".claude/skills/browser", Reason: "M6"},
			{ID: "skill-docs", Name: "Docs", VersionHash: "sr1", Path: ".claude/skills/docs", Reason: "Optional"},
		},
		IntentProfile: IntentProfile{
			TaskType:             "code",
			Complexity:           "L2",
			Domains:              []string{"runtime", "go"},
			RequiredCapabilities: []string{"test.run", "code.search"},
			RiskLevel:            "low",
			GroundingRequirement: "tests",
			Confidence:           0.91,
		},
		ProjectState: ProjectState{
			ID:      "project-001",
			Name:    "agent-gogo",
			Goal:    "Build runtime core",
			Status:  "ACTIVE",
			Summary: "M2 context builder",
		},
		TaskState: TaskState{
			ID:             "task-002",
			Goal:           "Implement deterministic context serializer",
			Status:         "READY",
			AttemptCount:   0,
			CacheVersion:   "task-v1",
			FrozenRevision: "rev-001",
		},
		RelevantMemories: []MemoryItem{
			{ID: "mem-002", Scope: "project", VersionHash: "mem2", Summary: "Use SQLite by default."},
			{ID: "mem-001", Scope: "project", VersionHash: "mem1", Summary: "Cache layers must be deterministic."},
		},
		AcceptanceCriteria: []AcceptanceCriterion{
			{ID: "ac-002", Text: "L3 changes do not alter L0-L2 keys"},
			{ID: "ac-001", Text: "same input serializes to identical bytes"},
		},
		EvidenceRefs: []EvidenceRef{
			{ID: "ev-002", Type: "test", Summary: "go test passed", ArtifactRef: "artifact://test", CreatedAt: mustTime("2026-04-28T10:02:00Z")},
			{ID: "ev-001", Type: "diff", Summary: "contextbuilder added", ArtifactRef: "artifact://diff", CreatedAt: mustTime("2026-04-28T10:01:00Z")},
		},
		RecentMessages: []Message{
			{ID: "msg-002", Role: "assistant", Text: "I will implement M2.", CreatedAt: mustTime("2026-04-28T10:01:00Z")},
			{ID: "msg-001", Role: "user", Text: "Continue M2.", CreatedAt: mustTime("2026-04-28T10:00:00Z")},
		},
		CurrentUserInput: "Implement M2 and write result.",
	}
}

func samplePackReordered() ContextPack {
	pack := samplePack()
	reverseMessages(pack.RuntimeRules)
	reverseMessages(pack.SecurityRules)
	reversePersonas(pack.ActivePersonas)
	reverseChannelCapabilities(pack.ChannelCapabilities)
	reverseFunctionSchemas(pack.MetaFunctionSchemas)
	reverseFunctionSchemas(pack.ActiveFunctionSchemas)
	reverseCapabilitySpecs(pack.ActiveCapabilities)
	reverseFunctionCards(pack.DeferredFunctionCandidates)
	reverseSkillInstructions(pack.ActiveSkillInstructions)
	reverseSkillPackageRefs(pack.DeferredSkillCandidates)
	reverseMemoryItems(pack.RelevantMemories)
	reverseAcceptanceCriteria(pack.AcceptanceCriteria)
	reverseEvidenceRefs(pack.EvidenceRefs)
	reverseMessages(pack.RecentMessages)
	pack.IntentProfile.Domains = []string{"go", "runtime"}
	pack.IntentProfile.RequiredCapabilities = []string{"code.search", "test.run"}
	return pack
}

func reverseMessages(values []Message) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func reversePersonas(values []Persona) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func reverseChannelCapabilities(values []ChannelCapability) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func reverseFunctionSchemas(values []FunctionSchema) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func reverseCapabilitySpecs(values []CapabilitySpec) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func reverseFunctionCards(values []FunctionCard) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func reverseSkillInstructions(values []SkillInstruction) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func reverseSkillPackageRefs(values []SkillPackageRef) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func reverseMemoryItems(values []MemoryItem) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func reverseAcceptanceCriteria(values []AcceptanceCriterion) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func reverseEvidenceRefs(values []EvidenceRef) {
	for i, j := 0, len(values)-1; i < j; i, j = i+1, j-1 {
		values[i], values[j] = values[j], values[i]
	}
}

func mustTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		panic(err)
	}
	return parsed
}
