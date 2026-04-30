package runtime

import (
	"context"
	"strings"

	"github.com/SukeyByte/agent-gogo/internal/chain"
	"github.com/SukeyByte/agent-gogo/internal/contextbuilder"
	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/function"
	intentpkg "github.com/SukeyByte/agent-gogo/internal/intent"
	"github.com/SukeyByte/agent-gogo/internal/skill"
	"github.com/SukeyByte/agent-gogo/internal/taskaware"
	"github.com/SukeyByte/agent-gogo/internal/textutil"
)

func (s *Service) buildRuntimeContext(ctx context.Context, project domain.Project, currentTaskID string, decision chain.Decision, profile intentpkg.Profile) (string, error) {
	if s.contextSerializer == nil {
		return "", nil
	}
	awareness, err := taskaware.BuildContextSnapshot(ctx, s.store, project, currentTaskID)
	if err != nil {
		return "", err
	}
	if err := s.log(ctx, "taskaware.digest", map[string]any{
		"project_id":      project.ID,
		"current_task_id": currentTaskID,
		"project_summary": awareness.ProjectState.Summary,
		"task_state":      awareness.TaskState,
	}); err != nil {
		return "", err
	}

	assets, err := s.loadRuntimeContextAssets(ctx, project, decision, profile, awareness.QueryText)
	if err != nil {
		return "", err
	}

	pack := contextbuilder.ContextPack{
		RuntimeRules: []contextbuilder.Message{
			{ID: "runtime-001", Role: "system", Text: "Use the runtime state machine, tool evidence, tester, and reviewer before marking work done.", VersionHash: "runtime-v1"},
		},
		SecurityRules: []contextbuilder.Message{
			{ID: "security-001", Role: "system", Text: "All side effects must go through Tool Runtime or Communication Runtime. Do not expose API keys.", VersionHash: "security-v1"},
		},
		ChannelCapabilities: []contextbuilder.ChannelCapability{
			{
				ChannelType:           s.communicationChannel,
				Capabilities:          []string{"send_message", "notify_done", "ask_confirmation"},
				SupportedMessageTypes: []string{"text", "artifact"},
				SupportsConfirmation:  true,
			},
		},
		IntentProfile:              profile.ContextProfile(),
		ActiveCapabilities:         capabilitySpecs(assets.ActiveFunctionSchemas),
		ActiveFunctionSchemas:      assets.ActiveFunctionSchemas,
		DeferredFunctionCandidates: assets.DeferredFunctionCards,
		ActiveSkillInstructions:    assets.ActiveSkills,
		DeferredSkillCandidates:    assets.DeferredSkills,
		ActivePersonas:             assets.ActivePersonas,
		RelevantMemories:           assets.RelevantMemories,
		ProjectState:               awareness.ProjectState,
		TaskState:                  awareness.TaskState,
		AcceptanceCriteria:         awareness.AcceptanceCriteria,
		EvidenceRefs:               awareness.EvidenceRefs,
		CurrentUserInput:           project.Goal,
	}
	serialized, err := s.contextSerializer.Serialize(ctx, pack)
	if err != nil {
		return "", err
	}
	text := limitContextText(serialized.Text, s.contextMaxChars)
	if err := s.log(ctx, "contextbuilder.serialize", map[string]any{
		"layer_keys":        serialized.LayerKeys,
		"block_keys":        serialized.BlockKeys,
		"text":              text,
		"context_max_chars": s.contextMaxChars,
		"truncated":         text != serialized.Text,
	}); err != nil {
		return "", err
	}
	return text, nil
}

func (s *Service) searchSkills(ctx context.Context, project domain.Project, profile intentpkg.Profile, decision chain.Decision) ([]contextbuilder.SkillInstruction, []contextbuilder.SkillPackageRef, error) {
	if s.skills == nil {
		return nil, nil, nil
	}
	query := strings.TrimSpace(project.Goal + " " + profile.TaskType + " " + strings.Join(decision.SkillTags, " "))
	cards, err := s.skills.Search(ctx, query, 4)
	if err != nil {
		return nil, nil, err
	}
	if len(cards) == 0 {
		cards, err = s.skills.Search(ctx, "story writing plot chapter fiction", 4)
		if err != nil {
			return nil, nil, err
		}
	}
	if err := s.log(ctx, "skill.search", cards); err != nil {
		return nil, nil, err
	}
	active := make([]contextbuilder.SkillInstruction, 0, minInt(2, len(cards)))
	for _, card := range firstSkillCards(cards, 2) {
		pkg, err := s.skills.Load(ctx, card.ID)
		if err != nil {
			return nil, nil, err
		}
		active = append(active, pkg.ContextInstruction())
		if err := s.log(ctx, "skill.load", map[string]any{"id": pkg.ID, "name": pkg.Name, "path": pkg.Path}); err != nil {
			return nil, nil, err
		}
	}
	deferred := make([]contextbuilder.SkillPackageRef, 0, len(cards))
	for _, card := range cards {
		deferred = append(deferred, contextbuilder.SkillPackageRef{
			ID:          card.ID,
			Name:        card.Name,
			VersionHash: card.VersionHash,
			Path:        card.Path,
			Reason:      card.Reason,
		})
	}
	return active, deferred, nil
}

