package gobridge

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func dialTestWebSocket(t *testing.T, handler http.Handler) (*websocket.Conn, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		server.Close()
		t.Fatalf("dial websocket: %v", err)
	}
	return conn, func() {
		_ = conn.Close()
		server.Close()
	}
}

func assertInboundFrameTooLarge(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	oversized := bytes.Repeat([]byte("x"), int(maxInboundFrameBytes)+1)
	if err := conn.WriteMessage(websocket.TextMessage, oversized); err != nil {
		t.Fatalf("write oversized frame: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, _, err := conn.ReadMessage()
	if err == nil {
		t.Fatal("oversized frame connection remained open")
	}
	if !websocket.IsCloseError(err, websocket.CloseMessageTooBig) {
		t.Fatalf("read error = %v, want close code %d", err, websocket.CloseMessageTooBig)
	}
}

func TestBridgeRejectsInboundFrameOverOneMiBAndKeepsServing(t *testing.T) {
	server := NewServer(NewHandlers())
	oversizedConn, cleanupOversized := dialTestWebSocket(t, server)
	defer cleanupOversized()
	assertInboundFrameTooLarge(t, oversizedConn)

	healthyConn, cleanupHealthy := dialTestWebSocket(t, server)
	defer cleanupHealthy()
	if err := healthyConn.WriteJSON(map[string]string{"type": "ping"}); err != nil {
		t.Fatalf("write healthy ping: %v", err)
	}
	if err := healthyConn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set healthy read deadline: %v", err)
	}
	var response map[string]string
	if err := healthyConn.ReadJSON(&response); err != nil {
		t.Fatalf("read healthy pong: %v", err)
	}
	if response["type"] != "pong" {
		t.Fatalf("response = %#v, want pong", response)
	}
}

func TestPairingRejectsInboundFrameOverOneMiB(t *testing.T) {
	conn, cleanup := dialTestWebSocket(t, http.HandlerFunc(handlePairingWebSocket))
	defer cleanup()
	assertInboundFrameTooLarge(t, conn)
}
