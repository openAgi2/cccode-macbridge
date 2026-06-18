package relay

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"testing"
	"time"
)

// TestActivationNonceReplayRejected 验证 P2-4：同一合法激活请求（相同 install_id+nonce）
// 在时间窗口内重放，第二次返回 409 activation_replayed。
func TestActivationNonceReplayRejected(t *testing.T) {
	_, httpServer := newTestServer(t, 100)
	publicKey, privateKey, _ := ed25519.GenerateKey(nil)
	installID := "inst_replay_test"
	bridgeAuth := "bridge-auth-replay-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

	// 构造两次使用完全相同 nonce/timestamp 的请求体。
	timestamp := time.Now().Unix()
	nonce := "nonce_replay_unique"
	encodedPublicKey := base64.StdEncoding.EncodeToString(publicKey)
	sig := ed25519.Sign(privateKey, activationPayload(installID, encodedPublicKey, bridgeAuth, timestamp, nonce))
	body := map[string]any{
		"installId":  installID,
		"publicKey":  encodedPublicKey,
		"bridgeAuth": bridgeAuth,
		"timestamp":  timestamp,
		"nonce":      nonce,
		"signature":  base64.StdEncoding.EncodeToString(sig),
	}

	resp1, _ := requestJSON(t, http.MethodPost, httpServer.URL+"/v1/activations/routes", "", body)
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first activation status=%d (want 201)", resp1.StatusCode)
	}
	// 同 nonce 重放：应被拒绝为 409。
	resp2, _ := requestJSON(t, http.MethodPost, httpServer.URL+"/v1/activations/routes", "", body)
	if resp2.StatusCode != http.StatusConflict {
		t.Fatalf("replayed activation status=%d (want 409)", resp2.StatusCode)
	}
}

// TestPairingClaimConflictOnDifferentSealedClaim 验证 P2-9：同 (route, claim) 的不同
// sealedClaim 返回 409 relay.pairing_claim_conflict；相同请求幂等成功。
func TestPairingClaimConflictOnDifferentSealedClaim(t *testing.T) {
	_, httpServer := newTestServer(t, 100)
	creds := provisionDevice(t, httpServer.URL)
	address := httpServer.URL + "/v1/routes/" + creds.routeID + "/pairing-claims"

	submit := func(sealed []byte) int {
		body := map[string]any{
			"claimId":     "claim_conflict_1",
			"capability":  "cap_secret_capability_value_here",
			"sealedClaim": sealed,
		}
		resp, _ := requestJSON(t, http.MethodPost, address, creds.bridgeAuth, body)
		return resp.StatusCode
	}

	if code := submit([]byte("sealed-A")); code != http.StatusOK {
		t.Fatalf("first claim submit status=%d (want 200)", code)
	}
	// 相同请求幂等成功。
	if code := submit([]byte("sealed-A")); code != http.StatusOK {
		t.Fatalf("idempotent resubmit status=%d (want 200)", code)
	}
	// 不同 sealedClaim → 冲突 409。
	if code := submit([]byte("sealed-B-different")); code != http.StatusConflict {
		t.Fatalf("conflicting resubmit status=%d (want 409)", code)
	}
}
