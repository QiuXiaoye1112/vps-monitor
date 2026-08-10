package realtime

import (
	"sync"
	"sync/atomic"
)

const (
	KindStatus   = "status"
	KindHistory  = "history"
	KindPing     = "ping"
	KindMetadata = "metadata"
)

// Event identifies the part of the monitor data that changed. The payload is
// intentionally small: the browser fetches only the affected node/record
// after receiving the event.
type Event struct {
	Kind   string `json:"kind"`
	UUID   string `json:"uuid"`
	TaskID uint   `json:"task_id,omitempty"`
	Time   string `json:"time,omitempty"`
	Value  int    `json:"value"`
}

type Hub struct {
	mu          sync.Mutex
	nextID      uint64
	subscribers map[uint64]chan Event
}

func NewHub() *Hub {
	return &Hub{subscribers: make(map[uint64]chan Event)}
}

func (h *Hub) Subscribe() (<-chan Event, func()) {
	// A bounded channel prevents a slow browser from blocking Agent ingest.
	// Status/history events are safe to coalesce at the consumer because the
	// follow-up request always reads the latest value.
	ch := make(chan Event, 128)
	id := atomic.AddUint64(&h.nextID, 1)

	h.mu.Lock()
	h.subscribers[id] = ch
	h.mu.Unlock()

	var once sync.Once
	return ch, func() {
		once.Do(func() {
			h.mu.Lock()
			delete(h.subscribers, id)
			close(ch)
			h.mu.Unlock()
		})
	}
}

func (h *Hub) Publish(event Event) {
	if event.Kind == "" || event.UUID == "" {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()
	for _, ch := range h.subscribers {
		select {
		case ch <- event:
		default:
			// Never block Agent ingestion for a slow browser. The next event
			// causes the browser to request the current value again.
		}
	}
}

var DefaultHub = NewHub()

func Publish(event Event) {
	DefaultHub.Publish(event)
}

func Subscribe() (<-chan Event, func()) {
	return DefaultHub.Subscribe()
}
