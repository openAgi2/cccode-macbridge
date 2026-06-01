package gobridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBridgeV1HelloFixtureDecodes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "bridge-v1", "hello.json"))
	if err != nil {
		t.Fatal(err)
	}

	var hello BridgeV1Hello
	if err := json.Unmarshal(data, &hello); err != nil {
		t.Fatal(err)
	}

	if hello.Type != "hello" {
		t.Fatalf("type = %q, want hello", hello.Type)
	}
	if hello.Protocol.Name != BridgeProtocolName {
		t.Fatalf("protocol.name = %q, want %q", hello.Protocol.Name, BridgeProtocolName)
	}
	if hello.Protocol.Version != BridgeProtocolVersion {
		t.Fatalf("protocol.version = %d, want %d", hello.Protocol.Version, BridgeProtocolVersion)
	}
	if hello.Client.DeviceID == "" {
		t.Fatal("client.deviceId must not be empty")
	}
}

func TestBridgeV1HelloAckFixtureRoundTrips(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", "bridge-v1", "hello_ack.json"))
	if err != nil {
		t.Fatal(err)
	}

	var ack BridgeV1HelloAck
	if err := json.Unmarshal(data, &ack); err != nil {
		t.Fatal(err)
	}
	if !ack.OK {
		t.Fatal("hello_ack fixture must be ok")
	}
	if ack.Bridge == nil {
		t.Fatal("hello_ack bridge profile missing")
	}
	if ack.Bridge.Protocol.Name != BridgeProtocolName {
		t.Fatalf("bridge.protocol.name = %q, want %q", ack.Bridge.Protocol.Name, BridgeProtocolName)
	}
	if ack.Capabilities == nil || !ack.Capabilities.WorkspaceList {
		t.Fatal("hello_ack capabilities.workspaceList must be true")
	}

	encoded, err := json.Marshal(ack)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["type"] != "hello_ack" {
		t.Fatalf("encoded type = %#v, want hello_ack", decoded["type"])
	}
}
