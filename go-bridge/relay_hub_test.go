package gobridge

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// ── Hub 基础操作 ─────────────────────────────────────────────────────────────

func TestRelayHubRegisterRoute(t *testing.T) {
	hub := NewRelayHub()
	routeID, auth, err := hub.RegisterRoute()
	if err != nil {
		t.Fatalf("register route: %v", err)
	}
	if routeID == "" {
		t.Fatal("route ID should not be empty")
	}
	if auth == "" {
		t.Fatal("auth should not be empty")
	}
}

func TestRelayHubRegisterDevice(t *testing.T) {
	hub := NewRelayHub()
	routeID, _, _ := hub.RegisterRoute()
	deviceAuth, err := hub.RegisterDevice(routeID, "dev-1")
	if err != nil {
		t.Fatalf("register device: %v", err)
	}
	if deviceAuth == "" {
		t.Fatal("device auth should not be empty")
	}
}

func TestRelayHubRegisterDeviceBadRoute(t *testing.T) {
	hub := NewRelayHub()
	_, err := hub.RegisterDevice("bad-route", "dev-1")
	if err == nil {
		t.Fatal("should fail for bad route")
	}
}

// ── Bridge 连接 ──────────────────────────────────────────────────────────────

func TestRelayHubConnectBridge(t *testing.T) {
	hub := NewRelayHub()
	routeID, auth, _ := hub.RegisterRoute()

	_, srvWS := setupWS(t)
	defer srvWS.Close()

	err := hub.ConnectBridge(routeID, auth, srvWS)
	if err != nil {
		t.Fatalf("connect bridge: %v", err)
	}

	online, _, _, ok := hub.RouteStatus(routeID)
	if !ok || !online {
		t.Fatal("route should be online")
	}
}

func TestRelayHubConnectBridgeBadAuth(t *testing.T) {
	hub := NewRelayHub()
	routeID, _, _ := hub.RegisterRoute()

	_, srvWS := setupWS(t)
	defer srvWS.Close()

	err := hub.ConnectBridge(routeID, "bad-auth", srvWS)
	if err == nil {
		t.Fatal("should reject bad auth")
	}
}

// ── Device 连接 ──────────────────────────────────────────────────────────────

func TestRelayHubConnectDevice(t *testing.T) {
	hub := NewRelayHub()
	routeID, bridgeAuth, _ := hub.RegisterRoute()
	deviceAuth, _ := hub.RegisterDevice(routeID, "dev-1")

	_, bridgeWS := setupWS(t)
	hub.ConnectBridge(routeID, bridgeAuth, bridgeWS)

	_, deviceWS := setupWS(t)
	err := hub.ConnectDevice(routeID, "dev-1", deviceAuth, deviceWS)
	if err != nil {
		t.Fatalf("connect device: %v", err)
	}
}

func TestRelayHubConnectDeviceBadAuth(t *testing.T) {
	hub := NewRelayHub()
	routeID, bridgeAuth, _ := hub.RegisterRoute()
	hub.RegisterDevice(routeID, "dev-1")

	_, bridgeWS := setupWS(t)
	hub.ConnectBridge(routeID, bridgeAuth, bridgeWS)

	_, deviceWS := setupWS(t)
	err := hub.ConnectDevice(routeID, "dev-1", "bad-auth", deviceWS)
	if err == nil {
		t.Fatal("should reject bad device auth")
	}
}

// ── Mailbox ──────────────────────────────────────────────────────────────────

func TestRelayHubMailboxEnqueueAndFetch(t *testing.T) {
	hub := NewRelayHub()
	routeID, _, _ := hub.RegisterRoute()
	hub.RegisterDevice(routeID, "dev-1")

	for i := 1; i <= 3; i++ {
		hub.ForwardFromBridge(routeID, testEnvelope(routeID, "bridge", "dev-1", uint64(i)))
	}

	frames, err := hub.FetchMailbox(routeID, "dev-1", 0)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}
}

func TestRelayHubMailboxAck(t *testing.T) {
	hub := NewRelayHub()
	routeID, _, _ := hub.RegisterRoute()
	hub.RegisterDevice(routeID, "dev-1")

	for i := 1; i <= 3; i++ {
		hub.ForwardFromBridge(routeID, testEnvelope(routeID, "bridge", "dev-1", uint64(i)))
	}

	err := hub.AckMailbox(routeID, "dev-1", 2)
	if err != nil {
		t.Fatalf("ack: %v", err)
	}

	frames, _ := hub.FetchMailbox(routeID, "dev-1", 0)
	if len(frames) != 1 {
		t.Fatalf("expected 1 unacked frame after ack, got %d", len(frames))
	}
}

