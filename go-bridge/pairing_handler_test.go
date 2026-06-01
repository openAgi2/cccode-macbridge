package gobridge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func setupPairingHandlerTest(t *testing.T) (*MemoryPairingStore, func()) {
	t.Helper()

	prevPairingStore := globalPairingStore
	prevDeviceStore := globalDeviceStore
	prevRegistry := globalPairingRegistry

	store := NewMemoryPairingStore()
	globalPairingStore = store
	globalDeviceStore = NewMemoryDeviceStore()
	globalPairingRegistry = &PairingPendingRegistry{conns: make(map[string]*PairingPendingConn)}

	cleanup := func() {
		globalPairingStore = prevPairingStore
		globalDeviceStore = prevDeviceStore
		globalPairingRegistry = prevRegistry
	}
	return store, cleanup
}

func openPairingHandlerConn(t *testing.T) (*websocket.Conn, func()) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(handlePairingWebSocket))
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial failed: %v", err)
	}

	cleanup := func() {
		_ = clientConn.Close()
		server.Close()
	}
	return clientConn, cleanup
}

func sendPairingClaim(t *testing.T, conn *websocket.Conn, pairingID, manualCode string) {
	t.Helper()
	payload := map[string]any{
		"type":       "pairing_claim",
		"pairingId":  pairingID,
		"manualCode": manualCode,
		"device": map[string]string{
			"deviceId":    "ios-device-1",
			"displayName": "Jack iPhone",
			"platform":    "ios",
		},
	}
	if err := conn.WriteJSON(payload); err != nil {
		t.Fatalf("write claim failed: %v", err)
	}
}

func readPairingMessage(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline failed: %v", err)
	}
	var payload map[string]any
	if err := conn.ReadJSON(&payload); err != nil {
		t.Fatalf("read json failed: %v", err)
	}
	return payload
}

func waitForPendingPairing(t *testing.T, pairingID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		globalPairingRegistry.mu.Lock()
		_, ok := globalPairingRegistry.conns[pairingID]
		globalPairingRegistry.mu.Unlock()
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("pending pairing %s was not registered", pairingID)
}

func finishPendingPairing(t *testing.T, pairingID string, conn *websocket.Conn) {
	t.Helper()
	waitForPendingPairing(t, pairingID)
	ok := globalPairingRegistry.NotifyComplete(pairingID, PairingCompletePush{
		Type: "pairing_complete",
		Device: PairingCompleteDevice{
			DeviceID: "ios-device-1",
			Token:    "devtok_test_123",
		},
		Bridge: PairingCompleteBridge{
			BridgeID:    "bridge-1",
			DisplayName: "Test Bridge",
			LocalURL:    "ws://127.0.0.1:8777/bridge",
		},
	})
	if !ok {
		t.Fatalf("notify complete returned false for pairing %s", pairingID)
	}

	message := readPairingMessage(t, conn)
	if got := message["type"]; got != "pairing_complete" {
		t.Fatalf("complete message type = %#v, want pairing_complete", got)
	}
}

func pairingErrorMessage(payload map[string]any) string {
	errorPayload, _ := payload["error"].(map[string]any)
	message, _ := errorPayload["message"].(string)
	return message
}

func TestHandlePairingWebSocketIgnoresLegacyPingProbe(t *testing.T) {
	store, restoreGlobals := setupPairingHandlerTest(t)
	defer restoreGlobals()

	session := NewPairingSession("bridge-1", "Test Bridge", "ws://127.0.0.1:8777", "", 5*time.Minute)
	if err := store.Create(session); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	clientConn, cleanupConn := openPairingHandlerConn(t)
	defer cleanupConn()

	if err := clientConn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
		t.Fatalf("write ping failed: %v", err)
	}
	sendPairingClaim(t, clientConn, session.ID, "")

	result := readPairingMessage(t, clientConn)
	if got := result["type"]; got != "pairing_result" {
		t.Fatalf("result type = %#v, want pairing_result", got)
	}
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("pairing result ok = %#v, want true", result["ok"])
	}
	if session.State != PairingClaimed {
		t.Fatalf("session state = %s, want %s", session.State, PairingClaimed)
	}
	if session.ClaimingDeviceID != "ios-device-1" {
		t.Fatalf("claiming device id = %q, want ios-device-1", session.ClaimingDeviceID)
	}

	finishPendingPairing(t, session.ID, clientConn)
}

func TestHandlePairingWebSocketSupportsManualCodeLookup(t *testing.T) {
	store, restoreGlobals := setupPairingHandlerTest(t)
	defer restoreGlobals()

	session := NewPairingSession("bridge-1", "Test Bridge", "ws://127.0.0.1:8777", "", 5*time.Minute)
	if err := store.Create(session); err != nil {
		t.Fatalf("create session failed: %v", err)
	}

	clientConn, cleanupConn := openPairingHandlerConn(t)
	defer cleanupConn()

	sendPairingClaim(t, clientConn, "", session.ManualCode)

	result := readPairingMessage(t, clientConn)
	if ok, _ := result["ok"].(bool); !ok {
		t.Fatalf("pairing result ok = %#v, want true", result["ok"])
	}
	if session.State != PairingClaimed {
		t.Fatalf("session state = %s, want %s", session.State, PairingClaimed)
	}
	if session.ClaimingDeviceName != "Jack iPhone" {
		t.Fatalf("claiming device name = %q, want Jack iPhone", session.ClaimingDeviceName)
	}

	finishPendingPairing(t, session.ID, clientConn)
}

func TestHandlePairingWebSocketReturnsErrorForInvalidJSONAfterProbe(t *testing.T) {
	_, restoreGlobals := setupPairingHandlerTest(t)
	defer restoreGlobals()

	clientConn, cleanupConn := openPairingHandlerConn(t)
	defer cleanupConn()

	if err := clientConn.WriteMessage(websocket.TextMessage, []byte("ping")); err != nil {
		t.Fatalf("write ping failed: %v", err)
	}
	if err := clientConn.WriteMessage(websocket.TextMessage, []byte("not-json")); err != nil {
		t.Fatalf("write invalid json failed: %v", err)
	}

	result := readPairingMessage(t, clientConn)
	if ok, _ := result["ok"].(bool); ok {
		t.Fatalf("pairing result ok = %#v, want false", result["ok"])
	}
	if message := pairingErrorMessage(result); message != "invalid message format" {
		t.Fatalf("error message = %q, want invalid message format", message)
	}
}

func TestHandlePairingWebSocketReturnsErrorWhenSessionMissing(t *testing.T) {
	_, restoreGlobals := setupPairingHandlerTest(t)
	defer restoreGlobals()

	clientConn, cleanupConn := openPairingHandlerConn(t)
	defer cleanupConn()

	sendPairingClaim(t, clientConn, "pair_missing", "")

	result := readPairingMessage(t, clientConn)
	if ok, _ := result["ok"].(bool); ok {
		t.Fatalf("pairing result ok = %#v, want false", result["ok"])
	}
	if message := pairingErrorMessage(result); message != "pairing session not found" {
		t.Fatalf("error message = %q, want pairing session not found", message)
	}
}
