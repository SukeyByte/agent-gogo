package contextbuilder

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SukeyByte/agent-gogo/internal/textutil"
)

const (
	defaultMaxEvidenceRefs   = 8
	defaultMaxRecentMessages = 12
)

type SerializerOptions struct {
	Version           string
	MaxEvidenceRefs   int
	MaxRecentMessages int
}

type Serializer struct {
	version           string
	maxEvidenceRefs   int
	maxRecentMessages int
}

func NewSerializer(options SerializerOptions) *Serializer {
	version := options.Version
	if version == "" {
		version = DefaultVersion
	}
	maxEvidenceRefs := options.MaxEvidenceRefs
	if maxEvidenceRefs <= 0 {
		maxEvidenceRefs = defaultMaxEvidenceRefs
	}
	maxRecentMessages := options.MaxRecentMessages
	if maxRecentMessages <= 0 {
		maxRecentMessages = defaultMaxRecentMessages
	}
	return &Serializer{
		version:           version,
		maxEvidenceRefs:   maxEvidenceRefs,
		maxRecentMessages: maxRecentMessages,
	}
}

func (s *Serializer) Serialize(ctx context.Context, pack ContextPack) (*SerializedContext, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	normalized := s.normalize(pack)
	l0, err := stableJSON(normalized.L0)
	if err != nil {
		return nil, err
	}
	l1, err := stableJSON(normalized.L1)
	if err != nil {
		return nil, err
	}
	l2, err := stableJSON(normalized.L2)
	if err != nil {
		return nil, err
	}
	l3, err := stableJSON(normalized.L3)
	if err != nil {
		return nil, err
	}

	layers := SerializedLayers{L0: l0, L1: l1, L2: l2, L3: l3}
	blockKeys, err := s.blockKeys(layers)
	if err != nil {
		return nil, err
	}
	return &SerializedContext{
		Text:      renderText(s.version, layers),
		LayerKeys: s.layerKeys(layers),
		BlockKeys: blockKeys,
		Layers:    layers,
		Version:   s.version,
	}, nil
}

func (s *Serializer) layerKeys(layers SerializedLayers) ContextLayerKeys {
	return ContextLayerKeys{
		L0: layerKey(s.version, LayerL0, layers.L0),
		L1: layerKey(s.version, LayerL1, layers.L1),
		L2: layerKey(s.version, LayerL2, layers.L2),
		L3: layerKey(s.version, LayerL3, layers.L3),
	}
}

func (s *Serializer) blockKeys(layers SerializedLayers) (ContextBlockKeys, error) {
	l0, err := blockKeys(s.version, LayerL0, layers.L0)
	if err != nil {
		return ContextBlockKeys{}, err
	}
	l1, err := blockKeys(s.version, LayerL1, layers.L1)
	if err != nil {
		return ContextBlockKeys{}, err
	}
	l2, err := blockKeys(s.version, LayerL2, layers.L2)
	if err != nil {
		return ContextBlockKeys{}, err
	}
	l3, err := blockKeys(s.version, LayerL3, layers.L3)
	if err != nil {
		return ContextBlockKeys{}, err
	}
	return ContextBlockKeys{L0: l0, L1: l1, L2: l2, L3: l3}, nil
}

func layerKey(version string, layer LayerName, text string) string {
	hash := sha256.Sum256([]byte(version + "\n" + string(layer) + "\n" + text))
	return fmt.Sprintf("%s:%s:%s", version, layer, hex.EncodeToString(hash[:]))
}

func blockKeys(version string, layer LayerName, layerJSON string) (map[string]string, error) {
	var blocks map[string]json.RawMessage
	if err := json.Unmarshal([]byte(layerJSON), &blocks); err != nil {
		return nil, err
	}
	result := make(map[string]string, len(blocks))
	for name, body := range blocks {
		result[name] = layerKey(version, LayerName(string(layer)+"."+name), string(body))
	}
	return result, nil
}

