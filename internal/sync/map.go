package sync

import (
	"iter"
	"sync"
)

func NewMap[K comparable, V any](size int) *Map[K, V] {
	return &Map[K, V]{items: make(map[K]V, size)}
}

// Map is a mutex synchronized map for concurrent use.
type Map[K comparable, V any] struct {
	lock  sync.RWMutex
	items map[K]V
}

func (m *Map[K, V]) Len() int {
	m.lock.Lock()
	defer m.lock.Unlock()
	return len(m.items)
}

func (m *Map[K, V]) Set(k K, v V) {
	m.lock.Lock()
	defer m.lock.Unlock()
	m.items[k] = v
}

func (m *Map[K, V]) Get(k K) (value V, ok bool) {
	m.lock.Lock()
	defer m.lock.Unlock()
	value, ok = m.items[k]
	return value, ok
}

func (m *Map[K, V]) GetValue(k K) (value V) {
	m.lock.Lock()
	defer m.lock.Unlock()
	return m.items[k]
}

func (m *Map[K, V]) Delete(k K) {
	m.lock.Lock()
	defer m.lock.Unlock()
	delete(m.items, k)
}

func (m *Map[K, V]) Clear(k K) {
	m.lock.Lock()
	defer m.lock.Unlock()
	clear(m.items)
}

// Access executes fn exclusively and passes the underlying slice.
//
// WARNING: Do not alias and use s outside of fn!
func (m *Map[K, V]) Access(fn func(s map[K]V) error) error {
	m.lock.Lock()
	defer m.lock.Unlock()
	return fn(m.items)
}

func (m *Map[K, V]) Seq() iter.Seq2[K, V] {
	m.lock.Lock()
	defer m.lock.Unlock()
	return func(yield func(K, V) bool) {
		for k, v := range m.items {
			if !yield(k, v) {
				break
			}
		}
	}
}

func (m *Map[K, V]) SeqRead() iter.Seq2[K, V] {
	m.lock.RLock()
	defer m.lock.RUnlock()
	return func(yield func(K, V) bool) {
		for k, v := range m.items {
			if !yield(k, v) {
				break
			}
		}
	}
}
