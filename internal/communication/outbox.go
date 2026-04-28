package communication

import (
	"context"
	"sync"
	"time"
)

type Outbox interface {
	Add(ctx context.Context, intent CommunicationIntent) (OutboxRecord, error)
	MarkRendered(ctx context.Context, intentID string, rendered RenderedMessage) error
	MarkDelivered(ctx context.Context, intentID string, receipt DeliveryReceipt) error
	MarkFailed(ctx context.Context, intentID string, err error) error
	List(ctx context.Context) ([]OutboxRecord, error)
}

type MemoryOutbox struct {
	mu      sync.Mutex
	records map[string]OutboxRecord
	order   []string
}

func NewMemoryOutbox() *MemoryOutbox {
	return &MemoryOutbox{
		records: map[string]OutboxRecord{},
		order:   []string{},
	}
}

func (o *MemoryOutbox) Add(ctx context.Context, intent CommunicationIntent) (OutboxRecord, error) {
	if err := ctx.Err(); err != nil {
		return OutboxRecord{}, err
	}
	o.mu.Lock()
	defer o.mu.Unlock()

	now := time.Now().UTC()
	record := OutboxRecord{
		Intent:    intent,
		Status:    DeliveryPending,
		CreatedAt: now,
		UpdatedAt: now,
	}
	o.records[intent.ID] = record
	o.order = append(o.order, intent.ID)
	return record, nil
}

func (o *MemoryOutbox) MarkRendered(ctx context.Context, intentID string, rendered RenderedMessage) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	record := o.records[intentID]
	record.Rendered = rendered
	record.UpdatedAt = time.Now().UTC()
	o.records[intentID] = record
	return nil
}

func (o *MemoryOutbox) MarkDelivered(ctx context.Context, intentID string, receipt DeliveryReceipt) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	record := o.records[intentID]
	record.Receipt = receipt
	record.Status = DeliveryDelivered
	record.UpdatedAt = time.Now().UTC()
	o.records[intentID] = record
	return nil
}

func (o *MemoryOutbox) MarkFailed(ctx context.Context, intentID string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	record := o.records[intentID]
	record.Status = DeliveryFailed
	if err != nil {
		record.Error = err.Error()
	}
	record.UpdatedAt = time.Now().UTC()
	o.records[intentID] = record
	return nil
}

func (o *MemoryOutbox) List(ctx context.Context) ([]OutboxRecord, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	records := make([]OutboxRecord, 0, len(o.order))
	for _, id := range o.order {
		records = append(records, o.records[id])
	}
	return records, nil
}
