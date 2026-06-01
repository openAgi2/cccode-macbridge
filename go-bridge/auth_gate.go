package gobridge

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gorilla/websocket"
)

// AuthMiddleware 封装 TrustedDeviceStore，提供 HTTP 请求级别的设备认证。
type AuthMiddleware struct {
	store TrustedDeviceStore
}

// NewAuthMiddleware 创建认证中间件。
func NewAuthMiddleware(store TrustedDeviceStore) *AuthMiddleware {
	return &AuthMiddleware{store: store}
}

// AuthenticateRequest 从 HTTP 请求中提取 Bearer token 和 Device-ID，
// 优先从 Authorization 头和 X-CCCode-Device-ID 头提取，
// 若缺失则从 URL query 参数 token 和 deviceId 提取（兼容 URLSessionWebSocketTask 丢弃 Authorization 头的情况）。
func (m *AuthMiddleware) AuthenticateRequest(r *http.Request) (*TrustedDeviceRecord, error) {
	token := extractBearerToken(r)
	deviceID := r.Header.Get("X-CCCode-Device-ID")

	// URLSessionWebSocketTask 可能丢弃 Authorization 头，从 query 参数兜底
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if deviceID == "" {
		deviceID = r.URL.Query().Get("deviceId")
	}

	return ValidateDeviceAuth(m.store, token, deviceID)
}

// IsPairingEndpoint 判断请求路径是否为配对端点（配对端点暂时跳过认证）。
func (m *AuthMiddleware) IsPairingEndpoint(r *http.Request) bool {
	return r.URL.Path == "/pairing"
}

// WriteAuthError 通过 WebSocket 发送 JSON 错误消息并关闭连接。
// 调用前必须已完成 WebSocket 升级。
func WriteAuthError(conn *websocket.Conn, code, message string) {
	errMsg := map[string]interface{}{
		"type": "error",
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	}
	data, _ := json.Marshal(errMsg)
	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		slog.Debug("auth_gate: 写入错误消息失败", "error", err)
	}
	conn.Close()
}

// extractBearerToken 从 Authorization 头提取 Bearer token。
func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(auth, "Bearer ")
}
