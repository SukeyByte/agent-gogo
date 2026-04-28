package communication

import "time"

type IntentType string

const (
	IntentSendMessage     IntentType = "send_message"
	IntentSendProgress    IntentType = "send_progress"
	IntentSendArtifact    IntentType = "send_artifact"
	IntentAskUser         IntentType = "ask_user"
	IntentAskConfirmation IntentType = "ask_confirmation"
	IntentSendOptions     IntentType = "send_options"
	IntentRequestAttach   IntentType = "request_attachment"
	IntentNotifyDone      IntentType = "notify_done"
	IntentNotifyBlocked   IntentType = "notify_blocked"
)

type RiskLevel string

const (
	RiskLow      RiskLevel = "low"
	RiskMedium   RiskLevel = "medium"
	RiskHigh     RiskLevel = "high"
	RiskCritical RiskLevel = "critical"
)

type ChannelEvent struct {
	ID          string
	ChannelID   string
	ChannelType string
	UserID      string
	SessionID   string
	ProjectID   string
	Type        string
	Message     string
	Payload     map[string]any
	CreatedAt   time.Time
}

type ChannelCapability struct {
	ChannelType           string
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

type CommunicationIntent struct {
	ID        string
	ChannelID string
	SessionID string
	ProjectID string
	Type      IntentType
	Payload   map[string]any
	RiskLevel RiskLevel
	CreatedAt time.Time
}

type RenderedMessage struct {
	ChannelID   string
	ChannelType string
	Type        IntentType
	Text        string
	Buttons     []Button
	Payload     map[string]any
}

type Button struct {
	ID    string
	Label string
	Value string
}

type DeliveryStatus string

const (
	DeliveryPending   DeliveryStatus = "PENDING"
	DeliveryDelivered DeliveryStatus = "DELIVERED"
	DeliveryFailed    DeliveryStatus = "FAILED"
)

type DeliveryReceipt struct {
	IntentID    string
	ChannelID   string
	Status      DeliveryStatus
	MessageID   string
	Error       string
	DeliveredAt time.Time
}

type OutboxRecord struct {
	Intent    CommunicationIntent
	Rendered  RenderedMessage
	Receipt   DeliveryReceipt
	Status    DeliveryStatus
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}
