// Package keymu provides a keyed mutex map: it serializes critical
// sections by string key while letting different keys run in parallel.
// Use it when a manager owns multiple independent resources (e.g. one
// mutex per template filename) so editing resource A never blocks B.
//
// The zero-value Map is ready to use; all methods are safe for concurrent use.
package keymu

import "sync"

// Map is a lazily-allocated map of per-key sync.Mutex. The internal map
// is guarded by mu; the per-key mutexes are returned to callers who own
// the Lock/Unlock cycle for their key.
type Map struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// get returns the existing or fresh mutex for key. The mutex is shared
// across callers, so same-key callers contend while distinct-key callers
// do not.
func (m *Map) get(key string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locks == nil {
		m.locks = map[string]*sync.Mutex{}
	}
	mu, ok := m.locks[key]
	if !ok {
		mu = &sync.Mutex{}
		m.locks[key] = mu
	}
	return mu
}

// WithLock acquires the mutex for key, runs fn, and releases on return
// (including panics), propagating fn's return value. Use this for the
// common read-modify-write pattern around a single resource:
//
//	err := km.WithLock(filename, func() error {
//	    cur, err := load(filename)
//	    if err != nil { return err }
//	    next := mutate(cur)
//	    return save(filename, next)
//	})
func (m *Map) WithLock(key string, fn func() error) error {
	mu := m.get(key)
	mu.Lock()
	defer mu.Unlock()
	return fn()
}

// Lock acquires the mutex for key and returns the unlock function.
// Prefer WithLock; use Lock only when the mutex must be held across a
// boundary that doesn't fit a closure (e.g. spanning multiple methods).
//
//	unlock := km.Lock(key)
//	defer unlock()
func (m *Map) Lock(key string) (unlock func()) {
	mu := m.get(key)
	mu.Lock()
	return mu.Unlock
}