func stableJSON(value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func renderText(version string, layers SerializedLayers) string {
	var b strings.Builder
	b.WriteString("ContextPackVersion: ")
	b.WriteString(version)
	b.WriteString("\n\n")
	renderLayer(&b, "L0 System Cache Layer", layers.L0)
	renderLayer(&b, "L1 Project / Route Cache Layer", layers.L1)
	renderLayer(&b, "L2 Task Cache Layer", layers.L2)
	renderLayer(&b, "L3 Dynamic Step Layer", layers.L3)
	return b.String()
}

func renderLayer(b *strings.Builder, title string, body string) {
	b.WriteString("## ")
	b.WriteString(title)
	b.WriteString("\n")
	b.WriteString("```json\n")
	b.WriteString(body)
	b.WriteString("\n```\n\n")
}

func (s *Serializer) normalize(pack ContextPack) normalizedPack {
	return normalizedPack{
		L0: normalizedL0{
			RuntimeRules:   normalizeMessages(pack.RuntimeRules, false),
			SecurityRules:  normalizeMessages(pack.SecurityRules, false),
			ActivePersonas: normalizePersonas(pack.ActivePersonas),
		},
		L1: normalizedL1{
			ChannelCapabilities:        normalizeChannelCapabilities(pack.ChannelCapabilities),
			MetaFunctionSchemas:        normalizeFunctionSchemas(pack.MetaFunctionSchemas, false),
			ActiveCapabilities:         normalizeCapabilitySpecs(pack.ActiveCapabilities),
			ActiveFunctionSchemas:      normalizeFunctionSchemas(pack.ActiveFunctionSchemas, true),
			DeferredFunctionCandidates: normalizeFunctionCards(pack.DeferredFunctionCandidates),
			ActiveSkillInstructions:    normalizeSkillInstructions(pack.ActiveSkillInstructions),
			DeferredSkillCandidates:    normalizeSkillPackageRefs(pack.DeferredSkillCandidates),
		},
		L2: normalizedL2{
			IntentProfile:      normalizeIntentProfile(pack.IntentProfile),
			ProjectState:       normalizeProjectState(pack.ProjectState),
			TaskState:          normalizeTaskState(pack.TaskState),
			RelevantMemories:   normalizeMemoryItems(pack.RelevantMemories),
			AcceptanceCriteria: normalizeAcceptanceCriteria(pack.AcceptanceCriteria),
		},
		L3: normalizedL3{
			EvidenceRefs:     normalizeEvidenceRefs(pack.EvidenceRefs, s.maxEvidenceRefs),
			RecentMessages:   normalizeRecentMessages(pack.RecentMessages, s.maxRecentMessages),
			CurrentUserInput: pack.CurrentUserInput,
		},
	}
}

type normalizedPack struct {
	L0 normalizedL0 `json:"l0"`
	L1 normalizedL1 `json:"l1"`
	L2 normalizedL2 `json:"l2"`
	L3 normalizedL3 `json:"l3"`
}

type normalizedL0 struct {
	RuntimeRules   []stableMessage `json:"runtime_rules"`
	SecurityRules  []stableMessage `json:"security_rules"`
	ActivePersonas []Persona       `json:"active_personas"`
}

type normalizedL1 struct {
	ChannelCapabilities        []stableChannelCapability `json:"channel_capabilities"`
	MetaFunctionSchemas        []FunctionSchema          `json:"meta_function_schemas"`
	ActiveCapabilities         []CapabilitySpec          `json:"active_capabilities"`
	ActiveFunctionSchemas      []FunctionSchema          `json:"active_function_schemas"`
	DeferredFunctionCandidates []FunctionCard            `json:"deferred_function_candidates"`
	ActiveSkillInstructions    []SkillInstruction        `json:"active_skill_instructions"`
	DeferredSkillCandidates    []SkillPackageRef         `json:"deferred_skill_candidates"`
}

type normalizedL2 struct {
	IntentProfile      IntentProfile         `json:"intent_profile"`
	ProjectState       stableProjectState    `json:"project_state"`
	TaskState          stableTaskState       `json:"task_state"`
	RelevantMemories   []MemoryItem          `json:"relevant_memories"`
	AcceptanceCriteria []AcceptanceCriterion `json:"acceptance_criteria"`
}

type normalizedL3 struct {
	EvidenceRefs     []stableEvidenceRef `json:"evidence_refs"`
	RecentMessages   []stableMessage     `json:"recent_messages"`
	CurrentUserInput string              `json:"current_user_input"`
}

type stableMessage struct {
	ID          string `json:"id"`
	Role        string `json:"role"`
	Text        string `json:"text"`
	VersionHash string `json:"version_hash"`
	CreatedAt   string `json:"created_at,omitempty"`
}

type stableChannelCapability struct {
	ChannelType           string     `json:"channel_type"`
	Capabilities          []string   `json:"capabilities"`
	SupportedMessageTypes []string   `json:"supported_message_types"`
	SupportedInteractions []string   `json:"supported_interactions"`
	MaxMessageLength      int        `json:"max_message_length"`
	MaxButtons            int        `json:"max_buttons"`
	FileSizeLimit         int64      `json:"file_size_limit"`
	SupportsAsyncReply    bool       `json:"supports_async_reply"`
	SupportsSyncPrompt    bool       `json:"supports_sync_prompt"`
	SupportsConfirmation  bool       `json:"supports_confirmation"`
	SupportsFileRequest   bool       `json:"supports_file_request"`
	SupportsStreaming     bool       `json:"supports_streaming"`
	PolicyLimits          []keyValue `json:"policy_limits"`
}

type keyValue struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type stableProjectState struct {
	ID      string        `json:"id"`
	Name    string        `json:"name"`
	Goal    string        `json:"goal"`
	Status  string        `json:"status"`
	Summary string        `json:"summary"`
	Digest  ProjectDigest `json:"digest"`
}

type stableTaskState struct {
	Goal                string            `json:"goal"`
	Status              string            `json:"status"`
	AttemptCount        int               `json:"attempt_count"`
	ID                  string            `json:"id"`
	Title               string            `json:"title"`
	Description         string            `json:"description"`
	DependsOn           []TaskLink        `json:"depends_on"`
	Blocks              []TaskLink        `json:"blocks"`
	SiblingStatusCounts []StatusCount     `json:"sibling_status_counts"`
	RecentAttempts      []AttemptSummary  `json:"recent_attempts"`
	RecentObservations  []EvidenceSummary `json:"recent_observations"`
	RecentFailures      []string          `json:"recent_failures"`
	CacheVersion        string            `json:"cache_version"`
	FrozenRevision      string            `json:"frozen_revision"`
}

type stableEvidenceRef struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Summary     string `json:"summary"`
	ArtifactRef string `json:"artifact_ref"`
	CreatedAt   string `json:"created_at"`
}

