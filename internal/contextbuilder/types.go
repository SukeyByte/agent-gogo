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
	ID          string
	Scope       string
	VersionHash string
	Summary     string
	ArtifactRef string
}

type ProjectState struct {
	ID      string
	Name    string
	Goal    string
	Status  string
	Summary string
}

type TaskState struct {
	ID             string
	Goal           string
	Status         string
	AttemptCount   int
	CacheVersion   string
	FrozenRevision string
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
