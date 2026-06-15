package h264

import (
	"context"
	"sync"
	"time"
)

// AccessUnit represents a complete H.264 access unit (one or more NALUs).
type AccessUnit struct {
	NALUs    []NALU
	Timestamp time.Time
	KeyFrame bool // True if contains IDR
}

// Subscriber receives access units from the hub.
type Subscriber struct {
	ID      string
	Channel chan AccessUnit
	cancel  context.CancelFunc
}

// AUHub fans out access units to multiple subscribers.
// Thread-safe via embedded mutex.
type AUHub struct {
	mu          sync.Mutex
	subscribers map[string]*Subscriber
	nextID      int
}

// NewAUHub creates a new access-unit fan-out hub.
func NewAUHub() *AUHub {
	return &AUHub{
		subscribers: make(map[string]*Subscriber),
	}
}

// Write adds an access unit to the hub for distribution.
// Non-blocking: drops AU to a subscriber if its channel buffer is full.
func (h *AUHub) Write(au AccessUnit) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for _, sub := range h.subscribers {
		select {
		case sub.Channel <- au:
		default:
			// Subscriber too slow — drop frame to avoid blocking the writer.
		}
	}
}

// Subscribe registers a new subscriber and returns it.
// The subscriber is automatically removed when ctx is cancelled.
func (h *AUHub) Subscribe(ctx context.Context) *Subscriber {
	h.mu.Lock()
	h.nextID++
	id := string(rune(h.nextID)) // simple unique ID
	ctx, cancel := context.WithCancel(ctx)

	sub := &Subscriber{
		ID:      id,
		Channel: make(chan AccessUnit, 64),
		cancel:  cancel,
	}
	h.subscribers[id] = sub
	h.mu.Unlock()

	go func() {
		defer h.Unsubscribe(id)
		<-ctx.Done()
	}()

	return sub
}

func (h *AUHub) Unsubscribe(id string) {
	h.mu.Lock()
	sub, ok := h.subscribers[id]
	if !ok {
		h.mu.Unlock()
		return
	}
	delete(h.subscribers, id)
	close(sub.Channel)
	h.mu.Unlock()
	sub.cancel()
}

// SubscriberCount returns the current number of active subscribers.
func (h *AUHub) SubscriberCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subscribers)
}
