package gobridge

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
)

// ─── Relay 管理 RPC ──────────────────────────────────────────────────────
//
// 方案 §10.2 / §13.3：
//   MacBridge Remote Access 页面增加 Relay 启用/关闭、endpoint、状态展示。
//   RuntimeConfig 传递 relay 配置给 go-bridge。
//   状态页区分 Direct/Relay/Offline。

// RelayManagement 管理 relay 的 runtime 配置和状态。
type RelayManagement struct {
	mu     sync.RWMutex
	config RelayConfigSnapshot
	status RelayRuntimeStatus
}

// RelayConfigSnapshot relay 配置快照。
type RelayConfigSnapshot struct {
	Enabled     bool   `json:"enabled"`
	Endpoint    string `json:"endpoint"`
	RouteID     string `json:"routeId,omitempty"`
	BridgeAuth  string `json:"-"` // 不序列化
	Connected   bool   `json:"connected"`
	LastConnect string `json:"lastConnect,omitempty"`
	LastError   string `json:"lastError,omitempty"`
}

// RelayRuntimeStatus relay 运行时状态。
type RelayRuntimeStatus struct {
	ConnectionMode ConnectionMode `json:"connectionMode"`
	IsEncrypted    bool           `json:"isEncrypted"`
	Uptime         string         `json:"uptime,omitempty"`
	DevicesOnline  int            `json:"devicesOnline"`
	MailboxPending int            `json:"mailboxPending"`
}

// NewRelayManagement 创建 relay 管理器。
func NewRelayManagement() *RelayManagement {
	return &RelayManagement{
		config: RelayConfigSnapshot{
			Connected: false,
		},
		status: RelayRuntimeStatus{
			ConnectionMode: ConnectionModeDisconnected,
		},
	}
}

// HandleSetRelayEnabled RPC handler: 设置 relay 启用状态。
func (rm *RelayManagement) HandleSetRelayEnabled(params json.RawMessage) (interface{}, error) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	wasEnabled := rm.config.Enabled
	rm.config.Enabled = req.Enabled

	if !req.Enabled {
		rm.config.Connected = false
		rm.status.ConnectionMode = ConnectionModeDisconnected
	}

	slog.Info("relay-management: enabled changed",
		"was", wasEnabled,
		"now", req.Enabled,
	)

	return map[string]interface{}{
		"ok":      true,
		"enabled": rm.config.Enabled,
	}, nil
}

// HandleSetRelayEndpoint RPC handler: 设置 relay endpoint。
func (rm *RelayManagement) HandleSetRelayEndpoint(params json.RawMessage) (interface{}, error) {
	var req struct {
		Endpoint string `json:"endpoint"`
	}
	if err := json.Unmarshal(params, &req); err != nil {
		return nil, fmt.Errorf("invalid params: %w", err)
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.config.Endpoint = req.Endpoint

	slog.Info("relay-management: endpoint changed", "endpoint", req.Endpoint)

	return map[string]interface{}{
		"ok":       true,
		"endpoint": rm.config.Endpoint,
	}, nil
}

// HandleGetRelayStatus RPC handler: 获取 relay 状态。
func (rm *RelayManagement) HandleGetRelayStatus() (interface{}, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return map[string]interface{}{
		"config":  rm.config,
		"runtime": rm.status,
		"display": ConnectionModeDisplayText(rm.status.ConnectionMode),
	}, nil
}

// HandleInitRelayPairing RPC handler: 启动配对流程。
func (rm *RelayManagement) HandleInitRelayPairing(identity *RelayCryptoIdentity) (interface{}, error) {
	if identity == nil {
		return nil, fmt.Errorf("relay identity not initialized")
	}

	rm.mu.RLock()
	endpoint := rm.config.Endpoint
	routeID := rm.config.RouteID
	rm.mu.RUnlock()

	if endpoint == "" {
		return nil, fmt.Errorf("relay endpoint not configured")
	}

	qr, err := GeneratePairingQR(routeID, identity.PublicKeyBytes(), endpoint)
	if err != nil {
		return nil, fmt.Errorf("generate pairing QR: %w", err)
	}

	slog.Info("relay-management: pairing initiated", "routeID", safeID(routeID))

	return map[string]interface{}{
		"ok":          true,
		"qr":          qr,
		"fingerprint": identity.Fingerprint(),
	}, nil
}

// HandleProcessRelayClaim RPC handler: 处理设备配对请求。
func (rm *RelayManagement) HandleProcessRelayClaim(
	params json.RawMessage,
	identity *RelayCryptoIdentity,
	deviceStore *TrustedDeviceStore,
) (interface{}, error) {
	var claim PairingClaim
	if err := json.Unmarshal(params, &claim); err != nil {
		return nil, fmt.Errorf("invalid claim: %w", err)
	}

	if identity == nil {
		return nil, fmt.Errorf("relay identity not initialized")
	}

	approve, err := ProcessPairingClaim(&claim, identity.PrivateKeyBytes())
	if err != nil {
		return nil, fmt.Errorf("process pairing claim: %w", err)
	}

	if approve.Approved {
		// 注册设备到 device store
		// （实际实现需要调用 TrustedDeviceStore 的设备注册方法）
		slog.Info("relay-management: device paired",
			"deviceID", safeID(approve.DeviceID),
			"hasAuth", approve.DeviceAuth != "",
		)
	}

	return approve, nil
}

// UpdateConnectionStatus 更新连接状态（内部调用）。
func (rm *RelayManagement) UpdateConnectionStatus(mode ConnectionMode, encrypted bool) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	rm.config.Connected = (mode != ConnectionModeDisconnected)
	rm.status.ConnectionMode = mode
	rm.status.IsEncrypted = encrypted
}

// GetConfig 返回配置快照。
func (rm *RelayManagement) GetConfig() RelayConfigSnapshot {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.config
}

// RelayManagementRPC 是 relay 管理 RPC 的路由注册。
// 返回 method -> handler 函数的映射。
func RelayManagementRPC(rm *RelayManagement, identity *RelayCryptoIdentity, deviceStore *TrustedDeviceStore) map[string]func(json.RawMessage) (interface{}, error) {
	return map[string]func(json.RawMessage) (interface{}, error){
		"set_relay_enabled":  rm.HandleSetRelayEnabled,
		"set_relay_endpoint": rm.HandleSetRelayEndpoint,
		"get_relay_status":   func(_ json.RawMessage) (interface{}, error) { return rm.HandleGetRelayStatus() },
		"init_relay_pairing": func(_ json.RawMessage) (interface{}, error) { return rm.HandleInitRelayPairing(identity) },
		"process_relay_claim": func(params json.RawMessage) (interface{}, error) {
			return rm.HandleProcessRelayClaim(params, identity, deviceStore)
		},
	}
}
