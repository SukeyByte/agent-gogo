package communication

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/google/uuid"
)

type CLIAdapter struct {
	channelID string
	writer    io.Writer
}

func NewCLIAdapter(channelID string, writer io.Writer) *CLIAdapter {
	return &CLIAdapter{channelID: channelID, writer: writer}
}

func (a *CLIAdapter) Capability(ctx context.Context) (ChannelCapability, error) {
	if err := ctx.Err(); err != nil {
		return ChannelCapability{}, err
	}
	return ChannelCapability{
		ChannelType:           "cli",
		SupportedMessageTypes: []string{"text"},
		SupportedInteractions: []string{"prompt", "confirm_yes_no"},
		MaxMessageLength:      12000,
		MaxButtons:            2,
		SupportsAsyncReply:    false,
		SupportsSyncPrompt:    true,
		SupportsConfirmation:  true,
		SupportsFileRequest:   false,
		SupportsStreaming:     false,
		PolicyLimits:          map[string]string{},
	}, nil
}

func (a *CLIAdapter) Deliver(ctx context.Context, message RenderedMessage) (DeliveryReceipt, error) {
	if err := ctx.Err(); err != nil {
		return DeliveryReceipt{}, err
	}
	if a.writer != nil {
		switch message.Type {
		case IntentAskConfirmation:
			_, _ = fmt.Fprintf(a.writer, "%s\n", message.Text)
			for _, button := range message.Buttons {
				_, _ = fmt.Fprintf(a.writer, "[%s] %s\n", button.Value, button.Label)
			}
		default:
			_, _ = fmt.Fprintln(a.writer, message.Text)
		}
	}
	return DeliveryReceipt{
		ChannelID:   a.channelID,
		Status:      DeliveryDelivered,
		MessageID:   uuid.NewString(),
		DeliveredAt: time.Now().UTC(),
	}, nil
}

type WebConsoleAdapter struct {
	channelID string
	mu        sync.Mutex
	messages  []RenderedMessage
}

func NewWebConsoleAdapter(channelID string) *WebConsoleAdapter {
	return &WebConsoleAdapter{channelID: channelID}
}

func (a *WebConsoleAdapter) Capability(ctx context.Context) (ChannelCapability, error) {
	if err := ctx.Err(); err != nil {
		return ChannelCapability{}, err
	}
	return ChannelCapability{
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

func (a *WebConsoleAdapter) Deliver(ctx context.Context, message RenderedMessage) (DeliveryReceipt, error) {
	if err := ctx.Err(); err != nil {
		return DeliveryReceipt{}, err
	}
	a.mu.Lock()
	a.messages = append(a.messages, message)
	a.mu.Unlock()
	return DeliveryReceipt{
		ChannelID:   a.channelID,
		Status:      DeliveryDelivered,
		MessageID:   uuid.NewString(),
		DeliveredAt: time.Now().UTC(),
	}, nil
}

func (a *WebConsoleAdapter) Messages() []RenderedMessage {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]RenderedMessage(nil), a.messages...)
}
