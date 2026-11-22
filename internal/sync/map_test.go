package sync

import (
	"errors"
	"iter"
	"maps"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMap(t *testing.T) {
	m := NewMap[int, string](0)

	require.Zero(t, m.Len())
	{
		v, ok := m.Get(1)
		require.Empty(t, v)
		require.False(t, ok)

		v = m.GetValue(1)
		require.Empty(t, v)
	}

	m.Set(1, "first")
	m.Set(2, "second")
	require.Equal(t, 2, m.Len())
	{
		v, ok := m.Get(1)
		require.Equal(t, "first", v)
		require.True(t, ok)

		v = m.GetValue(1)
		require.Equal(t, "first", v)

		v, ok = m.Get(2)
		require.Equal(t, "second", v)
		require.True(t, ok)

		v = m.GetValue(2)
		require.Equal(t, "second", v)
	}

	require.Equal(t, map[int]string{1: "first", 2: "second"}, maps.Collect(m.Seq()))
	require.Equal(t, map[int]string{1: "first", 2: "second"}, maps.Collect(m.SeqRead()))

	{
		i := 0
		for range m.Seq() {
			i++
			break
		}
		require.Equal(t, 1, i)
	}
	{
		i := 0
		for range m.SeqRead() {
			i++
			break
		}
		require.Equal(t, 1, i)
	}

	m.Delete(3)
	require.Equal(t, 2, m.Len())

	m.Delete(2)
	require.Equal(t, 1, m.Len())

	m.Clear()
	require.Zero(t, m.Len())

	{
		errTest := errors.New("test error")
		err := m.Access(func(s map[int]string) error {
			s[5] = "fifth"
			return errTest
		})
		require.Equal(t, errTest, err)
	}
	require.Equal(t, 1, m.Len())
}

// TestMapSeqLock makes sure writers wait for iterations to finish.
func TestMapSeqLock(t *testing.T) {
	for name, seq := range map[string]func(*Map[int, int]) iter.Seq2[int, int]{
		"Seq":     (*Map[int, int]).Seq,
		"SeqRead": (*Map[int, int]).SeqRead,
	} {
		t.Run(name, func(t *testing.T) {
			m := NewMap[int, int](0)
			m.Set(1, 1)
			setDone := make(chan struct{})
			for range seq(m) {
				go func() {
					m.Set(2, 2)
					close(setDone)
				}()
				select {
				case <-setDone:
					t.Fatal("Set completed during iteration")
				case <-time.After(50 * time.Millisecond):
				}
			}
			<-setDone
			require.Equal(t, 2, m.Len())
		})
	}
}
