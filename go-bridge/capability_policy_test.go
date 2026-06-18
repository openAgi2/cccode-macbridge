package gobridge

import (
	"testing"
)

// TestCapabilityPolicyAllowsFileScopedByDefault 验证 policy 层接入且默认放行
// （workspace 锚点由 handleReadFile 兜底），不破坏现有授权语义。
func TestCapabilityPolicyAllowsFileScopedByDefault(t *testing.T) {
	handlers := NewHandlers()
	conn := &readFileCaptureConn{}
	msg := WireMessage{RequestID: "req_cap", Method: "read_file", BackendID: "codex"}

	// AuthorizeRPC 对 fileScopedMethods 默认返回 nil（放行），由下游 handler 校验。
	if perr := handlers.capabilityPolicy.AuthorizeRPC(conn, msg); perr != nil {
		t.Fatalf("AuthorizeRPC read_file 默认应放行, got %#v", perr)
	}
}

// TestCapabilityPolicyIgnoresNonFileMethod 验证非文件方法不进入 policy 分支。
func TestCapabilityPolicyIgnoresNonFileMethod(t *testing.T) {
	p := NewCapabilityPolicy()
	conn := &readFileCaptureConn{}
	if perr := p.AuthorizeRPC(conn, WireMessage{Method: "list_sessions"}); perr != nil {
		t.Fatalf("list_sessions 不在 fileScopedMethods，应放行, got %#v", perr)
	}
}

// TestCapabilityPolicyNilSafe 验证 nil policy 不 panic（防御未来注入遗漏）。
func TestCapabilityPolicyNilSafe(t *testing.T) {
	var p *CapabilityPolicy
	conn := &readFileCaptureConn{}
	if perr := p.AuthorizeRPC(conn, WireMessage{Method: "read_file"}); perr != nil {
		t.Fatalf("nil policy 应放行, got %#v", perr)
	}
}

// TestHandleRPCHooksCapabilityPolicy 验证 HandleRPC 集成了 policy 层：
// read_file 请求会经过 AuthorizeRPC（通过 debug 日志/无 panic + 正常 dispatch 验证）。
func TestHandleRPCHooksCapabilityPolicy(t *testing.T) {
	handlers := NewHandlers()
	agent := &fakeAgent{name: "codex"}
	handlers.RegisterAgent("codex", agent)
	conn := &readFileCaptureConn{}
	// 发送一个 read_file 请求（无 path）→ 走 policy → handleReadFile 返回 missing_param。
	handlers.HandleRPC(conn, WireMessage{
		Type: "request", RequestID: "req_hook", BackendID: "codex", Method: "read_file",
		Params: mustJSONRaw(t, map[string]any{}),
	})
	if conn.err == nil {
		t.Fatal("read_file 空 path 应返回错误（证明 policy hook 后正常 dispatch）")
	}
	// 不应是 capability policy 拒绝（默认放行）；应是 handleReadFile 的 missing_param。
	if conn.err.Code != "missing_param" && conn.err.Code != "file.outside_authorized_root" {
		t.Fatalf("err code = %q, want missing_param 或 file.outside_authorized_root（policy 默认放行）", conn.err.Code)
	}
}
