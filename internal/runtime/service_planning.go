package runtime

import (
	"context"
	"fmt"
	"strings"

	comm "github.com/SukeyByte/agent-gogo/internal/communication"
	intentpkg "github.com/SukeyByte/agent-gogo/internal/intent"

	"github.com/SukeyByte/agent-gogo/internal/chain"
	"github.com/SukeyByte/agent-gogo/internal/discovery"
	"github.com/SukeyByte/agent-gogo/internal/domain"
	"github.com/SukeyByte/agent-gogo/internal/planner"
)

func (s *Service) PlanProject(ctx context.Context, projectID string) ([]domain.Task, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	project, err := s.store.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	chainDecision, err := s.routeProject(ctx, project)
	if err != nil {
		return nil, err
	}
	if err := s.log(ctx, "chain.router", chainDecision); err != nil {
		return nil, err
	}
	intentProfile, err := s.analyzeProject(ctx, project, chainDecision)
	if err != nil {
		return nil, err
	}
	if err := s.log(ctx, "intent.analyze", intentProfile); err != nil {
		return nil, err
	}
	planningContext, err := s.buildRuntimeContext(ctx, project, "", chainDecision, intentProfile)
	if err != nil {
		return nil, err
	}
	discoveryResult, err := s.runDiscovery(ctx, project, chainDecision, intentProfile)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(discoveryResult.Summary) != "" {
		planningContext = appendPlanningDiscovery(planningContext, discoveryResult)
		planningContext = limitContextText(planningContext, s.contextMaxChars)
	}
	s.setState(project.ID, planningContext, chainDecision, intentProfile)
	drafts, err := s.planner.PlanProject(ctx, planner.PlanRequest{
		Project:       project,
		UserInput:     project.Goal,
		ChainDecision: chainDecision,
		IntentProfile: intentProfile,
		ContextText:   planningContext,
	})
	if err != nil {
		return nil, err
	}
	drafts = normalizeTaskDependencies(drafts)
	if err := s.log(ctx, "planner.tasks", drafts); err != nil {
		return nil, err
	}

	type plannedTask struct {
		draft   domain.Task
		created domain.Task
	}
	planned := make([]plannedTask, 0, len(drafts))
	titleToID := map[string]string{}
	for _, draft := range drafts {
		draft.ProjectID = project.ID
		created, err := s.store.CreateTask(ctx, draft)
		if err != nil {
			return nil, err
		}
		planned = append(planned, plannedTask{draft: draft, created: created})
		titleToID[created.Title] = created.ID
	}
	for _, item := range planned {
		for _, dependsOn := range item.draft.DependsOn {
			dependencyID, ok := titleToID[dependsOn]
			if !ok {
				return nil, fmt.Errorf("planned task %q depends on unknown task %q", item.created.Title, dependsOn)
			}
			if dependencyID == item.created.ID {
				return nil, fmt.Errorf("planned task %q cannot depend on itself", item.created.Title)
			}
			if _, err := s.store.CreateTaskDependency(ctx, domain.TaskDependency{
				TaskID:          item.created.ID,
				DependsOnTaskID: dependencyID,
			}); err != nil {
				return nil, err
			}
		}
	}

	readyTasks := make([]domain.Task, 0, len(planned))
	validationErrors := make([]string, 0)
	for _, item := range planned {
		created := item.created
		if err := s.validator.ValidateTask(ctx, created); err != nil {
			blocked, transitionErr := s.store.TransitionTask(ctx, created.ID, domain.TaskStatusBlocked, "capability validation blocked planned task: "+err.Error())
			if transitionErr != nil {
				return nil, transitionErr
			}
			s.emitTaskBlocked(ctx, project.ID, blocked, "Task blocked by capability validation: "+created.Title+" - "+err.Error())
			validationErrors = append(validationErrors, fmt.Sprintf("%s: %s", created.Title, err.Error()))
			continue
		}
		ready, err := s.store.TransitionTask(ctx, created.ID, domain.TaskStatusReady, "validator accepted planned task")
		if err != nil {
			return nil, err
		}
		readyTasks = append(readyTasks, ready)
	}
	if err := s.emit(ctx, comm.NewMessageIntent(s.communicationChannel, fmt.Sprintf("Planned %d task(s)", len(readyTasks))), project.ID); err != nil {
		return nil, err
	}
	if len(validationErrors) > 0 {
		if err := s.emit(ctx, comm.NewBlockedIntent(s.communicationChannel, fmt.Sprintf("Blocked %d planned task(s) by capability policy", len(validationErrors)), map[string]any{
			"project_id": project.ID,
			"status":     string(domain.TaskStatusBlocked),
			"errors":     validationErrors,
		}), project.ID); err != nil {
			return nil, err
		}
		if len(readyTasks) == 0 {
			return nil, fmt.Errorf("no runnable tasks after capability validation: %s", strings.Join(validationErrors, "; "))
		}
	}
	s.saveSessionContext(ctx, project.ID)
	return readyTasks, nil
}