func normalizeMessages(messages []Message, includeCreatedAt bool) []stableMessage {
	copied := append([]Message(nil), messages...)
	sort.SliceStable(copied, func(i, j int) bool {
		return compareStrings(
			copied[i].ID, copied[j].ID,
			copied[i].VersionHash, copied[j].VersionHash,
			copied[i].Role, copied[j].Role,
			copied[i].Text, copied[j].Text,
			formatOptionalTime(copied[i].CreatedAt), formatOptionalTime(copied[j].CreatedAt),
		)
	})
	result := make([]stableMessage, 0, len(copied))
	seen := map[string]struct{}{}
	for _, message := range copied {
		key := strings.Join([]string{message.ID, message.VersionHash, message.Role, message.Text, formatOptionalTime(message.CreatedAt)}, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		stable := stableMessage{
			ID:          message.ID,
			Role:        message.Role,
			Text:        message.Text,
			VersionHash: message.VersionHash,
		}
		if includeCreatedAt {
			stable.CreatedAt = formatOptionalTime(message.CreatedAt)
		}
		result = append(result, stable)
	}
	return result
}

func normalizeChannelCapabilities(capabilities []ChannelCapability) []stableChannelCapability {
	result := make([]stableChannelCapability, 0, len(capabilities))
	for _, capability := range capabilities {
		result = append(result, stableChannelCapability{
			ChannelType:           capability.ChannelType,
			Capabilities:          sortedUniqueStrings(capability.Capabilities),
			SupportedMessageTypes: sortedUniqueStrings(capability.SupportedMessageTypes),
			SupportedInteractions: sortedUniqueStrings(capability.SupportedInteractions),
			MaxMessageLength:      capability.MaxMessageLength,
			MaxButtons:            capability.MaxButtons,
			FileSizeLimit:         capability.FileSizeLimit,
			SupportsAsyncReply:    capability.SupportsAsyncReply,
			SupportsSyncPrompt:    capability.SupportsSyncPrompt,
			SupportsConfirmation:  capability.SupportsConfirmation,
			SupportsFileRequest:   capability.SupportsFileRequest,
			SupportsStreaming:     capability.SupportsStreaming,
			PolicyLimits:          sortedKeyValues(capability.PolicyLimits),
		})
	}
	sort.SliceStable(result, func(i, j int) bool {
		return compareStrings(result[i].ChannelType, result[j].ChannelType, strings.Join(result[i].Capabilities, ","), strings.Join(result[j].Capabilities, ","))
	})
	return uniqueBy(result, func(item stableChannelCapability) string {
		return item.ChannelType + "\x00" + strings.Join(item.Capabilities, "\x00")
	})
}

func normalizeIntentProfile(profile IntentProfile) IntentProfile {
	profile.Domains = sortedUniqueStrings(profile.Domains)
	profile.RequiredCapabilities = sortedUniqueStrings(profile.RequiredCapabilities)
	return profile
}

func normalizeProjectState(state ProjectState) stableProjectState {
	return stableProjectState{
		ID:      state.ID,
		Name:    state.Name,
		Goal:    state.Goal,
		Status:  state.Status,
		Summary: state.Summary,
		Digest:  normalizeProjectDigest(state.Digest),
	}
}

func normalizeTaskState(state TaskState) stableTaskState {
	return stableTaskState{
		Goal:                state.Goal,
		Status:              state.Status,
		AttemptCount:        state.AttemptCount,
		ID:                  state.ID,
		Title:               state.Title,
		Description:         state.Description,
		DependsOn:           normalizeTaskLinks(state.DependsOn),
		Blocks:              normalizeTaskLinks(state.Blocks),
		SiblingStatusCounts: normalizeStatusCounts(state.SiblingStatusCounts),
		RecentAttempts:      normalizeAttemptSummaries(state.RecentAttempts),
		RecentObservations:  normalizeEvidenceSummaries(state.RecentObservations),
		RecentFailures:      sortedUniqueStrings(state.RecentFailures),
		CacheVersion:        state.CacheVersion,
		FrozenRevision:      state.FrozenRevision,
	}
}

func normalizeFunctionSchemas(schemas []FunctionSchema, sortByVersion bool) []FunctionSchema {
	result := append([]FunctionSchema(nil), schemas...)
	for i := range result {
		result[i].Tags = sortedUniqueStrings(result[i].Tags)
		result[i].TaskTypes = sortedUniqueStrings(result[i].TaskTypes)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if sortByVersion {
			return compareStrings(result[i].Name, result[j].Name, result[i].VersionHash, result[j].VersionHash)
		}
		return compareStrings(result[i].Name, result[j].Name, result[i].SchemaRef, result[j].SchemaRef)
	})
	return uniqueBy(result, func(item FunctionSchema) string {
		return item.Name + "\x00" + item.VersionHash + "\x00" + item.SchemaRef
	})
}

