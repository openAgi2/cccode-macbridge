package relay

import (
	"sync"
	"time"
)

type rateBucket struct {
	window time.Time
	count  int
}

type RateLimiter struct {
	mu       sync.Mutex
	limit    int
	interval time.Duration
	buckets  map[string]rateBucket
}

func NewRateLimiter(limit int, interval time.Duration) *RateLimiter {
	return &RateLimiter{limit: limit, interval: interval, buckets: make(map[string]rateBucket)}
}

func (l *RateLimiter) Allow(key string, now time.Time) bool {
	if l == nil || l.limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	bucket := l.buckets[key]
	if bucket.window.IsZero() || now.Sub(bucket.window) >= l.interval {
		bucket = rateBucket{window: now}
	}
	if bucket.count >= l.limit {
		l.buckets[key] = bucket
		return false
	}
	bucket.count++
	l.buckets[key] = bucket
	return true
}
