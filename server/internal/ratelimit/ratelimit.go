package ratelimit

// Package ratelimit provides per-connection token-bucket rate limiting for
// WebSocket intents. It is intentionally stateless per connection: no global
// account throttle lives here (that belongs in the queue manager).

import (
	"sync"
	"time"
)

// Limiter is a token-bucket rate limiter.
type Limiter struct {
	mu       sync.Mutex
	tokens   float64
	capacity float64
	rate     float64
	last     time.Time
}

// New creates a limiter with the given rate (tokens per second) and burst capacity.
func New(rate, capacity float64) *Limiter {
	return &Limiter{
		tokens:   capacity,
		capacity: capacity,
		rate:     rate,
		last:     time.Now(),
	}
}

// Allow reports whether one token is available and consumes it.
func (l *Limiter) Allow() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(l.last).Seconds()
	l.last = now
	l.tokens += elapsed * l.rate
	if l.tokens > l.capacity {
		l.tokens = l.capacity
	}
	if l.tokens < 1 {
		return false
	}
	l.tokens--
	return true
}
