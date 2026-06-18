package gobridge

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestPairingManualCodeAttemptsAreBounded 验证 P1-7：单 pairingId 连续失败达上限后
// 该 session 被失效，且始终返回 pairing.invalid_code（不泄漏码存在性）。
func TestPairingManualCodeAttemptsAreBounded(t *testing.T) {
	store, restoreGlobals := setupPairingHandlerTest(t)
	defer restoreGlobals()

	// 重置 gate，避免与其他测试的全局状态串扰。
	globalPairingGate = &pairingAttemptGate{
		sourceBuckets: make(map[string]*slidingBucket),
		pairFails:     make(map[string]*slidingBucket),
	}

	session := NewPairingSession("bridge-1", "Test Bridge", "ws://127.0.0.1:8777", "", 5*time.Minute)
	if err := store.Create(session); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	wrongCode := "000000"
	for i := 0; i < pairingAttemptConfig.perPairingFailLimit; i++ {
		conn, cleanupConn := openPairingHandlerConn(t)
		// pairingId 正确但 manualCode 错误 → invalid_code。
		sendPairingClaim(t, conn, session.ID, wrongCode)
		result := readPairingMessage(t, conn)
		if got := pairingErrorCode(result); got != "pairing.invalid_code" {
			t.Fatalf("attempt %d: code = %q, want pairing.invalid_code", i, got)
		}
		cleanupConn()
	}

	// 达到失败上限后该 session 应已被 Delete。
	if got, _ := store.Get(session.ID); got != nil {
		t.Fatalf("达到失败上限后 session 应被失效，但仍存在: %#v", got)
	}

	// 再用正确 manualCode 也无法 claim（session 已失效）→ 仍是 invalid_code（不泄漏）。
	conn, cleanupConn := openPairingHandlerConn(t)
	defer cleanupConn()
	sendPairingClaim(t, conn, session.ID, session.ManualCode)
	result := readPairingMessage(t, conn)
	if got := pairingErrorCode(result); got != "pairing.invalid_code" {
		t.Fatalf("失效后 code = %q, want pairing.invalid_code", got)
	}
}

// TestPairingSourceIPRateLimited 验证 P1-7 来源 IP 限流：单 IP 超过窗口配额后返回 rate_limited。
func TestPairingSourceIPRateLimited(t *testing.T) {
	_, restoreGlobals := setupPairingHandlerTest(t)
	defer restoreGlobals()

	// 用极小配额便于测试。
	prev := pairingAttemptConfig
	pairingAttemptConfig.sourceIPLimit = 3
	pairingAttemptConfig.perPairingFailLimit = 1000 // 放大，隔离 IP 限流维度
	t.Cleanup(func() { pairingAttemptConfig = prev })
	globalPairingGate = &pairingAttemptGate{
		sourceBuckets: make(map[string]*slidingBucket),
		pairFails:     make(map[string]*slidingBucket),
	}

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/pairing", nil)
	req.RemoteAddr = "203.0.113.9:5555"
	for i := 0; i < pairingAttemptConfig.sourceIPLimit; i++ {
		// acquire 通过即放行；release 后再 acquire 计入同一窗口。
		if perr := globalPairingGate.acquire(req); perr != nil {
			t.Fatalf("attempt %d 不应被限流: %v", i, perr)
		}
		globalPairingGate.release()
	}
	if perr := globalPairingGate.acquire(req); perr == nil || perr.Code != "pairing.rate_limited" {
		globalPairingGate.release()
		t.Fatalf("超配额应返回 pairing.rate_limited, got %#v", perr)
	}
}

// TestPairingValidClaimSucceeds 验证正常 pairingId+manualCode claim 仍成功（happy path 不被破坏）。
func TestPairingValidClaimSucceeds(t *testing.T) {
	store, restoreGlobals := setupPairingHandlerTest(t)
	defer restoreGlobals()
	globalPairingGate = &pairingAttemptGate{
		sourceBuckets: make(map[string]*slidingBucket),
		pairFails:     make(map[string]*slidingBucket),
	}

	session := NewPairingSession("bridge-1", "Test Bridge", "ws://127.0.0.1:8777", "", 5*time.Minute)
	if err := store.Create(session); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	conn, cleanupConn := openPairingHandlerConn(t)
	defer cleanupConn()
	sendPairingClaim(t, conn, session.ID, session.ManualCode)
	result := readPairingMessage(t, conn)
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("合法 claim 应成功: %#v", result)
	}
	if session.State != PairingClaimed {
		t.Fatalf("session state = %s, want claimed", session.State)
	}
}

// TestHmacEqualString 常量时间比较正确性。
func TestHmacEqualString(t *testing.T) {
	if !hmacEqualString("abc", "abc") {
		t.Fatal("equal strings 应返回 true")
	}
	if hmacEqualString("abc", "abd") {
		t.Fatal("不同字符串应返回 false")
	}
	if hmacEqualString("abc", "ab") {
		t.Fatal("不同长度应返回 false")
	}
	if !hmacEqualString("", "") {
		t.Fatal("空串相等应返回 true")
	}
}
