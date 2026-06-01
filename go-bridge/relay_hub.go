package gobridge

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// RelayHub 是在线密文转发和 mailbox 暂存的核心。
// 方案 §7：Relay 只处理外层信封，不解析内层 payload。
// 方案 §7.4：数据表不包含业务字段。
type RelayHub struct {
	mu     sync.RWMutex
	routes map[string]*relayRoute // routeID -> route
}

type relayRoute struct {
	routeID      string
	bridgeAuth   string // opaque bridge auth hash
	online       bool
	bridgeConn   *relayWSConn // Mac outbound WebSocket
	devices      map[string]*relayDevice
	mailbox      map[string][]mailboxFrame // deviceID -> frames (ordered by cursor)
	mailboxSeq   uint64                    // monotonic cursor counter
	mailboxBytes map[string]int64          // deviceID -> total bytes
}

type relayDevice struct {
	deviceID   string
	deviceAuth string // opaque device auth hash
	conn       *relayWSConn
	revoked    bool
	revokedAt  time.Time
}

type relayWSConn struct {
	ws     *websocket.Conn
	mu     sync.Mutex
	closed bool
}

type mailboxFrame struct {
	cursor     uint64
	envelope   json.RawMessage
	expiresAt  time.Time
	acked      bool
	insertedAt time.Time
	size       int
}

// safeID 返回 ID 的安全截断形式，用于日志。
func safeID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// NewRelayHub 创建空的 relay hub。
func NewRelayHub() *RelayHub {
	return &RelayHub{
		routes: make(map[string]*relayRoute),
	}
}

// RegisterRoute 注册或恢复一个 bridge route。
// 返回 routeID 和 bridge auth credential。
// 方案 §7.3 POST /v1/routes/register
func (h *RelayHub) RegisterRoute() (routeID, bridgeAuth string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	routeID = generateRelayID("rt_")
	bridgeAuth = generateRelayCredential()

	h.routes[routeID] = &relayRoute{
		routeID:      routeID,
		bridgeAuth:   hashCredential(bridgeAuth),
		online:       false,
		devices:      make(map[string]*relayDevice),
		mailbox:      make(map[string][]mailboxFrame),
		mailboxBytes: make(map[string]int64),
	}

	slog.Info("relay-hub: route registered", "routeID", safeID(routeID))
	return routeID, bridgeAuth, nil
}

// RegisterDevice 注册一个 device 到指定 route。
// 返回 deviceAuth credential。
func (h *RelayHub) RegisterDevice(routeID string, deviceID string) (deviceAuth string, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	route, ok := h.routes[routeID]
	if !ok {
		return "", fmt.Errorf("route not found: %s", safeID(routeID))
	}

	deviceAuth = generateRelayCredential()
	route.devices[deviceID] = &relayDevice{
		deviceID:   deviceID,
		deviceAuth: hashCredential(deviceAuth),
	}

	slog.Info("relay-hub: device registered", "routeID", safeID(routeID), "deviceID", safeID(deviceID))
	return deviceAuth, nil
}

// AuthorizeBridge 验证 bridge 的 opaque relay credential。
func (h *RelayHub) AuthorizeBridge(routeID, auth string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	route, ok := h.routes[routeID]
	return ok && verifyCredential(route.bridgeAuth, auth)
}

// AuthorizeDevice 验证未撤销 endpoint 的 opaque relay credential。
func (h *RelayHub) AuthorizeDevice(routeID, deviceID, auth string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	route, ok := h.routes[routeID]
	if !ok {
		return false
	}
	device, ok := route.devices[deviceID]
	return ok && !device.revoked && verifyCredential(device.deviceAuth, auth)
}

// ConnectBridge 设置 route 的 Mac outbound WebSocket。
// 方案 §7.3 WS /v1/routes/{routeId}/bridge
func (h *RelayHub) ConnectBridge(routeID string, auth string, ws *websocket.Conn) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	route, ok := h.routes[routeID]
	if !ok {
		return fmt.Errorf("route not found")
	}
	if !verifyCredential(route.bridgeAuth, auth) {
		return fmt.Errorf("bridge auth failed")
	}

	route.bridgeConn = &relayWSConn{ws: ws}
	route.online = true

	slog.Info("relay-hub: bridge connected", "routeID", safeID(routeID))
	return nil
}

// ConnectDevice 设置 device 的 WebSocket。
// 方案 §7.3 WS /v1/routes/{routeId}/devices/{deviceId}
func (h *RelayHub) ConnectDevice(routeID, deviceID, deviceAuth string, ws *websocket.Conn) error {
	h.mu.Lock()
	route, ok := h.routes[routeID]
	if !ok {
		h.mu.Unlock()
		return fmt.Errorf("route not found")
	}

	device, ok := route.devices[deviceID]
	if !ok {
		h.mu.Unlock()
		return fmt.Errorf("device not found")
	}
	if device.revoked {
		h.mu.Unlock()
		return fmt.Errorf("device revoked")
	}
	if !verifyCredential(device.deviceAuth, deviceAuth) {
		h.mu.Unlock()
		return fmt.Errorf("device auth failed")
	}

	device.conn = &relayWSConn{ws: ws}
	h.mu.Unlock()

	// 在线后立即补投 pending mailbox frames
	go h.deliverPendingMailbox(routeID, deviceID)

	slog.Info("relay-hub: device connected", "routeID", safeID(routeID), "deviceID", safeID(deviceID))
	return nil
}