func normalizeFunctionCards(cards []FunctionCard) []FunctionCard {
	result := append([]FunctionCard(nil), cards...)
	for i := range result {
		result[i].Tags = sortedUniqueStrings(result[i].Tags)
		result[i].TaskTypes = sortedUniqueStrings(result[i].TaskTypes)
		result[i].RequiredPermissions = sortedUniqueStrings(result[i].RequiredPermissions)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return compareStrings(result[i].Name, result[j].Name, result[i].SchemaRef, result[j].SchemaRef)
	})
	return uniqueBy(result, func(item FunctionCard) string {
		return item.Name + "\x00" + item.SchemaRef
	})
}

func normalizeCapabilitySpecs(specs []CapabilitySpec) []CapabilitySpec {
	result := append([]CapabilitySpec(nil), specs...)
	sort.SliceStable(result, func(i, j int) bool {
		return compareStrings(result[i].Name, result[j].Name, result[i].VersionHash, result[j].VersionHash)
	})
	return uniqueBy(result, func(item CapabilitySpec) string {
		return item.Name + "\x00" + item.VersionHash
	})
}

func normalizePersonas(personas []Persona) []Persona {
	result := append([]Persona(nil), personas...)
	sort.SliceStable(result, func(i, j int) bool {
		return compareStrings(result[i].ID, result[j].ID, result[i].VersionHash, result[j].VersionHash)
	})
	return uniqueBy(result, func(item Persona) string {
		return item.ID + "\x00" + item.VersionHash
	})
}

func normalizeSkillInstructions(skills []SkillInstruction) []SkillInstruction {
	result := append([]SkillInstruction(nil), skills...)
	for i := range result {
		result[i].AllowedTools = sortedUniqueStrings(result[i].AllowedTools)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return compareStrings(result[i].ID, result[j].ID, result[i].VersionHash, result[j].VersionHash)
	})
	return uniqueBy(result, func(item SkillInstruction) string {
		return item.ID + "\x00" + item.VersionHash
	})
}

