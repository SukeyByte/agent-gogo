package communication

import (
	"time"

	"github.com/google/uuid"
)

func NewIntent(channelID string, typ IntentType, payload map[string]any) CommunicationIntent {
	return CommunicationIntent{
		ID:        uuid.NewString(),
		ChannelID: channelID,
		Type:      typ,
		Payload:   copyPayload(payload),
		RiskLevel: RiskLow,
		CreatedAt: time.Now().UTC(),
	}
}

func NewMessageIntent(channelID string, text string) CommunicationIntent {
	return NewIntent(channelID, IntentSendMessage, map[string]any{"text": text})
}

func NewProgressIntent(channelID string, text string, payload map[string]any) CommunicationIntent {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["text"] = text
	return NewIntent(channelID, IntentSendProgress, payload)
}

func NewDoneIntent(channelID string, text string) CommunicationIntent {
	return NewIntent(channelID, IntentNotifyDone, map[string]any{"text": text})
}

func NewBlockedIntent(channelID string, text string, payload map[string]any) CommunicationIntent {
	if payload == nil {
		payload = map[string]any{}
	}
	payload["text"] = text
	return NewIntent(channelID, IntentNotifyBlocked, payload)
}

func NewConfirmationIntent(channelID string, title string, body string, risk RiskLevel) CommunicationIntent {
	intent := NewIntent(channelID, IntentAskConfirmation, map[string]any{
		"title":         title,
		"body":          body,
		"confirm_label": "Approve",
		"reject_label":  "Reject",
	})
	intent.RiskLevel = risk
	return intent
}

func copyPayload(payload map[string]any) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	result := make(map[string]any, len(payload))
	for key, value := range payload {
		result[key] = value
	}
	return result
}
