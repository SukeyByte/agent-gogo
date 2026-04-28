package communication

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrUnsupportedIntent       = errors.New("unsupported communication intent")
	ErrConfirmationUnsupported = errors.New("channel does not support confirmation")
)

type Renderer struct{}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) Render(intent CommunicationIntent, capability ChannelCapability) (RenderedMessage, error) {
	switch intent.Type {
	case IntentSendMessage, IntentSendProgress, IntentNotifyDone, IntentNotifyBlocked:
		return RenderedMessage{
			ChannelID:   intent.ChannelID,
			ChannelType: capability.ChannelType,
			Type:        intent.Type,
			Text:        textPayload(intent.Payload),
			Payload:     copyPayload(intent.Payload),
		}, nil
	case IntentAskConfirmation:
		if !capability.SupportsConfirmation {
			return RenderedMessage{}, ErrConfirmationUnsupported
		}
		title := stringPayload(intent.Payload, "title")
		body := stringPayload(intent.Payload, "body")
		if title == "" {
			title = "Confirmation required"
		}
		text := strings.TrimSpace(title + "\n" + body)
		return RenderedMessage{
			ChannelID:   intent.ChannelID,
			ChannelType: capability.ChannelType,
			Type:        intent.Type,
			Text:        text,
			Buttons: []Button{
				{ID: "confirm", Label: labelPayload(intent.Payload, "confirm_label", "Approve"), Value: "confirm"},
				{ID: "reject", Label: labelPayload(intent.Payload, "reject_label", "Reject"), Value: "reject"},
			},
			Payload: copyPayload(intent.Payload),
		}, nil
	default:
		return RenderedMessage{}, fmt.Errorf("%w: %s", ErrUnsupportedIntent, intent.Type)
	}
}

func textPayload(payload map[string]any) string {
	text := stringPayload(payload, "text")
	if text != "" {
		return text
	}
	return stringPayload(payload, "message")
}

func labelPayload(payload map[string]any, key string, fallback string) string {
	value := stringPayload(payload, key)
	if value == "" {
		return fallback
	}
	return value
}

func stringPayload(payload map[string]any, key string) string {
	value, ok := payload[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}
