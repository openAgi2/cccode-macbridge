package gobridge

import (
	"encoding/base64"
	"encoding/json"
	"testing"
)

type deliveryCaptureConn struct {
	device *TrustedDeviceRecord
	data   interface{}
	err    *WireError
}

func (c *deliveryCaptureConn) SendJSON(any) {}

func (c *deliveryCaptureConn) SendResult(_ string, data interface{}, err *WireError) {
	c.data = data
	c.err = err
}

func (c *deliveryCaptureConn) SendEvent(string, string, string, interface{}) {}
func (c *deliveryCaptureConn) AuthedDevice() *TrustedDeviceRecord            { return c.device }
func (c *deliveryCaptureConn) RemoteAddr() string                            { return "test:delivery" }
func (c *deliveryCaptureConn) Close() error                                  { return nil }

func deliveryPublicKey() string {
	return base64.StdEncoding.EncodeToString(make([]byte, 32))
}

func TestDeliveryRPCRequiresAuthenticatedDevice(t *testing.T) {
	handlers := NewHandlers()
	conn := &deliveryCaptureConn{}

	handlers.HandleRPC(conn, WireMessage{RequestID: "req_auth", Method: "get_delivery_prekey_status"})

	if conn.err == nil || conn.err.Code != "auth.required" {
		t.Fatalf("error = %#v, want auth.required", conn.err)
	}
}

func TestDeliveryRPCUsesAuthenticatedDeviceIdentity(t *testing.T) {
	handlers := NewHandlers()
	handlers.SetBridgeID("brg_real")
	deviceA := &deliveryCaptureConn{device: &TrustedDeviceRecord{DeviceID: "dev_a"}}
	deviceB := &deliveryCaptureConn{device: &TrustedDeviceRecord{DeviceID: "dev_b"}}
	params, err := json.Marshal(map[string]interface{}{
		"deviceId": "dev_b",
		"batchId":  "batch_a",
		"prekeys":  []map[string]string{{"prekeyId": "pk_a", "publicKey": deliveryPublicKey()}},
	})
	if err != nil {
		t.Fatal(err)
	}

	handlers.HandleRPC(deviceA, WireMessage{RequestID: "req_upload", Method: "upload_delivery_prekeys", Params: params})
	if deviceA.err != nil {
		t.Fatalf("upload error = %#v", deviceA.err)
	}

	handlers.HandleRPC(deviceA, WireMessage{RequestID: "req_status_a", Method: "get_delivery_prekey_status"})
	statusA, ok := deviceA.data.(PrekeyStatusResponse)
	if !ok || statusA.AvailableCount != 1 || statusA.LowWatermark != 10 || statusA.TargetCount != 32 || statusA.MaxCount != 64 {
		t.Fatalf("device A status = %#v", deviceA.data)
	}

	handlers.HandleRPC(deviceB, WireMessage{RequestID: "req_status_b", Method: "get_delivery_prekey_status"})
	statusB, ok := deviceB.data.(PrekeyStatusResponse)
	if !ok || statusB.AvailableCount != 0 {
		t.Fatalf("device B status = %#v, want empty", deviceB.data)
	}
}

func TestDeliveryRPCRejectsInvalidBatchAtomically(t *testing.T) {
	handlers := NewHandlers()
	conn := &deliveryCaptureConn{device: &TrustedDeviceRecord{DeviceID: "dev_atomic"}}
	params, err := json.Marshal(map[string]interface{}{
		"batchId": "batch_invalid",
		"prekeys": []map[string]string{
			{"prekeyId": "pk_good", "publicKey": deliveryPublicKey()},
			{"prekeyId": "pk_bad", "publicKey": "not-base64"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	handlers.HandleRPC(conn, WireMessage{RequestID: "req_invalid", Method: "upload_delivery_prekeys", Params: params})
	if conn.err == nil || conn.err.Code != "invalid_delivery_prekey_batch" {
		t.Fatalf("error = %#v, want invalid_delivery_prekey_batch", conn.err)
	}

	conn.err = nil
	handlers.HandleRPC(conn, WireMessage{RequestID: "req_status", Method: "get_delivery_prekey_status"})
	status, ok := conn.data.(PrekeyStatusResponse)
	if !ok || status.AvailableCount != 0 {
		t.Fatalf("status after invalid batch = %#v, want empty", conn.data)
	}
}
