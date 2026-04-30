package handlers

import (
	"encoding/json"
	"sync"
	"sync/atomic"

	"github.com/google/uuid"
)

type SSEEvent struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type SSEHub struct {
	mu         sync.Mutex
	clients    map[string]map[chan SSEEvent]struct{}
	buffer     map[string][]SSEEvent
	bufferSize int
	seq        atomic.Uint64
}

func NewSSEHub(bufferSize int) *SSEHub {
	if bufferSize <= 0 {
		bufferSize = 50
	}
	return &SSEHub{
		clients:    make(map[string]map[chan SSEEvent]struct{}),
		buffer:     make(map[string][]SSEEvent),
		bufferSize: bufferSize,
	}
}

func (h *SSEHub) Subscribe(channelID string) (<-chan SSEEvent, func()) {
	ch := make(chan SSEEvent, 64)
	h.mu.Lock()
	if h.clients[channelID] == nil {
		h.clients[channelID] = make(map[chan SSEEvent]struct{})
	}
	h.clients[channelID][ch] = struct{}{}
	h.mu.Unlock()

	unsubscribe := func() {
		h.mu.Lock()
		delete(h.clients[channelID], ch)
		if len(h.clients[channelID]) == 0 {
			delete(h.clients, channelID)
		}
		h.mu.Unlock()
		close(ch)
	}
	return ch, unsubscribe
}

func (h *SSEHub) Publish(channelID string, event SSEEvent) {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}

	h.mu.Lock()
	h.buffer[channelID] = append(h.buffer[channelID], event)
	if len(h.buffer[channelID]) > h.bufferSize {
		h.buffer[channelID] = h.buffer[channelID][len(h.buffer[channelID])-h.bufferSize:]
	}
	clients := h.clients[channelID]
	h.mu.Unlock()

	for ch := range clients {
		select {
		case ch <- event:
		default:
			// slow consumer: close and remove
			go func(c chan SSEEvent) {
				h.mu.Lock()
				delete(h.clients[channelID], c)
				h.mu.Unlock()
				close(c)
			}(ch)
		}
	}
}

func (h *SSEHub) Replay(channelID string, afterEventID string) []SSEEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	buf := h.buffer[channelID]
	if afterEventID == "" {
		return append([]SSEEvent(nil), buf...)
	}
	for i, ev := range buf {
		if ev.ID == afterEventID {
			if i+1 < len(buf) {
				return append([]SSEEvent(nil), buf[i+1:]...)
			}
			return nil
		}
	}
	return append([]SSEEvent(nil), buf...)
}

func (h *SSEHub) ClientCount(channelID string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients[channelID])
}
