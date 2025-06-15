package sync

import (
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMap(t *testing.T) {
	m := NewMap[int, string](0)

	require.Zero(t, m.Len())
	{
		v, ok := m.Get(1)
		require.Zero(t, v)
		require.False(t, ok)

		v = m.GetValue(1)
		require.Zero(t, v)
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

	require.Equal(t, map[int]string{1: "first", 2: "second"}, readMap(m.Seq()))
	require.Equal(t, map[int]string{1: "first", 2: "second"}, readMap(m.SeqRead()))

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

func readMap[K comparable, V any](i iter.Seq2[K, V]) map[K]V {
	m := make(map[K]V)
	for k, v := range i {
		m[k] = v
	}
	return m
}
