package gobridge

import (
	"encoding/json"
	"testing"
)

// ─── Relay Management RPC 测试 ───────────────────────────────────────────

func newTestRelayManagement() *RelayManagement {
	return NewRelayManagement()
}

func TestRelaySetEnabled(t *testing.T) {
	rm := newTestRelayManagement()

	// 初始状态：disabled
	cfg := rm.GetConfig()
	if cfg.Enabled {
		t.Error("should start disabled")
	}

	// 启用
	resp, err := rm.HandleSetRelayEnabled(json.RawMessage(`{"enabled": true}`))
	if err != nil {
		t.Fatal(err)
	}
	r := resp.(map[string]interface{})
	if !r["enabled"].(bool) {
		t.Error("should be enabled")
	}

	// 禁用
	resp, err = rm.HandleSetRelayEnabled(json.RawMessage(`{"enabled": false}`))
	if err != nil {
		t.Fatal(err)
	}
	r = resp.(map[string]interface{})
	if r["enabled"].(bool) {
		t.Error("should be disabled")
	}

	cfg = rm.GetConfig()
	if cfg.Connected {
		t.Error("disabling should disconnect")
	}
}

func TestRelaySetEndpoint(t *testing.T) {
	rm := newTestRelayManagement()

	resp, err := rm.HandleSetRelayEndpoint(json.RawMessage(`{"endpoint": "wss://relay.example.com"}`))
	if err != nil {
		t.Fatal(err)
	}
	r := resp.(map[string]interface{})
	if r["endpoint"] != "wss://relay.example.com" {
		t.Errorf("endpoint = %v", r["endpoint"])
	}

	cfg := rm.GetConfig()
	if cfg.Endpoint != "wss://relay.example.com" {
		t.Errorf("config endpoint = %q", cfg.Endpoint)
	}
}

func TestRelayGetStatus(t *testing.T) {
	rm := newTestRelayManagement()

	rm.HandleSetRelayEnabled(json.RawMessage(`{"enabled": true}`))
	rm.HandleSetRelayEndpoint(json.RawMessage(`{"endpoint": "wss://relay.example.com"}`))
	rm.UpdateConnectionStatus(ConnectionModeRelay, true)

	resp, err := rm.HandleGetRelayStatus()
	if err != nil {
		t.Fatal(err)
	}
	r := resp.(map[string]interface{})
	if r["display"] != "Relay - Encrypted" {
		t.Errorf("display = %v", r["display"])
	}

	config := r["config"].(RelayConfigSnapshot)
	if !config.Enabled {
		t.Error("should be enabled")
	}
	if !config.Connected {
		t.Error("should be connected")
	}
}

func TestRelayStatusDisplayModes(t *testing.T) {
	rm := newTestRelayManagement()

	tests := []struct {
		mode      ConnectionMode
		encrypted bool
		display   string
	}{
		{ConnectionModeDirectLocal, false, "Direct - Local"},
		{ConnectionModeDirectRemote, true, "Direct - Remote"},
		{ConnectionModeRelay, true, "Relay - Encrypted"},
		{ConnectionModeRelaySyncing, true, "Relay - Syncing"},
		{ConnectionModeRelayOffline, false, "Relay - Mac Offline"},
		{ConnectionModeDisconnected, false, "Disconnected"},
	}

	for _, tt := range tests {
		rm.UpdateConnectionStatus(tt.mode, tt.encrypted)
		resp, _ := rm.HandleGetRelayStatus()
		r := resp.(map[string]interface{})
		if r["display"] != tt.display {
			t.Errorf("mode=%v display=%q, want %q", tt.mode, r["display"], tt.display)
		}
	}
}

func TestRelayInvalidParams(t *testing.T) {
	rm := newTestRelayManagement()

	_, err := rm.HandleSetRelayEnabled(json.RawMessage(`invalid`))
	if err == nil {
		t.Error("should reject invalid JSON")
	}

	_, err = rm.HandleSetRelayEndpoint(json.RawMessage(`invalid`))
	if err == nil {
		t.Error("should reject invalid JSON")
	}
}

func TestRelayManagementRPCRegistration(t *testing.T) {
	rm := newTestRelayManagement()
	rpcs := RelayManagementRPC(rm, nil, nil)

	expected := []string{
		"set_relay_enabled",
		"set_relay_endpoint",
		"get_relay_status",
		"init_relay_pairing",
		"process_relay_claim",
	}

	for _, method := range expected {
		handler, ok := rpcs[method]
		if !ok {
			t.Errorf("RPC method %q not registered", method)
			continue
		}
		if handler == nil {
			t.Errorf("RPC handler for %q is nil", method)
		}
	}
}