func TestRelayHubMailboxFetchAfterCursor(t *testing.T) {
	hub := NewRelayHub()
	routeID, _, _ := hub.RegisterRoute()
	hub.RegisterDevice(routeID, "dev-1")

	for i := 1; i <= 5; i++ {
		hub.ForwardFromBridge(routeID, testEnvelope(routeID, "bridge", "dev-1", uint64(i)))
	}

	frames, _ := hub.FetchMailbox(routeID, "dev-1", 3)
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames after cursor 3, got %d", len(frames))
	}
}

// ── 撤销 ─────────────────────────────────────────────────────────────────────

func TestRelayHubRevokeDevice(t *testing.T) {
	hub := NewRelayHub()
	routeID, _, _ := hub.RegisterRoute()
	hub.RegisterDevice(routeID, "dev-1")

	for i := 1; i <= 3; i++ {
		hub.ForwardFromBridge(routeID, testEnvelope(routeID, "bridge", "dev-1", uint64(i)))
	}

	err := hub.RevokeDevice(routeID, "dev-1")
	if err != nil {
		t.Fatalf("revoke: %v", err)
	}

	frames, _ := hub.FetchMailbox(routeID, "dev-1", 0)
	if len(frames) != 0 {
		t.Fatalf("mailbox should be empty after revoke, got %d", len(frames))
	}

	err = hub.ForwardFromBridge(routeID, testEnvelope(routeID, "bridge", "dev-1", 4))
	if err == nil {
		t.Fatal("forward to revoked device should fail")
	}
}

// ── Route status 不泄漏 payload ──────────────────────────────────────────────

func TestRelayHubRouteStatusNoPayload(t *testing.T) {
	hub := NewRelayHub()
	routeID, _, _ := hub.RegisterRoute()
	hub.RegisterDevice(routeID, "dev-1")

	hub.ForwardFromBridge(routeID, testEnvelopeWithContent(routeID, "bridge", "dev-1", 1, "secret business content"))

	online, deviceCount, mailboxSize, ok := hub.RouteStatus(routeID)
	if !ok {
		t.Fatal("route should exist")
	}
	// 只暴露聚合统计
	if online {
		t.Fatal("should be online=false when no bridge connected")
	}
	_ = deviceCount
	_ = mailboxSize
}

// ── Device offline 时 mailbox 入队 ──────────────────────────────────────────

func TestRelayHubOfflineDeviceGetsMailbox(t *testing.T) {
	hub := NewRelayHub()
	routeID, _, _ := hub.RegisterRoute()
	hub.RegisterDevice(routeID, "dev-1")

	// Device 未连接，forward 应存入 mailbox
	err := hub.ForwardFromBridge(routeID, testEnvelope(routeID, "bridge", "dev-1", 1))
	if err != nil {
		t.Fatalf("forward to offline device: %v", err)
	}

	frames, _ := hub.FetchMailbox(routeID, "dev-1", 0)
	if len(frames) != 1 {
		t.Fatalf("expected 1 mailbox frame, got %d", len(frames))
	}
}

// ── 测试工具 ─────────────────────────────────────────────────────────────────

// setupWS 创建一对 WebSocket 连接（server 端 + client 端），返回 httptest.Server 和 server 端 *websocket.Conn。
func setupWS(t *testing.T) (*httptest.Server, *websocket.Conn) {
	t.Helper()
	var srvConn *websocket.Conn
	ready := make(chan struct{})

	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
		ws, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		srvConn = ws
		close(ready)
	}))

	wsURL := "ws" + strings.TrimPrefix(s.URL, "http")
	_, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		s.Close()
		t.Fatalf("dial: %v", err)
	}

	<-ready
	return s, srvConn
}

func testEnvelope(routeID, sender, dest string, counter uint64) json.RawMessage {
	return testEnvelopeWithContent(routeID, sender, dest, counter, "encrypted-payload")
}

func testEnvelopeWithContent(routeID, sender, dest string, counter uint64, content string) json.RawMessage {
	env := RelayEnvelope{
		Version:           1,
		RouteID:           routeID,
		SenderID:          sender,
		DestinationID:     dest,
		ChannelGeneration: 1,
		KeyEpochID:        "online",
		MessageID:         fmt.Sprintf("msg-%d", counter),
		Counter:           counter,
		Ciphertext:        []byte(content),
		CreatedAt:         time.Now().Format(time.RFC3339),
		ExpiresAt:         time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	}
	data, _ := json.Marshal(env)
	return data
}
