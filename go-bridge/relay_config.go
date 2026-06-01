package gobridge

import (
	"sync"
	"time"
)

// RelayConfig 管理 relay 连接配置。
// 方案 §10.1：relay endpoint、route credential、enabled 状态。
type RelayConfig struct {
	mu      sync.RWMutex
	enabled bool

	// Relay 服务端点（wss://relay.example.com）
	endpoint string

	// Mac bridge route ID（由 relay 服务分配）
	routeID string

	// Opaque relay 认证凭据（不复用 device token）
	credential string

	// 当前连接状态
	connected   bool
	lastConnect time.Time
	lastError   string
}

// NewRelayConfig 创建默认 relay 配置。
func NewRelayConfig() *RelayConfig {
	return &RelayConfig{}
}

// Enabled 返回 relay 是否启用。
func (rc *RelayConfig) Enabled() bool {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.enabled
}

// SetEnabled 设置 relay 启用状态。
func (rc *RelayConfig) SetEnabled(enabled bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.enabled = enabled
}

// Endpoint 返回 relay 服务端点。
func (rc *RelayConfig) Endpoint() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.endpoint
}

// SetEndpoint 设置 relay 服务端点。
func (rc *RelayConfig) SetEndpoint(endpoint string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.endpoint = endpoint
}

// RouteID 返回当前 route ID。
func (rc *RelayConfig) RouteID() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.routeID
}

// SetRouteID 设置 route ID。
func (rc *RelayConfig) SetRouteID(routeID string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.routeID = routeID
}

// Credential 返回 relay 认证凭据。
func (rc *RelayConfig) Credential() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.credential
}

// SetCredential 设置 relay 认证凭据。
func (rc *RelayConfig) SetCredential(cred string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.credential = cred
}

// Connected 返回当前连接状态。
func (rc *RelayConfig) Connected() bool {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.connected
}

// SetConnected 设置连接状态。
func (rc *RelayConfig) SetConnected(connected bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.connected = connected
	if connected {
		rc.lastConnect = time.Now()
		rc.lastError = ""
	}
}

// Status 返回 relay 状态摘要。
func (rc *RelayConfig) Status() RelayStatus {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return RelayStatus{
		Enabled:     rc.enabled,
		Endpoint:    rc.endpoint,
		RouteID:     rc.routeID,
		Connected:   rc.connected,
		LastConnect: rc.lastConnect,
		LastError:   rc.lastError,
	}
}

// RelayStatus 是 relay 连接状态的快照。
type RelayStatus struct {
	Enabled     bool      `json:"enabled"`
	Endpoint    string    `json:"endpoint,omitempty"`
	RouteID     string    `json:"routeId,omitempty"`
	Connected   bool      `json:"connected"`
	LastConnect time.Time `json:"lastConnect,omitempty"`
	LastError   string    `json:"lastError,omitempty"`
}

// HasRelayCapability 检查是否具备 relay 的基本配置条件。
func (rc *RelayConfig) HasRelayCapability() bool {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	return rc.enabled && rc.endpoint != ""
}
