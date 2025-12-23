package broadcast

import (
	"sync"
)

type Broadcast[StreamID, MsgType comparable] struct {
	lock    sync.Mutex
	streams map[StreamID]map[chan Message[MsgType]]struct{}
}

type Subscription[StreamID, MsgType comparable] struct {
	b        *Broadcast[StreamID, MsgType]
	c        chan Message[MsgType]
	streamID StreamID
}

func (s Subscription[StreamID, MsgType]) C() <-chan Message[MsgType] { return s.c }

// Close closes and unsubscribes the subscription.
// No-op if already closed.
func (s Subscription[StreamID, MsgType]) Close() {
	s.b.lock.Lock()
	defer s.b.lock.Unlock()

	stream, ok := s.b.streams[s.streamID]
	if !ok {
		return // The entire stream is already closed.
	}

	if _, ok := stream[s.c]; ok {
		close(s.c)
		if len(stream) == 1 {
			// Last subscriber in the stream, purge the stream.
			delete(s.b.streams, s.streamID)
			clear(stream)
		} else {
			delete(stream, s.c)
		}
	}
}

func New[StreamID, MsgType comparable]() *Broadcast[StreamID, MsgType] {
	return &Broadcast[StreamID, MsgType]{
		streams: make(map[StreamID]map[chan Message[MsgType]]struct{}),
	}
}

// Subscribe creates a new subscription.
func (b *Broadcast[StreamID, MsgType]) Subscribe(
	streamID StreamID, channelBufferSize int,
) Subscription[StreamID, MsgType] {
	b.lock.Lock()
	defer b.lock.Unlock()

	ch := make(chan Message[MsgType], channelBufferSize) // buffered
	_, ok := b.streams[streamID]
	if !ok {
		b.streams[streamID] = map[chan Message[MsgType]]struct{}{ch: {}}
	} else {
		b.streams[streamID][ch] = struct{}{}
	}
	return Subscription[StreamID, MsgType]{b: b, c: ch, streamID: streamID}
}

type Message[MsgType comparable] struct {
	Type    MsgType
	Payload any
}

// BroadcastAll sends msg to all subscribers of all streams (non-blocking).
func (b *Broadcast[StreamID, MsgType]) BroadcastAll(typ MsgType, payload any) (notified int) {
	b.lock.Lock()
	defer b.lock.Unlock()

	for _, stream := range b.streams {
		for ch := range stream {
			select {
			case ch <- Message[MsgType]{Type: typ, Payload: payload}:
				notified++
			default: // drop if subscriber is too slow
			}
		}
	}
	return notified
}

// Broadcast sends msg to all subscribers in stream streamID (non-blocking).
// -1 is returned if stream is not found.
func (b *Broadcast[StreamID, MsgType]) Broadcast(
	streamID StreamID, typ MsgType, payload any,
) (notified int) {
	b.lock.Lock()
	defer b.lock.Unlock()

	stream, ok := b.streams[streamID]
	if !ok {
		return -1
	}

	for ch := range stream {
		select {
		case ch <- Message[MsgType]{Type: typ, Payload: payload}:
			notified++
		default: // drop if subscriber is too slow
		}
	}
	return notified
}

// Close shuts down broadcaster.
func (b *Broadcast[StreamID, MsgType]) Close() {
	b.lock.Lock()
	defer b.lock.Unlock()

	for _, subs := range b.streams {
		for ch := range subs {
			close(ch)
		}
		clear(subs)
	}
	clear(b.streams)
}
