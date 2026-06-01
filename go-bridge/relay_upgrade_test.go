package gobridge

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func newRelayUpgradeIdentity(t *testing.T) *RelayCryptoIdentity {
	t.Helper()
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return &RelayCryptoIdentity{privateKey: privateKey, publicKey: privateKey.PublicKey()}
}

func relayUpgradePublicKey(t *testing.T) string {
	t.Helper()
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(privateKey.PublicKey().Bytes())
}

func TestEnableRelayPairingRequiresAuthenticatedDevice(t *testing.T) {
	handlers := NewHandlers()
	conn := &deliveryCaptureConn{}

	handlers.HandleRPC(conn, WireMessage{RequestID: "req_auth", Method: "enable_relay_pairing"})

	if conn.err == nil || conn.err.Code != "auth.required" {
		t.Fatalf("error = %#v, want auth.required", conn.err)
	}
}

func TestEnableRelayPairingFailsClosedWithoutProvisioner(t *testing.T) {
	handlers := NewHandlers()
	conn := &deliveryCaptureConn{device: &TrustedDeviceRecord{DeviceID: "dev-1"}}
	params, _ := json.Marshal(map[string]string{"identityPublicKey": relayUpgradePublicKey(t)})

	handlers.HandleRPC(conn, WireMessage{RequestID: "req_cfg", Method: "enable_relay_pairing", Params: params})

	if conn.err == nil || conn.err.Code != "relay.not_configured" {
		t.Fatalf("error = %#v, want relay.not_configured", conn.err)
	}
}

func TestEnableRelayPairingBindsAuthenticatedDeviceAndReturnsOpaqueProvision(t *testing.T) {
	store := NewMemoryDeviceStore()
	record := makeTestRecord("dev-auth")
	if err := store.AddDevice(record); err != nil {
		t.Fatal(err)
	}
	hub := NewRelayHub()
	routeID, bridgeAuth, err := hub.RegisterRoute()
	if err != nil {
		t.Fatal(err)
	}
	handlers := NewHandlers()
	identity := newRelayUpgradeIdentity(t)
	handlers.SetBridgeID("brg-auth")
	handlers.ConfigureRelayUpgrade(store, identity, relayUpgradeProvisionerForHub("wss://relay.example.com", routeID, bridgeAuth, hub))

	publicKey := relayUpgradePublicKey(t)
	params, _ := json.Marshal(map[string]string{
		"identityPublicKey": publicKey,
		"deviceId":          "dev-forged",
	})
	conn := &deliveryCaptureConn{device: &record}
	handlers.HandleRPC(conn, WireMessage{RequestID: "req_enable", Method: "enable_relay_pairing", Params: params})
	if conn.err != nil {
		t.Fatalf("enable relay error = %#v", conn.err)
	}
	response, ok := conn.data.(RelayUpgradeResponse)
	if !ok || response.RouteID != routeID || response.RelayEndpoint != "wss://relay.example.com" || response.DeviceAuth == "" || response.ChannelGeneration != 1 {
		t.Fatalf("response = %#v", conn.data)
	}
	if response.BridgeIdentityPublicKey != base64.StdEncoding.EncodeToString(identity.PublicKeyBytes()) || response.BridgeFingerprint != identity.Fingerprint() {
		t.Fatalf("bridge identity response = %#v", response)
	}
	if !hub.AuthorizeDevice(routeID, record.DeviceID, response.DeviceAuth) || hub.AuthorizeDevice(routeID, "dev-forged", response.DeviceAuth) {
		t.Fatal("relay device provision must bind the authenticated device identity")
	}
	bound, _ := store.LookupByDeviceID(record.DeviceID)
	if bound.IdentityPublicKey != publicKey || !bound.RelayEnabled || bound.RelayChannelGeneration != 1 {
		t.Fatalf("bound record = %#v", bound)
	}
}

