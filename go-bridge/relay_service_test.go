package gobridge

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestRelayServiceRoutesOpaqueFramesOverWebSockets(t *testing.T) {
	hub := NewRelayHub()
	server := httptest.NewServer(NewRelayService(hub))
	defer server.Close()

	routeID, bridgeAuth := registerRelayRoute(t, server.URL)
	deviceAuth := registerRelayDevice(t, server.URL, routeID, bridgeAuth, "dev-online")

	bridge := dialRelaySocket(t, server.URL, "/v1/routes/"+routeID+"/bridge", bridgeAuth)
	defer bridge.Close()
	device := dialRelaySocket(t, server.URL, "/v1/routes/"+routeID+"/devices/dev-online", deviceAuth)
	defer device.Close()

	macEnvelope := testEnvelope(routeID, "bridge", "dev-online", 1)
	if err := bridge.WriteMessage(websocket.TextMessage, macEnvelope); err != nil {
		t.Fatalf("bridge write: %v", err)
	}
	assertRelayFrame(t, device, macEnvelope)

	deviceEnvelope := testEnvelope(routeID, "dev-online", "bridge", 2)
	if err := device.WriteMessage(websocket.TextMessage, deviceEnvelope); err != nil {
		t.Fatalf("device write: %v", err)
	}
	assertRelayFrame(t, bridge, deviceEnvelope)
}

func TestRelayServiceRejectsBadCredentialsBeforeUpgrade(t *testing.T) {
	server := httptest.NewServer(NewRelayService(NewRelayHub()))
	defer server.Close()

	routeID, _ := registerRelayRoute(t, server.URL)
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/v1/routes/" + routeID + "/bridge"
	header := http.Header{"Authorization": []string{"Bearer wrong"}}
	conn, response, err := websocket.DefaultDialer.Dial(url, header)
	if conn != nil {
		conn.Close()
	}
	if err == nil || response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("dial response = %#v error = %v, want HTTP 401", response, err)
	}
}

func TestRelayServiceRejectsDeviceWritesAfterBridgeDisconnects(t *testing.T) {
	hub := NewRelayHub()
	server := httptest.NewServer(NewRelayService(hub))
	defer server.Close()

	routeID, bridgeAuth := registerRelayRoute(t, server.URL)
	deviceAuth := registerRelayDevice(t, server.URL, routeID, bridgeAuth, "dev-offline")
	bridge := dialRelaySocket(t, server.URL, "/v1/routes/"+routeID+"/bridge", bridgeAuth)
	device := dialRelaySocket(t, server.URL, "/v1/routes/"+routeID+"/devices/dev-offline", deviceAuth)
	defer device.Close()

	bridge.Close()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		online, _, _, _ := hub.RouteStatus(routeID)
		if !online {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if online, _, _, _ := hub.RouteStatus(routeID); online {
		t.Fatal("route remains online after bridge disconnect")
	}

	if err := device.WriteMessage(websocket.TextMessage, testEnvelope(routeID, "dev-offline", "bridge", 1)); err != nil {
		t.Fatalf("device write before rejection: %v", err)
	}
	_ = device.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, err := device.ReadMessage()
	if err == nil {
		t.Fatal("device write should close when bridge is offline")
	}
}

func registerRelayRoute(t *testing.T, baseURL string) (string, string) {
	t.Helper()
	response, err := http.Post(baseURL+"/v1/routes/register", "application/json", nil)
	if err != nil {
		t.Fatalf("register route: %v", err)
	}
	defer response.Body.Close()
	var payload struct {
		RouteID    string `json:"routeId"`
		BridgeAuth string `json:"bridgeAuth"`
	}
	if response.StatusCode != http.StatusCreated || json.NewDecoder(response.Body).Decode(&payload) != nil {
		t.Fatalf("register route response status = %d", response.StatusCode)
	}
	return payload.RouteID, payload.BridgeAuth
}

func registerRelayDevice(t *testing.T, baseURL, routeID, bridgeAuth, deviceID string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"deviceId": deviceID})
	request, err := http.NewRequest(http.MethodPost, baseURL+"/v1/routes/"+routeID+"/devices/register", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+bridgeAuth)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("register device: %v", err)
	}
	defer response.Body.Close()
	var payload struct {
		DeviceAuth string `json:"deviceAuth"`
	}
	if response.StatusCode != http.StatusCreated || json.NewDecoder(response.Body).Decode(&payload) != nil {
		t.Fatalf("register device response status = %d", response.StatusCode)
	}
	return payload.DeviceAuth
}

func dialRelaySocket(t *testing.T, baseURL, path, credential string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(baseURL, "http") + path
	header := http.Header{"Authorization": []string{"Bearer " + credential}}
	conn, response, err := websocket.DefaultDialer.Dial(url, header)
	if err != nil {
		t.Fatalf("dial %s response=%#v: %v", path, response, err)
	}
	return conn
}

func assertRelayFrame(t *testing.T, conn *websocket.Conn, want []byte) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, got, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read relay frame: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("relay frame changed:\n got: %s\nwant: %s", got, want)
	}
}
