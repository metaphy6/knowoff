package ratelimit

import "sync/atomic"

// ConnLimiter caps concurrent WebSocket connections so a burst of clients
// degrades gracefully (the caller rejects new connections, e.g. HTTP 503)
// instead of letting unbounded goroutines/sockets exhaust host CPU/memory.
// A max of 0 means unlimited.
type ConnLimiter struct {
	max     int64
	current int64
}

// NewConnLimiter creates a limiter that allows up to max concurrent
// acquisitions. max <= 0 disables the cap (always allow).
func NewConnLimiter(max int) *ConnLimiter {
	return &ConnLimiter{max: int64(max)}
}

// TryAcquire reserves one slot and reports whether it was available.
func (c *ConnLimiter) TryAcquire() bool {
	if c.max <= 0 {
		atomic.AddInt64(&c.current, 1)
		return true
	}
	for {
		cur := atomic.LoadInt64(&c.current)
		if cur >= c.max {
			return false
		}
		if atomic.CompareAndSwapInt64(&c.current, cur, cur+1) {
			return true
		}
	}
}

// Release frees a previously-acquired slot.
func (c *ConnLimiter) Release() {
	atomic.AddInt64(&c.current, -1)
}

// Current returns the number of currently-held slots.
func (c *ConnLimiter) Current() int64 {
	return atomic.LoadInt64(&c.current)
}