func TestRelayUpgradeProvisionerRegistersDeviceOnExternalRelay(t *testing.T) {
	const bridgeAuth = "bridge-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/routes/route-remote/devices/register" ||
			r.Header.Get("Authorization") != "Bearer "+bridgeAuth {
			t.Fatalf("request = %s auth=%q", r.URL.Path, r.Header.Get("Authorization"))
		}
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["deviceId"] != "dev-remote" {
			t.Fatalf("deviceId = %q", body["deviceId"])
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"deviceAuth":"device-secret"}`))
	}))
	defer server.Close()

	provision, err := relayUpgradeProvisioner(server.URL, "route-remote", bridgeAuth, nil)("dev-remote")
	if err != nil {
		t.Fatal(err)
	}
	if provision.Endpoint != server.URL || provision.RouteID != "route-remote" || provision.DeviceAuth != "device-secret" {
		t.Fatalf("provision = %#v", provision)
	}
}

func TestEnableRelayPairingRejectsIdentityReplacement(t *testing.T) {
	store := NewMemoryDeviceStore()
	record := makeTestRecord("dev-bound")
	_ = store.AddDevice(record)
	_ = store.EnableRelay(record.DeviceID, relayUpgradePublicKey(t), 1)
	handlers := NewHandlers()
	handlers.SetBridgeID("brg-bound")
	provisionCalls := 0
	handlers.ConfigureRelayUpgrade(store, newRelayUpgradeIdentity(t), func(string) (RelayUpgradeProvision, error) {
		provisionCalls++
		return RelayUpgradeProvision{Endpoint: "wss://relay.example.com", RouteID: "route", DeviceAuth: "auth"}, nil
	})

	params, _ := json.Marshal(map[string]string{"identityPublicKey": relayUpgradePublicKey(t)})
	conn := &deliveryCaptureConn{device: &record}
	handlers.HandleRPC(conn, WireMessage{RequestID: "req_rebind", Method: "enable_relay_pairing", Params: params})

	if conn.err == nil || conn.err.Code != "relay.identity_binding_failed" {
		t.Fatalf("error = %#v, want relay.identity_binding_failed", conn.err)
	}
	if provisionCalls != 0 {
		t.Fatalf("identity replacement provision calls = %d, want 0", provisionCalls)
	}
}

func TestEnableRelayPairingSerializesConcurrentIdentityBinding(t *testing.T) {
	store := NewMemoryDeviceStore()
	record := makeTestRecord("dev-race")
	if err := store.AddDevice(record); err != nil {
		t.Fatal(err)
	}
	handlers := NewHandlers()
	handlers.SetBridgeID("brg-race")
	provisionCalls := 0
	handlers.ConfigureRelayUpgrade(store, newRelayUpgradeIdentity(t), func(string) (RelayUpgradeProvision, error) {
		provisionCalls++
		return RelayUpgradeProvision{Endpoint: "wss://relay.example.com", RouteID: "route", DeviceAuth: "auth"}, nil
	})

	keys := []string{relayUpgradePublicKey(t), relayUpgradePublicKey(t)}
	conns := []*deliveryCaptureConn{{device: &record}, {device: &record}}
	var wg sync.WaitGroup
	for i := range conns {
		params, _ := json.Marshal(map[string]string{"identityPublicKey": keys[i]})
		wg.Add(1)
		go func(index int, raw json.RawMessage) {
			defer wg.Done()
			handlers.HandleRPC(conns[index], WireMessage{RequestID: "req_race", Method: "enable_relay_pairing", Params: raw})
		}(i, params)
	}
	wg.Wait()

	successes := 0
	failures := 0
	for _, conn := range conns {
		if conn.err == nil {
			successes++
			continue
		}
		if conn.err.Code == "relay.identity_binding_failed" {
			failures++
		}
	}
	if successes != 1 || failures != 1 || provisionCalls != 1 {
		t.Fatalf("successes=%d failures=%d provisionCalls=%d, want 1/1/1", successes, failures, provisionCalls)
	}
	bound, err := store.LookupByDeviceID(record.DeviceID)
	if err != nil || (bound.IdentityPublicKey != keys[0] && bound.IdentityPublicKey != keys[1]) {
		t.Fatalf("bound record = %#v, err = %v", bound, err)
	}
}