func normalizeSkillPackageRefs(skills []SkillPackageRef) []SkillPackageRef {
	result := append([]SkillPackageRef(nil), skills...)
	sort.SliceStable(result, func(i, j int) bool {
		return compareStrings(result[i].ID, result[j].ID, result[i].VersionHash, result[j].VersionHash)
	})
	return uniqueBy(result, func(item SkillPackageRef) string {
		return item.ID + "\x00" + item.VersionHash
	})
}

func normalizeMemoryItems(memories []MemoryItem) []MemoryItem {
	result := append([]MemoryItem(nil), memories...)
	for i := range result {
		result[i].Tags = sortedUniqueStrings(result[i].Tags)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return compareStrings(result[i].ID, result[j].ID, result[i].VersionHash, result[j].VersionHash)
	})
	return uniqueBy(result, func(item MemoryItem) string {
		return item.ID + "\x00" + item.VersionHash
	})
}

func normalizeProjectDigest(digest ProjectDigest) ProjectDigest {
	digest.StatusCounts = normalizeStatusCounts(digest.StatusCounts)
	digest.CompletedTasks = normalizeTaskSummaries(digest.CompletedTasks)
	digest.ActiveTasks = normalizeTaskSummaries(digest.ActiveTasks)
	digest.ProblemTasks = normalizeTaskSummaries(digest.ProblemTasks)
	digest.RecentEvents = normalizeEventSummaries(digest.RecentEvents)
	digest.RecentEvidence = normalizeEvidenceSummaries(digest.RecentEvidence)
	digest.Decisions = normalizeDecisionRecords(digest.Decisions)
	return digest
}

func normalizeStatusCounts(counts []StatusCount) []StatusCount {
	result := append([]StatusCount(nil), counts...)
	sort.SliceStable(result, func(i, j int) bool {
		return compareStrings(result[i].Status, result[j].Status)
	})
	return uniqueBy(result, func(item StatusCount) string {
		return item.Status
	})
}

func normalizeTaskSummaries(tasks []TaskSummary) []TaskSummary {
	result := append([]TaskSummary(nil), tasks...)
	for i := range result {
		result[i].DependsOn = normalizeTaskLinks(result[i].DependsOn)
		result[i].Blocks = normalizeTaskLinks(result[i].Blocks)
	}
	sort.SliceStable(result, func(i, j int) bool {
		return compareStrings(result[i].Status, result[j].Status, result[i].Title, result[j].Title, result[i].ID, result[j].ID)
	})
	return uniqueBy(result, func(item TaskSummary) string {
		return item.ID
	})
}

func normalizeTaskLinks(links []TaskLink) []TaskLink {
	result := append([]TaskLink(nil), links...)
	sort.SliceStable(result, func(i, j int) bool {
		return compareStrings(result[i].Title, result[j].Title, result[i].ID, result[j].ID)
	})
	return uniqueBy(result, func(item TaskLink) string {
		return item.ID
	})
}

func normalizeEventSummaries(events []EventSummary) []EventSummary {
	result := append([]EventSummary(nil), events...)
	sort.SliceStable(result, func(i, j int) bool {
		return compareStrings(result[i].TaskTitle, result[j].TaskTitle, result[i].AttemptID, result[j].AttemptID, result[i].Type, result[j].Type, result[i].Message, result[j].Message)
	})
	return uniqueBy(result, func(item EventSummary) string {
		return strings.Join([]string{item.TaskID, item.AttemptID, item.Type, item.Message}, "\x00")
	})
}

func normalizeEvidenceSummaries(evidence []EvidenceSummary) []EvidenceSummary {
	result := append([]EvidenceSummary(nil), evidence...)
	sort.SliceStable(result, func(i, j int) bool {
		return compareStrings(result[i].TaskTitle, result[j].TaskTitle, result[i].Type, result[j].Type, result[i].ID, result[j].ID)
	})
	return uniqueBy(result, func(item EvidenceSummary) string {
		return item.ID + "\x00" + item.EvidenceRef
	})
}

func normalizeDecisionRecords(decisions []DecisionRecord) []DecisionRecord {
	result := append([]DecisionRecord(nil), decisions...)
	sort.SliceStable(result, func(i, j int) bool {
		return compareStrings(result[i].TaskTitle, result[j].TaskTitle, result[i].AttemptID, result[j].AttemptID, result[i].Status, result[j].Status)
	})
	return uniqueBy(result, func(item DecisionRecord) string {
		return strings.Join([]string{item.TaskID, item.AttemptID, item.Status, item.Summary}, "\x00")
	})
}