func normalizeTaskDependencies(tasks []domain.Task) []domain.Task {
	if len(tasks) == 0 {
		return tasks
	}
	out := append([]domain.Task(nil), tasks...)
	titleSet := map[string]struct{}{}
	for _, task := range out {
		if title := strings.TrimSpace(task.Title); title != "" {
			titleSet[title] = struct{}{}
		}
	}
	researchTitle := firstDomainTaskTitleMatching(out, []string{"研究", "調研", "调研", "research", "context", "gather", "收集", "获取", "取得", "读取", "搜索"})
	reflectionTitle := firstDomainTaskTitleMatching(out, []string{"反思", "reflection", "review plan", "验收口径", "驗收口徑", "decomposition", "acceptance criteria"})
	finalMarkers := []string{"最终", "最終", "验收", "驗收", "验证", "驗證", "检查", "檢查", "自检", "自檢", "自查", "复核", "覆核", "完整性", "汇报", "匯報", "总结", "總結", "汇总", "彙總", "final", "verify", "validate", "validation", "self-check", "self check", "selfcheck", "correctness", "completeness", "report", "summary"}
	taskByTitle := map[string]domain.Task{}
	finalByTitle := map[string]bool{}
	for _, task := range out {
		title := strings.TrimSpace(task.Title)
		if title == "" {
			continue
		}
		taskByTitle[title] = task
		finalByTitle[title] = title != researchTitle && title != reflectionTitle && domainTaskMatches(task, finalMarkers)
	}
	for i := range out {
		title := strings.TrimSpace(out[i].Title)
		isFinalTask := finalByTitle[title]
		deps := make([]string, 0, len(out[i].DependsOn)+1)
		seen := map[string]struct{}{}
		addDep := func(dep string) {
			dep = strings.TrimSpace(dep)
			if dep == "" || dep == title {
				return
			}
			if _, ok := titleSet[dep]; !ok {
				return
			}
			if !isFinalTask {
				if _, ok := taskByTitle[dep]; ok && finalByTitle[dep] {
					return
				}
			}
			if _, ok := seen[dep]; ok {
				return
			}
			seen[dep] = struct{}{}
			deps = append(deps, dep)
		}
		switch title {
		case researchTitle:
		case reflectionTitle:
			if researchTitle != "" && researchTitle != reflectionTitle {
				addDep(researchTitle)
			}
		default:
			for _, dep := range out[i].DependsOn {
				addDep(dep)
			}
			if reflectionTitle != "" {
				addDep(reflectionTitle)
			} else if researchTitle != "" {
				addDep(researchTitle)
			}
			if isFinalTask {
				for _, candidate := range out {
					if strings.TrimSpace(candidate.Title) == title || strings.TrimSpace(candidate.Title) == "" {
						continue
					}
					if domainTaskMatches(candidate, finalMarkers) {
						continue
					}
					addDep(candidate.Title)
				}
			}
		}
		out[i].DependsOn = deps
	}
	return pruneDomainDependencyCycles(out)
}

func firstDomainTaskTitleMatching(tasks []domain.Task, markers []string) string {
	for _, task := range tasks {
		if domainTaskMatches(task, markers) && strings.TrimSpace(task.Title) != "" {
			return task.Title
		}
	}
	return ""
}

func domainTaskMatches(task domain.Task, markers []string) bool {
	text := strings.ToLower(task.Title + " " + task.Description + " " + strings.Join(task.AcceptanceCriteria, " "))
	for _, marker := range markers {
		if strings.Contains(text, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

func pruneDomainDependencyCycles(tasks []domain.Task) []domain.Task {
	out := append([]domain.Task(nil), tasks...)
	depsByTitle := map[string][]string{}
	for _, task := range out {
		depsByTitle[task.Title] = append([]string(nil), task.DependsOn...)
	}
	for i := range out {
		title := strings.TrimSpace(out[i].Title)
		kept := out[i].DependsOn[:0]
		for _, dep := range out[i].DependsOn {
			depsByTitle[title] = kept
			if domainDependencyReaches(depsByTitle, dep, title, map[string]bool{}) {
				continue
			}
			kept = append(kept, dep)
		}
		out[i].DependsOn = kept
		depsByTitle[title] = append([]string(nil), kept...)
	}
	return out
}

func domainDependencyReaches(depsByTitle map[string][]string, from string, target string, seen map[string]bool) bool {
	from = strings.TrimSpace(from)
	target = strings.TrimSpace(target)
	if from == "" || target == "" {
		return false
	}
	if from == target {
		return true
	}
	if seen[from] {
		return false
	}
	seen[from] = true
	for _, next := range depsByTitle[from] {
		if domainDependencyReaches(depsByTitle, next, target, seen) {
			return true
		}
	}
	return false
}

func (s *Service) runDiscovery(ctx context.Context, project domain.Project, decision chain.Decision, profile intentpkg.Profile) (discovery.Result, error) {
	if s.discovery == nil {
		return discovery.Result{}, nil
	}
	result, err := s.discovery.Discover(ctx, discovery.Request{
		Project:       project,
		ChainDecision: decision,
		IntentProfile: profile,
	})
	if err != nil {
		return discovery.Result{}, err
	}
	if err := s.log(ctx, "discovery.preplan", result); err != nil {
		return discovery.Result{}, err
	}
	return result, nil
}

func appendPlanningDiscovery(contextText string, result discovery.Result) string {
	summary := strings.TrimSpace(result.Summary)
	if summary == "" {
		return contextText
	}
	if strings.TrimSpace(contextText) == "" {
		return "[DISCOVERY]\n" + summary
	}
	return contextText + "\n\n[DISCOVERY]\n" + summary
}

func (s *Service) routeProject(ctx context.Context, project domain.Project) (chain.Decision, error) {
	if s.chainRouter == nil {
		return chain.Decision{}, nil
	}
	return s.chainRouter.Route(ctx, chain.Request{
		UserInput: project.Goal,
		ProjectID: project.ID,
		Channel:   s.communicationChannel,
	})
}

func (s *Service) analyzeProject(ctx context.Context, project domain.Project, decision chain.Decision) (intentpkg.Profile, error) {
	if s.intentAnalyzer == nil {
		return intentpkg.Profile{}, nil
	}
	return s.intentAnalyzer.Analyze(ctx, intentpkg.Request{
		UserInput:     project.Goal,
		ChainDecision: decision,
	})
}
