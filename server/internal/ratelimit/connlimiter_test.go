package ratelimit

import (
	"sync"
	"testing"
)

func TestConnLimiter_AcquireUpToCapacity(t *testing.T) {
	l := NewConnLimiter(2)
	if !l.TryAcquire() {
		t.Fatal("expected first acquire to succeed")
	}
	if !l.TryAcquire() {
		t.Fatal("expected second acquire to succeed")
	}
	if l.TryAcquire() {
		t.Fatal("expected third acquire to fail at capacity")
	}
}

func TestConnLimiter_ReleaseFreesASlot(t *testing.T) {
	l := NewConnLimiter(1)
	if !l.TryAcquire() {
		t.Fatal("expected acquire to succeed")
	}
	if l.TryAcquire() {
		t.Fatal("expected second acquire to fail at capacity")
	}
	l.Release()
	if !l.TryAcquire() {
		t.Fatal("expected acquire to succeed again after release")
	}
}

func TestConnLimiter_ZeroMeansUnlimited(t *testing.T) {
	l := NewConnLimiter(0)
	for i := 0; i < 1000; i++ {
		if !l.TryAcquire() {
			t.Fatalf("expected unlimited limiter to always acquire (iteration %d)", i)
		}
	}
}

func TestConnLimiter_ConcurrentAcquireNeverExceedsCapacity(t *testing.T) {
	const capacity = 50
	l := NewConnLimiter(capacity)

	var wg sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.TryAcquire() {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if accepted != capacity {
		t.Fatalf("expected exactly %d acquires to succeed under contention, got %d", capacity, accepted)
	}
	if l.Current() != capacity {
		t.Fatalf("expected Current() == %d, got %d", capacity, l.Current())
	}
}