func (s *Service) searchPersonas(ctx context.Context, project domain.Project, profile intentpkg.Profile) ([]contextbuilder.Persona, error) {
	active := append([]contextbuilder.Persona(nil), s.activePersonas...)
	if s.personas == nil {
		return active, nil
	}
	query := strings.TrimSpace(project.Goal + " " + profile.TaskType)
	cards, err := s.personas.Search(ctx, query, 2)
	if err != nil {
		return nil, err
	}
	if err := s.log(ctx, "persona.search", cards); err != nil {
		return nil, err
	}
	for _, card := range cards {
		persona, err := s.personas.Load(ctx, card.ID)
		if err != nil {
			return nil, err
		}
		active = append(active, persona.ContextPersona())
		if err := s.log(ctx, "persona.load", map[string]any{"id": persona.ID, "name": persona.Name, "path": persona.Path}); err != nil {
			return nil, err
		}
	}
	return active, nil
}

func (s *Service) searchMemories(ctx context.Context, project domain.Project, profile intentpkg.Profile, awarenessQuery string) ([]contextbuilder.MemoryItem, error) {
	if s.memories == nil {
		return nil, nil
	}
	cards, err := s.memories.Search(ctx, strings.TrimSpace(project.Goal+" "+profile.TaskType+" "+awarenessQuery), "project", 8)
	if err != nil {
		return nil, err
	}
	if err := s.log(ctx, "memory.search", cards); err != nil {
		return nil, err
	}
	items := make([]contextbuilder.MemoryItem, 0, len(cards))
	for _, card := range cards {
		item, err := s.memories.Load(ctx, card.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, item.ContextMemory())
	}
	return items, nil
}

func requiredCapabilities(decision chain.Decision, profile intentpkg.Profile) []string {
	values := append([]string(nil), profile.RequiredCapabilities...)
	values = append(values, decision.ToolNames...)
	return sortedUniqueStrings(values)
}

func firstFunctionCards(cards []function.Card, limit int) []function.Card {
	if limit > 0 && len(cards) > limit {
		return append([]function.Card(nil), cards[:limit]...)
	}
	return append([]function.Card(nil), cards...)
}

func firstSkillCards(cards []skill.Card, limit int) []skill.Card {
	if limit > 0 && len(cards) > limit {
		return append([]skill.Card(nil), cards[:limit]...)
	}
	return append([]skill.Card(nil), cards...)
}

func functionCardsForContext(cards []function.Card) []contextbuilder.FunctionCard {
	result := make([]contextbuilder.FunctionCard, 0, len(cards))
	for _, card := range cards {
		result = append(result, contextbuilder.FunctionCard{
			Name:                card.Name,
			Description:         card.Description,
			Tags:                append([]string(nil), card.Tags...),
			TaskTypes:           append([]string(nil), card.TaskTypes...),
			RiskLevel:           card.RiskLevel,
			InputSummary:        card.InputSummary,
			OutputSummary:       card.OutputSummary,
			Provider:            card.Provider,
			RequiredPermissions: append([]string(nil), card.RequiredPermissions...),
			SchemaRef:           card.SchemaRef,
			VersionHash:         card.VersionHash,
			Reason:              card.Reason,
		})
	}
	return result
}

func capabilitySpecs(schemas []contextbuilder.FunctionSchema) []contextbuilder.CapabilitySpec {
	result := make([]contextbuilder.CapabilitySpec, 0, len(schemas))
	for _, schema := range schemas {
		result = append(result, contextbuilder.CapabilitySpec{
			Name:        schema.Name,
			Description: schema.Description,
			RiskLevel:   schema.RiskLevel,
			VersionHash: schema.VersionHash,
		})
	}
	return result
}

func sortedUniqueStrings(values []string) []string {
	return textutil.SortedUniqueStrings(values)
}

func minInt(left int, right int) int {
	if left < right {
		return left
	}
	return right
}

func limitContextText(text string, maxChars int) string {
	if maxChars <= 0 || len(text) <= maxChars {
		return text
	}
	const suffix = "\n...[context truncated by runtime budget]"
	if maxChars <= len(suffix) {
		return text[:maxChars]
	}
	return text[:maxChars-len(suffix)] + suffix
}
