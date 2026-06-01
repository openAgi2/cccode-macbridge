package gobridge

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// RelayService 暴露只处理外层密文信封的 Relay transport API。
// 它与 Bridge RPC server 使用独立监听地址，避免 relay 获得 inner payload 路径。
type RelayService struct {
	hub      *RelayHub
	upgrader websocket.Upgrader
}

func NewRelayService(hub *RelayHub) *RelayService {
	return &RelayService{
		hub: hub,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(*http.Request) bool { return true },
		},
	}
}

func (s *RelayService) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	parts := splitRelayPath(r.URL.Path)
	switch {
	case r.Method == http.MethodPost && len(parts) == 3 && parts[0] == "v1" && parts[1] == "routes" && parts[2] == "register":
		s.handleRegisterRoute(w)
	case r.Method == http.MethodPost && len(parts) == 5 && parts[0] == "v1" && parts[1] == "routes" && parts[3] == "devices" && parts[4] == "register":
		s.handleRegisterDevice(w, r, parts[2])
	case r.Method == http.MethodGet && len(parts) == 4 && parts[0] == "v1" && parts[1] == "routes" && parts[3] == "bridge":
		s.handleBridgeSocket(w, r, parts[2])
	case r.Method == http.MethodGet && len(parts) == 5 && parts[0] == "v1" && parts[1] == "routes" && parts[3] == "devices":
		s.handleDeviceSocket(w, r, parts[2], parts[4])
	default:
		http.NotFound(w, r)
	}
}

func (s *RelayService) handleRegisterRoute(w http.ResponseWriter) {
	routeID, bridgeAuth, err := s.hub.RegisterRoute()
	if err != nil {
		writeRelayError(w, http.StatusInternalServerError, "relay.register_failed")
		return
	}
	writeRelayJSON(w, http.StatusCreated, map[string]string{"routeId": routeID, "bridgeAuth": bridgeAuth})
}

func (s *RelayService) handleRegisterDevice(w http.ResponseWriter, r *http.Request, routeID string) {
	if !s.hub.AuthorizeBridge(routeID, bearerCredential(r)) {
		writeRelayError(w, http.StatusUnauthorized, "relay.auth_failed")
		return
	}
	var params struct {
		DeviceID string `json:"deviceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil || strings.TrimSpace(params.DeviceID) == "" {
		writeRelayError(w, http.StatusBadRequest, "relay.invalid_device")
		return
	}
	deviceAuth, err := s.hub.RegisterDevice(routeID, params.DeviceID)
	if err != nil {
		writeRelayError(w, http.StatusBadRequest, "relay.register_failed")
		return
	}
	writeRelayJSON(w, http.StatusCreated, map[string]string{"deviceId": params.DeviceID, "deviceAuth": deviceAuth})
}

func (s *RelayService) handleBridgeSocket(w http.ResponseWriter, r *http.Request, routeID string) {
	auth := bearerCredential(r)
	if !s.hub.AuthorizeBridge(routeID, auth) {
		writeRelayError(w, http.StatusUnauthorized, "relay.auth_failed")
		return
	}
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	if err := s.hub.ConnectBridge(routeID, auth, ws); err != nil {
		closeRelaySocket(ws, websocket.ClosePolicyViolation, "relay authentication failed")
		return
	}
	defer s.hub.DisconnectBridge(routeID, ws)
	s.readBridgeFrames(routeID, ws)
}

func (s *RelayService) handleDeviceSocket(w http.ResponseWriter, r *http.Request, routeID, deviceID string) {
	auth := bearerCredential(r)
	if !s.hub.AuthorizeDevice(routeID, deviceID, auth) {
		writeRelayError(w, http.StatusUnauthorized, "relay.auth_failed")
		return
	}
	ws, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	if err := s.hub.ConnectDevice(routeID, deviceID, auth, ws); err != nil {
		closeRelaySocket(ws, websocket.ClosePolicyViolation, "relay authentication failed")
		return
	}
	defer s.hub.DisconnectDevice(routeID, deviceID, ws)
	s.readDeviceFrames(routeID, deviceID, ws)
}

func (s *RelayService) readBridgeFrames(routeID string, ws *websocket.Conn) {
	for {
		_, payload, err := ws.ReadMessage()
		if err != nil {
			return
		}
		if err := s.hub.ForwardFromBridge(routeID, json.RawMessage(payload)); err != nil {
			slog.Info("relay-service: bridge frame rejected", "routeID", safeID(routeID), "error", err)
			closeRelaySocket(ws, websocket.ClosePolicyViolation, "relay frame rejected")
			return
		}
	}
}

func (s *RelayService) readDeviceFrames(routeID, deviceID string, ws *websocket.Conn) {
	for {
		_, payload, err := ws.ReadMessage()
		if err != nil {
			return
		}
		if err := s.hub.ForwardFromDevice(routeID, deviceID, json.RawMessage(payload)); err != nil {
			slog.Info("relay-service: device frame rejected", "routeID", safeID(routeID), "deviceID", safeID(deviceID), "error", err)
			closeRelaySocket(ws, websocket.CloseTryAgainLater, "relay bridge offline")
			return
		}
	}
}

func splitRelayPath(path string) []string {
	return strings.FieldsFunc(path, func(r rune) bool { return r == '/' })
}

func bearerCredential(r *http.Request) string {
	const prefix = "Bearer "
	value := r.Header.Get("Authorization")
	if strings.HasPrefix(value, prefix) {
		return strings.TrimSpace(strings.TrimPrefix(value, prefix))
	}
	return ""
}

func closeRelaySocket(ws *websocket.Conn, code int, reason string) {
	_ = ws.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, reason), noRelayDeadline())
	_ = ws.Close()
}

func noRelayDeadline() time.Time {
	return time.Now().Add(time.Second)
}

func writeRelayJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		slog.Debug("relay-service: write response failed", "error", err)
	}
}

func writeRelayError(w http.ResponseWriter, status int, code string) {
	writeRelayJSON(w, status, map[string]interface{}{
		"error": map[string]string{"code": code, "message": code},
	})
}
