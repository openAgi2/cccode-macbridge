package gobridge

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// reqFromIP builds a stub *http.Request with the given RemoteAddr for IP-based
// rate-limit accounting.
func reqFromIP(ip string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = ip + ":1234"
	return r
}

// TestPairingGate_BucketCapacityFailClosed verifies T08: when sourceBuckets
// reaches maxPairingBuckets, new IPs are fail-closed (rate_limited) instead of
// creating unbounded growth.
func TestPairingGate_BucketCapacityFailClosed(t *testing.T) {
	g := newPairingAttemptGate()
	// Fill sourceBuckets to the cap with distinct IPs (each does one acquire so
	// the bucket is created).
	for i := 0; i < maxPairingBuckets; i++ {
		ip := "10.0." + itoa(i/256) + "." + itoa(i%256)
		if perr := g.acquire(reqFromIP(ip)); perr != nil {
			// Some acquires may hit the global-concurrent limit; release them so
			// activeClaims doesn't block subsequent IP creation. We only care
			// that the bucket map grew.
		} else {
			g.release()
		}
	}
	if len(g.sourceBuckets) < maxPairingBuckets {
		// concurrent claim limit may have blocked some; top up deterministically
		// by directly creating buckets to simulate a full map.
		for len(g.sourceBuckets) < maxPairingBuckets {
			ip := "172.16." + itoa(len(g.sourceBuckets)/256) + "." + itoa(len(g.sourceBuckets)%256)
			if perr := g.acquire(reqFromIP(ip)); perr == nil {
				g.release()
			}
		}
	}

	// One more distinct IP must be fail-closed.
	perr := g.acquire(reqFromIP("192.0.2.1"))
	if perr == nil {
		t.Fatal("expected rate_limited (fail-closed) when sourceBuckets at capacity, got nil")
		g.release()
	}
	if perr.Code != "pairing.rate_limited" {
		t.Errorf("code = %q, want pairing.rate_limited", perr.Code)
	}
}

// TestPairingGate_TTLSweepReclaimsIdleBuckets verifies T08: after the window
// elapses with no new counts, sweepStale reclaims the bucket so the map does
// not grow monotonically.
func TestPairingGate_TTLSweepReclaimsIdleBuckets(t *testing.T) {
	g := newPairingAttemptGate()
	// Create a few IP buckets.
	for _, ip := range []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"} {
		if perr := g.acquire(reqFromIP(ip)); perr != nil {
			t.Fatalf("acquire %s: %v", ip, perr)
		}
		g.release()
	}
	if got := len(g.sourceBuckets); got != 3 {
		t.Fatalf("sourceBuckets = %d, want 3", got)
	}

	// Advance past the window; the next acquire triggers sweepStale, which
	// should reclaim buckets whose window has fully elapsed.
	future := time.Now().Add(pairingAttemptConfig.sourceIPWindow + time.Second)
	// Manually drive a sweep by calling recordPairingFailure (which sweeps) with
	// a future time — but sweep uses the bucket's own window. Instead, simulate
	// elapsed time by acquiring with a future time is not possible via the public
	// API (acquire uses time.Now). Use sweepStale directly with a future now.
	g.mu.Lock()
	g.sweepStale(future)
	g.mu.Unlock()

	if got := len(g.sourceBuckets); got != 0 {
		t.Fatalf("after TTL sweep sourceBuckets = %d, want 0 (idle buckets not reclaimed)", got)
	}
}

// TestPairingGate_PairFailsCapacityFailClosed verifies T08: pairFails does not
// create a long-lived bucket for arbitrary pairingId once at capacity.
func TestPairingGate_PairFailsCapacityFailClosed(t *testing.T) {
	g := newPairingAttemptGate()
	// Fill pairFails to cap.
	now := time.Now()
	for i := 0; i < maxPairingBuckets; i++ {
		g.recordPairingFailure("pair-"+itoa(i), now)
	}
	if len(g.pairFails) < maxPairingBuckets {
		t.Fatalf("pairFails = %d, want %d", len(g.pairFails), maxPairingBuckets)
	}
	before := len(g.pairFails)
	// One more arbitrary pairingId must NOT create a new bucket (returns false,
	// does not grow the map).
	exhausted := g.recordPairingFailure("pair-overflow", now)
	if exhausted {
		t.Error("overflow pairingId reported exhausted; expected silent skip (no bucket created)")
	}
	if got := len(g.pairFails); got != before {
		t.Errorf("pairFails grew to %d after capacity (want stable %d) — arbitrary pairingId created a bucket", got, before)
	}
}

// TestPairingGate_RecordPairingFailureExhaustsAtLimit verifies the per-pairingId
// failure limit still works (regression guard alongside the new capacity logic).
func TestPairingGate_RecordPairingFailureExhaustsAtLimit(t *testing.T) {
	g := newPairingAttemptGate()
	now := time.Now()
	for i := 0; i < pairingAttemptConfig.perPairingFailLimit-1; i++ {
		if g.recordPairingFailure("pair-x", now) {
			t.Fatalf("reported exhausted at failure %d (before limit)", i+1)
		}
	}
	// The limit-th failure should report exhausted.
	if !g.recordPairingFailure("pair-x", now) {
		t.Fatal("expected exhausted at perPairingFailLimit, got false")
	}
}

// itoa local helper (strconv would also work; kept dependency-free).
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
