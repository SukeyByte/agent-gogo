package contextbuilder

import (
	"context"
	"time"
)

const DefaultVersion = "contextpack.v1"

type LayerName string

const (
	LayerL0 LayerName = "L0"
	LayerL1 LayerName = "L1"
	LayerL2 LayerName = "L2"
	LayerL3 LayerName = "L3"
)

type ContextSerializer interface {
	Serialize(ctx context.Context, pack ContextPack) (*SerializedContext, error)
}

type ContextPack struct {
	RuntimeRules               []Message
	SecurityRules              []Message
	ChannelCapabilities        []ChannelCapability
	IntentProfile              IntentProfile
	MetaFunctionSchemas        []FunctionSchema
	ActiveFunctionSchemas      []FunctionSchema
	DeferredFunctionCandidates []FunctionCard
	ActiveCapabilities         []CapabilitySpec
	ActivePersonas             []Persona
	ActiveSkillInstructions    []SkillInstruction
	DeferredSkillCandidates    []SkillPackageRef
	RelevantMemories           []MemoryItem
	ProjectState               ProjectState
	TaskState                  TaskState
	AcceptanceCriteria         []AcceptanceCriterion
	EvidenceRefs               []EvidenceRef
	RecentMessages             []Message
	CurrentUserInput           string
}

type Message struct {
	ID          string
	Role        string
	Text        string
	VersionHash string
	CreatedAt   time.Time
}

type ChannelCapability struct {
	ChannelType           string
	Capabilities          []string
	SupportedMessageTypes []string
	SupportedInteractions []string
	MaxMessageLength      int
	MaxButtons            int
	FileSizeLimit         int64
	SupportsAsyncReply    bool
	SupportsSyncPrompt    bool
	SupportsConfirmation  bool
	SupportsFileRequest   bool
	SupportsStreaming     bool
	PolicyLimits          map[string]string
}

type IntentProfile struct {
	TaskType              string
	Complexity            string
	Domains               []string
	RequiredCapabilities  []string
	RiskLevel             string
	NeedsUserConfirmation bool
	GroundingRequirement  string
	Confidence            float64
}

type FunctionSchema struct {
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

type FunctionCard struct {
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

type CapabilitySpec struct {
	Name        string
	Description string
	RiskLevel   string
	VersionHash string
}

type Persona struct {
	ID           string
	Name         string
	VersionHash  string
	Instructions string
}

type SkillInstruction struct {
	ID           string
	Name         string
	VersionHash  string
	Instructions string
	AllowedTools []string
}

type SkillPackageRef struct {
	ID          string
	Name        string
	VersionHash string
	Path        string
	Reason      string
}

type MemoryItem struct {
	ID              string
	Scope           string
	Type            string
	Tags            []string
	VersionHash     string
	Summary         string
	ArtifactRef     string
	EvidenceRef     string
	SourceTaskID    string
	SourceAttemptID string
	Confidence      float64
}

type ProjectState struct {
	ID      string
	Name    string
	Goal    string
	Status  string
	Summary string
	Digest  ProjectDigest
}

type TaskState struct {
	ID                  string
	Title               string
	Goal                string
	Status              string
	Description         string
	AttemptCount        int
	DependsOn           []TaskLink
	Blocks              []TaskLink
	SiblingStatusCounts []StatusCount
	RecentAttempts      []AttemptSummary
	RecentObservations  []EvidenceSummary
	RecentFailures      []string
	CacheVersion        string
	FrozenRevision      string
}

type ProjectDigest struct {
	TaskCount      int
	StatusCounts   []StatusCount
	CompletedTasks []TaskSummary
	ActiveTasks    []TaskSummary
	ProblemTasks   []TaskSummary
	RecentEvents   []EventSummary
	RecentEvidence []EvidenceSummary
	Decisions      []DecisionRecord
}

type StatusCount struct {
	Status string
	Count  int
}

type TaskSummary struct {
	ID                string
	Title             string
	Status            string
	Description       string
	AttemptCount      int
	DependsOn         []TaskLink
	Blocks            []TaskLink
	LatestObservation string
	LatestEvidenceRef string
	LatestFailure     string
}

type TaskLink struct {
	ID     string
	Title  string
	Status string
}

type EventSummary struct {
	TaskID    string
	TaskTitle string
	AttemptID string
	Type      string
	FromState string
	ToState   string
	Message   string
}

type EvidenceSummary struct {
	ID          string
	TaskID      string
	TaskTitle   string
	Type        string
	Summary     string
	EvidenceRef string
}

type DecisionRecord struct {
	TaskID      string
	TaskTitle   string
	AttemptID   string
	Status      string
	Summary     string
	EvidenceRef string
}

type AttemptSummary struct {
	ID     string
	Number int
	Status string
	Error  string
}

type AcceptanceCriterion struct {
	ID   string
	Text string
}

type EvidenceRef struct {
	ID          string
	Type        string
	Summary     string
	ArtifactRef string
	CreatedAt   time.Time
}

type SerializedContext struct {
	Text      string
	LayerKeys ContextLayerKeys
	BlockKeys ContextBlockKeys
	Layers    SerializedLayers
	Version   string
}

type SerializedLayers struct {
	L0 string
	L1 string
	L2 string
	L3 string
}

type ContextLayerKeys struct {
	L0 string
	L1 string
	L2 string
	L3 string
}

type ContextBlockKeys struct {
	L0 map[string]string
	L1 map[string]string
	L2 map[string]string
	L3 map[string]string
}
