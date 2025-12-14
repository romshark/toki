package broadcast_test

import (
	"testing"

	"github.com/romshark/toki/internal/webedit/internal/broadcast"
	"github.com/stretchr/testify/require"
)

func TestSubscribeAndBroadcast(t *testing.T) {
	b := broadcast.New[string]()
	sub1 := b.Subscribe(1)
	defer sub1.Close()
	sub2 := b.Subscribe(1)
	sub2.Close() // Close and unsubscribe immediately.
	sub3 := b.Subscribe(1)
	defer sub3.Close()

	notified := b.Broadcast("hello", 42)
	require.Equal(t, broadcast.Message[string]{Type: "hello", Payload: 42}, <-sub1.C())
	require.Equal(t, broadcast.Message[string]{Type: "hello", Payload: 42}, <-sub3.C())
	require.Equal(t, 2, notified)
}

func TestSlowSubscriberDrop(t *testing.T) {
	b := broadcast.New[string]()
	const subCap = 3
	sub := b.Subscribe(subCap)
	defer sub.Close()
	subUnbuffered := b.Subscribe(0)
	defer subUnbuffered.Close()

	// Fill buffer
	b.Broadcast("fill1", nil)
	b.Broadcast("fill2", nil)
	b.Broadcast("fill3", nil)

	// This message should be dropped, since channel is full
	b.Broadcast("drop", nil)

	// Drain everything
	msgs := []string{}
	for range subCap {
		m := <-sub.C()
		msgs = append(msgs, m.Type)
	}
	require.Len(t, sub.C(), 0)
	require.Equal(t, []string{"fill1", "fill2", "fill3"}, msgs)
	require.Len(t, subUnbuffered.C(), 0)
}

func TestBroadcasterClose(t *testing.T) {
	b := broadcast.New[string]()
	sub1 := b.Subscribe(1)
	sub2 := b.Subscribe(1)

	b.Close()

	// After close, all channels must be closed
	for _, sub := range []broadcast.Subscription[string]{sub1, sub2} {
		select {
		case _, ok := <-sub.C():
			if ok {
				t.Fatal("expected closed channel")
			}
		default:
			t.Fatal("expected closed channel immediately")
		}
	}
}
