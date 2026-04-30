package handlers

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/sukeke/agent-gogo/internal/communication"
)

type WebConsoleAdapter struct {
	channelID string
	hub       *SSEHub
}

func NewWebConsoleAdapter(channelID string, hub *SSEHub) *WebConsoleAdapter {
	return &WebConsoleAdapter{channelID: channelID, hub: hub}
}

func (a *WebConsoleAdapter) Capability(ctx context.Context) (communication.ChannelCapability, error) {
	if err := ctx.Err(); err != nil {
		return communication.ChannelCapability{}, err
	}
	return communication.ChannelCapability{
		ChannelType:           "web",
		SupportedMessageTypes: []string{"text", "task_card", "artifact"},
		SupportedInteractions: []string{"modal", "button"},
		MaxMessageLength:      32000,
		MaxButtons:            8,
		SupportsAsyncReply:    true,
		SupportsSyncPrompt:    false,
		SupportsConfirmation:  true,
		SupportsFileRequest:   true,
		SupportsStreaming:     true,
		PolicyLimits:          map[string]string{},
	}, nil
}

func (a *WebConsoleAdapter) Deliver(ctx context.Context, message communication.RenderedMessage) (communication.DeliveryReceipt, error) {
	if err := ctx.Err(); err != nil {
		return communication.DeliveryReceipt{}, err
	}

	data, err := json.Marshal(map[string]any{
		"channel_id":   message.ChannelID,
		"channel_type": message.ChannelType,
		"type":         string(message.Type),
		"text":         message.Text,
		"buttons":      message.Buttons,
		"payload":      message.Payload,
	})
	if err != nil {
		return communication.DeliveryReceipt{}, err
	}

	eventType := "message"
	switch message.Type {
	case communication.IntentAskConfirmation:
		eventType = "confirmation"
	case communication.IntentNotifyDone:
		eventType = "done"
	case communication.IntentNotifyBlocked:
		eventType = "blocked"
	case communication.IntentSendProgress:
		eventType = "progress"
	case communication.IntentSendArtifact:
		eventType = "artifact"
	}

	a.hub.Publish(a.channelID, SSEEvent{
		Type: eventType,
		Data: data,
	})

	return communication.DeliveryReceipt{
		ChannelID:   a.channelID,
		Status:      communication.DeliveryDelivered,
		MessageID:   uuid.NewString(),
		DeliveredAt: time.Now().UTC(),
	}, nil
}
