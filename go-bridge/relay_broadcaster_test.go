package gobridge

import (
	"testing"
)

type relayBroadcastCaptureConn struct {
	device   *TrustedDeviceRecord
	messages []any
}

func (c *relayBroadcastCaptureConn) SendJSON(v any) {
	c.messages = append(c.messages, v)
}

func (c *relayBroadcastCaptureConn) SendResult(string, interface{}, *WireError) {}

func (c *relayBroadcastCaptureConn) SendEvent(string, string, string, interface{}) {}

func (c *relayBroadcastCaptureConn) AuthedDevice() *TrustedDeviceRecord {
	return c.device
}

func (c *relayBroadcastCaptureConn) RemoteAddr() string { return "relay:test-device" }

func (c *relayBroadcastCaptureConn) Close() error { return nil }

func TestBroadcasterSupportsRelayConnection(t *testing.T) {
	conn := &relayBroadcastCaptureConn{
		device: &TrustedDeviceRecord{DeviceID: "dev_relay"},
	}
	broadcaster := NewBroadcaster()
	key := SubscriptionKey{BackendID: "codex", SessionID: "ses_relay"}

	broadcaster.Subscribe(conn, key)
	broadcaster.Send(BroadcastEvent{
		BackendID: "codex",
		SessionID: "ses_relay",
		Message:   map[string]string{"type": "event"},
	})

	if len(conn.messages) != 1 {
		t.Fatalf("relay connection message count = %d, want 1", len(conn.messages))
	}
	deviceIDs := broadcaster.SubscriberDeviceIDs("codex", "ses_relay")
	if len(deviceIDs) != 1 || deviceIDs[0] != "dev_relay" {
		t.Fatalf("relay subscribed devices = %#v, want dev_relay", deviceIDs)
	}
}