// DisconnectBridge 清理当前 bridge socket；旧连接关闭不覆盖更新后的 socket。
func (h *RelayHub) DisconnectBridge(routeID string, ws *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	route, ok := h.routes[routeID]
	if !ok || route.bridgeConn == nil || route.bridgeConn.ws != ws {
		return
	}
	route.bridgeConn.closed = true
	route.bridgeConn = nil
	route.online = false
}

// DisconnectDevice 清理当前 device socket；旧连接关闭不覆盖更新后的 socket。
func (h *RelayHub) DisconnectDevice(routeID, deviceID string, ws *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	route, ok := h.routes[routeID]
	if !ok {
		return
	}
	device, ok := route.devices[deviceID]
	if !ok || device.conn == nil || device.conn.ws != ws {
		return
	}
	device.conn.closed = true
	device.conn = nil
}

// ForwardFromBridge 接收 Mac 发来的密文信封，路由到目标 device。
// 信封包含 destinationId 字段用于路由。
// 方案 §7：Relay 只路由和暂存加密信封，不读取 Bridge RPC 内容。
func (h *RelayHub) ForwardFromBridge(routeID string, envelope json.RawMessage) error {
	h.mu.RLock()
	route, ok := h.routes[routeID]
	if !ok {
		h.mu.RUnlock()
		return fmt.Errorf("route not found")
	}

	// 解析外层信封获取目标
	var env RelayEnvelope
	if err := json.Unmarshal(envelope, &env); err != nil {
		h.mu.RUnlock()
		return fmt.Errorf("invalid envelope: %w", err)
	}

	deviceID := env.DestinationID
	device, ok := route.devices[deviceID]
	if !ok {
		h.mu.RUnlock()
		return fmt.Errorf("device not found: %s", safeID(deviceID))
	}
	if device.revoked {
		h.mu.RUnlock()
		return fmt.Errorf("device revoked: %s", safeID(deviceID))
	}

	// device 在线则直接转发
	if device.conn != nil && !device.conn.closed {
		conn := device.conn
		h.mu.RUnlock()
		conn.mu.Lock()
		defer conn.mu.Unlock()
		return conn.ws.WriteMessage(websocket.TextMessage, envelope)
	}

	h.mu.RUnlock()

	// device 离线则存入 mailbox
	return h.enqueueMailbox(routeID, deviceID, envelope)
}

// ForwardFromDevice 接收 device 发来的密文信封，路由到 Mac。
func (h *RelayHub) ForwardFromDevice(routeID, deviceID string, envelope json.RawMessage) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	route, ok := h.routes[routeID]
	if !ok {
		return fmt.Errorf("route not found")
	}
	device, ok := route.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found")
	}
	if device.revoked {
		return fmt.Errorf("device revoked")
	}

	if route.bridgeConn == nil || route.bridgeConn.closed {
		return fmt.Errorf("bridge offline")
	}

	conn := route.bridgeConn
	conn.mu.Lock()
	defer conn.mu.Unlock()
	return conn.ws.WriteMessage(websocket.TextMessage, envelope)
}

// FetchMailbox 补取 device 的 pending mailbox frames。
// 方案 §7.3 GET /v1/mailbox?after=<cursor>
func (h *RelayHub) FetchMailbox(routeID, deviceID string, afterCursor uint64) ([]json.RawMessage, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	route, ok := h.routes[routeID]
	if !ok {
		return nil, fmt.Errorf("route not found")
	}

	frames := route.mailbox[deviceID]
	var result []json.RawMessage
	for _, f := range frames {
		if f.cursor > afterCursor && !f.acked {
			result = append(result, f.envelope)
		}
	}

	slog.Debug("relay-hub: mailbox fetch", "routeID", safeID(routeID), "deviceID", safeID(deviceID), "after", afterCursor, "count", len(result))
	return result, nil
}

// AckMailbox 确认已投递的 cursor。
// 方案 §7.3 POST /v1/mailbox/ack
func (h *RelayHub) AckMailbox(routeID, deviceID string, cursor uint64) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	route, ok := h.routes[routeID]
	if !ok {
		return fmt.Errorf("route not found")
	}

	frames := route.mailbox[deviceID]
	ackedBytes := int64(0)
	for i := range frames {
		if frames[i].cursor <= cursor && !frames[i].acked {
			frames[i].acked = true
			ackedBytes += int64(frames[i].size)
		}
	}
	route.mailboxBytes[deviceID] -= ackedBytes

	slog.Debug("relay-hub: mailbox ack", "routeID", safeID(routeID), "deviceID", safeID(deviceID), "cursor", cursor)
	return nil
}

