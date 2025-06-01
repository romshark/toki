package sync

import (
	"iter"
	"sync"
)

func NewSlice[T any](capacity int) *Slice[T] {
	return &Slice[T]{items: make([]T, 0, capacity)}
}

// Slice is a mutex synchronized slice for concurrent use.
type Slice[T any] struct {
	lock  sync.Mutex
	items []T
}

func (a *Slice[T]) At(index int) T {
	a.lock.Lock()
	defer a.lock.Unlock()
	return a.items[index]
}

func (a *Slice[T]) Append(t T) (index int) {
	a.lock.Lock()
	defer a.lock.Unlock()
	index = len(a.items)
	a.items = append(a.items, t)
	return index
}

// Access executes fn exclusively and passes the underlying slice.
//
// WARNING: Do not alias and use s outside of fn!
func (a *Slice[T]) Access(fn func(s []T) error) error {
	a.lock.Lock()
	defer a.lock.Unlock()
	return fn(a.items)
}

func (a *Slice[T]) Len() int {
	a.lock.Lock()
	defer a.lock.Unlock()
	return len(a.items)
}

func (a *Slice[T]) Seq() iter.Seq[T] {
	return func(yield func(T) bool) {
		a.lock.Lock()
		defer a.lock.Unlock()
		for _, i := range a.items {
			if !yield(i) {
				break
			}
		}
	}
}
