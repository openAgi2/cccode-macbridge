package relay

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterBoundsAndExpiresBuckets(t *testing.T) {
	now := time.Unix(1000, 0)
	limiter := NewBoundedRateLimiter(2, time.Minute, 3, 2*time.Minute)
	for _, key := range []string{"a", "b", "c"} {
		if !limiter.Allow(key, now) {
			t.Fatalf("initial key %q was rejected", key)
		}
	}
	if limiter.Allow("d", now) {
		t.Fatal("new key was allowed after bucket capacity was reached")
	}
	stats := limiter.Stats()
	if stats.Buckets != 3 || stats.CapacityDenied != 1 {
		t.Fatalf("stats after capacity denial = %+v", stats)
	}

	later := now.Add(3 * time.Minute)
	if !limiter.Allow("d", later) {
		t.Fatal("new key was rejected after stale buckets expired")
	}
	stats = limiter.Stats()
	if stats.Buckets != 1 || stats.Cleaned != 3 {
		t.Fatalf("stats after cleanup = %+v", stats)
	}
}

func TestRateLimiterTracksRateDenials(t *testing.T) {
	now := time.Unix(1000, 0)
	limiter := NewBoundedRateLimiter(1, time.Minute, 8, 2*time.Minute)
	if !limiter.Allow("key", now) {
		t.Fatal("first request rejected")
	}
	if limiter.Allow("key", now) {
		t.Fatal("second request allowed above rate")
	}
	if got := limiter.Stats().RateDenied; got != 1 {
		t.Fatalf("rate denied = %d, want 1", got)
	}
}

func TestClientIPTrustsForwardingHeaderOnlyFromConfiguredProxy(t *testing.T) {
	networks, err := parseTrustedProxyCIDRs([]string{"127.0.0.0/8"})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{trustedProxyNets: networks}

	direct := httptest.NewRequest("GET", "http://relay.test/", nil)
	direct.RemoteAddr = "203.0.113.9:4321"
	direct.Header.Set("CF-Connecting-IP", "198.51.100.7")
	if got := server.clientIP(direct); got != "203.0.113.9" {
		t.Fatalf("direct client IP = %q, want remote address", got)
	}

	proxied := httptest.NewRequest("GET", "http://relay.test/", nil)
	proxied.RemoteAddr = "127.0.0.1:4321"
	proxied.Header.Set("CF-Connecting-IP", "198.51.100.7")
	if got := server.clientIP(proxied); got != "198.51.100.7" {
		t.Fatalf("proxied client IP = %q, want forwarding header", got)
	}

	invalid := httptest.NewRequest("GET", "http://relay.test/", nil)
	invalid.RemoteAddr = "127.0.0.1:4321"
	invalid.Header.Set("CF-Connecting-IP", "not-an-ip")
	if got := server.clientIP(invalid); got != "127.0.0.1" {
		t.Fatalf("invalid forwarded client IP = %q, want proxy address", got)
	}
}
