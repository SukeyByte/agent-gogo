package observability

import "context"

type Logger interface {
	Log(ctx context.Context, stage string, payload any) error
}

type NoopLogger struct{}

func (NoopLogger) Log(ctx context.Context, stage string, payload any) error {
	return ctx.Err()
}