func normalizeAttemptSummaries(attempts []AttemptSummary) []AttemptSummary {
	result := append([]AttemptSummary(nil), attempts...)
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Number != result[j].Number {
			return result[i].Number < result[j].Number
		}
		return result[i].ID < result[j].ID
	})
	return uniqueBy(result, func(item AttemptSummary) string {
		return item.ID
	})
}

func normalizeAcceptanceCriteria(criteria []AcceptanceCriterion) []AcceptanceCriterion {
	result := append([]AcceptanceCriterion(nil), criteria...)
	sort.SliceStable(result, func(i, j int) bool {
		return compareStrings(result[i].ID, result[j].ID, result[i].Text, result[j].Text)
	})
	return uniqueBy(result, func(item AcceptanceCriterion) string {
		return item.ID + "\x00" + item.Text
	})
}

func normalizeEvidenceRefs(refs []EvidenceRef, maxItems int) []stableEvidenceRef {
	recent := append([]EvidenceRef(nil), refs...)
	sort.SliceStable(recent, func(i, j int) bool {
		if !recent[i].CreatedAt.Equal(recent[j].CreatedAt) {
			return recent[i].CreatedAt.After(recent[j].CreatedAt)
		}
		return recent[i].ID < recent[j].ID
	})
	if len(recent) > maxItems {
		recent = recent[:maxItems]
	}
	sort.SliceStable(recent, func(i, j int) bool {
		if !recent[i].CreatedAt.Equal(recent[j].CreatedAt) {
			return recent[i].CreatedAt.Before(recent[j].CreatedAt)
		}
		return recent[i].ID < recent[j].ID
	})
	result := make([]stableEvidenceRef, 0, len(recent))
	seen := map[string]struct{}{}
	for _, ref := range recent {
		key := ref.ID + "\x00" + formatOptionalTime(ref.CreatedAt)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, stableEvidenceRef{
			ID:          ref.ID,
			Type:        ref.Type,
			Summary:     ref.Summary,
			ArtifactRef: ref.ArtifactRef,
			CreatedAt:   formatOptionalTime(ref.CreatedAt),
		})
	}
	return result
}

func selectRecentMessages(messages []Message, maxItems int) []Message {
	recent := append([]Message(nil), messages...)
	sort.SliceStable(recent, func(i, j int) bool {
		if !recent[i].CreatedAt.Equal(recent[j].CreatedAt) {
			return recent[i].CreatedAt.After(recent[j].CreatedAt)
		}
		return recent[i].ID < recent[j].ID
	})
	if len(recent) > maxItems {
		recent = recent[:maxItems]
	}
	sort.SliceStable(recent, func(i, j int) bool {
		if !recent[i].CreatedAt.Equal(recent[j].CreatedAt) {
			return recent[i].CreatedAt.Before(recent[j].CreatedAt)
		}
		return recent[i].ID < recent[j].ID
	})
	return recent
}

func normalizeRecentMessages(messages []Message, maxItems int) []stableMessage {
	recent := selectRecentMessages(messages, maxItems)
	result := make([]stableMessage, 0, len(recent))
	seen := map[string]struct{}{}
	for _, message := range recent {
		key := strings.Join([]string{message.ID, message.Role, message.Text, formatOptionalTime(message.CreatedAt)}, "\x00")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, stableMessage{
			ID:        message.ID,
			Role:      message.Role,
			Text:      message.Text,
			CreatedAt: formatOptionalTime(message.CreatedAt),
		})
	}
	return result
}

func sortedUniqueStrings(values []string) []string {
	return textutil.SortedUniqueStrings(values)
}

func sortedKeyValues(values map[string]string) []keyValue {
	if len(values) == 0 {
		return []keyValue{}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]keyValue, 0, len(keys))
	for _, key := range keys {
		result = append(result, keyValue{Key: key, Value: values[key]})
	}
	return result
}

func uniqueBy[T any](items []T, keyFn func(T) string) []T {
	result := make([]T, 0, len(items))
	seen := map[string]struct{}{}
	for _, item := range items {
		key := keyFn(item)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, item)
	}
	if result == nil {
		return []T{}
	}
	return result
}

func compareStrings(values ...string) bool {
	for i := 0; i < len(values); i += 2 {
		if values[i] == values[i+1] {
			continue
		}
		return values[i] < values[i+1]
	}
	return false
}

func formatOptionalTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
