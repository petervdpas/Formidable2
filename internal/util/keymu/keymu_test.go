package keymu

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestWithLock_SerializesSameKey runs N goroutines that all hold the
// same key for a short interval. If serialization works, the observed
// in-flight counter never exceeds 1.
func TestWithLock_SerializesSameKey(t *testing.T) {
	t.Parallel()

	var km Map
	var inFlight, peak int32

	var wg sync.WaitGroup
	const N = 50
	for range N {
		wg.Go(func() {
			_ = km.WithLock("k", func() error {
				cur := atomic.AddInt32(&inFlight, 1)
				for {
					p := atomic.LoadInt32(&peak)
					if cur <= p || atomic.CompareAndSwapInt32(&peak, p, cur) {
						break
					}
				}
				time.Sleep(2 * time.Millisecond)
				atomic.AddInt32(&inFlight, -1)
				return nil
			})
		})
	}
	wg.Wait()

	if peak != 1 {
		t.Fatalf("peak in-flight under same key = %d, want 1", peak)
	}
}

// TestWithLock_DistinctKeysParallel verifies different keys do not
// block each other. With 4 keys held for 25ms each, total wall time
// must be well under 4×25ms.
func TestWithLock_DistinctKeysParallel(t *testing.T) {
	t.Parallel()

	var km Map
	const hold = 25 * time.Millisecond
	keys := []string{"a", "b", "c", "d"}

	var wg sync.WaitGroup
	start := time.Now()
	for _, k := range keys {
		wg.Go(func() {
			_ = km.WithLock(k, func() error {
				time.Sleep(hold)
				return nil
			})
		})
	}
	wg.Wait()
	elapsed := time.Since(start)

	// Generous slack for scheduling jitter, but well under serialization.
	if elapsed > 3*hold {
		t.Fatalf("parallel keys took %v, want < %v (serialized would be ~%v)",
			elapsed, 3*hold, time.Duration(len(keys))*hold)
	}
}

// TestWithLock_PropagatesError verifies fn errors flow back unchanged.
func TestWithLock_PropagatesError(t *testing.T) {
	t.Parallel()

	var km Map
	want := errors.New("boom")
	got := km.WithLock("k", func() error { return want })
	if !errors.Is(got, want) {
		t.Fatalf("error = %v, want %v", got, want)
	}
}

// TestWithLock_ReleasesOnPanic verifies the mutex is released even if
// fn panics, so a subsequent call doesn't deadlock.
func TestWithLock_ReleasesOnPanic(t *testing.T) {
	t.Parallel()

	var km Map
	func() {
		defer func() { _ = recover() }()
		_ = km.WithLock("k", func() error { panic("nope") })
	}()

	done := make(chan struct{})
	go func() {
		_ = km.WithLock("k", func() error { return nil })
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("WithLock after panic deadlocked")
	}
}

// TestLock_BareUnlock confirms the bare Lock/unlock pair works for
// callers that can't fit a closure.
func TestLock_BareUnlock(t *testing.T) {
	t.Parallel()

	var km Map
	unlock := km.Lock("k")
	unlock()

	// Should not deadlock on a fresh acquire after explicit unlock.
	done := make(chan struct{})
	go func() {
		unlock2 := km.Lock("k")
		unlock2()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("second Lock after Unlock deadlocked")
	}
}

// TestZeroValueReady verifies a zero-value Map works without
// initialization.
func TestZeroValueReady(t *testing.T) {
	t.Parallel()
	var km Map
	if err := km.WithLock("k", func() error { return nil }); err != nil {
		t.Fatalf("zero-value Map.WithLock failed: %v", err)
	}
}
