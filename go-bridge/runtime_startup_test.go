package gobridge

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReadyFrameRoundTrip(t *testing.T) {
	drivers := []string{"claude", "codex", "opencode"}
	frame := RuntimeReadyFrame{
		Type:        "runtime_ready",
		Port:        8777,
		BridgeEpoch: "12345-12345",
		Drivers:     drivers,
	}

	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal ready frame: %v", err)
	}

	parsed, err := ParseReadyFrame(data)
	if err != nil {
		t.Fatalf("parse ready frame: %v", err)
	}

	if parsed.Type != "runtime_ready" {
		t.Errorf("type = %q, want %q", parsed.Type, "runtime_ready")
	}
	if parsed.Port != 8777 {
		t.Errorf("port = %d, want %d", parsed.Port, 8777)
	}
	if parsed.BridgeEpoch != "12345-12345" {
		t.Errorf("bridgeEpoch = %q, want %q", parsed.BridgeEpoch, "12345-12345")
	}
	if len(parsed.Drivers) != 3 {
		t.Fatalf("drivers len = %d, want 3", len(parsed.Drivers))
	}
	for i, d := range drivers {
		if parsed.Drivers[i] != d {
			t.Errorf("drivers[%d] = %q, want %q", i, parsed.Drivers[i], d)
		}
	}
}

func TestReadyFrameJSONKeys(t *testing.T) {
	// 确认 camelCase JSON tag
	frame := RuntimeReadyFrame{
		Type:        "runtime_ready",
		Port:        9999,
		BridgeEpoch: "ep",
		Drivers:     []string{"x"},
	}
	data, _ := json.Marshal(frame)
	s := string(data)

	// camelCase 校验
	if !strings.Contains(s, `"type"`) {
		t.Error("missing type key")
	}
	if !strings.Contains(s, `"bridgeEpoch"`) {
		t.Error("missing bridgeEpoch key (camelCase)")
	}
	if strings.Contains(s, `"BridgeEpoch"`) {
		t.Error("found PascalCase BridgeEpoch — should be camelCase")
	}
}

func TestErrorFrameRoundTrip(t *testing.T) {
	frame := RuntimeErrorFrame{
		Type:    "runtime_error",
		Code:    RuntimeErrorPortBindFailed,
		Message: "bind: address already in use",
	}

	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("marshal error frame: %v", err)
	}

	parsed, err := ParseErrorFrame(data)
	if err != nil {
		t.Fatalf("parse error frame: %v", err)
	}

	if parsed.Type != "runtime_error" {
		t.Errorf("type = %q, want %q", parsed.Type, "runtime_error")
	}
	if parsed.Code != RuntimeErrorPortBindFailed {
		t.Errorf("code = %q, want %q", parsed.Code, RuntimeErrorPortBindFailed)
	}
	if parsed.Message != "bind: address already in use" {
		t.Errorf("message = %q, want %q", parsed.Message, "bind: address already in use")
	}
}

func TestParseReadyFrameWrongType(t *testing.T) {
	data := []byte(`{"type":"runtime_error","code":"x","message":"y"}`)
	_, err := ParseReadyFrame(data)
	if err == nil {
		t.Error("expected error for wrong type, got nil")
	}
}

func TestParseErrorFrameWrongType(t *testing.T) {
	data := []byte(`{"type":"runtime_ready","port":1}`)
	_, err := ParseErrorFrame(data)
	if err == nil {
		t.Error("expected error for wrong type, got nil")
	}
}

func TestErrorCodeConstants(t *testing.T) {
	tests := []struct {
		constant string
		want     string
	}{
		{RuntimeErrorPortBindFailed, "runtime_error.port_bind_failed"},
		{RuntimeErrorNoAgents, "runtime_error.no_agents"},
		{RuntimeErrorConfigInvalid, "runtime_error.config_invalid"},
	}
	for _, tt := range tests {
		if tt.constant != tt.want {
			t.Errorf("constant = %q, want %q", tt.constant, tt.want)
		}
	}
}

func TestReadyFrameContainsCorrectFields(t *testing.T) {
	drivers := []string{"codex"}
	frame := RuntimeReadyFrame{
		Type:        "runtime_ready",
		Port:        8080,
		BridgeEpoch: "abc",
		Drivers:     drivers,
	}
	data, _ := json.Marshal(frame)

	// 验证所有字段都出现在 JSON 中
	s := string(data)
	if !strings.Contains(s, `"port":8080`) {
		t.Errorf("JSON missing port: %s", s)
	}
	if !strings.Contains(s, `"drivers":["codex"]`) {
		t.Errorf("JSON missing drivers: %s", s)
	}
	if !strings.Contains(s, `"bridgeEpoch":"abc"`) {
		t.Errorf("JSON missing bridgeEpoch: %s", s)
	}
}
