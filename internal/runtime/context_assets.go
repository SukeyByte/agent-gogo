package runtime

import (
	"context"

	"github.com/SukeyByte/agent-gogo/internal/chain"
	"github.com/SukeyByte/agent-gogo/internal/contextbuilder"
	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/function"
	intentpkg "github.com/SukeyByte/agent-gogo/internal/intent"
)

type runtimeContextAssets struct {
	ActiveFunctionSchemas []contextbuilder.FunctionSchema
	DeferredFunctionCards []contextbuilder.FunctionCard
	ActiveSkills          []contextbuilder.SkillInstruction
	DeferredSkills        []contextbuilder.SkillPackageRef
	ActivePersonas        []contextbuilder.Persona
	RelevantMemories      []contextbuilder.MemoryItem
}

func (s *Service) loadRuntimeContextAssets(ctx context.Context, project domain.Project, decision chain.Decision, profile intentpkg.Profile, awarenessQuery string) (runtimeContextAssets, error) {
	var assets runtimeContextAssets
	if s.functions != nil {
		cards, err := s.functions.Search(ctx, function.SearchRequest{
			Query:                project.Goal,
			TaskType:             profile.TaskType,
			Domains:              profile.Domains,
			RequiredCapabilities: requiredCapabilities(decision, profile),
			Limit:                8,
		})
		if err != nil {
			return runtimeContextAssets{}, err
		}
		if err := s.log(ctx, "function.search", cards); err != nil {
			return runtimeContextAssets{}, err
		}
		assets.DeferredFunctionCards = functionCardsForContext(cards)
		active, err := s.functions.Activate(ctx, firstFunctionCards(cards, 4))
		if err != nil {
			return runtimeContextAssets{}, err
		}
		assets.ActiveFunctionSchemas = active.ContextSchemas()
	}

	activeSkills, deferredSkills, err := s.searchSkills(ctx, project, profile, decision)
	if err != nil {
		return runtimeContextAssets{}, err
	}
	assets.ActiveSkills = activeSkills
	assets.DeferredSkills = deferredSkills

	activePersonas, err := s.searchPersonas(ctx, project, profile)
	if err != nil {
		return runtimeContextAssets{}, err
	}
	assets.ActivePersonas = activePersonas

	relevantMemories, err := s.searchMemories(ctx, project, profile, awarenessQuery)
	if err != nil {
		return runtimeContextAssets{}, err
	}
	assets.RelevantMemories = relevantMemories
	return assets, nil
}
