package broadcast_test

import (
	"testing"

	"github.com/romshark/toki/internal/webedit/internal/broadcast"
	"github.com/stretchr/testify/require"
)

func TestSubscribeAndBroadcast(t *testing.T) {
	b := broadcast.New[int, string]()

	const streamID1 = 123

	sub1 := b.Subscribe(streamID1, 1)
	defer sub1.Close()

	sub2 := b.Subscribe(streamID1, 1)
	sub2.Close() // Close and unsubscribe immediately.

	const streamID2 = 321
	sub3 := b.Subscribe(streamID2, 1)
	defer sub3.Close()
	sub4 := b.Subscribe(streamID2, 1)
	defer sub4.Close()

	notified1 := b.Broadcast(streamID1, "hello first", 42)
	notified2 := b.Broadcast(streamID2, "hello second", 43)

	require.Equal(t, 1, notified1)
	require.Equal(t,
		broadcast.Message[string]{Type: "hello first", Payload: 42}, <-sub1.C())

	require.Equal(t, 2, notified2)
	require.Equal(t,
		broadcast.Message[string]{Type: "hello second", Payload: 43}, <-sub3.C())
	require.Equal(t,
		broadcast.Message[string]{Type: "hello second", Payload: 43}, <-sub4.C())
}

func TestSlowSubscriberDrop(t *testing.T) {
	b := broadcast.New[int, string]()
	const subCap = 3

	const streamID = 123

	sub := b.Subscribe(streamID, subCap)
	defer sub.Close()

	subUnbuffered := b.Subscribe(1, 0)
	defer subUnbuffered.Close()

	// Fill buffer
	b.Broadcast(streamID, "fill1", nil)
	b.Broadcast(streamID, "fill2", nil)
	b.Broadcast(streamID, "fill3", nil)

	// This message should be dropped, since channel is full
	b.Broadcast(1, "drop", nil)

	// Drain everything
	msgs := make([]string, 0, subCap)
	for range subCap {
		m := <-sub.C()
		msgs = append(msgs, m.Type)
	}

	require.Len(t, sub.C(), 0)
	require.Equal(t, []string{"fill1", "fill2", "fill3"}, msgs)
	require.Len(t, subUnbuffered.C(), 0)
}

func TestBroadcasterClose(t *testing.T) {
	b := broadcast.New[int, string]()

	const streamID1 = 123
	const streamID2 = 321

	sub1 := b.Subscribe(streamID1, 1)
	sub2 := b.Subscribe(streamID2, 1)

	b.Close()

	// After close, all channels must be closed
	for _, sub := range []broadcast.Subscription[int, string]{sub1, sub2} {
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

func TestBroadcastStreamNotFound(t *testing.T) {
	b := broadcast.New[int, string]()

	notified := b.Broadcast(999, "nope", nil)
	require.Equal(t, -1, notified)
}

func TestBroadcastAll(t *testing.T) {
	b := broadcast.New[int, string]()

	const streamID1 = 123
	const streamID2 = 321

	sub1 := b.Subscribe(streamID1, 1)
	defer sub1.Close()
	sub2 := b.Subscribe(streamID2, 1)
	defer sub2.Close()
	sub3 := b.Subscribe(streamID2, 1)
	defer sub3.Close()

	notified := b.BroadcastAll("all", 123)
	require.Equal(t, 3, notified)

	require.Equal(t, "all", (<-sub1.C()).Type)
	require.Equal(t, "all", (<-sub2.C()).Type)
	require.Equal(t, "all", (<-sub3.C()).Type)
}

func TestSubscriptionCloseIdempotent(t *testing.T) {
	b := broadcast.New[int, string]()
	sub := b.Subscribe(1, 1)
	sub.Close()
	sub.Close() // Must be a no-op.
}

func TestSubscriptionCloseAfterBroadcasterClose(t *testing.T) {
	b := broadcast.New[int, string]()
	sub := b.Subscribe(1, 1)
	b.Close()
	sub.Close() // Must be safe and no-op
}

func TestLastSubscriberPurgesStream(t *testing.T) {
	b := broadcast.New[int, string]()

	const streamID = 123

	sub := b.Subscribe(streamID, 1)
	sub.Close()

	// stream should be gone, broadcast must return -1
	notified := b.Broadcast(streamID, "test", nil)
	require.Equal(t, -1, notified)
}
