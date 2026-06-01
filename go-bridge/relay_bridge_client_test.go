package gobridge

import (
	"bytes"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestRelayBridgeClientBasic 验证 relay bridge client 的基本生命周期。
func TestRelayBridgeClientBasic(t *testing.T) {
	hub := NewRelayHub()
	routeID, bridgeAuth, _ := hub.RegisterRoute()
	bridgeIdentityDir := t.TempDir()
	identity, _ := LoadOrCreateRelayCryptoIdentity(bridgeIdentityDir)
	handlers := NewHandlers()

	client := NewRelayBridgeClient(handlers, hub, identity, "brg-test", routeID, bridgeAuth)

	if client.Connected() {
		t.Error("should not be connected initially")
	}
	if client.ActiveDeviceCount() != 0 {
		t.Error("should start with 0 active devices")
	}

	client.Close()

	if client.Connected() {
		t.Error("should not be connected after close")
	}
}

func TestRelayBridgeClientConnectAuthenticatesBridgeSocket(t *testing.T) {
	authSeen := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authSeen <- r.Header.Get("Authorization")
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		_, _, _ = ws.ReadMessage()
	}))
	defer server.Close()

	client := NewRelayBridgeClient(NewHandlers(), nil, nil, "brg-test", "route-test", "bridge-secret")
	if err := client.Connect("ws" + strings.TrimPrefix(server.URL, "http")); err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	select {
	case header := <-authSeen:
		if header != "Bearer bridge-secret" {
			t.Fatalf("Authorization = %q", header)
		}
	case <-time.After(time.Second):
		t.Fatal("bridge socket connection not observed")
	}
}

func TestRelayBridgeClientProcessesClientHello(t *testing.T) {
	// 设置基础设施
	hub := NewRelayHub()
	routeID, bridgeAuth, _ := hub.RegisterRoute()
	deviceID := "dev-hello-proc"
	_, _ = hub.RegisterDevice(routeID, deviceID)

	bridgeIdentityDir := t.TempDir()
	identity, _ := LoadOrCreateRelayCryptoIdentity(bridgeIdentityDir)

	store := NewMemoryDeviceStore()
	iosPriv, _ := ecdh.X25519().GenerateKey(rand.Reader)
	iosPubB64 := base64.StdEncoding.EncodeToString(iosPriv.PublicKey().Bytes())
	store.AddDevice(TrustedDeviceRecord{
		DeviceID:          deviceID,
		IdentityPublicKey: iosPubB64,
	})

	handlers := NewHandlers()
	handlers.SetBridgeID("brg-test")
	handlers.ConfigureRelayUpgrade(store, identity, nil)

	client := NewRelayBridgeClient(handlers, hub, identity, "brg-test", routeID, bridgeAuth)

	// 构造一个有效的 OnlineClientHello
	authKey, err := identity.DeriveIdentityAuthKey(iosPriv.PublicKey().Bytes(), "brg-test", deviceID)
	if err != nil {
		t.Fatalf("derive auth key: %v", err)
	}

	iosEphPriv, _ := ecdh.X25519().GenerateKey(rand.Reader)
	clientRandom := make([]byte, 32)
	rand.Read(clientRandom)

	hello := OnlineClientHello{
		Type:                  "online_client_hello",
		BridgeID:              "brg-test",
		DeviceID:              deviceID,
		ChannelGeneration:     1,
		IOSEphemeralPublicKey: base64.StdEncoding.EncodeToString(iosEphPriv.PublicKey().Bytes()),
		ClientRandom:          base64.StdEncoding.EncodeToString(clientRandom),
	}

	// 计算 auth tag
	canonical, _ := canonicalOnlineClientHello(hello)
	hello.AuthTag = base64.StdEncoding.EncodeToString(hmacSHA256(authKey, canonical))

	// 设置 mock connection，让 handleClientHello 能发送响应
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		ws, _ := upgrader.Upgrade(w, r, nil)
		// 保持连接，让 bridge client 可以写入
		time.Sleep(2 * time.Second)
		ws.Close()
	}))
	defer mockServer.Close()

	mockWSURL := "ws" + strings.TrimPrefix(mockServer.URL, "http")
	mockConn, _, err := websocket.DefaultDialer.Dial(mockWSURL, nil)
	if err != nil {
		t.Fatalf("mock dial: %v", err)
	}
	defer mockConn.Close()

	client.mu.Lock()
	client.conn = mockConn
	client.mu.Unlock()

	client.handleClientHello(hello)

	// 验证设备已注册
	if client.ActiveDeviceCount() != 1 {
		t.Errorf("active devices = %d, want 1", client.ActiveDeviceCount())
	}

	client.mu.Lock()
	relayConn := client.devices[deviceID]
	client.mu.Unlock()
	if relayConn == nil {
		t.Fatal("relay device connection was not registered")
	}
	if bytes.Equal(relayConn.macToIosKey, make([]byte, len(relayConn.macToIosKey))) ||
		bytes.Equal(relayConn.iosToMacKey, make([]byte, len(relayConn.iosToMacKey))) {
		t.Fatal("relay device connection retained destroyed handshake key material")
	}

	client.Close()
}

func TestRelayBridgeClientRejectsUnknownDevice(t *testing.T) {
	hub := NewRelayHub()
	routeID, bridgeAuth, _ := hub.RegisterRoute()
	bridgeIdentityDir := t.TempDir()
	identity, _ := LoadOrCreateRelayCryptoIdentity(bridgeIdentityDir)

	store := NewMemoryDeviceStore()
	handlers := NewHandlers()
	handlers.SetBridgeID("brg-test")
	handlers.ConfigureRelayUpgrade(store, identity, nil)

	client := NewRelayBridgeClient(handlers, hub, identity, "brg-test", routeID, bridgeAuth)

	hello := OnlineClientHello{
		Type:              "online_client_hello",
		BridgeID:          "brg-test",
		DeviceID:          "dev-nonexistent",
		ChannelGeneration: 1,
	}

	client.handleClientHello(hello)

	if client.ActiveDeviceCount() != 0 {
		t.Error("unknown device should not be registered")
	}
	client.Close()
}
