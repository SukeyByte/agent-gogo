package communication

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

var ErrChannelNotFound = errors.New("communication channel not found")

type Runtime struct {
	renderer *Renderer
	outbox   Outbox
	adapters map[string]Adapter
}

func NewRuntime(outbox Outbox, renderer *Renderer) *Runtime {
	if outbox == nil {
		outbox = NewMemoryOutbox()
	}
	if renderer == nil {
		renderer = NewRenderer()
	}
	return &Runtime{
		renderer: renderer,
		outbox:   outbox,
		adapters: map[string]Adapter{},
	}
}

func (r *Runtime) RegisterChannel(channelID string, adapter Adapter) {
	r.adapters[channelID] = adapter
}

func (r *Runtime) Dispatch(ctx context.Context, intent CommunicationIntent) (DeliveryReceipt, error) {
	if err := ctx.Err(); err != nil {
		return DeliveryReceipt{}, err
	}
	if intent.ID == "" {
		intent.ID = uuid.NewString()
	}
	if intent.Payload == nil {
		intent.Payload = map[string]any{}
	}
	if intent.RiskLevel == "" {
		intent.RiskLevel = RiskLow
	}
	if intent.CreatedAt.IsZero() {
		intent.CreatedAt = nowUTC()
	}

	if _, err := r.outbox.Add(ctx, intent); err != nil {
		return DeliveryReceipt{}, err
	}
	adapter, ok := r.adapters[intent.ChannelID]
	if !ok {
		err := ErrChannelNotFound
		_ = r.outbox.MarkFailed(ctx, intent.ID, err)
		return DeliveryReceipt{}, err
	}
	capability, err := adapter.Capability(ctx)
	if err != nil {
		_ = r.outbox.MarkFailed(ctx, intent.ID, err)
		return DeliveryReceipt{}, err
	}
	rendered, err := r.renderer.Render(intent, capability)
	if err != nil {
		_ = r.outbox.MarkFailed(ctx, intent.ID, err)
		return DeliveryReceipt{}, err
	}
	if err := r.outbox.MarkRendered(ctx, intent.ID, rendered); err != nil {
		return DeliveryReceipt{}, err
	}
	receipt, err := adapter.Deliver(ctx, rendered)
	if err != nil {
		_ = r.outbox.MarkFailed(ctx, intent.ID, err)
		return DeliveryReceipt{}, err
	}
	receipt.IntentID = intent.ID
	if receipt.Status == "" {
		receipt.Status = DeliveryDelivered
	}
	if err := r.outbox.MarkDelivered(ctx, intent.ID, receipt); err != nil {
		return DeliveryReceipt{}, err
	}
	return receipt, nil
}

func (r *Runtime) Outbox() Outbox {
	return r.outbox
}

func nowUTC() time.Time {
	return time.Now().UTC()
}
