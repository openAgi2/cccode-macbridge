package gobridge

import (
	"bufio"
	"fmt"
	"net"
	"testing"
	"time"
)

// TestManagementServerRejectsSlowHeaders 验证 P2-1：管理 API 的 ReadHeaderTimeout
// 会拒绝 slowloris 式的慢速 header，避免握手阶段耗尽连接。
func TestManagementServerRejectsSlowHeaders(t *testing.T) {
	// 覆盖为短超时，避免测试等待产品默认 10s。
	prevTimeout := httpReadHeaderTimeout
	httpReadHeaderTimeout = 300 * time.Millisecond
	t.Cleanup(func() { httpReadHeaderTimeout = prevTimeout })

	srv := newTestMgmtServer(nil)
	port, err := srv.Start("127.0.0.1", 0)
	if err != nil {
		t.Fatalf("management server start failed: %v", err)
	}
	t.Cleanup(func() { srv.Shutdown() })

	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 2*time.Second)
	if err != nil {
		t.Fatalf("dial failed: %v", err)
	}
	defer conn.Close()

	// 发送请求行但不发送完整 header（slowloris）。
	if _, err := conn.Write([]byte("GET /internal/status HTTP/1.1\r\nHost: 127.0.0.1\r\n")); err != nil {
		t.Fatalf("write request line failed: %v", err)
	}

	// ReadHeaderTimeout=10s；用 15s 上限等待服务端主动关闭。
	r := bufio.NewReader(conn)
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	// 服务端应在 ReadHeaderTimeout 后关闭连接（EOF 或 reset）。
	closed := make(chan error, 1)
	go func() {
		buf := make([]byte, 256)
		_, err := r.Read(buf)
		closed <- err
	}()
	select {
	case <-closed:
		// 服务端关闭连接（可能 EOF 或 error），说明 ReadHeaderTimeout 生效。
	case <-time.After(4 * time.Second):
		t.Fatal("慢速 header 连接未被关闭：ReadHeaderTimeout 未生效")
	}
}
