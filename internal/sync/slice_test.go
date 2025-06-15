package sync

import (
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSlice(t *testing.T) {
	s := NewSlice[int](0)

	require.Zero(t, s.Len())
	require.Panics(t, func() { s.At(0) })

	{
		var errTest = errors.New("test error")
		err := s.Access(func(s []int) error {
			require.Len(t, s, 0)
			return errTest
		})
		require.Equal(t, errTest, err)
	}

	s.Append(1)
	s.Append(2)
	require.Equal(t, 2, s.Len())
	require.Equal(t, 1, s.At(0))
	require.Equal(t, 2, s.At(1))
	require.Panics(t, func() { s.At(2) })

	require.Equal(t, []int{1, 2}, readSlice(s.Seq()))
	require.Equal(t, []int{1, 2}, readSlice(s.SeqRead()))

	{
		i := 0
		for range s.Seq() {
			i++
			break
		}
		require.Equal(t, 1, i)
	}
	{
		i := 0
		for range s.SeqRead() {
			i++
			break
		}
		require.Equal(t, 1, i)
	}
}

func readSlice[V any](i iter.Seq[V]) []V {
	var s []V
	for v := range i {
		s = append(s, v)
	}
	return s
}
