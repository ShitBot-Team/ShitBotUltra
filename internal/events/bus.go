package events

import (
	"encoding/json"
	"sync"
	"time"
)

type Event struct {
	ID   uint64          `json:"id"`
	Type string          `json:"type"`
	Time time.Time       `json:"time"`
	Data json.RawMessage `json:"data"`
}

// Bus only carries management-safe summaries, never tokens, chat or bind codes.
// Slow subscribers are disconnected and resume from the bounded replay window.
type Bus struct {
	mu      sync.Mutex
	next    uint64
	history []Event
	subs    map[chan Event]struct{}
	closed  bool
}

func New() *Bus { return &Bus{subs: map[chan Event]struct{}{}} }

func (b *Bus) Publish(kind string, data any) {
	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.next++
	e := Event{ID: b.next, Type: kind, Time: time.Now().UTC(), Data: payload}
	b.history = append(b.history, e)
	if len(b.history) > 512 {
		b.history = append([]Event(nil), b.history[len(b.history)-512:]...)
	}
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
			close(ch)
			delete(b.subs, ch)
		}
	}
}

func (b *Bus) Subscribe(after uint64) ([]Event, <-chan Event, func(), bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed || len(b.subs) >= 128 {
		return nil, nil, func() {}, false
	}
	ch := make(chan Event, 64)
	b.subs[ch] = struct{}{}
	backlog := []Event{}
	if after > b.next || (after != 0 && len(b.history) > 0 && after < b.history[0].ID-1) {
		backlog = append(backlog, Event{ID: b.next, Type: "reset", Time: time.Now().UTC(), Data: json.RawMessage(`{"reason":"replay_window_expired","refresh":true}`)})
	} else {
		for _, e := range b.history {
			if e.ID > after {
				backlog = append(backlog, e)
			}
		}
	}
	cancel := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
	return backlog, ch, cancel, true
}

func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	for ch := range b.subs {
		close(ch)
		delete(b.subs, ch)
	}
}
