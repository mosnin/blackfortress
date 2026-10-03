package runtime

import (
	"encoding/json"
	"sync"
	"time"
)

// Event is pushed to SSE subscribers of /v1/events.
type Event struct {
	Type string    `json:"type"`
	Time time.Time `json:"time"`
	Data any       `json:"data"`
}

// Hub fans events out to subscribers. Slow subscribers drop events rather
// than block publishers.
type Hub struct {
	mu   sync.Mutex
	subs map[chan []byte]struct{}
}

func NewHub() *Hub { return &Hub{subs: map[chan []byte]struct{}{}} }

func (h *Hub) Subscribe() (<-chan []byte, func()) {
	ch := make(chan []byte, 64)

	h.mu.Lock()
	h.subs[ch] = struct{}{}
	h.mu.Unlock()

	return ch, func() {
		h.mu.Lock()
		if _, ok := h.subs[ch]; ok {
			delete(h.subs, ch)
			close(ch)
		}
		h.mu.Unlock()
	}
}

func (h *Hub) Publish(typ string, data any) {
	payload, err := json.Marshal(Event{Type: typ, Time: time.Now().UTC(), Data: data})
	if err != nil {
		return
	}

	frame := []byte("event: " + typ + "\ndata: " + string(payload) + "\n\n")

	h.mu.Lock()
	defer h.mu.Unlock()

	for ch := range h.subs {
		select {
		case ch <- frame:
		default:
		}
	}
}
