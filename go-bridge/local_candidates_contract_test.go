package gobridge

import (
	"encoding/json"
	"net"
	"os"
	"strings"
	"testing"
)

// Phase A 双路径配对契约测试:RelayFirstResult.LocalURLs / HelloURLs.Locals /
// BridgeV1CurrentURLs.Locals(双结构一致)、hello_ack locals 过滤 primary、fixture 同步。
// 防 wire/persistence 字段混淆导致 iOS 静默解不出 LAN 候选(评审 P1-2 Codable 映射 + round-3 P2-1)。

func TestRelayFirstResult_LocalURLsJSONTag(t *testing.T) {
	out, err := json.Marshal(RelayFirstResult{LocalURLs: []string{"ws://192.168.1.25:8777/bridge"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"localUrls"`) {
		t.Errorf("missing localUrls key in %s", out)
	}
	if strings.Contains(string(out), `"localURLs"`) {
		t.Errorf("must NOT serialize as localURLs (iOS CodingKeys {case localURLs = \"localUrls\"} would mismatch): %s", out)
	}
	empty, _ := json.Marshal(RelayFirstResult{})
	if strings.Contains(string(empty), "localUrls") {
		t.Errorf("empty LocalURLs must be omitted (omitempty): %s", empty)
	}
}

func TestHelloURLs_LocalsJSONTag(t *testing.T) {
	out, err := json.Marshal(HelloURLs{Local: "ws://192.168.1.25:8777/bridge", Locals: []string{"ws://192.168.1.26:8777/bridge"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(out), `"locals"`) {
		t.Errorf("missing locals key in %s", out)
	}
	empty, _ := json.Marshal(HelloURLs{Local: "ws://192.168.1.25:8777/bridge"})
	if strings.Contains(string(empty), "locals") {
		t.Errorf("empty Locals must be omitted (omitempty): %s", empty)
	}
}

// round-3 P2-1:运行时 HelloURLs 与 schema BridgeV1CurrentURLs 都描述 hello_ack.bridge.currentURLs,
// Locals 必须序列化为同一 JSON 键 locals 且值一致,防 payload 与 contract 漂移。
func TestLocals_DualStructureConsistency(t *testing.T) {
	runtimeBytes, _ := json.Marshal(HelloURLs{Local: "l", Locals: []string{"a", "b"}})
	schemaBytes, _ := json.Marshal(BridgeV1CurrentURLs{Local: "l", Locals: []string{"a", "b"}})
	var rt, sc map[string]json.RawMessage
	if err := json.Unmarshal(runtimeBytes, &rt); err != nil {
		t.Fatalf("unmarshal runtime: %v", err)
	}
	if err := json.Unmarshal(schemaBytes, &sc); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	rtLocals, rtOK := rt["locals"]
	scLocals, scOK := sc["locals"]
	if !rtOK {
		t.Errorf("HelloURLs missing locals key: %s", runtimeBytes)
	}
	if !scOK {
		t.Errorf("BridgeV1CurrentURLs missing locals key: %s", schemaBytes)
	}
	if rtOK && scOK && string(rtLocals) != string(scLocals) {
		t.Errorf("locals drift between runtime HelloURLs and schema BridgeV1CurrentURLs: runtime=%s schema=%s", rtLocals, scLocals)
	}
}

// A.3:hello_ack locals 仅承载 secondary(primary=local 已过滤 + 去重),防 iOS 重复存储。
func TestHandleHelloWithRemoteURLs_LocalsFiltersPrimary(t *testing.T) {
	primary := "ws://192.168.1.25:8777/bridge"
	secondary := "ws://192.168.1.26:8777/bridge"
	ack := HandleHelloWithRemoteURLs(
		&HelloMessage{Type: "hello", Protocol: HelloProtocol{Name: BridgeProtocolName, Version: BridgeProtocolVersion}},
		nil, "brg", "Mac", "0.1", primary, "", nil,
		[]string{primary, secondary, secondary}, // 含 primary + 重复
		nil, "", nil, nil,
	)
	if ack == nil || ack.Bridge == nil {
		t.Fatalf("ack/bridge nil")
	}
	if ack.Bridge.CurrentURLs.Local != primary {
		t.Fatalf("Local = %q, want %q", ack.Bridge.CurrentURLs.Local, primary)
	}
	if len(ack.Bridge.CurrentURLs.Locals) != 1 || ack.Bridge.CurrentURLs.Locals[0] != secondary {
		t.Fatalf("Locals = %v, want [%s] (primary filtered + dedup)", ack.Bridge.CurrentURLs.Locals, secondary)
	}
}

// fixture 同步:hello_ack.json 必须含 locals,且全部 URL 为干净 LAN ws:// 候选
// (无 loopback/link-local/Tailscale CGNAT/公网 IP 泄漏),primary 不重复出现在 locals。
func TestHelloAckFixture_LocalsPresentAndClean(t *testing.T) {
	data, err := os.ReadFile("testdata/bridge-v1/hello_ack.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var ack struct {
		Bridge struct {
			CurrentURLs struct {
				Local   string   `json:"local"`
				Locals  []string `json:"locals"`
				Remote  *string  `json:"remote"`
				Remotes []string `json:"remotes"`
			} `json:"currentURLs"`
		} `json:"bridge"`
	}
	if err := json.Unmarshal(data, &ack); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if len(ack.Bridge.CurrentURLs.Locals) == 0 {
		t.Fatal("fixture currentURLs.locals missing/empty; round-3 P2-1 requires locals present + synced")
	}
	all := append([]string{ack.Bridge.CurrentURLs.Local}, ack.Bridge.CurrentURLs.Locals...)
	for _, u := range all {
		if !strings.HasPrefix(u, "ws://") {
			t.Errorf("fixture URL %q must be ws:// LAN candidate (not wss/relay)", u)
			continue
		}
		hostPort := strings.TrimPrefix(u, "ws://")
		host := strings.SplitN(hostPort, ":", 2)[0]
		ip := net.ParseIP(host)
		if ip == nil {
			t.Errorf("fixture URL %q host %q not an IP", u, host)
			continue
		}
		if !isLanCandidateIPv4(ip) {
			t.Errorf("fixture URL %q is not a clean LAN candidate (loopback/link-local/Tailscale/public leaked)", u)
		}
	}
	for _, l := range ack.Bridge.CurrentURLs.Locals {
		if l == ack.Bridge.CurrentURLs.Local {
			t.Errorf("primary %q duplicated in locals (locals must be secondary only)", ack.Bridge.CurrentURLs.Local)
		}
	}
}
