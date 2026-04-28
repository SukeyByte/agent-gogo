package communication

import "context"

type Adapter interface {
	Capability(ctx context.Context) (ChannelCapability, error)
	Deliver(ctx context.Context, message RenderedMessage) (DeliveryReceipt, error)
}
