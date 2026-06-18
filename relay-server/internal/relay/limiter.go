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
	mu             sync.Mutex
	limit          int
	interval       time.Duration
	bucketTTL      time.Duration
	maxBuckets     int
	lastCleanup    time.Time
	buckets        map[string]rateBucket
	rateDenied     uint64
	capacityDenied uint64
	cleaned        uint64
}

func NewRateLimiter(limit int, interval time.Duration) *RateLimiter {
	return NewBoundedRateLimiter(limit, interval, 4096, 2*interval)
}

func NewBoundedRateLimiter(limit int, interval time.Duration, maxBuckets int, bucketTTL time.Duration) *RateLimiter {
	if maxBuckets <= 0 {
		maxBuckets = 4096
	}
	if bucketTTL < interval {
		bucketTTL = 2 * interval
	}
	return &RateLimiter{
		limit:      limit,
		interval:   interval,
		bucketTTL:  bucketTTL,
		maxBuckets: maxBuckets,
		buckets:    make(map[string]rateBucket),
	}
}

func (l *RateLimiter) Allow(key string, now time.Time) bool {
	if l == nil || l.limit <= 0 {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.lastCleanup.IsZero() || now.Sub(l.lastCleanup) >= l.interval {
		l.cleanupLocked(now)
	}
	bucket, exists := l.buckets[key]
	if !exists && len(l.buckets) >= l.maxBuckets {
		l.capacityDenied++
		return false
	}
	if bucket.window.IsZero() || now.Sub(bucket.window) >= l.interval {
		bucket = rateBucket{window: now}
	}
	if bucket.count >= l.limit {
		l.buckets[key] = bucket
		l.rateDenied++
		return false
	}
	bucket.count++
	l.buckets[key] = bucket
	return true
}

func (l *RateLimiter) cleanupLocked(now time.Time) {
	for key, bucket := range l.buckets {
		if now.Sub(bucket.window) >= l.bucketTTL {
			delete(l.buckets, key)
			l.cleaned++
		}
	}
	l.lastCleanup = now
}

type RateLimiterStats struct {
	Buckets        int
	RateDenied     uint64
	CapacityDenied uint64
	Cleaned        uint64
}

func (l *RateLimiter) Stats() RateLimiterStats {
	if l == nil {
		return RateLimiterStats{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return RateLimiterStats{
		Buckets:        len(l.buckets),
		RateDenied:     l.rateDenied,
		CapacityDenied: l.capacityDenied,
		Cleaned:        l.cleaned,
	}
}