// RevokeDevice 撤销 device 并清除其未投递密文。
// 方案 §7.3 POST /v1/devices/revoke
func (h *RelayHub) RevokeDevice(routeID, deviceID string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	route, ok := h.routes[routeID]
	if !ok {
		return fmt.Errorf("route not found")
	}

	device, ok := route.devices[deviceID]
	if !ok {
		return fmt.Errorf("device not found")
	}

	device.revoked = true
	now := time.Now()
	device.revokedAt = now

	// 清除 mailbox
	delete(route.mailbox, deviceID)
	delete(route.mailboxBytes, deviceID)

	// 关闭连接
	if device.conn != nil {
		device.conn.mu.Lock()
		if !device.conn.closed {
			device.conn.ws.WriteControl(
				websocket.CloseGoingAway,
				websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "device revoked"),
				time.Now().Add(time.Second),
			)
			device.conn.ws.Close()
			device.conn.closed = true
		}
		device.conn.mu.Unlock()
	}

	slog.Info("relay-hub: device revoked", "routeID", safeID(routeID), "deviceID", safeID(deviceID))
	return nil
}

// RouteStatus 返回 route 的状态摘要（仅用于 diagnostics/metrics）。
func (h *RelayHub) RouteStatus(routeID string) (online bool, deviceCount int, mailboxSize int, ok bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	route, exists := h.routes[routeID]
	if !exists {
		return false, 0, 0, false
	}

	totalFrames := 0
	for _, frames := range route.mailbox {
		totalFrames += len(frames)
	}

	return route.online, len(route.devices), totalFrames, true
}

// ── 内部方法 ─────────────────────────────────────────────────────────────────

func (h *RelayHub) enqueueMailbox(routeID, deviceID string, envelope json.RawMessage) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	route, ok := h.routes[routeID]
	if !ok {
		return fmt.Errorf("route not found")
	}

	// 容量检查：单设备 50MB
	const maxMailboxBytes = 50 * 1024 * 1024
	currentBytes := route.mailboxBytes[deviceID]
	if currentBytes+int64(len(envelope)) > maxMailboxBytes {
		// 淘汰最早的未确认 frame
		h.evictOldestUnacked(route, deviceID, int64(len(envelope)))
	}

	route.mailboxSeq++
	frame := mailboxFrame{
		cursor:     route.mailboxSeq,
		envelope:   envelope,
		expiresAt:  time.Now().Add(24 * time.Hour),
		insertedAt: time.Now(),
		size:       len(envelope),
	}

	route.mailbox[deviceID] = append(route.mailbox[deviceID], frame)
	route.mailboxBytes[deviceID] += int64(len(envelope))

	slog.Debug("relay-hub: mailbox enqueue", "routeID", safeID(routeID), "deviceID", safeID(deviceID), "cursor", frame.cursor, "size", len(envelope))
	return nil
}

func (h *RelayHub) evictOldestUnacked(route *relayRoute, deviceID string, neededBytes int64) {
	frames := route.mailbox[deviceID]
	evictedBytes := int64(0)
	for i := range frames {
		if frames[i].acked {
			continue
		}
		evictedBytes += int64(frames[i].size)
		frames[i].acked = true // 标记为已处理（实际是丢弃）
		if evictedBytes >= neededBytes {
			break
		}
	}
	route.mailboxBytes[deviceID] -= evictedBytes
}

func (h *RelayHub) deliverPendingMailbox(routeID, deviceID string) {
	// 获取所有 pending frames
	h.mu.RLock()
	route, ok := h.routes[routeID]
	if !ok {
		h.mu.RUnlock()
		return
	}
	device, ok := route.devices[deviceID]
	if !ok || device.conn == nil || device.conn.closed {
		h.mu.RUnlock()
		return
	}
	conn := device.conn
	frames := make([]mailboxFrame, len(route.mailbox[deviceID]))
	copy(frames, route.mailbox[deviceID])
	h.mu.RUnlock()

	// 按序发送
	for _, f := range frames {
		if f.acked {
			continue
		}
		conn.mu.Lock()
		err := conn.ws.WriteMessage(websocket.TextMessage, f.envelope)
		conn.mu.Unlock()
		if err != nil {
			slog.Warn("relay-hub: mailbox delivery failed", "deviceID", safeID(deviceID), "cursor", f.cursor, "error", err)
			return
		}
	}
}

// ── 工具函数 ─────────────────────────────────────────────────────────────────

func generateRelayID(prefix string) string {
	b := make([]byte, 16)
	rand.Read(b)
	return prefix + base64.RawURLEncoding.EncodeToString(b)
}

func generateRelayCredential() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashCredential(cred string) string {
	h := sha256.Sum256([]byte(cred))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

func verifyCredential(hashed, plain string) bool {
	expected := hashCredential(plain)
	return hmac.Equal([]byte(hashed), []byte(expected))
}
