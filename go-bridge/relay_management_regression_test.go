package gobridge

import (
	"encoding/json"
	"testing"
)

// ─── Phase 3 MacBridge Product Regression Gate ────────────────────────────
//
// 方案 §13.3 / §16.10：
//   UI 不把 relay 可达性冒充 backend 在线
//   ConnectionMode 文案区分 relay/backend 状态
//   enable/disable 失败展示诊断信息

// TestRegressionR1_RelayOnlineNotBackendOnline 验证 relay 状态不等同于 backend 在线。
func TestRegressionR1_RelayOnlineNotBackendOnline(t *testing.T) {
	rm := newTestRelayManagement()

	// Relay 连接成功，但 Mac 可能 backend 不在线
	rm.UpdateConnectionStatus(ConnectionModeRelayOffline, false)

	resp, _ := rm.HandleGetRelayStatus()
	r := resp.(map[string]interface{})
	display := r["display"].(string)

	// 文案必须明确说 "Mac Offline"，不能说 "connected" 或 "online"
	if display != "Relay - Mac Offline" {
		t.Errorf("display = %q, want 'Relay - Mac Offline'", display)
	}

	// config.Connected 应为 true（relay 可达）但 mode 是 offline
	config := r["config"].(RelayConfigSnapshot)
	if !config.Connected {
		t.Error("relay should report connected=true even when Mac offline")
	}
}

// TestRegressionR2_ConnectionModeNeverClaimsBackendStatus 验证 ConnectionMode 文案不声称 backend 状态。
func TestRegressionR2_ConnectionModeNeverClaimsBackendStatus(t *testing.T) {
	// 所有 ConnectionMode 文案不应包含 "backend" 字样
	modes := []ConnectionMode{
		ConnectionModeDirectLocal,
		ConnectionModeDirectRemote,
		ConnectionModeRelay,
		ConnectionModeRelaySyncing,
		ConnectionModeRelayOffline,
		ConnectionModeDisconnected,
	}

	for _, mode := range modes {
		text := ConnectionModeDisplayText(mode)
		if containsStr(text, "backend") || containsStr(text, "Backend") {
			t.Errorf("mode %q display text %q should not mention 'backend'", mode, text)
		}
		if containsStr(text, "online") || containsStr(text, "Online") {
			t.Errorf("mode %q display text %q should not claim 'online'", mode, text)
		}
	}
}

// TestRegressionR3_DisableRelayDisconnects 验证禁用 relay 时断开连接。
func TestRegressionR3_DisableRelayDisconnects(t *testing.T) {
	rm := newTestRelayManagement()

	// 启用并连接
	rm.HandleSetRelayEnabled(json.RawMessage(`{"enabled": true}`))
	rm.UpdateConnectionStatus(ConnectionModeRelay, true)

	cfg := rm.GetConfig()
	if !cfg.Connected {
		t.Fatal("should be connected")
	}

	// 禁用
	rm.HandleSetRelayEnabled(json.RawMessage(`{"enabled": false}`))

	cfg = rm.GetConfig()
	if cfg.Enabled {
		t.Error("should be disabled")
	}
	if cfg.Connected {
		t.Error("disabling should disconnect")
	}

	// 状态应为 Disconnected
	resp, _ := rm.HandleGetRelayStatus()
	r := resp.(map[string]interface{})
	if r["display"] != "Disconnected" {
		t.Errorf("display = %q, want 'Disconnected'", r["display"])
	}
}

// TestRegressionR4_EndpointConfigPersisted 验证 endpoint 配置持久化。
func TestRegressionR4_EndpointConfigPersisted(t *testing.T) {
	rm := newTestRelayManagement()

	rm.HandleSetRelayEndpoint(json.RawMessage(`{"endpoint": "wss://relay.example.com"}`))
	rm.HandleSetRelayEnabled(json.RawMessage(`{"enabled": false}`))

	// endpoint 在禁用后仍保留
	cfg := rm.GetConfig()
	if cfg.Endpoint != "wss://relay.example.com" {
		t.Errorf("endpoint = %q, should persist after disable", cfg.Endpoint)
	}

	// 重新启用后 endpoint 仍在
	rm.HandleSetRelayEnabled(json.RawMessage(`{"enabled": true}`))
	cfg = rm.GetConfig()
	if cfg.Endpoint != "wss://relay.example.com" {
		t.Errorf("endpoint = %q, should persist across enable/disable cycles", cfg.Endpoint)
	}
}

// TestRegressionR5_StatusExposesDiagnostics 验证状态暴露诊断信息。
func TestRegressionR5_StatusExposesDiagnostics(t *testing.T) {
	rm := newTestRelayManagement()

	rm.HandleSetRelayEnabled(json.RawMessage(`{"enabled": true}`))
	rm.HandleSetRelayEndpoint(json.RawMessage(`{"endpoint": "wss://relay.example.com"}`))
	rm.UpdateConnectionStatus(ConnectionModeRelaySyncing, true)

	resp, _ := rm.HandleGetRelayStatus()
	r := resp.(map[string]interface{})

	// 应包含 display 文案
	if r["display"] == nil {
		t.Error("should expose display text")
	}

	// 应包含 config（endpoint/enabled 等）
	if r["config"] == nil {
		t.Error("should expose config")
	}

	// 应包含 runtime 状态
	if r["runtime"] == nil {
		t.Error("should expose runtime status")
	}

	// config 不应暴露 bridgeAuth
	config := r["config"].(RelayConfigSnapshot)
	if config.BridgeAuth != "" {
		t.Error("config should not expose bridgeAuth")
	}
}
