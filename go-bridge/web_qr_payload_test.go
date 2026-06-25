package gobridge

import (
	"crypto/ecdh"
	"crypto/rand"
	"net/url"
	"strings"
	"testing"
	"time"
)

// relayTestIdentity builds a fresh X25519 RelayCryptoIdentity for tests (mirrors relay_upgrade_test.go).
func relayTestIdentity(t *testing.T) *RelayCryptoIdentity {
	t.Helper()
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &RelayCryptoIdentity{privateKey: privateKey, publicKey: privateKey.PublicKey()}
}

// TestWebQRPayloadFormat asserts the Flow C web QR: same pairing session re-encoded as an
// https URL the phone's system camera opens. See docs/protocol/relay-v1.md (web pairing QR).
func TestWebQRPayloadFormat(t *testing.T) {
	const relayEndpoint = "wss://relay.example.com:8443"
	const routeID = "route_abc"
	session := NewPairingSessionWithRemoteURLs("bridge-1", "My Bridge", "ws://192.168.1.5:8777", nil, 5*time.Minute)
	identity := relayTestIdentity(t)

	if err := addRelayFirstPairingPayload(session, relayEndpoint, routeID, identity); err != nil {
		t.Fatalf("addRelayFirstPairingPayload: %v", err)
	}

	if session.WebQRPayload == "" {
		t.Fatal("WebQRPayload empty after relay payload added")
	}

	// Must be an https URL at <relay-host>/web/?...
	parsed, err := url.Parse(session.WebQRPayload)
	if err != nil {
		t.Fatalf("WebQRPayload not a valid URL: %v (payload=%q)", err, session.WebQRPayload)
	}
	if parsed.Scheme != "https" {
		t.Errorf("scheme = %q, want https (payload=%q)", parsed.Scheme, session.WebQRPayload)
	}
	if parsed.Host != "relay.example.com:8443" {
		t.Errorf("host = %q, want relay.example.com:8443", parsed.Host)
	}
	if !strings.HasPrefix(parsed.Path, "/web") {
		t.Errorf("path = %q, want /web/...", parsed.Path)
	}

	q := parsed.Query()
	for _, key := range []string{"id", "code", "relay", "relayRoute", "relayBridgeKey", "relayFingerprint", "relayCapability"} {
		if q.Get(key) == "" {
			t.Errorf("missing/empty web QR param %q (payload=%q)", key, session.WebQRPayload)
		}
	}
	if q.Get("id") != session.ID {
		t.Errorf("web QR id = %q, want session.ID %q", q.Get("id"), session.ID)
	}
	if q.Get("code") != session.ManualCode {
		t.Errorf("web QR code = %q, want ManualCode %q", q.Get("code"), session.ManualCode)
	}
	if q.Get("relay") != relayEndpoint {
		t.Errorf("web QR relay = %q, want %q", q.Get("relay"), relayEndpoint)
	}
	if q.Get("relayRoute") != routeID {
		t.Errorf("web QR relayRoute = %q, want %q", q.Get("relayRoute"), routeID)
	}
	if !strings.HasPrefix(q.Get("relayCapability"), "paircap_") {
		t.Errorf("web QR relayCapability = %q, want paircap_ prefix", q.Get("relayCapability"))
	}
	// Web is relay-only: no LAN-only host/port/name params.
	for _, unwanted := range []string{"host", "port", "name", "remote"} {
		if _, ok := q[unwanted]; ok {
			t.Errorf("web QR should not carry LAN-only param %q (payload=%q)", unwanted, session.WebQRPayload)
		}
	}

	// Same pairing session: iOS QR id/code must match the web QR id/code.
	iosQ, err := url.Parse(strings.Replace(session.QRPayload, "cccode://", "http://", 1))
	if err != nil {
		t.Fatalf("iOS QRPayload not parseable: %v", err)
	}
	if iosQ.Query().Get("id") != q.Get("id") || iosQ.Query().Get("code") != q.Get("code") {
		t.Error("iOS QR and web QR must share id+code (same pairing session)")
	}
}

// TestWebQRPayloadAbsentWithoutRelay: when relay is not configured, WebQRPayload stays empty
// (the session is LAN-only; no web QR to offer).
func TestWebQRPayloadAbsentWithoutRelay(t *testing.T) {
	session := NewPairingSessionWithRemoteURLs("bridge-1", "My Bridge", "ws://192.168.1.5:8777", nil, 5*time.Minute)
	// addRelayFirstPairingPayload is only called when relay is configured (management_api.go guard);
	// so a LAN-only session never gets a WebQRPayload.
	if session.WebQRPayload != "" {
		t.Errorf("LAN-only session should have empty WebQRPayload, got %q", session.WebQRPayload)
	}
}
