package provider

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestTimeoutProviderCancelsSlowChat(t *testing.T) {
	llm := ChatFunc(func(ctx context.Context, req ChatRequest) (ChatResponse, error) {
		<-ctx.Done()
		return ChatResponse{}, ctx.Err()
	})
	wrapped := NewTimeoutProvider(llm, 5*time.Millisecond)
	_, err := wrapped.Chat(context.Background(), ChatRequest{Model: "test"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline exceeded, got %v", err)
	}
}
