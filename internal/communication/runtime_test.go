package communication

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRuntimeDispatchesCLIMessageAndRecordsOutbox(t *testing.T) {
	ctx := context.Background()
	var output bytes.Buffer
	outbox := NewMemoryOutbox()
	runtime := NewRuntime(outbox, NewRenderer())
	runtime.RegisterChannel("cli", NewCLIAdapter("cli", &output))

	receipt, err := runtime.Dispatch(ctx, NewMessageIntent("cli", "hello from runtime"))
	if err != nil {
		t.Fatalf("dispatch message: %v", err)
	}
	if receipt.Status != DeliveryDelivered {
		t.Fatalf("expected delivered receipt, got %s", receipt.Status)
	}
	if !strings.Contains(output.String(), "hello from runtime") {
		t.Fatalf("expected CLI output, got %q", output.String())
	}

	records, err := outbox.List(ctx)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one outbox record, got %d", len(records))
	}
	if records[0].Status != DeliveryDelivered {
		t.Fatalf("expected delivered outbox record, got %s", records[0].Status)
	}
	if records[0].Rendered.ChannelType != "cli" {
		t.Fatalf("expected CLI rendered message, got %s", records[0].Rendered.ChannelType)
	}
}

func TestAskConfirmationRespectsChannelCapability(t *testing.T) {
	ctx := context.Background()
	outbox := NewMemoryOutbox()
	runtime := NewRuntime(outbox, NewRenderer())
	runtime.RegisterChannel("limited", unsupportedConfirmationAdapter{})

	intent := NewConfirmationIntent("limited", "Dangerous action", "Proceed?", RiskHigh)
	if _, err := runtime.Dispatch(ctx, intent); !errors.Is(err, ErrConfirmationUnsupported) {
		t.Fatalf("expected confirmation unsupported error, got %v", err)
	}

	records, err := outbox.List(ctx)
	if err != nil {
		t.Fatalf("list outbox: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected one failed outbox record, got %d", len(records))
	}
	if records[0].Status != DeliveryFailed {
		t.Fatalf("expected failed outbox record, got %s", records[0].Status)
	}
	if records[0].Intent.RiskLevel != RiskHigh {
		t.Fatalf("expected high risk intent, got %s", records[0].Intent.RiskLevel)
	}
}

func TestWebConsoleAdapterStoresRenderedConfirmation(t *testing.T) {
	ctx := context.Background()
	web := NewWebConsoleAdapter("web")
	runtime := NewRuntime(NewMemoryOutbox(), NewRenderer())
	runtime.RegisterChannel("web", web)

	intent := NewConfirmationIntent("web", "Approve deploy", "Ship this change?", RiskHigh)
	if _, err := runtime.Dispatch(ctx, intent); err != nil {
		t.Fatalf("dispatch confirmation: %v", err)
	}

	messages := web.Messages()
	if len(messages) != 1 {
		t.Fatalf("expected one web message, got %d", len(messages))
	}
	if messages[0].Type != IntentAskConfirmation {
		t.Fatalf("expected ask_confirmation, got %s", messages[0].Type)
	}
	if len(messages[0].Buttons) != 2 {
		t.Fatalf("expected two confirmation buttons, got %d", len(messages[0].Buttons))
	}
}

type unsupportedConfirmationAdapter struct{}

func (a unsupportedConfirmationAdapter) Capability(ctx context.Context) (ChannelCapability, error) {
	if err := ctx.Err(); err != nil {
		return ChannelCapability{}, err
	}
	return ChannelCapability{
		ChannelType:           "limited",
		SupportedMessageTypes: []string{"text"},
		SupportsConfirmation:  false,
	}, nil
}

func (a unsupportedConfirmationAdapter) Deliver(ctx context.Context, message RenderedMessage) (DeliveryReceipt, error) {
	if err := ctx.Err(); err != nil {
		return DeliveryReceipt{}, err
	}
	return DeliveryReceipt{ChannelID: "limited", Status: DeliveryDelivered}, nil
}
