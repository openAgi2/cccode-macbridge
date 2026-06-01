package gobridge

import (
	"log/slog"
	"sync"
)

// DeviceConnRegistry tracks active WebSocket connections by device ID.
// Used to disconnect devices when they are revoked.
type DeviceConnRegistry struct {
	mu    sync.Mutex
	conns map[string][]*Conn // deviceID → connections
}

var globalDeviceConnRegistry = &DeviceConnRegistry{
	conns: make(map[string][]*Conn),
}

func (r *DeviceConnRegistry) Register(deviceID string, conn *Conn) {
	if deviceID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.conns[deviceID] = append(r.conns[deviceID], conn)
}

func (r *DeviceConnRegistry) Unregister(deviceID string, conn *Conn) {
	if deviceID == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	conns := r.conns[deviceID]
	for i, c := range conns {
		if c == conn {
			r.conns[deviceID] = append(conns[:i], conns[i+1:]...)
			break
		}
	}
	if len(r.conns[deviceID]) == 0 {
		delete(r.conns, deviceID)
	}
}

// DisconnectDevice 关闭指定设备的所有连接，发送 device_revoked 事件。
func (r *DeviceConnRegistry) DisconnectDevice(deviceID string) {
	r.mu.Lock()
	conns := r.conns[deviceID]
	delete(r.conns, deviceID)
	r.mu.Unlock()

	for _, conn := range conns {
		slog.Info("go-bridge: marking device revoked", "deviceId", deviceID)
		conn.revoked = true
		conn.SendJSON(map[string]interface{}{
			"type":    "event",
			"event":   "device_revoked",
			"message": "设备授权已取消，请重新授权",
		})
	}
}
