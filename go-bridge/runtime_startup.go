package gobridge

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// ── 启动契约帧类型 ──────────────────────────────────────────────────────────
// go-bridge 作为子进程启动时，通过 stdout/stderr 输出结构化 JSON 帧，
// 父进程（Mac App）据此判断就绪、失败等状态。

// 启动就绪帧，写入 stdout
type RuntimeReadyFrame struct {
	Type          string   `json:"type"`                    // 固定 "runtime_ready"
	Port          int      `json:"port"`                    // 实际监听端口
	BridgeEpoch   string   `json:"bridgeEpoch"`             // 启动唯一标识
	Drivers       []string `json:"drivers"`                 // 已注册的后端 driver 列表
	ManagementURL string   `json:"managementUrl,omitempty"` // 管理 API 地址（product 模式）
	PID           int      `json:"pid"`                     // 进程 PID
}

// 启动错误帧，写入 stderr
type RuntimeErrorFrame struct {
	Type    string `json:"type"`    // 固定 "runtime_error"
	Code    string `json:"code"`    // 错误码，见下方常量
	Message string `json:"message"` // 人类可读错误信息
}

// ── 错误码常量 ───────────────────────────────────────────────────────────────

const (
	RuntimeErrorPortBindFailed = "runtime_error.port_bind_failed"
	RuntimeErrorNoAgents       = "runtime_error.no_agents"
	RuntimeErrorConfigInvalid  = "runtime_error.config_invalid"
)

// WriteReadyFrame 将就绪帧以 JSON 形式写入 stdout 并追加换行。
func WriteReadyFrame(port int, drivers []string, managementURL string, dataDirPath string) {
	frame := RuntimeReadyFrame{
		Type:          "runtime_ready",
		Port:          port,
		BridgeEpoch:   generateEpoch(),
		Drivers:       drivers,
		ManagementURL: managementURL,
		PID:           os.Getpid(),
	}
	data, _ := json.Marshal(frame)
	fmt.Fprintf(os.Stdout, "%s\n", data)
	os.Stdout.Sync()

	// 写入 data-dir/runtime.json 供 MacBridge 发现外部启动的 go-bridge
	if dataDirPath != "" {
		runtimePath := dataDirPath + "/runtime.json"
		if err := os.WriteFile(runtimePath, data, 0644); err != nil {
			slog.Error("go-bridge: runtime.json 写入失败", "path", runtimePath, "error", err)
		}
	}
}

// WriteErrorFrame 将错误帧以 JSON 形式写入 stderr 并追加换行。
func WriteErrorFrame(code, message string) {
	frame := RuntimeErrorFrame{
		Type:    "runtime_error",
		Code:    code,
		Message: message,
	}
	data, _ := json.Marshal(frame)
	fmt.Fprintf(os.Stderr, "%s\n", data)
	os.Stderr.Sync()
}

// ParseReadyFrame 从 JSON 字节解析就绪帧，用于测试。
func ParseReadyFrame(data []byte) (*RuntimeReadyFrame, error) {
	var frame RuntimeReadyFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		return nil, err
	}
	if frame.Type != "runtime_ready" {
		return nil, fmt.Errorf("unexpected frame type: %s", frame.Type)
	}
	return &frame, nil
}

// ParseErrorFrame 从 JSON 字节解析错误帧，用于测试。
func ParseErrorFrame(data []byte) (*RuntimeErrorFrame, error) {
	var frame RuntimeErrorFrame
	if err := json.Unmarshal(data, &frame); err != nil {
		return nil, err
	}
	if frame.Type != "runtime_error" {
		return nil, fmt.Errorf("unexpected frame type: %s", frame.Type)
	}
	return &frame, nil
}

// generateEpoch 生成启动唯一标识（时间戳 + PID）。
func generateEpoch() string {
	return fmt.Sprintf("%d-%d", time.Now().UnixMilli(), os.Getpid())
}
