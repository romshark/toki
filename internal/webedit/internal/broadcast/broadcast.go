package broadcast

import (
	"sync"
)

type Broadcast[MsgType comparable] struct {
	lock sync.Mutex
	subs map[chan Message[MsgType]]struct{}
}

type Subscription[MsgType comparable] struct {
	b *Broadcast[MsgType]
	c chan Message[MsgType]
}

func (s Subscription[MsgType]) C() <-chan Message[MsgType] { return s.c }

// Close closes and unsubscribes the subscription.
// No-op if already closed.
func (s Subscription[MsgType]) Close() {
	s.b.lock.Lock()
	defer s.b.lock.Unlock()
	if _, ok := s.b.subs[s.c]; ok {
		close(s.c)
		delete(s.b.subs, s.c)
	}
}

func New[MsgType comparable]() *Broadcast[MsgType] {
	return &Broadcast[MsgType]{
		subs: make(map[chan Message[MsgType]]struct{}),
	}
}

// Subscribe creates a new subscription.
func (b *Broadcast[MsgType]) Subscribe(bufferSize int) Subscription[MsgType] {
	b.lock.Lock()
	defer b.lock.Unlock()
	ch := make(chan Message[MsgType], bufferSize) // buffered
	b.subs[ch] = struct{}{}
	return Subscription[MsgType]{b: b, c: ch}
}

type Message[MsgType comparable] struct {
	Type    MsgType
	Payload any
}

// Broadcast sends msg to all subscribers (non-blocking).
func (b *Broadcast[MsgType]) Broadcast(typ MsgType, payload any) (notified int) {
	b.lock.Lock()
	defer b.lock.Unlock()
	for ch := range b.subs {
		select {
		case ch <- Message[MsgType]{Type: typ, Payload: payload}:
			notified++
		default: // drop if subscriber too slow
		}
	}
	return notified
}

// Close shuts down broadcaster.
func (b *Broadcast[MsgType]) Close() {
	b.lock.Lock()
	defer b.lock.Unlock()
	for ch := range b.subs {
		close(ch)
	}
	clear(b.subs)
}
